package orchestrator

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/client"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

// runWithNode2 runs one cycle against mock nodes where Node 2 replies with delib, and returns
// the result plus the raw JSON body Node 2 received.
func runWithNode2(t *testing.T, delib model.DeliberateResponse) (*model.OrchestrateResponse, map[string]any) {
	t.Helper()
	node3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(model.FilterResponse{
			Chunks: []model.SensoryChunk{
				chunk("c1", "[Session 1] User: Please remember that on 2024-03-05 Mira moved to Denver.", 0.9),
				chunk("c2", "[Session 2] User: Mira's sister Ada adopted a greyhound named Pilot.", 0.8),
			},
			TotalChunks: 2, SalientChunks: 2,
		})
	}))
	defer node3.Close()
	node1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/memory/recall" {
			_ = json.NewEncoder(w).Encode(model.RecallResponse{})
			return
		}
		_ = json.NewEncoder(w).Encode(model.ConsolidateResponse{Status: "success"})
	}))
	defer node1.Close()
	var raw map[string]any
	node2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &raw)
		_ = json.NewEncoder(w).Encode(delib)
	}))
	defer node2.Close()

	orch := NewOrchestrator(client.Config{
		SensoryURL: node3.URL, WorkingURL: node2.URL, KnowledgeURL: node1.URL,
		DefaultTimeout: 2 * time.Second, DeliberateTimeout: 2 * time.Second,
	})
	res, err := orch.RunCycle(context.Background(), model.OrchestrateRequest{
		RawInput: "transcript", TaskDirective: "Where does Mira live?", MaxTokens: 128, SessionID: "s",
	}, "trc-node2-usage")
	if err != nil {
		t.Fatal(err)
	}
	return res, raw
}

func readyDelib() model.DeliberateResponse {
	return model.DeliberateResponse{Status: "ready", StepIndex: 1, Thought: "Denver.", ProposedAction: "ANSWER", IsComplete: true, PromptTokens: 260}
}

func TestRunCycle_SendsPrepackedBudget(t *testing.T) {
	res, raw := runWithNode2(t, readyDelib())
	if raw["prepacked"] != true {
		t.Errorf("prepacked = %v, want true", raw["prepacked"])
	}
	// Defaults: 4096 context - max(512 output reserve, 128 max_tokens) - 256 template reserve.
	want := float64(DefaultContextTokens - DefaultOutputReserveTokens - DefaultPromptReserveTokens)
	if raw["prompt_budget_tokens"] != want {
		t.Errorf("prompt_budget_tokens = %v, want %v", raw["prompt_budget_tokens"], want)
	}
	if got := res.Stages[2].ContextBudget.PromptBudgetTokens; float64(got) != want {
		t.Errorf("reported budget %d differs from the one sent (%v)", got, want)
	}
}

func TestRunCycle_MergesNode2ContextUsage(t *testing.T) {
	delib := readyDelib()
	delib.ContextUsage = &model.ContextUsage{
		SensoryReceived: 2, SensoryKept: 2, FactsReceived: 0, FactsKept: 0,
		EstimatedPromptTokens: 250, ActualPromptTokens: 261, PromptWindowTokens: 3584,
	}
	res, _ := runWithNode2(t, delib)
	b := res.Stages[2].ContextBudget
	if b.Node2Usage != model.Node2UsageReported || b.Node2ContextUsage == nil || *b.Node2ContextUsage != *delib.ContextUsage {
		t.Fatalf("node 2 usage not merged: %+v", b)
	}
	if b.ActualPromptTokens != 261 || !b.WithinBudget || b.Node2SecondCut {
		t.Errorf("actual=%d within=%v second_cut=%v; want 261, true, false", b.ActualPromptTokens, b.WithinBudget, b.Node2SecondCut)
	}
	if b.ChunksPacked != 2 || res.Stages[2].Status != "success" || !res.LoopComplete {
		t.Errorf("tool side or status changed: packed=%d status=%s loop=%v", b.ChunksPacked, res.Stages[2].Status, res.LoopComplete)
	}
	// The concise output carries the same counts.
	s := res.Concise().Deliberation.ContextBudget
	if s == nil || s.Node2Usage != model.Node2UsageReported || s.Node2ContextUsage.SensoryKept != 2 || s.ActualPromptTokens != 261 {
		t.Errorf("concise summary lacks node 2 usage: %+v", s)
	}
}

func TestRunCycle_FlagsNode2SecondCutWithoutFailing(t *testing.T) {
	delib := readyDelib()
	delib.ContextUsage = &model.ContextUsage{
		SensoryReceived: 2, SensoryKept: 1, SensoryDropped: 1,
		ActualPromptTokens: 230, PromptWindowTokens: 3584,
	}
	res, _ := runWithNode2(t, delib)
	b := res.Stages[2].ContextBudget
	if !b.Node2SecondCut || !b.WithinBudget {
		t.Errorf("second_cut=%v within=%v; want true, true", b.Node2SecondCut, b.WithinBudget)
	}
	if res.Status != "completed" || !res.LoopComplete || res.Stages[2].Status != "success" {
		t.Errorf("a second cut is telemetry only, got status=%s loop=%v stage=%s", res.Status, res.LoopComplete, res.Stages[2].Status)
	}
}

func TestRunCycle_WithinBudgetUsesNode2Window(t *testing.T) {
	delib := readyDelib()
	// Over Node 2's own window, though under the tool's prompt limit (4096-512).
	delib.ContextUsage = &model.ContextUsage{SensoryReceived: 2, SensoryKept: 2, ActualPromptTokens: 3100, PromptWindowTokens: 3000}
	res, _ := runWithNode2(t, delib)
	b := res.Stages[2].ContextBudget
	if b.WithinBudget || res.Stages[2].Status != "over_budget" || res.LoopComplete {
		t.Errorf("within=%v stage=%s loop=%v; want false, over_budget, false", b.WithinBudget, res.Stages[2].Status, res.LoopComplete)
	}
}

func TestRunCycle_FallsBackWithoutContextUsage(t *testing.T) {
	res, _ := runWithNode2(t, readyDelib())
	b := res.Stages[2].ContextBudget
	if b.Node2Usage != model.Node2UsageUnknown || b.Node2ContextUsage != nil || b.Node2SecondCut {
		t.Errorf("older Node 2 must be marked unknown: %+v", b)
	}
	if b.ActualPromptTokens != 260 || !b.WithinBudget || res.Stages[2].Status != "success" {
		t.Errorf("fallback should use prompt_tokens: actual=%d within=%v status=%s", b.ActualPromptTokens, b.WithinBudget, res.Stages[2].Status)
	}
	// Unknown Node 2 counts are absent rather than zero, so they can't read as "Node 2 kept nothing".
	out, _ := json.Marshal(res.Concise())
	if !strings.Contains(string(out), `"node2_usage":"unknown"`) || strings.Contains(string(out), "node2_context_usage") || strings.Contains(string(out), "sensory_kept") {
		t.Errorf("concise fallback output: %s", out)
	}
}
