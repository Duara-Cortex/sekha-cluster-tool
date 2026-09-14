package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/config"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/telemetry"
)

// Config is an alias to config.Config.
type Config = config.Config


// BaseClient handles HTTP requests with connection pooling and trace injection.
type BaseClient struct {
	httpClient *http.Client
}

// NewBaseClient creates a BaseClient with optimised connection pooling and keep-alive settings.
func NewBaseClient(timeout time.Duration) *BaseClient {
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

	return &BaseClient{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}
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
