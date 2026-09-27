package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// generateTestCACert creates a valid temporary Root CA certificate PEM.
func generateTestCACert(t *testing.T) []byte {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1001),
		Subject: pkix.Name{
			CommonName:   "Sekha Config Test Root CA",
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
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}

func setupEmptyEnvFile(t *testing.T) {
	t.Helper()
	emptyEnv := filepath.Join(t.TempDir(), "empty.env")
	if err := os.WriteFile(emptyEnv, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write empty env file: %v", err)
	}
	t.Setenv("CLUSTER_ENV_FILE", emptyEnv)
}

func TestValidateCACert_MissingFile(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "nonexistent", "ca.pem")
	source := "--tls-ca-cert flag"

	err := ValidateCACert(missingPath, source)
	if err == nil {
		t.Fatal("expected error for missing CA file, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, missingPath) {
		t.Errorf("expected error to contain path '%s', got: %s", missingPath, errMsg)
	}
	if !strings.Contains(errMsg, source) {
		t.Errorf("expected error to contain source '%s', got: %s", source, errMsg)
	}
	if !strings.Contains(errMsg, "no such file or directory") {
		t.Errorf("expected error to contain OS error 'no such file or directory', got: %s", errMsg)
	}
}

func TestValidateCACert_DirectoryInsteadOfFile(t *testing.T) {
	dirPath := t.TempDir()
	source := "environment variable CLUSTER_TLS_CA_CERT"

	err := ValidateCACert(dirPath, source)
	if err == nil {
		t.Fatal("expected error when CA path is a directory, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, dirPath) {
		t.Errorf("expected error to contain path '%s', got: %s", dirPath, errMsg)
	}
	if !strings.Contains(errMsg, source) {
		t.Errorf("expected error to contain source '%s', got: %s", source, errMsg)
	}
	if !strings.Contains(errMsg, "is a directory") {
		t.Errorf("expected error to contain 'is a directory', got: %s", errMsg)
	}
}

func TestValidateCACert_NonPEMFile(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "invalid-ca.crt")
	if err := os.WriteFile(badFile, []byte("--- NOT A REAL CERTIFICATE ---"), 0644); err != nil {
		t.Fatalf("failed to write invalid ca file: %v", err)
	}
	source := "--tls-ca-cert flag"

	err := ValidateCACert(badFile, source)
	if err == nil {
		t.Fatal("expected error for non-PEM file, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "contains no valid PEM certificates") {
		t.Errorf("expected error to mention 'contains no valid PEM certificates', got: %s", errMsg)
	}
	expectedCmd := "openssl x509 -in " + badFile + " -noout -subject"
	if !strings.Contains(errMsg, expectedCmd) {
		t.Errorf("expected error to suggest '%s', got: %s", expectedCmd, errMsg)
	}
	if !strings.Contains(errMsg, source) {
		t.Errorf("expected error to contain source '%s', got: %s", source, errMsg)
	}
}

func TestValidateCACert_RelativePath(t *testing.T) {
	relPath := "nonexistent-rel-ca.pem"
	absPath, _ := filepath.Abs(relPath)
	source := "--tls-ca-cert flag"

	err := ValidateCACert(relPath, source)
	if err == nil {
		t.Fatal("expected error for relative nonexistent path, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, relPath) {
		t.Errorf("expected error to contain relative path '%s', got: %s", relPath, errMsg)
	}
	if !strings.Contains(errMsg, absPath) {
		t.Errorf("expected error to contain resolved absolute path '%s', got: %s", absPath, errMsg)
	}
	if !strings.Contains(errMsg, "resolved to") {
		t.Errorf("expected error to mention 'resolved to', got: %s", errMsg)
	}
}

func TestValidateCACert_ValidPEM(t *testing.T) {
	caPEM := generateTestCACert(t)
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "valid-ca.crt")
	if err := os.WriteFile(caPath, caPEM, 0644); err != nil {
		t.Fatalf("failed to write valid CA file: %v", err)
	}

	if err := ValidateCACert(caPath, "--tls-ca-cert flag"); err != nil {
		t.Errorf("expected nil error for valid CA, got: %v", err)
	}

	pool, err := LoadCACertPool(caPath, "--tls-ca-cert flag")
	if err != nil {
		t.Fatalf("expected LoadCACertPool to succeed for valid CA, got: %v", err)
	}
	if pool == nil {
		t.Fatal("expected non-nil CertPool")
	}
}

func TestValidateTLS_ConfigSources(t *testing.T) {
	setupEmptyEnvFile(t)

	t.Run("empty TLSCACert passes", func(t *testing.T) {
		cfg := &Config{}
		if err := cfg.ValidateTLS(); err != nil {
			t.Errorf("expected nil error for unconfigured TLS CA, got: %v", err)
		}
	})

	t.Run("source from flag", func(t *testing.T) {
		cfg, err := Load(FlagOverrides{
			TLSCACert: "/missing/flag-ca.pem",
		})
		if err != nil {
			t.Fatalf("unexpected Load error: %v", err)
		}
		if cfg.TLSCACertSource != "--tls-ca-cert flag" {
			t.Errorf("expected source '--tls-ca-cert flag', got '%s'", cfg.TLSCACertSource)
		}
		err = cfg.ValidateTLS()
		if err == nil {
			t.Fatal("expected ValidateTLS error, got nil")
		}
		if !strings.Contains(err.Error(), "--tls-ca-cert flag") {
			t.Errorf("expected error to mention flag source, got: %s", err.Error())
		}
	})

	t.Run("source from OS env", func(t *testing.T) {
		t.Setenv("CLUSTER_TLS_CA_CERT", "/missing/os-ca.pem")
		cfg, err := Load(FlagOverrides{})
		if err != nil {
			t.Fatalf("unexpected Load error: %v", err)
		}
		if cfg.TLSCACertSource != "environment variable CLUSTER_TLS_CA_CERT" {
			t.Errorf("expected source 'environment variable CLUSTER_TLS_CA_CERT', got '%s'", cfg.TLSCACertSource)
		}
		err = cfg.ValidateTLS()
		if err == nil {
			t.Fatal("expected ValidateTLS error, got nil")
		}
		if !strings.Contains(err.Error(), "environment variable CLUSTER_TLS_CA_CERT") {
			t.Errorf("expected error to mention OS env source, got: %s", err.Error())
		}
	})

	t.Run("source from .env file", func(t *testing.T) {
		tmpDir := t.TempDir()
		envFile := filepath.Join(tmpDir, "cluster.env")
		if err := os.WriteFile(envFile, []byte("CLUSTER_TLS_CA_CERT=/missing/dotenv-ca.pem\n"), 0644); err != nil {
			t.Fatalf("failed to write test env file: %v", err)
		}
		t.Setenv("CLUSTER_ENV_FILE", envFile)

		cfg, err := Load(FlagOverrides{})
		if err != nil {
			t.Fatalf("unexpected Load error: %v", err)
		}
		if !strings.HasPrefix(cfg.TLSCACertSource, ".env file") {
			t.Errorf("expected source starting with '.env file', got '%s'", cfg.TLSCACertSource)
		}
		err = cfg.ValidateTLS()
		if err == nil {
			t.Fatal("expected ValidateTLS error, got nil")
		}
		if !strings.Contains(err.Error(), ".env file") {
			t.Errorf("expected error to mention .env source, got: %s", err.Error())
		}
	})
}
