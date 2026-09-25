package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/config"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/telemetry"
)

// Config is an alias to config.Config.
type Config = config.Config

// TLSOptions defines custom certificate and TLS verification parameters.
type TLSOptions struct {
	CACertPath string
	Insecure   bool
}

// ErrUnauthorized represents an actionable HTTP 401 Unauthorized error.
type ErrUnauthorized struct {
	StatusCode int
	URL        string
	Body       string
}

func (e *ErrUnauthorized) Error() string {
	msg := fmt.Sprintf("HTTP 401 Unauthorized from %s: API key is invalid or missing. Check .env (CLUSTER_API_KEY / SEKHA_API_KEY) or pass --token", e.URL)
	if e.Body != "" {
		msg += fmt.Sprintf(" (response: %s)", e.Body)
	}
	return msg
}

// FormatUnauthorizedError constructs an actionable error message for HTTP 401 responses.
func FormatUnauthorizedError(url, body string) error {
	return &ErrUnauthorized{
		StatusCode: http.StatusUnauthorized,
		URL:        url,
		Body:       body,
	}
}

// BaseClient handles HTTP requests with connection pooling, TLS configuration, and trace injection.
type BaseClient struct {
	httpClient *http.Client
	apiKey     string
	tlsCACert  string
	insecure   bool
}

// NewBaseClient creates a BaseClient with optimised connection pooling, keep-alive settings,
// and optional TLS transport configuration (custom root CA cert pool and InsecureSkipVerify).
func NewBaseClient(timeout time.Duration, apiKey string, tlsArgs ...interface{}) *BaseClient {
	var caCertPath string
	var insecure bool

	for _, arg := range tlsArgs {
		switch v := arg.(type) {
		case string:
			caCertPath = v
		case bool:
			insecure = v
		case TLSOptions:
			caCertPath = v.CACertPath
			insecure = v.Insecure
		case *TLSOptions:
			if v != nil {
				caCertPath = v.CACertPath
				insecure = v.Insecure
			}
		}
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   500 * time.Millisecond,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   1 * time.Second,
		ExpectContinueTimeout: 500 * time.Millisecond,
	}

	if caCertPath != "" || insecure {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: insecure,
		}
		if caCertPath != "" {
			caCertPool, err := x509.SystemCertPool()
			if err != nil || caCertPool == nil {
				caCertPool = x509.NewCertPool()
			}
			if caBytes, err := os.ReadFile(caCertPath); err == nil {
				caCertPool.AppendCertsFromPEM(caBytes)
			}
			tlsConfig.RootCAs = caCertPool
		}
		transport.TLSClientConfig = tlsConfig
	}

	return &BaseClient{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		apiKey:    apiKey,
		tlsCACert: caCertPath,
		insecure:  insecure,
	}
}

// NewBaseClientWithTLS creates a BaseClient with explicit TLS settings.
func NewBaseClientWithTLS(timeout time.Duration, apiKey string, caCertPath string, insecure bool) *BaseClient {
	return NewBaseClient(timeout, apiKey, caCertPath, insecure)
}

// Transport returns the underlying *http.Transport for testing or inspection.
func (c *BaseClient) Transport() *http.Transport {
	if t, ok := c.httpClient.Transport.(*http.Transport); ok {
		return t
	}
	return nil
}

// TLSCACert returns the custom CA certificate file path.
func (c *BaseClient) TLSCACert() string {
	return c.tlsCACert
}

// Insecure reports whether TLS certificate verification is bypassed.
func (c *BaseClient) Insecure() bool {
	return c.insecure
}

// HTTPClient returns the configured *http.Client.
func (c *BaseClient) HTTPClient() *http.Client {
	return c.httpClient
}

// PostJSON marshals a request payload, attaches headers, and decodes the JSON response.
func (c *BaseClient) PostJSON(ctx context.Context, url string, payload interface{}, target interface{}, traceID string) error {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to serialise request payload: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to initialise HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	telemetry.InjectTraceID(req, traceID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP POST request failed to %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body from %s: %w", url, err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return FormatUnauthorizedError(url, string(respBytes))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned error HTTP %d from %s: %s", resp.StatusCode, url, string(respBytes))
	}

	if target != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, target); err != nil {
			return fmt.Errorf("failed to decode JSON response from %s: %w (body: %s)", url, err, string(respBytes))
		}
	}

	return nil
}

// GetJSON performs an HTTP GET request, attaches headers, and decodes the JSON response.
func (c *BaseClient) GetJSON(ctx context.Context, url string, target interface{}, traceID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to initialise HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	telemetry.InjectTraceID(req, traceID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP GET request failed to %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body from %s: %w", url, err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return FormatUnauthorizedError(url, string(respBytes))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned error HTTP %d from %s: %s", resp.StatusCode, url, string(respBytes))
	}

	if target != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, target); err != nil {
			return fmt.Errorf("failed to decode JSON response from %s: %w", url, err)
		}
	}

	return nil
}
