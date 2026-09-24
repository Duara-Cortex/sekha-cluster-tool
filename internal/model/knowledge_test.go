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

func TestRecallResponse_FormatMarkdown(t *testing.T) {
	t.Run("empty nodes", func(t *testing.T) {
		resp := RecallResponse{
			Nodes:          []ScoredNode{},
			QueryLatencyMS: 0.5,
		}
		got := resp.FormatMarkdown()
		expected := "No associative nodes matched the query."
		if got != expected {
			t.Errorf("expected '%s', got '%s'", expected, got)
		}
		if resp.FormatConcise() != expected {
			t.Errorf("FormatConcise should match FormatMarkdown")
		}
	})

	t.Run("nil response", func(t *testing.T) {
		var resp *RecallResponse
		got := resp.FormatMarkdown()
		expected := "No associative nodes matched the query."
		if got != expected {
			t.Errorf("expected '%s', got '%s'", expected, got)
		}
	})

	t.Run("nodes with anchors and edges", func(t *testing.T) {
		resp := RecallResponse{
			Nodes: []ScoredNode{
				{
					Node: Node{
						ID:         "node-001",
						Label:      "Kernel Panic Mitigation",
						EntityType: "policy",
						Summary:    "Threshold 70C triggers auxiliary fan override and emergency thermal throttling.",
						Anchors:    []string{"#project:kestrel", "#hardware:thermal"},
					},
					Score:    0.95,
					SimScore: 0.85,
				},
				{
					Node: Node{
						ID:         "node-002",
						Label:      "Fan Controller Override",
						EntityType: "device",
						Summary:    "Hardware PWM fan controller interface for secondary cooling.",
						Anchors:    []string{"#hardware:thermal"},
					},
					Score:    0.88,
					SimScore: 0.78,
				},
				{
					Node: Node{
						ID:         "node-003",
						Label:      "Thermal Monitoring Daemon",
						EntityType: "service",
						Summary:    "Background daemon polling CPU die temperatures every 100ms.",
						Anchors:    []string{"#service:monitor"},
					},
					Score:    0.82,
					SimScore: 0.72,
				},
				{
					Node: Node{
						ID:         "node-004",
						Label:      "NVMe Write Cache Policy",
						EntityType: "policy",
						Summary:    "Flush write-back cache on thermal warning events to prevent data loss.",
						Anchors:    nil,
					},
					Score:    0.75,
					SimScore: 0.65,
				},
				{
					Node: Node{
						ID:         "node-005",
						Label:      "Alert Dispatch Handler",
						EntityType: "event",
						Summary:    "Routes critical alerts to telemetry channel.",
						Anchors:    []string{},
					},
					Score:    0.70,
					SimScore: 0.60,
				},
			},
			Edges: []Edge{
				{
					SourceID:     "node-001",
					TargetID:     "node-002",
					RelationType: "governs",
					Weight:       0.90,
				},
				{
					SourceID:     "node-001",
					TargetID:     "node-004",
					RelationType: "triggers",
					Weight:       0.75,
				},
			},
			QueryLatencyMS: 1.25,
		}

		out := resp.FormatMarkdown()

		// Verify header
		if !strings.Contains(out, "# Recall Results (5 nodes, 1.25ms)") {
			t.Errorf("missing expected header in output:\n%s", out)
		}

		// Verify nodes
		expectedNode1 := "- [node-001] **Kernel Panic Mitigation** (`policy`, score: 0.95, sim: 0.85)\n  Summary: Threshold 70C triggers auxiliary fan override and emergency thermal throttling.\n  Anchors: #project:kestrel, #hardware:thermal"
		if !strings.Contains(out, expectedNode1) {
			t.Errorf("missing node-001 in output:\n%s", out)
		}

		expectedNode4 := "- [node-004] **NVMe Write Cache Policy** (`policy`, score: 0.75, sim: 0.65)\n  Summary: Flush write-back cache on thermal warning events to prevent data loss.\n  Anchors: none"
		if !strings.Contains(out, expectedNode4) {
			t.Errorf("missing node-004 (anchors: none) in output:\n%s", out)
		}

		// Verify relational subgraph
		if !strings.Contains(out, "## Relational Subgraph (2 edges)") {
			t.Errorf("missing relational subgraph header in output:\n%s", out)
		}
		expectedEdge1 := "- `node-001` --(governs, weight: 0.90)--> `node-002`"
		if !strings.Contains(out, expectedEdge1) {
			t.Errorf("missing edge-1 in output:\n%s", out)
		}
		expectedEdge2 := "- `node-001` --(triggers, weight: 0.75)--> `node-004`"
		if !strings.Contains(out, expectedEdge2) {
			t.Errorf("missing edge-2 in output:\n%s", out)
		}

		// Verify compactness (< 1.5 KB)
		if len(out) >= 1536 {
			t.Errorf("expected compact output (< 1.5 KB), got %d bytes", len(out))
		}

		// FormatConcise should match
		if resp.FormatConcise() != out {
			t.Errorf("FormatConcise output does not match FormatMarkdown")
		}
	})

	t.Run("nodes without edges", func(t *testing.T) {
		resp := RecallResponse{
			Nodes: []ScoredNode{
				{
					Node: Node{
						ID:         "node-001",
						Label:      "Solo Node",
						EntityType: "concept",
						Summary:    "A node with no relational edges.",
					},
					Score:    0.90,
					SimScore: 0.80,
				},
			},
			Edges:          []Edge{},
			QueryLatencyMS: 0.85,
		}

		out := resp.FormatMarkdown()
		if strings.Contains(out, "Relational Subgraph") {
			t.Errorf("expected no relational subgraph header when edges slice is empty, got:\n%s", out)
		}
	})
}

func TestRecallResponse_Filter(t *testing.T) {
	newSampleResponse := func() RecallResponse {
		return RecallResponse{
			Nodes: []ScoredNode{
				{
					Node: Node{
						ID:         "node-001",
						Label:      "Kernel Policy",
						EntityType: "policy",
						Summary:    "Kernel failure mitigation policy",
					},
					Score:    0.95,
					SimScore: 0.85,
				},
				{
					Node: Node{
						ID:         "node-002",
						Label:      "Fan Controller",
						EntityType: "device",
						Summary:    "Hardware fan controller interface",
					},
					Score:    0.80,
					SimScore: 0.70,
				},
				{
					Node: Node{
						ID:         "node-003",
						Label:      "Thermal Config",
						EntityType: "config",
						Summary:    "Configuration for thermal alerts",
					},
					Score:    0.65,
					SimScore: 0.55,
				},
				{
					Node: Node{
						ID:         "node-004",
						Label:      "Fallback Policy",
						EntityType: "policy",
						Summary:    "Secondary thermal policy",
					},
					Score:    0.50,
					SimScore: 0.40,
				},
			},
			Edges: []Edge{
				{SourceID: "node-001", TargetID: "node-002", RelationType: "governs", Weight: 0.90}, // policy -> device
				{SourceID: "node-001", TargetID: "node-004", RelationType: "triggers", Weight: 0.80}, // policy -> policy
				{SourceID: "node-002", TargetID: "node-003", RelationType: "uses", Weight: 0.70},     // device -> config
				{SourceID: "node-003", TargetID: "node-004", RelationType: "updates", Weight: 0.60},  // config -> policy
			},
			QueryLatencyMS: 1.25,
		}
	}

	t.Run("filter by entity type case-insensitively", func(t *testing.T) {
		resp := newSampleResponse()
		resp.Filter("POLICY", 0)

		if len(resp.Nodes) != 2 {
			t.Fatalf("expected 2 policy nodes, got %d", len(resp.Nodes))
		}
		if resp.Nodes[0].ID != "node-001" || resp.Nodes[1].ID != "node-004" {
			t.Errorf("unexpected nodes retained: %+v", resp.Nodes)
		}
		// Edges: only node-001 -> node-004 should be retained
		if len(resp.Edges) != 1 {
			t.Fatalf("expected 1 edge connecting policy nodes, got %d", len(resp.Edges))
		}
		if resp.Edges[0].SourceID != "node-001" || resp.Edges[0].TargetID != "node-004" {
			t.Errorf("unexpected edge retained: %+v", resp.Edges[0])
		}
	})

	t.Run("filter by min-score", func(t *testing.T) {
		resp := newSampleResponse()
		resp.Filter("", 0.70)

		if len(resp.Nodes) != 2 {
			t.Fatalf("expected 2 nodes with score >= 0.70, got %d", len(resp.Nodes))
		}
		if resp.Nodes[0].ID != "node-001" || resp.Nodes[1].ID != "node-002" {
			t.Errorf("unexpected nodes retained: %+v", resp.Nodes)
		}
		// Edges: only node-001 -> node-002 should be retained
		if len(resp.Edges) != 1 {
			t.Fatalf("expected 1 edge connecting nodes with score >= 0.70, got %d", len(resp.Edges))
		}
		if resp.Edges[0].SourceID != "node-001" || resp.Edges[0].TargetID != "node-002" {
			t.Errorf("unexpected edge retained: %+v", resp.Edges[0])
		}
	})

	t.Run("combined filter by type and min-score", func(t *testing.T) {
		resp := newSampleResponse()
		// Only policy with score >= 0.65 -> only node-001 (0.95), node-004 is 0.50
		resp.Filter("policy", 0.65)

		if len(resp.Nodes) != 1 {
			t.Fatalf("expected 1 node, got %d", len(resp.Nodes))
		}
		if resp.Nodes[0].ID != "node-001" {
			t.Errorf("expected node-001, got %s", resp.Nodes[0].ID)
		}
		// All edges should be pruned because no other nodes were retained
		if len(resp.Edges) != 0 {
			t.Fatalf("expected 0 edges, got %d", len(resp.Edges))
		}
	})

	t.Run("edge pruning removes orphaned relations", func(t *testing.T) {
		resp := newSampleResponse()
		// Filter by config -> only node-003 retained
		resp.Filter("config", 0)

		if len(resp.Nodes) != 1 || resp.Nodes[0].ID != "node-003" {
			t.Fatalf("expected only node-003, got %+v", resp.Nodes)
		}
		if len(resp.Edges) != 0 {
			t.Errorf("expected all edges pruned, got %d edges", len(resp.Edges))
		}
	})

	t.Run("empty criteria retains all", func(t *testing.T) {
		resp := newSampleResponse()
		resp.Filter("", 0.0)

		if len(resp.Nodes) != 4 {
			t.Errorf("expected all 4 nodes retained, got %d", len(resp.Nodes))
		}
		if len(resp.Edges) != 4 {
			t.Errorf("expected all 4 edges retained, got %d", len(resp.Edges))
		}
	})

	t.Run("nil response does not panic", func(t *testing.T) {
		var resp *RecallResponse
		resp.Filter("policy", 0.8)
	})
}


