package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/telemetry"
)

func TestWorkingClient_Deliberate_StatelessNode2(t *testing.T) {
	var receivedTraceID string
	var receivedReq model.DeliberateRequest

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/working/deliberate" {
			http.NotFound(w, r)
			return
		}
		receivedTraceID = r.Header.Get(telemetry.HeaderTraceID)
		_ = json.NewDecoder(r.Body).Decode(&receivedReq)

		resp := model.DeliberateResponse{
			Status:           "ready",
			StepIndex:        1,
			Thought:          "Synthesised plan from context.",
			ProposedAction:   "EXECUTE_TASK",
			IsComplete:       true,
			PromptTokens:     120,
			CompletionTokens: 25,
			TotalTokens:      145,
			EvaluationRate:   35.2,
			GenerationRate:   18.4,
			ActiveGoal:       receivedReq.Objective,
			TrajectoryLength: 1,
			Timestamp:        time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	workingClient := NewWorkingClient(mockServer.URL, 2*time.Second)
	req := model.DeliberateRequest{
		Objective: "Test stateless deliberation",
		SensoryChunks: []model.SensoryChunk{
			{ID: "c1", Text: "salient sensor fact", Salience: 0.9},
		},
		LongTermContext: []string{"fact 1", "fact 2"},
		MaxTokens:       128,
		Temperature:     0.2,
	}

	traceID := "trc-working-test-123"
	resp, err := workingClient.Deliberate(context.Background(), req, traceID)
	if err != nil {
		t.Fatalf("unexpected error from Deliberate: %v", err)
	}

	if receivedTraceID != traceID {
		t.Errorf("expected trace ID %q, got %q", traceID, receivedTraceID)
	}
	if receivedReq.Objective != "Test stateless deliberation" {
		t.Errorf("expected objective %q, got %q", req.Objective, receivedReq.Objective)
	}
	if resp.StepIndex != 1 {
		t.Errorf("expected StepIndex 1, got %d", resp.StepIndex)
	}
	if resp.TrajectoryLength != 1 {
		t.Errorf("expected TrajectoryLength 1, got %d", resp.TrajectoryLength)
	}
	if resp.ProposedAction != "EXECUTE_TASK" {
		t.Errorf("expected ProposedAction 'EXECUTE_TASK', got %q", resp.ProposedAction)
	}
	if !resp.IsComplete {
		t.Errorf("expected IsComplete true, got false")
	}
}

func TestWorkingClient_GetHealth(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/working/health" {
			http.NotFound(w, r)
			return
		}
		resp := model.WorkingHealthResponse{
			Status:         "healthy",
			Node:           "node2",
			Port:           8083,
			Service:        "sekha-working-scratchpad",
			LlamaInference: "reachable",
			UptimeSeconds:  12345,
			Timestamp:      time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	workingClient := NewWorkingClient(mockServer.URL, 2*time.Second)
	health, err := workingClient.GetHealth(context.Background(), "trc-health-1")
	if err != nil {
		t.Fatalf("unexpected error from GetHealth: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("expected Status 'healthy', got %q", health.Status)
	}
	if health.Service != "sekha-working-scratchpad" {
		t.Errorf("expected Service 'sekha-working-scratchpad', got %q", health.Service)
	}
	if health.LlamaInference != "reachable" {
		t.Errorf("expected LlamaInference 'reachable', got %q", health.LlamaInference)
	}
	if health.UptimeSeconds != 12345 {
		t.Errorf("expected UptimeSeconds 12345, got %d", health.UptimeSeconds)
	}
}
