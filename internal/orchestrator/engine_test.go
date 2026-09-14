package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/client"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/telemetry"
)

func TestOrchestrator_FullCycleSuccess(t *testing.T) {
	var receivedTraceIDs []string

	// Mock Node 3 (Sensory Filter)
	node3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceIDs = append(receivedTraceIDs, r.Header.Get(telemetry.HeaderTraceID))
		resp := model.FilterResponse{
			Chunks: []model.SensoryChunk{
				{
					ID:        "chk-001",
					Text:      "High temperature alert on node 2 core",
					Salience:  0.92,
					Source:    "syslog",
					Timestamp: time.Now(),
				},
			},
			TotalChunks:    5,
			SalientChunks:  1,
			NoiseDiscarded: 4,
			ReductionRate:  0.80,
			LatencyMS:      0.45,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer node3Server.Close()

	// Mock Node 1 (Knowledge Recall & Consolidate)
	node1Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceIDs = append(receivedTraceIDs, r.Header.Get(telemetry.HeaderTraceID))
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/api/v1/memory/recall" {
			resp := model.RecallResponse{
				Nodes: []model.ScoredNode{
					{
						Node: model.Node{
							ID:         "fact-thermal-01",
							EntityType: "hardware_policy",
							Label:      "Core Thermal Threshold",
							Summary:    "Core temperature above 80C requires fan speed increase to level 3.",
						},
						Score: 0.95,
					},
				},
				QueryLatencyMS: 1.25,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/api/v1/memory/consolidate" {
			resp := model.ConsolidateResponse{
				Status:            "success",
				TraceID:           r.Header.Get(telemetry.HeaderTraceID),
				Message:           "Trace enqueued for background consolidation",
				EntitiesExtracted: 2,
				NodesFused:        1,
				EdgesReinforced:   1,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer node1Server.Close()

	// Mock Node 2 (Working Scratchpad Deliberation)
	node2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceIDs = append(receivedTraceIDs, r.Header.Get(telemetry.HeaderTraceID))
		w.Header().Set("Content-Type", "application/json")

		resp := model.DeliberateResponse{
			Status:           "ready",
			StepIndex:        1,
			Thought:          "Temperature spike detected. Policy requires increasing fan level.",
			ProposedAction:   "SET_FAN_SPEED_LEVEL_3",
			IsComplete:       true,
			PromptTokens:     145,
			CompletionTokens: 32,
			TotalTokens:      177,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer node2Server.Close()

	cfg := client.Config{
		SensoryURL:        node3Server.URL,
		WorkingURL:        node2Server.URL,
		KnowledgeURL:      node1Server.URL,
		DefaultTimeout:    2 * time.Second,
		DeliberateTimeout: 2 * time.Second,
	}

	orch := NewOrchestrator(cfg)
	traceID := "trc-test-cycle-12345"

	req := model.OrchestrateRequest{
		RawInput:               "syslog raw data: node 2 core temp 82C warning",
		TaskDirective:          "Manage cluster thermals",
		FilterThreshold:        0.45,
		RecallTopK:             3,
		MaxTokens:              128,
		SessionID:              "session-test-01",
		SynchronousConsolidate: true,
	}

	res, err := orch.RunCycle(context.Background(), req, traceID)
	if err != nil {
		t.Fatalf("unexpected error running cognitive cycle: %v", err)
	}

	if res.TraceID != traceID {
		t.Errorf("expected TraceID '%s', got '%s'", traceID, res.TraceID)
	}
	if res.Status != "completed" {
		t.Errorf("expected Status 'completed', got '%s'", res.Status)
	}
	if !res.IsComplete {
		t.Errorf("expected IsComplete true, got false")
	}
	if res.ProposedAction != "SET_FAN_SPEED_LEVEL_3" {
		t.Errorf("expected ProposedAction 'SET_FAN_SPEED_LEVEL_3', got '%s'", res.ProposedAction)
	}
	if len(res.Stages) != 4 {
		t.Fatalf("expected 4 stage telemetry entries, got %d", len(res.Stages))
	}
	for i, stage := range res.Stages {
		if stage.Status != "success" {
			t.Errorf("stage %d (%s) failed with error: %s", i, stage.StageName, stage.Error)
		}
	}

	// Verify Trace ID was propagated to all 4 requests
	if len(receivedTraceIDs) != 4 {
		t.Errorf("expected 4 HTTP calls received, got %d", len(receivedTraceIDs))
	}
	for _, id := range receivedTraceIDs {
		if id != traceID {
			t.Errorf("expected trace header '%s', got '%s'", traceID, id)
		}
	}
}

func TestOrchestrator_SensoryFallbackOnFailure(t *testing.T) {
	// Node 3 is broken / returning 500
	node3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer node3Server.Close()

	// Node 1 (Knowledge Recall & Consolidate)
	node1Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/memory/recall" {
			_ = json.NewEncoder(w).Encode(model.RecallResponse{Nodes: []model.ScoredNode{}})
			return
		}
		_ = json.NewEncoder(w).Encode(model.ConsolidateResponse{Status: "queued"})
	}))
	defer node1Server.Close()

	// Node 2 (Working Deliberation)
	node2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.DeliberateResponse{
			Status:         "ready",
			Thought:        "Sensory fallback handled cleanly.",
			ProposedAction: "LOG_RAW_METRIC",
			IsComplete:     true,
		})
	}))
	defer node2Server.Close()

	cfg := client.Config{
		SensoryURL:        node3Server.URL,
		WorkingURL:        node2Server.URL,
		KnowledgeURL:      node1Server.URL,
		DefaultTimeout:    1 * time.Second,
		DeliberateTimeout: 1 * time.Second,
	}

	orch := NewOrchestrator(cfg)
	req := model.OrchestrateRequest{
		RawInput:      "critical fallback payload",
		TaskDirective: "Diagnose error",
	}

	res, err := orch.RunCycle(context.Background(), req, "trc-fallback-test")
	if err != nil {
		t.Fatalf("cycle should not fail completely on sensory fallback: %v", err)
	}

	if res.Stages[0].Status != "failed" {
		t.Errorf("expected stage 1 status 'failed', got '%s'", res.Stages[0].Status)
	}
	if len(res.Sensory.Chunks) != 1 || res.Sensory.Chunks[0].Source != "fallback_unfiltered" {
		t.Errorf("expected fallback sensory chunk to be created")
	}
	if res.ProposedAction != "LOG_RAW_METRIC" {
		t.Errorf("expected deliberation to proceed despite sensory stage failure")
	}
}
