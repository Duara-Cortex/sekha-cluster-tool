package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/config"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

const execMainEnv = "SEKHA_TEST_EXEC_MAIN"

// TestMain lets tests re-execute this test binary as the real CLI, so OS argument limits apply.
func TestMain(m *testing.M) {
	if os.Getenv(execMainEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

var sizeMatrix = []struct {
	name  string
	bytes int
	parts int
}{
	{"1KB", 1 << 10, 1},
	{"90KB", 90 << 10, 1},
	{"256KiB_single_arg", 256 << 10, 1},
	{"500KB_repeated", 500 << 10, 5},
}

// naturalText returns exactly n bytes of prose-like filler (no token-shaped strings that the
// consolidate secret scanner would reject).
func naturalText(n int) string {
	words := []string{"the", "river", "carried", "silt", "past", "quiet", "orchards", "while", "herons",
		"waited", "near", "reeds", "and", "farmers", "counted", "barrels", "of", "apples", "before", "dusk."}
	var sb strings.Builder
	for i := 0; sb.Len() < n; i++ {
		sb.WriteString(words[i%len(words)])
		sb.WriteByte(' ')
	}
	return sb.String()[:n]
}

// splitParts splits s into k parts at byte offsets, deliberately ignoring word boundaries.
func splitParts(s string, k int) []string {
	parts := make([]string, 0, k)
	size := (len(s) + k - 1) / k
	for start := 0; start < len(s); start += size {
		end := min(start+size, len(s))
		parts = append(parts, s[start:end])
	}
	return parts
}

func repeatFlag(name string, parts []string) []string {
	args := make([]string, 0, 2*len(parts))
	for _, p := range parts {
		args = append(args, "--"+name, p)
	}
	return args
}

// isolateEnv points config discovery at an empty .env so tests never load a real API key or URLs.
func isolateEnv(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "empty.env")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLUSTER_ENV_FILE", empty)
	t.Setenv("CLUSTER_MAX_INPUT_BYTES", "")
}

// mockCluster records what each node received and answers like a healthy cluster.
type mockCluster struct {
	mu             sync.Mutex
	sensory        *httptest.Server
	knowledge      *httptest.Server
	working        *httptest.Server
	filterBodies   []string
	consolidations []model.ConsolidateRequest
	deliberations  []model.DeliberateRequest
	requests       int
}

func newMockCluster(t *testing.T) *mockCluster {
	t.Helper()
	mc := &mockCluster{}
	mc.sensory = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mc.mu.Lock()
		mc.requests++
		mc.filterBodies = append(mc.filterBodies, string(body))
		mc.mu.Unlock()
		// Split into 8 KiB chunks with descending salience, like Node 3 over a long transcript.
		var chunks []model.SensoryChunk
		for i, part := range splitParts(string(body), max(1, (len(body)+8191)/8192)) {
			chunks = append(chunks, model.SensoryChunk{
				ID: fmt.Sprintf("chk-%03d", i), Text: part, Salience: 0.95 - float64(i%50)*0.01, Timestamp: time.Now(),
			})
		}
		_ = json.NewEncoder(w).Encode(model.FilterResponse{Chunks: chunks, TotalChunks: len(chunks), SalientChunks: len(chunks)})
	}))
	mc.knowledge = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mc.mu.Lock()
		mc.requests++
		mc.mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/recall") {
			_ = json.NewEncoder(w).Encode(model.RecallResponse{Nodes: []model.ScoredNode{}})
			return
		}
		var req model.ConsolidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mc.mu.Lock()
		mc.consolidations = append(mc.consolidations, req)
		mc.mu.Unlock()
		_ = json.NewEncoder(w).Encode(model.ConsolidateResponse{Status: "success", TraceID: req.TraceID})
	}))
	mc.working = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req model.DeliberateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		mc.mu.Lock()
		mc.requests++
		mc.deliberations = append(mc.deliberations, req)
		mc.mu.Unlock()
		_ = json.NewEncoder(w).Encode(model.DeliberateResponse{Status: "ready", StepIndex: 1, Thought: "ok", IsComplete: true})
	}))
	t.Cleanup(func() {
		mc.sensory.Close()
		mc.knowledge.Close()
		mc.working.Close()
	})
	return mc
}

func (mc *mockCluster) global() GlobalFlags {
	return GlobalFlags{SensoryURL: mc.sensory.URL, KnowledgeURL: mc.knowledge.URL, WorkingURL: mc.working.URL}
}

func TestSizeMatrix_Filter(t *testing.T) {
	isolateEnv(t)
	for _, tc := range sizeMatrix {
		t.Run(tc.name, func(t *testing.T) {
			mc := newMockCluster(t)
			text := naturalText(tc.bytes)
			out, code := captureOutputWithExit(func() {
				runFilter(mc.global(), repeatFlag("text", splitParts(text, tc.parts)))
			})
			if code != 0 {
				t.Fatalf("filter exited %d: %s", code, out)
			}
			if len(mc.filterBodies) != 1 || mc.filterBodies[0] != text {
				t.Fatalf("Node 3 did not receive the exact %d-byte payload (got %d bodies)", len(text), len(mc.filterBodies))
			}
		})
	}
}

func TestSizeMatrix_Orchestrate(t *testing.T) {
	isolateEnv(t)
	for _, tc := range sizeMatrix {
		t.Run(tc.name, func(t *testing.T) {
			mc := newMockCluster(t)
			text := naturalText(tc.bytes)
			args := append(repeatFlag("input", splitParts(text, tc.parts)), "--task", "Summarise the orchard ledger")
			out, code := captureOutputWithExit(func() { runOrchestrate(mc.global(), args) })
			if code != 0 {
				t.Fatalf("orchestrate exited %d: %s", code, out)
			}
			if len(mc.filterBodies) != 1 || mc.filterBodies[0] != text {
				t.Fatalf("Node 3 did not receive the exact %d-byte payload", len(text))
			}
			var resp model.OrchestrateResponse
			if err := json.Unmarshal([]byte(out), &resp); err != nil {
				t.Fatalf("invalid orchestrate JSON: %v", err)
			}
			budget := resp.Stages[2].ContextBudget
			if budget == nil {
				t.Fatal("stages[2].context_budget missing")
			}
			if budget.EstimatedPromptTokens > budget.PromptBudgetTokens || !budget.WithinBudget {
				t.Errorf("Node 2 prompt over budget: %+v", budget)
			}
			if len(text) > 16<<10 && (!budget.Truncated || len(budget.Decisions) == 0) {
				t.Errorf("expected truncation details for %s, got %+v", tc.name, budget)
			}
			// Stage 4 still consolidates the full, untruncated sensory stream.
			var consolidated strings.Builder
			for _, item := range mc.consolidations[0].SensoryContext {
				consolidated.WriteString(item.Text)
			}
			if consolidated.String() != text {
				t.Errorf("consolidated sensory context was altered: got %d bytes, want %d", consolidated.Len(), len(text))
			}
		})
	}
}

func TestSizeMatrix_Consolidate(t *testing.T) {
	isolateEnv(t)
	for _, tc := range sizeMatrix {
		t.Run(tc.name, func(t *testing.T) {
			mc := newMockCluster(t)
			text := naturalText(tc.bytes)
			trace, _ := json.Marshal(model.ConsolidateRequest{
				SessionID:      "sess-size",
				TaskGoal:       "Archive ledger",
				SensoryContext: []model.SensoryItem{{Text: text, Salience: 0.9}},
			})
			out, code := captureOutputWithExit(func() {
				runConsolidate(mc.global(), repeatFlag("trace", splitParts(string(trace), tc.parts)))
			})
			if code != 0 {
				t.Fatalf("consolidate exited %d: %s", code, out)
			}
			if len(mc.consolidations) != 1 || mc.consolidations[0].SensoryContext[0].Text != text {
				t.Fatalf("Node 1 did not receive the exact %d-byte sensory text", len(text))
			}
		})
	}
}

func TestSizeMatrix_ConsolidateInput(t *testing.T) {
	isolateEnv(t)
	for _, tc := range sizeMatrix {
		t.Run(tc.name, func(t *testing.T) {
			mc := newMockCluster(t)
			text := naturalText(tc.bytes)
			args := append(repeatFlag("input", splitParts(text, tc.parts)), "--goal", "Archive ledger", "--session-id", "sess-size")
			out, code := captureOutputWithExit(func() { runConsolidate(mc.global(), args) })
			if code != 0 {
				t.Fatalf("consolidate exited %d: %s", code, out)
			}
			got := mc.consolidations[0]
			if len(got.SensoryContext) != 1 || got.SensoryContext[0].Text != text || got.TaskGoal != "Archive ledger" {
				t.Fatalf("Node 1 did not receive the exact %d-byte input as sensory_context", len(text))
			}
		})
	}
}

func TestInputLimit_RejectsWithoutSending(t *testing.T) {
	isolateEnv(t)
	mc := newMockCluster(t)
	over := naturalText(config.DefaultMaxInputBytes + 1)

	cases := map[string]func(){
		"filter":      func() { runFilter(mc.global(), repeatFlag("text", splitParts(over, 4))) },
		"orchestrate": func() { runOrchestrate(mc.global(), repeatFlag("input", splitParts(over, 4))) },
		"consolidate": func() {
			runConsolidate(mc.global(), repeatFlag("trace", splitParts(`{"session_id":"s","task_goal":"`+over+`"}`, 4)))
		},
		"consolidate --input": func() { runConsolidate(mc.global(), repeatFlag("input", splitParts(over, 4))) },
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			out, code := captureOutputWithExit(run)
			if code != 1 {
				t.Fatalf("expected exit 1, got %d", code)
			}
			if !strings.Contains(out, "exceeding the 1048576-byte input limit") || !strings.Contains(out, "never truncated") {
				t.Errorf("error is not actionable: %s", out)
			}
		})
	}
	if mc.requests != 0 {
		t.Errorf("expected no requests to the cluster, got %d", mc.requests)
	}

	t.Run("limit is configurable", func(t *testing.T) {
		out, code := captureOutputWithExit(func() {
			runFilter(mc.global(), append(repeatFlag("text", []string{naturalText(2048)}), "--max-input-bytes", "1024"))
		})
		if code != 1 || !strings.Contains(out, "2048 bytes, exceeding the 1024-byte") {
			t.Errorf("expected --max-input-bytes to be enforced, got exit %d: %s", code, out)
		}
	})
}

func TestReadInput_Sources(t *testing.T) {
	var inline concatFlag
	_ = inline.Set("abc")
	_ = inline.Set("def")
	if got, err := readInput(&inline, "input", "", 10); err != nil || got != "abcdef" {
		t.Errorf("expected concatenated 'abcdef', got %q (%v)", got, err)
	}
	if _, err := readInput(&inline, "input", "some.txt", 10); err == nil {
		t.Error("expected error when both inline input and --file are given")
	}

	path := filepath.Join(t.TempDir(), "in.txt")
	_ = os.WriteFile(path, []byte("0123456789AB"), 0o600)
	if _, err := readInput(&concatFlag{}, "input", path, 10); err == nil || !strings.Contains(err.Error(), "never truncated") {
		t.Errorf("expected oversize file to fail, got %v", err)
	}
	if got, err := readInput(&concatFlag{}, "input", path, 12); err != nil || got != "0123456789AB" {
		t.Errorf("expected full file at exact limit, got %q (%v)", got, err)
	}
	if _, err := readLimited(strings.NewReader("0123456789AB"), "stdin input", 11); err == nil {
		t.Error("expected oversize stdin to fail")
	}
}

func TestParseArgs_PayloadValuesReachSubcommand(t *testing.T) {
	_, cmd, args := parseArgs([]string{"orchestrate", "--input", "--verbose log line", "--input", "-a", "--timeout", "2s"})
	if cmd != "orchestrate" {
		t.Fatalf("unexpected subcommand %q", cmd)
	}
	want := []string{"--input", "--verbose log line", "--input", "-a"}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Errorf("payload values were consumed as global flags: got %q, want %q", args, want)
	}
}

// TestExecBinary_SizeMatrix runs the real CLI as a child process so the OS argv limits apply.
// Linux caps a single argument at 128 KiB (MAX_ARG_STRLEN), so there the 250 KB case uses two
// repeated flags; macOS and others pass it as one argument.
func TestExecBinary_SizeMatrix(t *testing.T) {
	isolateEnv(t)
	emptyEnv := os.Getenv("CLUSTER_ENV_FILE")
	for _, tc := range sizeMatrix {
		if tc.bytes < 128<<10 {
			continue // below every OS per-argument limit; covered in-process by TestSizeMatrix_Filter
		}
		t.Run(tc.name, func(t *testing.T) {
			parts := tc.parts
			if runtime.GOOS == "linux" && tc.bytes/parts > 120<<10 {
				parts = (tc.bytes + (120 << 10) - 1) / (120 << 10)
			}
			mc := newMockCluster(t)
			text := naturalText(tc.bytes)
			args := append([]string{"--sensory-url", mc.sensory.URL, "filter"}, repeatFlag("text", splitParts(text, parts))...)
			cmd := exec.Command(os.Args[0], args...)
			cmd.Env = []string{execMainEnv + "=1", "CLUSTER_ENV_FILE=" + emptyEnv, "HOME=" + t.TempDir()}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("CLI failed with %d parts: %v\n%.500s", parts, err, out)
			}
			if len(mc.filterBodies) != 1 || mc.filterBodies[0] != text {
				t.Fatalf("Node 3 did not receive the exact %d-byte payload", len(text))
			}
		})
	}
}
