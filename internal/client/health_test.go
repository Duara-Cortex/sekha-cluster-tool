package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/config"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

func TestProbeClusterHealth_ConcurrentSpeed(t *testing.T) {
	// Each mock server sleeps 200ms
	mockSensory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(model.SensoryStatsResponse{
			Status:           "healthy",
			BufferCapacityMB: 64.0,
			BufferUsageMB:    10.3,
			FillPercent:      16.2,
			TotalIngested:    50386,
			DroppedPackets:   0,
		})
	}))
	defer mockSensory.Close()

	mockWorking := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(model.WorkingHealthResponse{
			Status:         "healthy",
			LlamaInference: "reachable",
			Service:        "sekha-working-scratchpad",
			UptimeSeconds:  396000,
		})
	}))
	defer mockWorking.Close()

	mockKnowledge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(model.KnowledgeHealthResponse{
			Status:        "healthy",
			NodeCount:     229,
			EdgeCount:     882,
			Service:       "sekha-knowledge-store",
			UptimeSeconds: 93600,
			EmbeddingEngine: &model.EmbeddingEngineHealth{
				Enabled:   true,
				Status:    "reachable",
				URL:       "http://localhost:8086",
				Dimension: 384,
			},
		})
	}))
	defer mockKnowledge.Close()

	cfg := config.Config{
		SensoryURL:   mockSensory.URL,
		WorkingURL:   mockWorking.URL,
		KnowledgeURL: mockKnowledge.URL,
	}

	start := time.Now()
	res := ProbeClusterHealth(context.Background(), cfg, 500*time.Millisecond, "trc-test-speed")
	elapsed := time.Since(start)

	if !res.AllHealthy {
		t.Fatalf("expected all nodes healthy, got %+v", res)
	}
	if res.OnlineCount != 3 {
		t.Fatalf("expected 3 online nodes, got %d", res.OnlineCount)
	}

	// 3 servers sleeping 200ms each must run in parallel and finish in under 450ms (not sequential 600ms)
	if elapsed > 450*time.Millisecond {
		t.Errorf("expected parallel execution (< 450ms), took %v", elapsed)
	}
}

func TestProbeClusterHealth_DegradedAndUnconfigured(t *testing.T) {
	// Node 3 is healthy
	mockSensory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(model.SensoryStatsResponse{
			Status:           "healthy",
			BufferCapacityMB: 64.0,
			BufferUsageMB:    10.3,
		})
	}))
	defer mockSensory.Close()

	// Node 2 is closed/offline
	mockWorking := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	workingURL := mockWorking.URL
	mockWorking.Close() // Immediately close to simulate offline

	// Node 1 is unconfigured (empty URL)
	cfg := config.Config{
		SensoryURL:   mockSensory.URL,
		WorkingURL:   workingURL,
		KnowledgeURL: "",
	}

	res := ProbeClusterHealth(context.Background(), cfg, 500*time.Millisecond, "trc-degraded")
	if res.AllHealthy {
		t.Errorf("expected AllHealthy to be false")
	}
	if res.OnlineCount != 1 {
		t.Errorf("expected 1 online node, got %d", res.OnlineCount)
	}
	if res.ClusterStatus != "degraded_or_partially_offline" {
		t.Errorf("expected degraded status, got '%s'", res.ClusterStatus)
	}

	dashboard := res.FormatDashboard()
	if !strings.Contains(dashboard, "Cluster State: DEGRADED (1/3 Online)") {
		t.Errorf("dashboard missing degraded cluster state:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "[✗] Node 2: Working Memory Layer") {
		t.Errorf("dashboard missing offline working node:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "[!] Node 1: Long-Term Knowledge Layer ((not configured))") {
		t.Errorf("dashboard missing unconfigured knowledge node:\n%s", dashboard)
	}

	pingOut := res.FormatPing()
	if !strings.Contains(pingOut, "[OK] Node 3 (Sensory)") {
		t.Errorf("ping output missing OK for sensory node:\n%s", pingOut)
	}
	if !strings.Contains(pingOut, "[FAIL] Node 2 (Working)") {
		t.Errorf("ping output missing FAIL for working node:\n%s", pingOut)
	}
	if !strings.Contains(pingOut, "[FAIL] Node 1 (Knowledge) - unconfigured") {
		t.Errorf("ping output missing unconfigured for knowledge node:\n%s", pingOut)
	}
	if !strings.Contains(pingOut, "Cluster: DEGRADED (1/3 nodes online)") {
		t.Errorf("ping output missing degraded summary:\n%s", pingOut)
	}
}

func TestFormattingHelpers(t *testing.T) {
	// formatNumber
	if got := formatNumber(0); got != "0" {
		t.Errorf("formatNumber(0) = %s, want 0", got)
	}
	if got := formatNumber(999); got != "999" {
		t.Errorf("formatNumber(999) = %s, want 999", got)
	}
	if got := formatNumber(1000); got != "1,000" {
		t.Errorf("formatNumber(1000) = %s, want 1,000", got)
	}
	if got := formatNumber(50386); got != "50,386" {
		t.Errorf("formatNumber(50386) = %s, want 50,386", got)
	}

	// formatUptime
	if got := formatUptime(45); got != "0m 45s" {
		t.Errorf("formatUptime(45) = %s, want 0m 45s", got)
	}
	if got := formatUptime(125); got != "2m 05s" {
		t.Errorf("formatUptime(125) = %s, want 2m 05s", got)
	}
	if got := formatUptime(7200); got != "2h 00m" {
		t.Errorf("formatUptime(7200) = %s, want 2h 00m", got)
	}
	if got := formatUptime(396000); got != "4d 14h" {
		t.Errorf("formatUptime(396000) = %s, want 4d 14h", got)
	}
	if got := formatUptime(93600); got != "1d 02h" {
		t.Errorf("formatUptime(93600) = %s, want 1d 02h", got)
	}
}
