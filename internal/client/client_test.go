package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// generateTestPKI creates a temporary Root CA and a server certificate signed by that CA.
func generateTestPKI(t *testing.T) (caPEM []byte, serverCert tls.Certificate) {
	t.Helper()

	// 1. Generate CA
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1001),
		Subject: pkix.Name{
			CommonName:   "Sekha Cluster Test Root CA",
			Organization: []string{"Duara-Cortex"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create CA certificate: %v", err)
	}
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	// 2. Generate Server Certificate signed by CA
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate server key: %v", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1002),
		Subject: pkix.Name{
			CommonName:   "127.0.0.1",
			Organization: []string{"Duara-Cortex"},
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:    []string{"localhost"},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create server certificate: %v", err)
	}
	serverPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})

	serverCert, err = tls.X509KeyPair(serverPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("failed to load server key pair: %v", err)
	}

	return caPEM, serverCert
}

func TestBaseClient_TLSConfiguration(t *testing.T) {
	caPEM, serverCert := generateTestPKI(t)

	// Save custom CA cert to a temporary file
	tmpDir := t.TempDir()
	caCertPath := filepath.Join(tmpDir, "cluster-ca.crt")
	if err := os.WriteFile(caCertPath, caPEM, 0644); err != nil {
		t.Fatalf("failed to write CA cert file: %v", err)
	}

	// Start HTTPS test server with the CA-signed certificate
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
	}
	ts.StartTLS()
	defer ts.Close()

	ctx := context.Background()

	t.Run("fails without custom CA or insecure flag", func(t *testing.T) {
		// Default client should reject self-signed/custom CA
		client := NewBaseClient(2*time.Second, "")
		var target map[string]interface{}
		err := client.GetJSON(ctx, ts.URL, &target, "trc-tls-fail")
		if err == nil {
			t.Fatal("expected TLS verification failure with default client, got nil")
		}
	})

	t.Run("succeeds with custom CA certificate file", func(t *testing.T) {
		// Client configured with custom CA root certificate
		client := NewBaseClient(2*time.Second, "", caCertPath, false)
		if client.TLSCACert() != caCertPath {
			t.Errorf("expected TLSCACert '%s', got '%s'", caCertPath, client.TLSCACert())
		}
		if client.Insecure() {
			t.Errorf("expected Insecure to be false")
		}

		var target map[string]interface{}
		err := client.GetJSON(ctx, ts.URL, &target, "trc-tls-ca")
		if err != nil {
			t.Fatalf("expected HTTPS request with custom CA to succeed, got: %v", err)
		}
		if target["status"] != "ok" {
			t.Errorf("expected status 'ok', got %v", target["status"])
		}

		// Also test PostJSON over HTTPS with custom CA
		var postTarget map[string]interface{}
		err = client.PostJSON(ctx, ts.URL, map[string]string{"foo": "bar"}, &postTarget, "trc-tls-post")
		if err != nil {
			t.Fatalf("expected HTTPS POST with custom CA to succeed, got: %v", err)
		}
		if postTarget["status"] != "ok" {
			t.Errorf("expected status 'ok', got %v", postTarget["status"])
		}
	})

	t.Run("succeeds with InsecureSkipVerify: true", func(t *testing.T) {
		// Client configured with insecure = true (skip TLS verification)
		client := NewBaseClient(2*time.Second, "", "", true)
		if !client.Insecure() {
			t.Errorf("expected Insecure to be true")
		}

		var target map[string]interface{}
		err := client.GetJSON(ctx, ts.URL, &target, "trc-tls-insecure")
		if err != nil {
			t.Fatalf("expected HTTPS request with insecure=true to succeed, got: %v", err)
		}
		if target["status"] != "ok" {
			t.Errorf("expected status 'ok', got %v", target["status"])
		}
	})

	t.Run("constructor with TLSOptions struct", func(t *testing.T) {
		client := NewBaseClient(2*time.Second, "", TLSOptions{
			CACertPath: caCertPath,
			Insecure:   false,
		})
		var target map[string]interface{}
		err := client.GetJSON(ctx, ts.URL, &target, "trc-tls-struct")
		if err != nil {
			t.Fatalf("expected HTTPS request with TLSOptions to succeed, got: %v", err)
		}
	})

	t.Run("NewBaseClientWithTLS helper constructor", func(t *testing.T) {
		client := NewBaseClientWithTLS(2*time.Second, "", caCertPath, false)
		var target map[string]interface{}
		err := client.GetJSON(ctx, ts.URL, &target, "trc-tls-helper")
		if err != nil {
			t.Fatalf("expected HTTPS request with NewBaseClientWithTLS to succeed, got: %v", err)
		}
	})
}

func TestBaseClient_401UnauthorizedDiagnostics(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized access to knowledge layer"}`))
	}))
	defer ts.Close()

	ctx := context.Background()
	client := NewBaseClient(2*time.Second, "")

	t.Run("GetJSON 401 diagnostic message", func(t *testing.T) {
		var target map[string]interface{}
		err := client.GetJSON(ctx, ts.URL+"/api/v1/memory/health", &target, "trc-401-get")
		if err == nil {
			t.Fatal("expected error on HTTP 401, got nil")
		}

		var uErr *ErrUnauthorized
		if !errors.As(err, &uErr) {
			t.Errorf("expected error to be of type *ErrUnauthorized, got %T: %v", err, err)
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "HTTP 401 Unauthorized") {
			t.Errorf("expected 'HTTP 401 Unauthorized' in error, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "API key is invalid or missing") {
			t.Errorf("expected 'API key is invalid or missing' in error, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "CLUSTER_API_KEY") || !strings.Contains(errMsg, "SEKHA_API_KEY") {
			t.Errorf("expected .env hint (CLUSTER_API_KEY / SEKHA_API_KEY) in error, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "--token") {
			t.Errorf("expected '--token' hint in error, got: %s", errMsg)
		}
	})

	t.Run("PostJSON 401 diagnostic message", func(t *testing.T) {
		var target map[string]interface{}
		err := client.PostJSON(ctx, ts.URL+"/api/v1/memory/recall", map[string]string{"query": "test"}, &target, "trc-401-post")
		if err == nil {
			t.Fatal("expected error on HTTP 401, got nil")
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "HTTP 401 Unauthorized") {
			t.Errorf("expected 'HTTP 401 Unauthorized' in error, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "CLUSTER_API_KEY") || !strings.Contains(errMsg, "--token") {
			t.Errorf("expected actionable advice in error, got: %s", errMsg)
		}
	})
}
