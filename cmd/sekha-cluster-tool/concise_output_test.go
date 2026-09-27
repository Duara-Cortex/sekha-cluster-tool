package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

// chattyCluster answers like the live cluster on a large payload: Node 3 splits the input into
// many small chunks and Node 1 recalls many low-similarity nodes, so full output is huge.
type chattyCluster struct {
	sensory, knowledge, working *httptest.Server
	consolidateDelay            time.Duration
	failConsolidate             bool
}

func newChattyCluster(t *testing.T) *chattyCluster {
	t.Helper()
	cc := &chattyCluster{}
	cc.sensory = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var chunks []model.SensoryChunk
		for i, part := range splitParts(string(body), max(1, len(body)/108)) {
			chunks = append(chunks, model.SensoryChunk{ID: fmt.Sprintf("chk-%04d", i), Text: part, Salience: 0.9, Timestamp: time.Now()})
		}
		_ = json.NewEncoder(w).Encode(model.FilterResponse{Chunks: chunks, TotalChunks: len(chunks), SalientChunks: len(chunks)})
	}))
	cc.knowledge = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/recall") {
			var nodes []model.ScoredNode
			for i := range 50 {
				nodes = append(nodes, model.ScoredNode{
					Node:     model.Node{ID: fmt.Sprintf("n-%02d", i), Label: "Orchard ledger entry", Summary: strings.Repeat("apples counted at dusk ", 10)},
					Score:    0.4,
					SimScore: 0.2,
				})
			}
			_ = json.NewEncoder(w).Encode(model.RecallResponse{Nodes: nodes})
			return
		}
		if cc.failConsolidate {
			http.Error(w, "graph write failed", http.StatusInternalServerError)
			return
		}
		select {
		case <-time.After(cc.consolidateDelay):
		case <-r.Context().Done():
			return
		}
		_ = json.NewEncoder(w).Encode(model.ConsolidateResponse{Status: "success", Synchronous: true, EntitiesExtracted: 3, Message: "ok"})
	}))
	cc.working = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(model.DeliberateResponse{Status: "ready", StepIndex: 1, Thought: "Stored the record.", ProposedAction: "STORE", IsComplete: true, PromptTokens: 900})
	}))
	t.Cleanup(func() {
		cc.sensory.Close()
		cc.knowledge.Close()
		cc.working.Close()
	})
	return cc
}

func (cc *chattyCluster) global() GlobalFlags {
	return GlobalFlags{SensoryURL: cc.sensory.URL, KnowledgeURL: cc.knowledge.URL, WorkingURL: cc.working.URL}
}

// topLevelKeys returns the object's top-level keys in output order.
func topLevelKeys(t *testing.T, out string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		t.Fatalf("output is not a JSON object: %v", err)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

// stagesEnd returns the byte offset just past the stages[] array.
func stagesEnd(t *testing.T, out string) int {
	t.Helper()
	start := strings.Index(out, `"stages": [`)
	if start < 0 {
		t.Fatalf("stages[] missing: %.300s", out)
	}
	dec := json.NewDecoder(strings.NewReader(out[start+len(`"stages": `):]))
	var stages json.RawMessage
	if err := dec.Decode(&stages); err != nil {
		t.Fatal(err)
	}
	return start + len(`"stages": `) + int(dec.InputOffset())
}

func TestOrchestrate_ConciseOutputBounded(t *testing.T) {
	isolateEnv(t)
	cc := newChattyCluster(t)
	text := naturalText(500 << 10)
	args := append(repeatFlag("input", splitParts(text, 5)), "--directive", "Remember this record", "--sync")

	out, code := captureOutputWithExit(func() { runOrchestrate(cc.global(), args) })
	if code != 0 {
		t.Fatalf("orchestrate exited %d: %.500s", code, out)
	}
	t.Logf("500 KB input: concise output %d bytes, stages[] ends at byte %d", len(out), stagesEnd(t, out))
	if len(out) > 8<<10 {
		t.Errorf("concise output is %d bytes for a 500 KB input, want under 8 KB", len(out))
	}
	if end := stagesEnd(t, out); end > 2048 {
		t.Errorf("stages[] ends at byte %d, want within the first 2 KB", end)
	}
	if strings.Contains(out, "orchards while herons") {
		t.Error("concise output echoes payload text")
	}

	want := []string{"status", "is_complete", "stages", "final_thought", "proposed_action", "trace_id", "session_id", "total_duration_ms", "loop_complete"}
	keys := topLevelKeys(t, out)
	if strings.Join(keys[:len(want)], ",") != strings.Join(want, ",") {
		t.Errorf("top-level field order = %v, want prefix %v", keys, want)
	}

	var got model.OrchestrateSummary
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" || !got.LoopComplete {
		t.Errorf("status=%q loop_complete=%v, want completed/true", got.Status, got.LoopComplete)
	}
	wantStages := []string{"1_sensory_filter", "2_long_term_recall", "3_working_deliberate", "4_memory_consolidate"}
	for i, st := range got.Stages {
		if st.StageName != wantStages[i] || st.Status != "success" {
			t.Errorf("stages[%d] = %s/%s, want %s/success", i, st.StageName, st.Status, wantStages[i])
		}
	}
	if got.Sensory.TotalChunks < 4000 {
		t.Errorf("sensory.total_chunks = %d, want the Node 3 count", got.Sensory.TotalChunks)
	}
	gate := got.Recall.RelevanceGate
	if gate == nil || gate.NodesIn != 50 || gate.NodesDropped != 50 || gate.MinSimScore == 0 {
		t.Errorf("relevance gate summary = %+v, want 50 in / 50 dropped with thresholds", gate)
	}
	if strings.Contains(out, `"dropped"`) || strings.Contains(out, `"kept"`) {
		t.Error("concise output still carries per-node gate lists")
	}
	if got.Consolidation.Status != "success" || got.Consolidation.EntitiesExtracted != 3 || !got.Consolidation.Synchronous {
		t.Errorf("consolidation summary = %+v", got.Consolidation)
	}
}

func TestOrchestrate_FullFlagReturnsEverything(t *testing.T) {
	isolateEnv(t)
	cc := newChattyCluster(t)
	text := naturalText(90 << 10)
	args := append(repeatFlag("input", []string{text}), "--directive", "Remember this record", "--full")

	out, code := captureOutputWithExit(func() { runOrchestrate(cc.global(), args) })
	if code != 0 {
		t.Fatalf("orchestrate --full exited %d: %.500s", code, out)
	}
	var resp model.OrchestrateResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatal(err)
	}
	var echoed strings.Builder
	for _, c := range resp.Sensory.Chunks {
		echoed.WriteString(c.Text)
	}
	if echoed.String() != text {
		t.Errorf("--full sensory chunks carry %d bytes, want the full %d-byte payload", echoed.Len(), len(text))
	}
	if g := resp.Stages[1].RelevanceGate; g == nil || len(g.Dropped) != 50 {
		t.Errorf("--full relevance gate lost its dropped[] list: %+v", g)
	}
	if !resp.LoopComplete || resp.Status != "completed" {
		t.Errorf("status=%q loop_complete=%v", resp.Status, resp.LoopComplete)
	}
}

func TestOrchestrate_FailedStageIsReportedAndExitsNonZero(t *testing.T) {
	isolateEnv(t)

	t.Run("partial", func(t *testing.T) {
		cc := newChattyCluster(t)
		cc.failConsolidate = true
		out, code := captureOutputWithExit(func() {
			runOrchestrate(cc.global(), []string{"--input", "a short record", "--directive", "Remember this record"})
		})
		if code == 0 {
			t.Fatal("expected a non-zero exit when stage 4 fails")
		}
		if code != exitCycleIncomplete {
			t.Errorf("exit = %d, want %d", code, exitCycleIncomplete)
		}
		var got model.OrchestrateSummary
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("stdout is not one JSON object: %v\n%s", err, out)
		}
		if got.Status != "partial" || got.LoopComplete {
			t.Errorf("status=%q loop_complete=%v, want partial/false", got.Status, got.LoopComplete)
		}
		if !got.IsComplete {
			t.Error("is_complete should still carry Node 2's flag")
		}
		if st := got.Stages[3]; st.Status != "failed" || !strings.Contains(st.Error, "graph write failed") {
			t.Errorf("stages[3] = %+v", st)
		}
	})

	t.Run("failed", func(t *testing.T) {
		down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, strings.Repeat("upstream unavailable ", 200), http.StatusBadGateway)
		}))
		defer down.Close()
		global := GlobalFlags{SensoryURL: down.URL, KnowledgeURL: down.URL, WorkingURL: down.URL}
		out, code := captureOutputWithExit(func() {
			runOrchestrate(global, []string{"--input", "a short record", "--directive", "Remember this record"})
		})
		if code != exitCycleIncomplete {
			t.Errorf("exit = %d, want %d", code, exitCycleIncomplete)
		}
		var got model.OrchestrateSummary
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("stdout is not one JSON object: %v", err)
		}
		if got.Status != "failed" || got.LoopComplete {
			t.Errorf("status=%q loop_complete=%v, want failed/false", got.Status, got.LoopComplete)
		}
		if end := stagesEnd(t, out); end > 2048 {
			t.Errorf("four long stage errors pushed stages[] to byte %d", end)
		}
	})
}

func TestConsolidateDeadline_FromConfig(t *testing.T) {
	isolateEnv(t)
	cc := newChattyCluster(t)
	cc.consolidateDelay = 300 * time.Millisecond
	orchestrate := func(extra ...string) (model.OrchestrateSummary, int, string) {
		args := append([]string{"--input", "a short record", "--directive", "Remember this record", "--sync"}, extra...)
		out, code := captureOutputWithExit(func() { runOrchestrate(cc.global(), args) })
		var got model.OrchestrateSummary
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, out)
		}
		return got, code, out
	}

	t.Run("default is generous", func(t *testing.T) {
		got, code, _ := orchestrate()
		if code != 0 || got.Stages[3].Status != "success" {
			t.Errorf("exit %d, stage 4 %+v", code, got.Stages[3])
		}
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("CLUSTER_CONSOLIDATE_TIMEOUT_MS", "50")
		got, code, _ := orchestrate()
		if code != exitCycleIncomplete || got.Status != "partial" {
			t.Fatalf("exit %d status %q, want %d partial", code, got.Status, exitCycleIncomplete)
		}
		msg := got.Stages[3].Error
		if !strings.Contains(msg, "50ms") || !strings.Contains(msg, "environment variable CLUSTER_CONSOLIDATE_TIMEOUT_MS") {
			t.Errorf("deadline error does not name the value and source: %s", msg)
		}
		if got.Consolidation.Status != "failed" || !got.Consolidation.Synchronous {
			t.Errorf("failed --sync consolidation should keep synchronous=true: %+v", got.Consolidation)
		}
	})

	t.Run(".env file", func(t *testing.T) {
		envFile := filepath.Join(t.TempDir(), "cluster.env")
		_ = os.WriteFile(envFile, []byte("CLUSTER_CONSOLIDATE_TIMEOUT_MS=60\n"), 0o600)
		t.Setenv("CLUSTER_ENV_FILE", envFile)
		got, _, _ := orchestrate()
		msg := got.Stages[3].Error
		if !strings.Contains(msg, "60ms") || !strings.Contains(msg, ".env file ("+envFile+")") {
			t.Errorf("deadline error does not name the .env source: %s", msg)
		}
	})

	t.Run("flag beats env", func(t *testing.T) {
		t.Setenv("CLUSTER_CONSOLIDATE_TIMEOUT_MS", "50")
		got, code, _ := orchestrate("--consolidate-timeout", "5s")
		if code != 0 || !got.LoopComplete {
			t.Errorf("exit %d, stage 4 %+v", code, got.Stages[3])
		}
		got, _, _ = orchestrate("--consolidate-timeout", "40ms")
		if msg := got.Stages[3].Error; !strings.Contains(msg, "40ms") || !strings.Contains(msg, "--consolidate-timeout flag") {
			t.Errorf("deadline error does not name the flag: %s", msg)
		}
	})

	t.Run("standalone consolidate", func(t *testing.T) {
		t.Setenv("CLUSTER_CONSOLIDATE_TIMEOUT_MS", "50")
		args := []string{"--input", "a short record", "--goal", "Remember this record", "--sync"}
		out, code := captureOutputWithExit(func() { runConsolidate(cc.global(), args) })
		if code != 1 {
			t.Fatalf("exit %d, want 1", code)
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(out), &e); err != nil || e["status"] != "error" {
			t.Fatalf("expected the outputError shape, got %s", out)
		}
		if msg := e["error"].(string); !strings.Contains(msg, "50ms") || !strings.Contains(msg, "CLUSTER_CONSOLIDATE_TIMEOUT_MS") {
			t.Errorf("deadline error does not name the value and source: %s", msg)
		}
		for _, flagArgs := range [][]string{{"--consolidate-timeout", "5s"}, {"--timeout", "5s"}} {
			out, code = captureOutputWithExit(func() { runConsolidate(cc.global(), append(args, flagArgs...)) })
			if code != 0 || !strings.Contains(out, `"status": "success"`) {
				t.Errorf("%v should override env: exit %d %s", flagArgs, code, out)
			}
		}
	})
}

func TestFilter_ConciseByDefault(t *testing.T) {
	isolateEnv(t)
	cc := newChattyCluster(t)
	text := naturalText(90 << 10)

	out, code := captureOutputWithExit(func() { runFilter(cc.global(), []string{"--text", text}) })
	if code != 0 {
		t.Fatalf("filter exited %d: %.300s", code, out)
	}
	if len(out) > 1024 || strings.Contains(out, "orchards") {
		t.Errorf("default filter output echoes the payload (%d bytes)", len(out))
	}
	var got filterSummary
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.TotalChunks == 0 || got.ChunksOmitted != got.TotalChunks {
		t.Errorf("filter summary = %+v", got)
	}

	out, _ = captureOutputWithExit(func() { runFilter(cc.global(), []string{"--text", text, "--full"}) })
	var full model.FilterResponse
	if err := json.Unmarshal([]byte(out), &full); err != nil || len(full.Chunks) != got.TotalChunks {
		t.Errorf("--full should return every chunk: %v, %d chunks", err, len(full.Chunks))
	}
}

func TestUsage_DocumentsNewOptions(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	printUsage()
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	for _, want := range []string{"--full", "loop_complete", "CLUSTER_CONSOLIDATE_TIMEOUT_MS", "CLUSTER_SENSORY_TIMEOUT_MS", "--consolidate-timeout", "Exit codes"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("--help does not mention %s", want)
		}
	}
}

func TestParseArgs_NewFlagsReachSubcommand(t *testing.T) {
	global, cmd, args := parseArgs([]string{"orchestrate", "--consolidate-timeout", "40ms", "--sensory-timeout=5s", "--full", "--timeout", "2s"})
	if cmd != "orchestrate" || global.Timeout != 2*time.Second {
		t.Fatalf("cmd=%q timeout=%v", cmd, global.Timeout)
	}
	want := []string{"--consolidate-timeout", "40ms", "--sensory-timeout=5s", "--full"}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Errorf("new flags were consumed as global flags: got %q, want %q", args, want)
	}
}
