package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
		expectedJSON              bool
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
		{
			name:               "positional consolidation arguments with anchor and sync",
			args:               []string{"consolidate", "Project Kestrel", "INGEST_PORT: 51742; AUTH_HEADER: X-Kestrel-Key", "-a", "#project:kestrel", "--sync"},
			expectedSubcommand: "consolidate",
			expectedAnchors:    []string{"#project:kestrel"},
			expectedArgs:       []string{"Project Kestrel", "INGEST_PORT: 51742; AUTH_HEADER: X-Kestrel-Key", "--sync"},
		},
		{
			name:               "orchestrate with task flag",
			args:               []string{"orchestrate", "--input", "syslog alert", "--task", "Mitigate high temp", "--sync"},
			expectedSubcommand: "orchestrate",
			expectedArgs:       []string{"--input", "syslog alert", "--task", "Mitigate high temp", "--sync"},
		},
		{
			name:               "json flag before status subcommand",
			args:               []string{"--json", "status"},
			expectedSubcommand: "status",
			expectedJSON:       true,
			expectedArgs:       []string{},
		},
		{
			name:               "json flag after ping subcommand",
			args:               []string{"ping", "--json"},
			expectedSubcommand: "ping",
			expectedJSON:       true,
			expectedArgs:       []string{},
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
			if global.JSON != tt.expectedJSON {
				t.Errorf("expected JSON %v, got %v", tt.expectedJSON, global.JSON)
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

func captureStdout(f func()) string {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	f()

	_ = w.Close()
	os.Stdout = oldStdout
	out := <-done
	return out
}

func TestRunConsolidate_PositionalShortcut(t *testing.T) {
	var capturedReq model.ConsolidateRequest
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(body, &capturedReq); err != nil {
			t.Errorf("failed to unmarshal request body: %v", err)
		}

		resp := model.ConsolidateResponse{
			Status:            "success",
			TraceID:           r.Header.Get("X-Trace-ID"),
			Message:           "Trace enqueued",
			Synchronous:       capturedReq.Synchronous,
			EntitiesExtracted: 1,
			NodesFused:        1,
			EdgesReinforced:   0,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	global := GlobalFlags{
		KnowledgeURL: mockServer.URL,
	}

	captureStdout(func() {
		runConsolidate(global, []string{"Project Kestrel", "PORT: 51742", "-a", "#project:kestrel", "--sync"})
	})

	if capturedReq.TaskGoal != "Store Project Kestrel configuration" {
		t.Errorf("expected TaskGoal 'Store Project Kestrel configuration', got '%s'", capturedReq.TaskGoal)
	}
	if capturedReq.Outcome != "success" {
		t.Errorf("expected Outcome 'success', got '%s'", capturedReq.Outcome)
	}
	if capturedReq.Status != "completed" {
		t.Errorf("expected Status 'completed', got '%s'", capturedReq.Status)
	}
	if !strings.HasPrefix(capturedReq.SessionID, "sess-memorise-") {
		t.Errorf("expected SessionID prefix 'sess-memorise-', got '%s'", capturedReq.SessionID)
	}
	if !capturedReq.Synchronous {
		t.Errorf("expected Synchronous true, got false")
	}

	// Verify sensory_context
	if len(capturedReq.SensoryContext) != 1 {
		t.Fatalf("expected 1 sensory context item, got %d", len(capturedReq.SensoryContext))
	}
	sc := capturedReq.SensoryContext[0]
	if sc.ID != "fact-01" {
		t.Errorf("expected sensory ID 'fact-01', got '%s'", sc.ID)
	}
	expectedText := "Project Kestrel config: PORT: 51742"
	if sc.Text != expectedText {
		t.Errorf("expected sensory text '%s', got '%s'", expectedText, sc.Text)
	}
	if sc.Salience != 1.0 {
		t.Errorf("expected salience 1.0, got %f", sc.Salience)
	}
	if sc.Source != "user" {
		t.Errorf("expected source 'user', got '%s'", sc.Source)
	}
	if sc.Timestamp.IsZero() {
		t.Errorf("expected non-zero sensory timestamp")
	}

	// Verify trajectory
	if len(capturedReq.Trajectory) != 1 {
		t.Fatalf("expected 1 trajectory step, got %d", len(capturedReq.Trajectory))
	}
	tr := capturedReq.Trajectory[0]
	if tr.StepIndex != 0 {
		t.Errorf("expected step index 0, got %d", tr.StepIndex)
	}
	expectedThought := "Committed Project Kestrel configuration to long-term memory"
	if tr.Thought != expectedThought {
		t.Errorf("expected thought '%s', got '%s'", expectedThought, tr.Thought)
	}
	if tr.Status != "completed" {
		t.Errorf("expected trajectory status 'completed', got '%s'", tr.Status)
	}
	if tr.Timestamp.IsZero() {
		t.Errorf("expected non-zero trajectory timestamp")
	}

	// Verify anchors
	if len(capturedReq.Anchors) != 1 || capturedReq.Anchors[0] != "#project:kestrel" {
		t.Errorf("expected anchors ['#project:kestrel'], got %v", capturedReq.Anchors)
	}

	// Sub-test: Summary already starting with label should be used as-is
	captureStdout(func() {
		runConsolidate(global, []string{"Project Kestrel", "Project Kestrel INGEST_PORT: 51742", "--sync"})
	})
	if len(capturedReq.SensoryContext) != 1 || capturedReq.SensoryContext[0].Text != "Project Kestrel INGEST_PORT: 51742" {
		t.Errorf("expected summary as-is, got: %s", capturedReq.SensoryContext[0].Text)
	}

	// Sub-test: Custom --goal, --session-id, and --outcome flags
	captureStdout(func() {
		runConsolidate(global, []string{"Project Kestrel", "PORT: 51742", "--goal", "Custom Kestrel Goal", "--session-id", "sess-custom-99", "--outcome", "failure"})
	})
	if capturedReq.TaskGoal != "Custom Kestrel Goal" {
		t.Errorf("expected TaskGoal 'Custom Kestrel Goal', got '%s'", capturedReq.TaskGoal)
	}
	if capturedReq.SessionID != "sess-custom-99" {
		t.Errorf("expected SessionID 'sess-custom-99', got '%s'", capturedReq.SessionID)
	}
	if capturedReq.Outcome != "failure" {
		t.Errorf("expected Outcome 'failure', got '%s'", capturedReq.Outcome)
	}

	// Sub-test: Single positional argument treated as goal
	captureStdout(func() {
		runConsolidate(global, []string{"Single Positional Goal"})
	})
	if capturedReq.TaskGoal != "Single Positional Goal" {
		t.Errorf("expected TaskGoal 'Single Positional Goal', got '%s'", capturedReq.TaskGoal)
	}
}

func TestRunConsolidate_TracePrecedence(t *testing.T) {
	var capturedReq model.ConsolidateRequest
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedReq)

		resp := model.ConsolidateResponse{
			Status:  "success",
			TraceID: r.Header.Get("X-Trace-ID"),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	global := GlobalFlags{
		KnowledgeURL: mockServer.URL,
	}

	traceJSON := `{
		"session_id": "sess-explicit-trace",
		"task_goal": "Explicit Goal From Trace",
		"outcome": "success",
		"sensory_context": [
			{
				"id": "fact-explicit-99",
				"text": "Explicit trace sensory fact",
				"salience": 0.88
			}
		],
		"trajectory": [
			{
				"step_index": 0,
				"thought": "Explicit deliberation step"
			}
		]
	}`

	captureStdout(func() {
		runConsolidate(global, []string{"--trace", traceJSON, "Positional Label", "Positional Summary", "-a", "#project:kestrel"})
	})

	if capturedReq.SessionID != "sess-explicit-trace" {
		t.Errorf("expected SessionID 'sess-explicit-trace', got '%s'", capturedReq.SessionID)
	}
	if capturedReq.TaskGoal != "Explicit Goal From Trace" {
		t.Errorf("expected TaskGoal 'Explicit Goal From Trace', got '%s'", capturedReq.TaskGoal)
	}
	if len(capturedReq.SensoryContext) != 1 || capturedReq.SensoryContext[0].ID != "fact-explicit-99" {
		t.Errorf("expected explicit sensory context to override positional, got: %+v", capturedReq.SensoryContext)
	}
	if capturedReq.SensoryContext[0].Text != "Explicit trace sensory fact" {
		t.Errorf("expected explicit sensory text, got: %s", capturedReq.SensoryContext[0].Text)
	}
	if len(capturedReq.Trajectory) != 1 || capturedReq.Trajectory[0].Thought != "Explicit deliberation step" {
		t.Errorf("expected explicit trajectory to override positional, got: %+v", capturedReq.Trajectory)
	}
	// Anchors should still be merged
	if len(capturedReq.Anchors) != 1 || capturedReq.Anchors[0] != "#project:kestrel" {
		t.Errorf("expected anchor '#project:kestrel', got %v", capturedReq.Anchors)
	}
}

func TestRunOrchestrate_TaskFlagAlias(t *testing.T) {
	var capturedFilterTask string
	var capturedFilterText string
	var capturedDelibReq model.DeliberateRequest

	mockSensory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedFilterTask = r.URL.Query().Get("task")
		capturedFilterText = string(body)

		resp := model.FilterResponse{
			Chunks: []model.SensoryChunk{
				{
					ID:        "chk-001",
					Text:      "Temperature alert",
					Salience:  0.95,
					Source:    "syslog",
					Timestamp: time.Now(),
				},
			},
			TotalChunks:   1,
			SalientChunks: 1,
			ReductionRate: 0.0,
			LatencyMS:     0.5,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockSensory.Close()

	mockKnowledge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/recall") {
			resp := model.RecallResponse{
				Nodes:          []model.ScoredNode{},
				Edges:          []model.Edge{},
				QueryLatencyMS: 0.8,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		resp := model.ConsolidateResponse{
			Status:  "success",
			TraceID: r.Header.Get("X-Trace-ID"),
			Message: "Trace enqueued",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockKnowledge.Close()

	mockWorking := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedDelibReq)

		resp := model.DeliberateResponse{
			Status:           "ready",
			StepIndex:        1,
			Thought:          "Mitigating temperature alert",
			ProposedAction:   "FAN_SPEED_HIGH",
			IsComplete:       true,
			PromptTokens:     100,
			CompletionTokens: 25,
			TotalTokens:      125,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockWorking.Close()

	global := GlobalFlags{
		SensoryURL:   mockSensory.URL,
		KnowledgeURL: mockKnowledge.URL,
		WorkingURL:   mockWorking.URL,
	}

	t.Run("--task sets TaskDirective when --directive is empty", func(t *testing.T) {
		captureStdout(func() {
			runOrchestrate(global, []string{"--input", "temp alert syslog", "--task", "Mitigate high temperature"})
		})

		if capturedFilterTask != "Mitigate high temperature" {
			t.Errorf("expected sensory task query param 'Mitigate high temperature', got '%s'", capturedFilterTask)
		}
		if capturedFilterText != "temp alert syslog" {
			t.Errorf("expected sensory text 'temp alert syslog', got '%s'", capturedFilterText)
		}
		if capturedDelibReq.Objective != "Mitigate high temperature" {
			t.Errorf("expected DeliberateRequest.Objective 'Mitigate high temperature', got '%s'", capturedDelibReq.Objective)
		}
	})

	t.Run("--directive sets TaskDirective", func(t *testing.T) {
		captureStdout(func() {
			runOrchestrate(global, []string{"--input", "temp alert syslog", "--directive", "Directive only goal"})
		})

		if capturedFilterTask != "Directive only goal" {
			t.Errorf("expected sensory task query param 'Directive only goal', got '%s'", capturedFilterTask)
		}
		if capturedDelibReq.Objective != "Directive only goal" {
			t.Errorf("expected DeliberateRequest.Objective 'Directive only goal', got '%s'", capturedDelibReq.Objective)
		}
	})

	t.Run("--directive takes precedence over --task if both provided", func(t *testing.T) {
		captureStdout(func() {
			runOrchestrate(global, []string{"--input", "temp alert syslog", "--directive", "Primary directive", "--task", "Secondary task"})
		})

		if capturedFilterTask != "Primary directive" {
			t.Errorf("expected sensory task query param 'Primary directive', got '%s'", capturedFilterTask)
		}
		if capturedDelibReq.Objective != "Primary directive" {
			t.Errorf("expected DeliberateRequest.Objective 'Primary directive', got '%s'", capturedDelibReq.Objective)
		}
	})
}

func setupMockCluster(sensoryHandler, workingHandler, knowledgeHandler http.HandlerFunc) (mockSensory, mockWorking, mockKnowledge *httptest.Server, global GlobalFlags) {
	if sensoryHandler != nil {
		mockSensory = httptest.NewServer(sensoryHandler)
	}
	if workingHandler != nil {
		mockWorking = httptest.NewServer(workingHandler)
	}
	if knowledgeHandler != nil {
		mockKnowledge = httptest.NewServer(knowledgeHandler)
	}

	global = GlobalFlags{}
	if mockSensory != nil {
		global.SensoryURL = mockSensory.URL
	}
	if mockWorking != nil {
		global.WorkingURL = mockWorking.URL
	}
	if mockKnowledge != nil {
		global.KnowledgeURL = mockKnowledge.URL
	}
	return
}

func defaultHealthySensoryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.SensoryStatsResponse{
			Status:           "healthy",
			BufferCapacityMB: 64.0,
			BufferUsageMB:    10.3,
			FillPercent:      16.2,
			TotalIngested:    50386,
			DroppedPackets:   0,
			ClassifierTelemetry: &model.ClassifierTelemetry{
				TotalEvaluated:      30836,
				TotalSalient:        30503,
				NoiseReductionRatio: 0.011,
			},
		})
	}
}

func defaultHealthyWorkingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.WorkingHealthResponse{
			Status:         "healthy",
			LlamaInference: "reachable",
			Service:        "sekha-working-scratchpad",
			UptimeSeconds:  396000,
		})
	}
}

func defaultHealthyKnowledgeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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
	}
}

func TestRunStatus_DashboardOutput(t *testing.T) {
	s, w, k, global := setupMockCluster(
		defaultHealthySensoryHandler(),
		defaultHealthyWorkingHandler(),
		defaultHealthyKnowledgeHandler(),
	)
	defer s.Close()
	defer w.Close()
	defer k.Close()

	var out string
	out = captureStdout(func() {
		code := runStatus(global, []string{})
		if code != 0 {
			t.Errorf("expected exit code 0, got %d", code)
		}
	})

	expectedSubstrings := []string{
		"Sekha Tri-Node Edge Cognitive Cluster Status",
		"Cluster State: ALL NODES HEALTHY (3/3 Online)",
		"[●] Node 3: Sensory Layer",
		"Status:       HEALTHY",
		"Buffer Usage: 10.3 MB / 64.0 MB (16.2% fill)",
		"Throughput:   50,386 ingested | 0 dropped",
		"Gating:       30,503 salient / 30,836 evaluated (1.1% noise reduced)",
		"[●] Node 2: Working Memory Layer",
		"SLM Engine:   REACHABLE (llama-server :8082)",
		"Service:      sekha-working-scratchpad (uptime: 4d 14h)",
		"[●] Node 1: Long-Term Knowledge Layer",
		"Graph Scale:  229 nodes | 882 relational edges",
		"Embedder:     REACHABLE (384-D :8086)",
		"Service:      sekha-knowledge-store (uptime: 1d 02h)",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(out, sub) {
			t.Errorf("expected dashboard to contain '%s', got:\n%s", sub, out)
		}
	}
}

func TestRunStatus_JSONOutput(t *testing.T) {
	s, w, k, global := setupMockCluster(
		defaultHealthySensoryHandler(),
		defaultHealthyWorkingHandler(),
		defaultHealthyKnowledgeHandler(),
	)
	defer s.Close()
	defer w.Close()
	defer k.Close()

	out := captureStdout(func() {
		code := runStatus(global, []string{"--json"})
		if code != 0 {
			t.Errorf("expected exit code 0, got %d", code)
		}
	})

	var res map[string]interface{}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("failed to decode JSON output: %v, raw:\n%s", err, out)
	}

	if res["cluster_status"] != "all_nodes_healthy" {
		t.Errorf("expected cluster_status 'all_nodes_healthy', got '%v'", res["cluster_status"])
	}

	nodes, ok := res["nodes"].([]interface{})
	if !ok || len(nodes) != 3 {
		t.Fatalf("expected 3 nodes in json, got %d", len(nodes))
	}
}

func TestRunStatus_OfflineResilience(t *testing.T) {
	s, w, k, global := setupMockCluster(
		defaultHealthySensoryHandler(),
		defaultHealthyWorkingHandler(),
		defaultHealthyKnowledgeHandler(),
	)
	defer w.Close()
	defer k.Close()
	// Close sensory node immediately to simulate failure
	s.Close()

	out := captureStdout(func() {
		code := runStatus(global, []string{})
		if code != 0 {
			t.Errorf("expected exit code 0 even when degraded, got %d", code)
		}
	})

	if !strings.Contains(out, "Cluster State: DEGRADED (2/3 Online)") {
		t.Errorf("expected degraded cluster state, got:\n%s", out)
	}
	if !strings.Contains(out, "[✗] Node 3: Sensory Layer") {
		t.Errorf("expected sensory node marked offline, got:\n%s", out)
	}
	if !strings.Contains(out, "OFFLINE") {
		t.Errorf("expected OFFLINE indicator, got:\n%s", out)
	}
}

func TestRunPing_HealthyOutput(t *testing.T) {
	s, w, k, global := setupMockCluster(
		defaultHealthySensoryHandler(),
		defaultHealthyWorkingHandler(),
		defaultHealthyKnowledgeHandler(),
	)
	defer s.Close()
	defer w.Close()
	defer k.Close()

	var code int
	out := captureStdout(func() {
		code = runPing(global, []string{})
	})

	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	expectedLines := []string{
		"[OK] Node 3 (Sensory)",
		"[OK] Node 2 (Working)",
		"[OK] Node 1 (Knowledge)",
		"Cluster: HEALTHY (3/3 nodes online)",
	}

	for _, line := range expectedLines {
		if !strings.Contains(out, line) {
			t.Errorf("expected ping output to contain '%s', got:\n%s", line, out)
		}
	}
}

func TestRunPing_JSONOutput(t *testing.T) {
	s, w, k, global := setupMockCluster(
		defaultHealthySensoryHandler(),
		defaultHealthyWorkingHandler(),
		defaultHealthyKnowledgeHandler(),
	)
	defer s.Close()
	defer w.Close()
	defer k.Close()

	var code int
	out := captureStdout(func() {
		code = runPing(global, []string{"--json"})
	})

	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	var res struct {
		AllHealthy bool `json:"all_healthy"`
		Nodes      map[string]struct {
			Reachable bool    `json:"reachable"`
			PingMS    float64 `json:"ping_ms"`
			URL       string  `json:"url"`
		} `json:"nodes"`
		Timestamp string `json:"timestamp"`
	}

	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("failed to decode ping JSON output: %v, raw:\n%s", err, out)
	}

	if !res.AllHealthy {
		t.Errorf("expected AllHealthy true, got false")
	}
	if !res.Nodes["sensory"].Reachable || !res.Nodes["working"].Reachable || !res.Nodes["knowledge"].Reachable {
		t.Errorf("expected all 3 nodes reachable, got %+v", res.Nodes)
	}
}

func TestRunPing_OfflineResilience(t *testing.T) {
	s, w, k, global := setupMockCluster(
		defaultHealthySensoryHandler(),
		defaultHealthyWorkingHandler(),
		defaultHealthyKnowledgeHandler(),
	)
	defer s.Close()
	defer k.Close()
	// Close working node to simulate failure
	w.Close()

	var code int
	out := captureStdout(func() {
		code = runPing(global, []string{})
	})

	if code != 1 {
		t.Errorf("expected exit code 1 for degraded cluster, got %d", code)
	}

	if !strings.Contains(out, "[FAIL] Node 2 (Working)") {
		t.Errorf("expected [FAIL] for Node 2, got:\n%s", out)
	}
	if !strings.Contains(out, "Cluster: DEGRADED (2/3 nodes online)") {
		t.Errorf("expected degraded summary, got:\n%s", out)
	}

	// Also test JSON output under offline condition
	var jsonCode int
	jsonOut := captureStdout(func() {
		jsonCode = runPing(global, []string{"--json"})
	})

	if jsonCode != 1 {
		t.Errorf("expected exit code 1 for degraded cluster in JSON mode, got %d", jsonCode)
	}

	var res struct {
		AllHealthy bool `json:"all_healthy"`
		Nodes      map[string]struct {
			Reachable bool `json:"reachable"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &res); err != nil {
		t.Fatalf("failed to decode ping JSON output: %v", err)
	}
	if res.AllHealthy {
		t.Errorf("expected AllHealthy to be false")
	}
	if res.Nodes["working"].Reachable {
		t.Errorf("expected working node to be unreachable")
	}
	if !res.Nodes["sensory"].Reachable || !res.Nodes["knowledge"].Reachable {
		t.Errorf("expected sensory and knowledge to remain reachable")
	}
}

func TestRunPing_ConcurrentExecution(t *testing.T) {
	slowSensory := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		defaultHealthySensoryHandler()(w, r)
	}
	slowWorking := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		defaultHealthyWorkingHandler()(w, r)
	}
	slowKnowledge := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		defaultHealthyKnowledgeHandler()(w, r)
	}

	s, w, k, global := setupMockCluster(slowSensory, slowWorking, slowKnowledge)
	defer s.Close()
	defer w.Close()
	defer k.Close()

	start := time.Now()
	var code int
	captureStdout(func() {
		code = runPing(global, []string{})
	})
	elapsed := time.Since(start)

	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	// 3 servers sleeping 200ms must run in parallel and take < 450ms, not sequential 600ms
	if elapsed > 450*time.Millisecond {
		t.Errorf("expected parallel execution (< 450ms), took %v", elapsed)
	}
}



