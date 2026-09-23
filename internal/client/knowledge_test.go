package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

func TestKnowledgeClient_Recall_DefaultLeanSchema(t *testing.T) {
	var capturedURL string
	var capturedBody model.RecallRequest

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &capturedBody)

		// Mock lean schema response from Node 1
		resp := model.RecallResponse{
			Nodes: []model.ScoredNode{
				{
					Node: model.Node{
						ID:         "node-lean-1",
						EntityType: "concept",
						Label:      "Lean Node",
						Summary:    "Node with lean schema",
						Anchors:    []string{"#project:kestrel"},
					},
					Score:       0.92,
					AnchorScore: 1.0,
				},
			},
			Edges:          []model.Edge{},
			QueryLatencyMS: 1.1,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	client := NewKnowledgeClient(mockServer.URL, 2*time.Second)
	req := model.RecallRequest{
		Query:             "test query",
		TopK:              5,
		Anchors:           []string{"#project:kestrel"},
		AnchorMode:        "boost",
		IncludeEmbeddings: false,
	}

	res, err := client.Recall(context.Background(), req, "trc-recall-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(capturedURL, "include_embeddings") {
		t.Errorf("expected no include_embeddings in URL when false, got: %s", capturedURL)
	}
	if capturedBody.IncludeEmbeddings != false {
		t.Errorf("expected capturedBody.IncludeEmbeddings false, got true")
	}
	if len(capturedBody.Anchors) != 1 || capturedBody.Anchors[0] != "#project:kestrel" {
		t.Errorf("expected captured anchors ['#project:kestrel'], got %v", capturedBody.Anchors)
	}
	if capturedBody.AnchorMode != "boost" {
		t.Errorf("expected captured anchor mode 'boost', got %s", capturedBody.AnchorMode)
	}

	if len(res.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(res.Nodes))
	}
	if len(res.Nodes[0].Embedding) != 0 {
		t.Errorf("expected empty embedding in lean schema, got %v", res.Nodes[0].Embedding)
	}
	if len(res.Nodes[0].Anchors) != 1 || res.Nodes[0].Anchors[0] != "#project:kestrel" {
		t.Errorf("expected anchors on recalled node, got %v", res.Nodes[0].Anchors)
	}
}

func TestKnowledgeClient_Recall_IncludeEmbeddings(t *testing.T) {
	var capturedURL string
	var capturedBody model.RecallRequest

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &capturedBody)

		resp := model.RecallResponse{
			Nodes: []model.ScoredNode{
				{
					Node: model.Node{
						ID:        "node-emb-1",
						Label:     "Embedding Node",
						Embedding: []float32{0.1, 0.2, 0.3},
						Anchors:   []string{"#env:prod"},
					},
					Score: 0.85,
				},
			},
			Edges:          []model.Edge{},
			QueryLatencyMS: 0.9,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	client := NewKnowledgeClient(mockServer.URL, 2*time.Second)
	req := model.RecallRequest{
		Query:             "vector query",
		TopK:              3,
		Anchors:           []string{"#env:prod"},
		AnchorMode:        "filter",
		IncludeEmbeddings: true,
	}

	res, err := client.Recall(context.Background(), req, "trc-recall-emb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedURL, "include_embeddings=true") {
		t.Errorf("expected include_embeddings=true in URL, got: %s", capturedURL)
	}
	if !capturedBody.IncludeEmbeddings {
		t.Errorf("expected capturedBody.IncludeEmbeddings true")
	}
	if capturedBody.AnchorMode != "filter" {
		t.Errorf("expected capturedBody.AnchorMode 'filter', got %s", capturedBody.AnchorMode)
	}

	if len(res.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(res.Nodes))
	}
	if len(res.Nodes[0].Embedding) != 3 {
		t.Errorf("expected 3 embedding floats, got %d", len(res.Nodes[0].Embedding))
	}
}

func TestKnowledgeClient_Consolidate_AnchorForwarding(t *testing.T) {
	var capturedBody model.ConsolidateRequest

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &capturedBody)

		resp := model.ConsolidateResponse{
			Status:            "success",
			TraceID:           capturedBody.TraceID,
			Message:           "Trace consolidated",
			Synchronous:       capturedBody.Synchronous,
			EntitiesExtracted: 2,
			NodesFused:        1,
			EdgesReinforced:   1,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	client := NewKnowledgeClient(mockServer.URL, 2*time.Second)
	req := model.ConsolidateRequest{
		SessionID:   "session-test-anchor",
		TaskGoal:    "Deploy service",
		Outcome:     "success",
		Anchors:     []string{"#project:kestrel", "#env:staging"},
		Synchronous: true,
	}

	res, err := client.Consolidate(context.Background(), req, "trc-cons-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "success" {
		t.Errorf("expected Status 'success', got %s", res.Status)
	}
	if len(capturedBody.Anchors) != 2 {
		t.Fatalf("expected 2 anchors forwarded in consolidation request, got %d: %v",
			len(capturedBody.Anchors), capturedBody.Anchors)
	}
	if capturedBody.Anchors[0] != "#project:kestrel" || capturedBody.Anchors[1] != "#env:staging" {
		t.Errorf("expected anchors ['#project:kestrel', '#env:staging'], got %v", capturedBody.Anchors)
	}
}
