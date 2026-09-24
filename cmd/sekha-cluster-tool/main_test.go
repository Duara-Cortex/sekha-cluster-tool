package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

func TestParseArgs_FlagPositions(t *testing.T) {
	tests := []struct {
		name                      string
		args                      []string
		expectedSubcommand        string
		expectedSensory           string
		expectedWorking           string
		expectedKnowledge         string
		expectedAPIKey            string
		expectedVerbose           bool
		expectedTimeout           time.Duration
		expectedAnchors           []string
		expectedAnchorMode        string
		expectedIncludeEmbeddings bool
		expectedFormat            string
		expectedEntityType        string
		expectedMinScore          float64
		expectedArgs              []string
	}{
		{
			name:               "subcommand only",
			args:               []string{"status"},
			expectedSubcommand: "status",
			expectedArgs:       []string{},
		},
		{
			name:               "flag before subcommand",
			args:               []string{"--sensory-url", "http://192.168.8.183:8081", "status"},
			expectedSubcommand: "status",
			expectedSensory:    "http://192.168.8.183:8081",
			expectedArgs:       []string{},
		},
		{
			name:               "flag after subcommand",
			args:               []string{"status", "--sensory-url", "http://192.168.8.183:8081"},
			expectedSubcommand: "status",
			expectedSensory:    "http://192.168.8.183:8081",
			expectedArgs:       []string{},
		},
		{
			name:               "inline flags both before and after",
			args:               []string{"--sensory-url=http://sensory:8081", "status", "--working-url=http://working:8083"},
			expectedSubcommand: "status",
			expectedSensory:    "http://sensory:8081",
			expectedWorking:    "http://working:8083",
			expectedArgs:       []string{},
		},
		{
			name:               "legacy aliases",
			args:               []string{"--node3-url", "http://node3:8081", "status", "--node2-url", "http://node2:8083", "--node1-url", "http://node1:8084"},
			expectedSubcommand: "status",
			expectedSensory:    "http://node3:8081",
			expectedWorking:    "http://node2:8083",
			expectedKnowledge:  "http://node1:8084",
			expectedArgs:       []string{},
		},
		{
			name:               "orchestration url alias for working url",
			args:               []string{"--orchestration-url", "http://orch:8083", "status"},
			expectedSubcommand: "status",
			expectedWorking:    "http://orch:8083",
			expectedArgs:       []string{},
		},
		{
			name:               "subcommand with subcommand-specific flags interleaved with global flags",
			args:               []string{"--sensory-url", "http://sensory:8081", "filter", "--text", "hello world", "--verbose", "--timeout", "500ms"},
			expectedSubcommand: "filter",
			expectedSensory:    "http://sensory:8081",
			expectedVerbose:    true,
			expectedTimeout:    500 * time.Millisecond,
			expectedArgs:       []string{"--text", "hello world"},
		},
		{
			name:               "subcommand args before global flag",
			args:               []string{"filter", "--text", "hello world", "--sensory-url", "http://sensory:8081"},
			expectedSubcommand: "filter",
			expectedSensory:    "http://sensory:8081",
			expectedArgs:       []string{"--text", "hello world"},
		},
		{
			name:               "anchor flag -a before subcommand",
			args:               []string{"-a", "#project:kestrel", "recall", "--query", "port"},
			expectedSubcommand: "recall",
			expectedAnchors:    []string{"#project:kestrel"},
			expectedArgs:       []string{"--query", "port"},
		},
		{
			name:               "repeatable -a flags after subcommand",
			args:               []string{"recall", "--query", "port", "-a", "#project:kestrel", "-a", "#auth:jwt"},
			expectedSubcommand: "recall",
			expectedAnchors:    []string{"#project:kestrel", "#auth:jwt"},
			expectedArgs:       []string{"--query", "port"},
		},
		{
			name:               "comma-delimited inline --anchor flag",
			args:               []string{"recall", "--anchor=#project:kestrel,#auth:jwt", "--query", "port"},
			expectedSubcommand: "recall",
			expectedAnchors:    []string{"#project:kestrel,#auth:jwt"},
			expectedArgs:       []string{"--query", "port"},
		},
		{
			name:                      "anchor-mode and include-embeddings flags",
			args:                      []string{"recall", "--query", "port", "--anchor-mode", "filter", "--include-embeddings"},
			expectedSubcommand:        "recall",
			expectedAnchorMode:        "filter",
			expectedIncludeEmbeddings: true,
			expectedArgs:              []string{"--query", "port"},
		},
		{
			name:               "api-key flag before subcommand",
			args:               []string{"--api-key", "secret-test-key", "status"},
			expectedSubcommand: "status",
			expectedAPIKey:     "secret-test-key",
			expectedArgs:       []string{},
		},
		{
			name:               "inline api-key flag after subcommand",
			args:               []string{"recall", "--api-key=inline-secret-key", "--query", "port"},
			expectedSubcommand: "recall",
			expectedAPIKey:     "inline-secret-key",
			expectedArgs:       []string{"--query", "port"},
		},
		{
			name:               "legacy key alias",
			args:               []string{"--key", "legacy-key", "status"},
			expectedSubcommand: "status",
			expectedAPIKey:     "legacy-key",
			expectedArgs:       []string{},
		},
		{
			name:               "format flag before subcommand",
			args:               []string{"--format", "markdown", "recall", "--query", "port"},
			expectedSubcommand: "recall",
			expectedFormat:     "markdown",
			expectedArgs:       []string{"--query", "port"},
		},
		{
			name:               "format flag after subcommand",
			args:               []string{"recall", "--query", "port", "--format", "concise"},
			expectedSubcommand: "recall",
			expectedFormat:     "concise",
			expectedArgs:       []string{"--query", "port"},
		},
		{
			name:               "inline format flag",
			args:               []string{"recall", "--format=json", "--query", "port"},
			expectedSubcommand: "recall",
			expectedFormat:     "json",
			expectedArgs:       []string{"--query", "port"},
		},
		{
			name:               "type flag --type before subcommand",
			args:               []string{"--type", "policy", "recall", "--query", "port"},
			expectedSubcommand: "recall",
			expectedEntityType: "policy",
			expectedArgs:       []string{"--query", "port"},
		},
		{
			name:               "type flag -t alias after subcommand",
			args:               []string{"recall", "--query", "port", "-t", "config"},
			expectedSubcommand: "recall",
			expectedEntityType: "config",
			expectedArgs:       []string{"--query", "port"},
		},
		{
			name:               "min-score flag after subcommand",
			args:               []string{"recall", "--query", "port", "--min-score", "0.75"},
			expectedSubcommand: "recall",
			expectedMinScore:   0.75,
			expectedArgs:       []string{"--query", "port"},
		},
		{
			name:               "combined flags: anchor, type, min-score, format",
			args:               []string{"recall", "--query", "ingest port", "-a", "#project:kestrel", "--type", "config", "--min-score=0.65", "--format", "concise"},
			expectedSubcommand: "recall",
			expectedAnchors:    []string{"#project:kestrel"},
			expectedEntityType: "config",
			expectedMinScore:   0.65,
			expectedFormat:     "concise",
			expectedArgs:       []string{"--query", "ingest port"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			global, subcmd, remaining := parseArgs(tt.args)

			if subcmd != tt.expectedSubcommand {
				t.Errorf("expected subcommand '%s', got '%s'", tt.expectedSubcommand, subcmd)
			}
			if global.SensoryURL != tt.expectedSensory {
				t.Errorf("expected SensoryURL '%s', got '%s'", tt.expectedSensory, global.SensoryURL)
			}
			if global.WorkingURL != tt.expectedWorking {
				t.Errorf("expected WorkingURL '%s', got '%s'", tt.expectedWorking, global.WorkingURL)
			}
			if global.KnowledgeURL != tt.expectedKnowledge {
				t.Errorf("expected KnowledgeURL '%s', got '%s'", tt.expectedKnowledge, global.KnowledgeURL)
			}
			if global.APIKey != tt.expectedAPIKey {
				t.Errorf("expected APIKey '%s', got '%s'", tt.expectedAPIKey, global.APIKey)
			}
			if global.Verbose != tt.expectedVerbose {
				t.Errorf("expected Verbose %v, got %v", tt.expectedVerbose, global.Verbose)
			}
			if global.Timeout != tt.expectedTimeout {
				t.Errorf("expected Timeout %v, got %v", tt.expectedTimeout, global.Timeout)
			}
			if len(global.Anchors) != len(tt.expectedAnchors) {
				t.Errorf("expected Anchors %v, got %v", tt.expectedAnchors, global.Anchors)
			} else {
				for i := range global.Anchors {
					if global.Anchors[i] != tt.expectedAnchors[i] {
						t.Errorf("anchor[%d]: expected '%s', got '%s'", i, tt.expectedAnchors[i], global.Anchors[i])
					}
				}
			}
			if global.AnchorMode != tt.expectedAnchorMode {
				t.Errorf("expected AnchorMode '%s', got '%s'", tt.expectedAnchorMode, global.AnchorMode)
			}
			if global.IncludeEmbeddings != tt.expectedIncludeEmbeddings {
				t.Errorf("expected IncludeEmbeddings %v, got %v", tt.expectedIncludeEmbeddings, global.IncludeEmbeddings)
			}
			if global.Format != tt.expectedFormat {
				t.Errorf("expected Format '%s', got '%s'", tt.expectedFormat, global.Format)
			}
			if global.EntityType != tt.expectedEntityType {
				t.Errorf("expected EntityType '%s', got '%s'", tt.expectedEntityType, global.EntityType)
			}
			if global.MinScore != tt.expectedMinScore {
				t.Errorf("expected MinScore %f, got %f", tt.expectedMinScore, global.MinScore)
			}
			if len(remaining) != len(tt.expectedArgs) {
				t.Fatalf("expected remaining args length %d, got %d (%v)", len(tt.expectedArgs), len(remaining), remaining)
			}
			for i := range remaining {
				if remaining[i] != tt.expectedArgs[i] {
					t.Errorf("arg[%d]: expected '%s', got '%s'", i, tt.expectedArgs[i], remaining[i])
				}
			}
		})
	}
}

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		input       string
		expected    string
		expectError bool
	}{
		{"", "json", false},
		{"json", "json", false},
		{"concise", "concise", false},
		{"markdown", "markdown", false},
		{"  MARKDOWN  ", "markdown", false},
		{"CONCISE", "concise", false},
		{"xml", "", true},
		{"yaml", "", true},
		{"csv", "", true},
	}

	for _, tt := range tests {
		got, err := validateFormat(tt.input)
		if tt.expectError {
			if err == nil {
				t.Errorf("expected error for input '%s', got nil", tt.input)
			} else {
				expectedErr := fmt.Sprintf("invalid --format '%s': must be 'json', 'concise', or 'markdown'", tt.input)
				if err.Error() != expectedErr {
					t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
				}
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error for input '%s': %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		}
	}
}

func TestRenderRecallOutput(t *testing.T) {
	resp := &model.RecallResponse{
		Nodes: []model.ScoredNode{
			{
				Node: model.Node{
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
				Node: model.Node{
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
				Node: model.Node{
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
				Node: model.Node{
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
				Node: model.Node{
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
		Edges: []model.Edge{
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

	t.Run("--format json", func(t *testing.T) {
		var buf bytes.Buffer
		err := renderRecallOutput(&buf, resp, "json")
		if err != nil {
			t.Fatalf("unexpected error rendering json: %v", err)
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Fatalf("failed to decode JSON output: %v", err)
		}
		nodes, ok := decoded["nodes"].([]interface{})
		if !ok || len(nodes) != 5 {
			t.Errorf("expected 5 nodes in JSON, got %v", decoded["nodes"])
		}
	})

	t.Run("--format concise", func(t *testing.T) {
		var buf bytes.Buffer
		err := renderRecallOutput(&buf, resp, "concise")
		if err != nil {
			t.Fatalf("unexpected error rendering concise: %v", err)
		}
		out := buf.String()

		if !strings.Contains(out, "# Recall Results (5 nodes, 1.25ms)") {
			t.Errorf("missing header in concise output:\n%s", out)
		}
		if !strings.Contains(out, "- [node-001] **Kernel Panic Mitigation** (`policy`, score: 0.95, sim: 0.85)") {
			t.Errorf("missing node-001 in concise output:\n%s", out)
		}
		if !strings.Contains(out, "  Summary: Threshold 70C triggers auxiliary fan override and emergency thermal throttling.") {
			t.Errorf("missing summary in concise output:\n%s", out)
		}
		if !strings.Contains(out, "  Anchors: #project:kestrel, #hardware:thermal") {
			t.Errorf("missing anchors in concise output:\n%s", out)
		}
		if !strings.Contains(out, "  Anchors: none") {
			t.Errorf("missing 'Anchors: none' in concise output:\n%s", out)
		}
		if !strings.Contains(out, "## Relational Subgraph (2 edges)") {
			t.Errorf("missing relational subgraph header in concise output:\n%s", out)
		}
		if !strings.Contains(out, "- `node-001` --(governs, weight: 0.90)--> `node-002`") {
			t.Errorf("missing edge 1 in concise output:\n%s", out)
		}

		// Verify compactness (<1.5 KB)
		if len(out) >= 1536 {
			t.Errorf("expected concise output < 1.5 KB, got %d bytes", len(out))
		}
	})

	t.Run("--format markdown", func(t *testing.T) {
		var buf bytes.Buffer
		err := renderRecallOutput(&buf, resp, "markdown")
		if err != nil {
			t.Fatalf("unexpected error rendering markdown: %v", err)
		}
		out := buf.String()

		if !strings.Contains(out, "# Recall Results (5 nodes, 1.25ms)") {
			t.Errorf("missing header in markdown output:\n%s", out)
		}
		if len(out) >= 1536 {
			t.Errorf("expected markdown output < 1.5 KB, got %d bytes", len(out))
		}
	})

	t.Run("empty nodes", func(t *testing.T) {
		emptyResp := &model.RecallResponse{
			Nodes: []model.ScoredNode{},
		}
		var buf bytes.Buffer
		err := renderRecallOutput(&buf, emptyResp, "markdown")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "No associative nodes matched the query." {
			t.Errorf("expected 'No associative nodes matched the query.', got '%s'", strings.TrimSpace(buf.String()))
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		var buf bytes.Buffer
		err := renderRecallOutput(&buf, resp, "yaml")
		if err == nil {
			t.Fatalf("expected error for invalid format 'yaml', got nil")
		}
		expectedErr := "invalid --format 'yaml': must be 'json', 'concise', or 'markdown'"
		if err.Error() != expectedErr {
			t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})
}

func TestRecall_EntityFilteringAndPruning(t *testing.T) {
	newResponse := func() *model.RecallResponse {
		return &model.RecallResponse{
			Nodes: []model.ScoredNode{
				{
					Node: model.Node{
						ID:         "node-001",
						Label:      "Network Gateway Policy",
						EntityType: "policy",
						Summary:    "Ingress filtering policy",
						Anchors:    []string{"#project:kestrel"},
					},
					Score:    0.95,
					SimScore: 0.85,
				},
				{
					Node: model.Node{
						ID:         "node-002",
						Label:      "Ethernet Device",
						EntityType: "device",
						Summary:    "Physical eth0 interface",
					},
					Score:    0.80,
					SimScore: 0.70,
				},
				{
					Node: model.Node{
						ID:         "node-003",
						Label:      "Port Ingest Config",
						EntityType: "config",
						Summary:    "Telemetry ingest port mapping",
						Anchors:    []string{"#project:kestrel"},
					},
					Score:    0.72,
					SimScore: 0.62,
				},
				{
					Node: model.Node{
						ID:         "node-004",
						Label:      "Low Priority Config",
						EntityType: "config",
						Summary:    "Legacy fallback port",
					},
					Score:    0.50,
					SimScore: 0.40,
				},
			},
			Edges: []model.Edge{
				{SourceID: "node-001", TargetID: "node-002", RelationType: "controls", Weight: 0.90},
				{SourceID: "node-001", TargetID: "node-003", RelationType: "configures", Weight: 0.85},
				{SourceID: "node-003", TargetID: "node-004", RelationType: "overrides", Weight: 0.70},
			},
			QueryLatencyMS: 1.25,
		}
	}

	t.Run("entity type filtering drops non-matching types", func(t *testing.T) {
		resp := newResponse()
		resp.Filter("config", 0.0)

		if len(resp.Nodes) != 2 {
			t.Fatalf("expected 2 config nodes, got %d", len(resp.Nodes))
		}
		for _, n := range resp.Nodes {
			if !strings.EqualFold(n.EntityType, "config") {
				t.Errorf("expected only config entity type, got %s", n.EntityType)
			}
		}
		// Edges: node-001 is dropped, so controls and configures are pruned; node-003 -> node-004 is kept
		if len(resp.Edges) != 1 {
			t.Fatalf("expected 1 edge connecting config nodes, got %d", len(resp.Edges))
		}
		if resp.Edges[0].SourceID != "node-003" || resp.Edges[0].TargetID != "node-004" {
			t.Errorf("unexpected edge retained: %+v", resp.Edges[0])
		}
	})

	t.Run("min-score filtering drops nodes below threshold", func(t *testing.T) {
		resp := newResponse()
		resp.Filter("", 0.75)

		// Retained: node-001 (0.95), node-002 (0.80). node-003 (0.72) and node-004 (0.50) dropped
		if len(resp.Nodes) != 2 {
			t.Fatalf("expected 2 nodes with score >= 0.75, got %d", len(resp.Nodes))
		}
		if resp.Nodes[0].ID != "node-001" || resp.Nodes[1].ID != "node-002" {
			t.Errorf("unexpected nodes retained: %+v", resp.Nodes)
		}
		// Edges: only node-001 -> node-002 should remain
		if len(resp.Edges) != 1 {
			t.Fatalf("expected 1 edge connecting retained nodes, got %d", len(resp.Edges))
		}
		if resp.Edges[0].SourceID != "node-001" || resp.Edges[0].TargetID != "node-002" {
			t.Errorf("unexpected edge retained: %+v", resp.Edges[0])
		}
	})

	t.Run("edge pruning removes orphaned relations when target dropped", func(t *testing.T) {
		resp := newResponse()
		// Retain only node-001 (0.95)
		resp.Filter("", 0.90)

		if len(resp.Nodes) != 1 || resp.Nodes[0].ID != "node-001" {
			t.Fatalf("expected only node-001, got %+v", resp.Nodes)
		}
		if len(resp.Edges) != 0 {
			t.Fatalf("expected all edges pruned, got %d edges", len(resp.Edges))
		}
	})

	t.Run("combined pipeline with concise format", func(t *testing.T) {
		resp := newResponse()
		// Filter by config with min-score 0.65 -> only node-003 (0.72)
		resp.Filter("config", 0.65)

		if len(resp.Nodes) != 1 || resp.Nodes[0].ID != "node-003" {
			t.Fatalf("expected only node-003, got %+v", resp.Nodes)
		}
		if len(resp.Edges) != 0 {
			t.Fatalf("expected 0 edges, got %d", len(resp.Edges))
		}

		var buf bytes.Buffer
		if err := renderRecallOutput(&buf, resp, "concise"); err != nil {
			t.Fatalf("render error: %v", err)
		}
		out := buf.String()

		if !strings.Contains(out, "# Recall Results (1 nodes, 1.25ms)") {
			t.Errorf("missing header in output:\n%s", out)
		}
		if !strings.Contains(out, "- [node-003] **Port Ingest Config** (`config`, score: 0.72, sim: 0.62)") {
			t.Errorf("missing node-003 in output:\n%s", out)
		}
		if strings.Contains(out, "Relational Subgraph") {
			t.Errorf("should not contain relational subgraph when edges are pruned:\n%s", out)
		}
	})
}

