package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

func TestSensoryClient_GetStats_Node3Payload(t *testing.T) {
	// Mock Node 3 returning raw float uptime_seconds and byte fields
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sensory/stats" {
			http.NotFound(w, r)
			return
		}
		rawJSON := `{
			"capacity_bytes": 67108864,
			"used_bytes": 10843436,
			"fill_percent": 16.157978773117065,
			"current_item_count": 50386,
			"total_ingested_count": 50386,
			"total_ingested_bytes": 10843436,
			"ingestion_rate_kbps": 0,
			"dropped_packets": 0,
			"dropped_bytes": 0,
			"uptime_seconds": 165026.113641325,
			"classifier_telemetry": {
				"total_evaluated": 29898,
				"total_salient": 29898,
				"total_discarded": 0,
				"noise_reduction_ratio": 0
			}
		}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rawJSON))
	})

	server := httptest.NewServer(mockHandler)
	defer server.Close()

	sensoryClient := NewSensoryClient(server.URL, 2*time.Second)
	stats, err := sensoryClient.GetStats(context.Background(), "trc-test-stats")
	if err != nil {
		t.Fatalf("unexpected error from GetStats: %v", err)
	}

	if stats.Status != "healthy" {
		t.Errorf("expected Status 'healthy', got '%s'", stats.Status)
	}
	if stats.Service != "sekha-sensory-buffer" {
		t.Errorf("expected Service 'sekha-sensory-buffer', got '%s'", stats.Service)
	}
	if stats.UptimeSeconds != 165026 {
		t.Errorf("expected UptimeSeconds 165026, got %d", stats.UptimeSeconds)
	}
	if stats.BufferCapacityMB != 64.0 {
		t.Errorf("expected BufferCapacityMB 64.0, got %f", stats.BufferCapacityMB)
	}
	if stats.BufferUsageMB != 10.34 {
		t.Errorf("expected BufferUsageMB 10.34, got %f", stats.BufferUsageMB)
	}
	if stats.BufferUsagePct != 16.16 {
		t.Errorf("expected BufferUsagePct 16.16, got %f", stats.BufferUsagePct)
	}
	if stats.TotalIngested != 50386 {
		t.Errorf("expected TotalIngested 50386, got %d", stats.TotalIngested)
	}
	if stats.ClassifierTelemetry == nil {
		t.Fatal("expected ClassifierTelemetry to be populated")
	}
	if stats.ClassifierTelemetry.TotalEvaluated != 29898 {
		t.Errorf("expected TotalEvaluated 29898, got %d", stats.ClassifierTelemetry.TotalEvaluated)
	}
}

func TestSensoryClient_Filter_Node3NativePayload(t *testing.T) {
	var capturedQuery string
	var capturedBody string
	var capturedContentType string

	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		capturedContentType = r.Header.Get("Content-Type")
		bodyBytes := make([]byte, 1024)
		n, _ := r.Body.Read(bodyBytes)
		capturedBody = string(bodyBytes[:n])

		rawJSON := `{
			"total_evaluated": 1,
			"salient_count": 1,
			"discarded_count": 0,
			"noise_reduction_ratio": 0,
			"threshold": 0.45,
			"salient_chunks": [
				{
					"chunk": {
						"seq": 1,
						"timestamp": "2026-09-15T10:58:37Z",
						"origin": "filter_stream",
						"data": "CRITICAL kernel panic risk: nvme0n1 write latency spiked to 4200ms",
						"size_bytes": 0
					},
					"metrics": {
						"entropy": 4.83,
						"salience_score": 0.98,
						"is_salient": true
					}
				}
			]
		}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rawJSON))
	})

	server := httptest.NewServer(mockHandler)
	defer server.Close()

	sensoryClient := NewSensoryClient(server.URL, 2*time.Second)
	req := model.FilterRequest{
		Text:          "CRITICAL kernel panic risk: nvme0n1 write latency spiked to 4200ms",
		TaskDirective: "cluster health",
		Threshold:     0.45,
	}

	resp, err := sensoryClient.Filter(context.Background(), req, "trc-test-filter")
	if err != nil {
		t.Fatalf("unexpected error from Filter: %v", err)
	}

	if capturedContentType != "text/plain" {
		t.Errorf("expected Content-Type 'text/plain', got '%s'", capturedContentType)
	}
	if capturedBody != req.Text {
		t.Errorf("expected body '%s', got '%s'", req.Text, capturedBody)
	}
	if capturedQuery == "" {
		t.Errorf("expected non-empty query string")
	}

	if resp.TotalChunks != 1 {
		t.Errorf("expected TotalChunks 1, got %d", resp.TotalChunks)
	}
	if resp.SalientChunks != 1 {
		t.Errorf("expected SalientChunks 1, got %d", resp.SalientChunks)
	}
	if resp.NoiseDiscarded != 0 {
		t.Errorf("expected NoiseDiscarded 0, got %d", resp.NoiseDiscarded)
	}
	if len(resp.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(resp.Chunks))
	}
	chunk := resp.Chunks[0]
	if chunk.ID != "chnk-001" {
		t.Errorf("expected chunk ID 'chnk-001', got '%s'", chunk.ID)
	}
	if chunk.Text != req.Text {
		t.Errorf("expected chunk Text '%s', got '%s'", req.Text, chunk.Text)
	}
	if chunk.Salience != 0.98 {
		t.Errorf("expected Salience 0.98, got %f", chunk.Salience)
	}
	if chunk.Source != "filter_stream" {
		t.Errorf("expected Source 'filter_stream', got '%s'", chunk.Source)
	}
}

func TestSensoryClient_Filter_NoiseDiscarded(t *testing.T) {
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawJSON := `{
			"total_evaluated": 1,
			"salient_count": 0,
			"discarded_count": 1,
			"noise_reduction_ratio": 1.0,
			"threshold": 0.45,
			"salient_chunks": null
		}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rawJSON))
	})

	server := httptest.NewServer(mockHandler)
	defer server.Close()

	sensoryClient := NewSensoryClient(server.URL, 2*time.Second)
	req := model.FilterRequest{
		Text:      "ok ok ok ok ok ok ok ok",
		Threshold: 0.45,
	}

	resp, err := sensoryClient.Filter(context.Background(), req, "trc-test-noise")
	if err != nil {
		t.Fatalf("unexpected error from Filter: %v", err)
	}

	if resp.TotalChunks != 1 {
		t.Errorf("expected TotalChunks 1, got %d", resp.TotalChunks)
	}
	if resp.SalientChunks != 0 {
		t.Errorf("expected SalientChunks 0, got %d", resp.SalientChunks)
	}
	if resp.NoiseDiscarded != 1 {
		t.Errorf("expected NoiseDiscarded 1, got %d", resp.NoiseDiscarded)
	}
	if resp.ReductionRate != 1.0 {
		t.Errorf("expected ReductionRate 1.0, got %f", resp.ReductionRate)
	}
	if len(resp.Chunks) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(resp.Chunks))
	}
}
