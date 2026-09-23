package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNormalizeAnchor(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"#project:kestrel", "#project:kestrel"},
		{"project:kestrel", "#project:kestrel"},
		{" #PROJECT:KESTREL ", "#project:kestrel"},
		{"#auth:jwt", "#auth:jwt"},
		{"auth:jwt", "#auth:jwt"},
		{"", ""},
		{"#", ""},
		{"   ", ""},
		{"  #  ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeAnchor(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeAnchor(%q) = %q, expected %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseAnchors(t *testing.T) {
	tests := []struct {
		name     string
		raw      []string
		expected []string
	}{
		{
			name:     "single anchor prefixed",
			raw:      []string{"#project:kestrel"},
			expected: []string{"#project:kestrel"},
		},
		{
			name:     "single anchor unprefixed",
			raw:      []string{"project:kestrel"},
			expected: []string{"#project:kestrel"},
		},
		{
			name:     "repeatable anchors with case variants and duplicates",
			raw:      []string{"#project:kestrel", "pattern:circuit-breaker", " #PROJECT:KESTREL ", "#auth:jwt"},
			expected: []string{"#project:kestrel", "#pattern:circuit-breaker", "#auth:jwt"},
		},
		{
			name:     "comma-delimited anchors",
			raw:      []string{"#project:kestrel, #pattern:circuit-breaker , auth:jwt"},
			expected: []string{"#project:kestrel", "#pattern:circuit-breaker", "#auth:jwt"},
		},
		{
			name:     "mixed repeatable and comma-delimited",
			raw:      []string{"-a", "#project:kestrel,#env:prod", "cluster:eu-west"},
			expected: []string{"#-a", "#project:kestrel", "#env:prod", "#cluster:eu-west"},
		},
		{
			name:     "empty items",
			raw:      []string{"", "  ", "#", ",,,"},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAnchors(tt.raw...)
			if len(got) != len(tt.expected) {
				t.Fatalf("ParseAnchors() returned %d items (%v), expected %d (%v)",
					len(got), got, len(tt.expected), tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("item[%d] = %q, expected %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestRecallResponse_LeanSchemaSerialization(t *testing.T) {
	// Raw JSON returned by sekha-knowledge-store when include_embeddings=false (lean schema)
	rawLeanJSON := `{
		"nodes": [
			{
				"id": "node-kestrel-1",
				"entity_type": "decision",
				"label": "Kestrel Ingest Daemon Port",
				"summary": "Configured port 8084 for Project Kestrel",
				"score": 1.95,
				"anchors": ["#project:kestrel"],
				"anchor_score": 1.0,
				"created_at": "2026-09-21T19:00:00Z",
				"last_accessed_at": "2026-09-21T19:05:00Z"
			}
		],
		"edges": [
			{
				"source_id": "node-kestrel-1",
				"target_id": "node-kestrel-2",
				"relation_type": "depends_on",
				"weight": 0.9,
				"created_at": "2026-09-21T19:00:00Z"
			}
		],
		"query_latency_ms": 1.25
	}`

	var resp RecallResponse
	if err := json.Unmarshal([]byte(rawLeanJSON), &resp); err != nil {
		t.Fatalf("failed to unmarshal lean schema JSON: %v", err)
	}

	// 1. Verify typed access in Go
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(resp.Nodes))
	}
	node := resp.Nodes[0]
	if node.ID != "node-kestrel-1" {
		t.Errorf("expected ID 'node-kestrel-1', got %q", node.ID)
	}
	if node.EntityType != "decision" {
		t.Errorf("expected EntityType 'decision', got %q", node.EntityType)
	}
	if node.Label != "Kestrel Ingest Daemon Port" {
		t.Errorf("expected Label 'Kestrel Ingest Daemon Port', got %q", node.Label)
	}
	if node.Score != 1.95 {
		t.Errorf("expected Score 1.95, got %f", node.Score)
	}
	if len(node.Anchors) != 1 || node.Anchors[0] != "#project:kestrel" {
		t.Errorf("expected Anchors ['#project:kestrel'], got %v", node.Anchors)
	}
	if node.AnchorScore != 1.0 {
		t.Errorf("expected AnchorScore 1.0, got %f", node.AnchorScore)
	}
	if len(node.Embedding) != 0 {
		t.Errorf("expected nil/empty Embedding, got %v", node.Embedding)
	}
	if node.SimScore != 0.0 {
		t.Errorf("expected SimScore 0.0, got %f", node.SimScore)
	}

	// 2. Verify re-serialization to JSON preserves the lean schema
	outBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal RecallResponse: %v", err)
	}
	outStr := string(outBytes)

	// Ensure float embeddings are NOT in the serialized output
	if strings.Contains(outStr, `"embedding"`) {
		t.Errorf("expected 'embedding' to be omitted from lean JSON output, got:\n%s", outStr)
	}

	// Ensure zero values not in the input are NOT artificially inflated
	if strings.Contains(outStr, `"sim_score"`) {
		t.Errorf("expected 'sim_score' to be omitted when zero, got:\n%s", outStr)
	}
	if strings.Contains(outStr, `"frequency_score"`) {
		t.Errorf("expected 'frequency_score' to be omitted when zero, got:\n%s", outStr)
	}
	if strings.Contains(outStr, `"0001-01-01T00:00:00Z"`) {
		t.Errorf("expected zero time not to be serialized, got:\n%s", outStr)
	}
}

func TestRecallResponse_IncludeEmbeddingsSerialization(t *testing.T) {
	rawWithEmbJSON := `{
		"nodes": [
			{
				"id": "node-emb-1",
				"label": "Node with Embeddings",
				"embedding": [0.1, 0.2, 0.3, 0.4],
				"score": 0.88,
				"anchors": ["#env:prod"]
			}
		],
		"edges": [],
		"query_latency_ms": 0.95
	}`

	var resp RecallResponse
	if err := json.Unmarshal([]byte(rawWithEmbJSON), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(resp.Nodes))
	}
	if len(resp.Nodes[0].Embedding) != 4 {
		t.Errorf("expected 4 embedding floats, got %d", len(resp.Nodes[0].Embedding))
	}

	outBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal RecallResponse: %v", err)
	}
	outStr := string(outBytes)
	if !strings.Contains(outStr, `"embedding"`) {
		t.Errorf("expected 'embedding' in output JSON when present, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "0.1") {
		t.Errorf("expected float value 0.1 in output JSON, got:\n%s", outStr)
	}
}

func TestRecallResponse_FieldProjection(t *testing.T) {
	rawProjJSON := `{
		"nodes": [
			{
				"id": "node-proj-1",
				"label": "Projected Node",
				"score": 0.77
			}
		],
		"edges": [],
		"query_latency_ms": 0.55
	}`

	var resp RecallResponse
	if err := json.Unmarshal([]byte(rawProjJSON), &resp); err != nil {
		t.Fatalf("failed to unmarshal projected JSON: %v", err)
	}

	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(resp.Nodes))
	}
	if resp.Nodes[0].ID != "node-proj-1" {
		t.Errorf("expected ID 'node-proj-1', got %q", resp.Nodes[0].ID)
	}

	outBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var rawMap map[string]any
	if err := json.Unmarshal(outBytes, &rawMap); err != nil {
		t.Fatalf("failed to parse output back: %v", err)
	}

	nodeObj := rawMap["nodes"].([]any)[0].(map[string]any)
	if len(nodeObj) != 3 {
		t.Fatalf("expected exactly 3 fields in projected node object (id, label, score), got %d: %+v", len(nodeObj), nodeObj)
	}
}

func TestRecallResponse_ConstructedInMemory(t *testing.T) {
	now := time.Now().UTC()
	resp := RecallResponse{
		Nodes: []ScoredNode{
			{
				Node: Node{
					ID:         "node-mem-1",
					EntityType: "concept",
					Label:      "In-Memory Node",
					Summary:    "Directly initialized in test",
					Anchors:    []string{"#project:kestrel"},
					CreatedAt:  &now,
				},
				Score:       0.95,
				AnchorScore: 1.0,
			},
		},
		Edges:          []Edge{},
		QueryLatencyMS: 0.42,
	}

	outBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	outStr := string(outBytes)

	if !strings.Contains(outStr, `"id":"node-mem-1"`) {
		t.Errorf("expected id in output JSON, got: %s", outStr)
	}
	if !strings.Contains(outStr, `"anchors":["#project:kestrel"]`) {
		t.Errorf("expected anchors in output JSON, got: %s", outStr)
	}
	if !strings.Contains(outStr, `"anchor_score":1`) {
		t.Errorf("expected anchor_score in output JSON, got: %s", outStr)
	}
	if strings.Contains(outStr, `"sim_score"`) {
		t.Errorf("expected zero sim_score to be omitted, got: %s", outStr)
	}
	if strings.Contains(outStr, `"embedding"`) {
		t.Errorf("expected nil embedding to be omitted, got: %s", outStr)
	}
}
