package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/client"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

var isolationTopics = [10][3]string{
	{"glacier", "meltwater", "svalbard"},
	{"violin", "varnish", "luthier"},
	{"sourdough", "starter", "fermentation"},
	{"volcano", "magma", "basalt"},
	{"chess", "gambit", "endgame"},
	{"orchid", "pollination", "greenhouse"},
	{"satellite", "telemetry", "orbit"},
	{"cheese", "rennet", "curd"},
	{"lighthouse", "keeper", "foghorn"},
	{"falcon", "falconry", "hood"},
}

const templateOverheadTokens = 200 // what the mock Node 2 template adds (~184 measured live); below the 256 reserve

// leakyKnowledge simulates Node 1 in the worst case: every consolidated cycle is extracted into
// a concept node that comes back on every later recall with a high sim_score, the run's anchor,
// and a recency-boosted blended score — exactly the conditions that leaked prior cycles before.
type leakyKnowledge struct {
	mu       sync.Mutex
	concepts []model.ScoredNode
}

func (k *leakyKnowledge) handler(w http.ResponseWriter, r *http.Request) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if strings.HasSuffix(r.URL.Path, "/consolidate") {
		var req model.ConsolidateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		for i, t := range isolationTopics {
			if strings.Contains(req.SensoryContext[0].Text, t[0]) {
				k.concepts = append(k.concepts, model.ScoredNode{
					Node: model.Node{
						ID: fmt.Sprintf("concept-cycle-%d", i), EntityType: "concept",
						Label:   strings.Join(t[:2], " "),
						Summary: fmt.Sprintf("Concept from prior discussion: %s, %s, %s", t[0], t[1], t[2]),
						Anchors: []string{"#run:beam-armc"},
					},
					Score: 0.99, SimScore: 0.90,
				})
			}
		}
		_ = json.NewEncoder(w).Encode(model.ConsolidateResponse{Status: "success", TraceID: req.TraceID})
		return
	}

	var req model.RecallRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	nodes := append([]model.ScoredNode(nil), k.concepts...)
	for i, t := range isolationTopics {
		if strings.Contains(req.Query, t[0]) {
			nodes = append(nodes, model.ScoredNode{
				Node: model.Node{
					ID: fmt.Sprintf("background-%d", i), EntityType: "reference",
					Label:   t[0] + " background",
					Summary: fmt.Sprintf("Reference notes on %s and %s", t[0], t[1]),
					Anchors: []string{"#run:beam-armc"},
				},
				Score: 0.85, SimScore: 0.80,
			})
		}
	}
	nodes = append(nodes,
		model.ScoredNode{Node: model.Node{ID: "unrelated-episode", EntityType: "episode", Label: "Quarterly spreadsheet",
			Summary: "Quarterly spreadsheet reconciliation for procurement"}, Score: 0.70, SimScore: 0.35},
		model.ScoredNode{Node: model.Node{ID: "hop-neighbour", EntityType: "concept", Label: "Adjacent concept",
			Summary: "Linked through graph expansion"}, Score: 0.60, HopDistance: 1},
	)
	_ = json.NewEncoder(w).Encode(model.RecallResponse{Nodes: nodes})
}

// echoWorking simulates Node 2: the thought names what was actually in its prompt, and
// prompt_tokens reports what that prompt would cost including the template.
func echoWorking(t *testing.T, received *[]model.DeliberateRequest, mu *sync.Mutex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req model.DeliberateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad deliberate request: %v", err)
		}
		mu.Lock()
		*received = append(*received, req)
		mu.Unlock()

		prompt := EstimateTokens(req.Objective) + templateOverheadTokens
		var focus []string
		for _, c := range req.SensoryChunks {
			prompt += EstimateTokens(c.Text) + perItemOverheadTokens
			for term := range terms(c.Text) {
				for _, topic := range isolationTopics {
					if term == topic[0] {
						focus = append(focus, term)
					}
				}
			}
		}
		for _, f := range req.LongTermContext {
			prompt += EstimateTokens(f) + perItemOverheadTokens
		}
		thought := fmt.Sprintf("Focus: %s. Recalled: %s", strings.Join(focus, ","), strings.Join(req.LongTermContext, " | "))
		_ = json.NewEncoder(w).Encode(model.DeliberateResponse{
			Status: "ready", StepIndex: 1, Thought: thought, ProposedAction: "ANSWER", IsComplete: true, PromptTokens: prompt,
		})
	}
}

func TestOrchestrator_RelevanceIsolationAcrossTenCycles(t *testing.T) {
	knowledge := &leakyKnowledge{}
	node1 := httptest.NewServer(http.HandlerFunc(knowledge.handler))
	defer node1.Close()

	node3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(model.FilterResponse{
			Chunks: []model.SensoryChunk{{ID: "chk-0", Text: string(body), Salience: 0.9, Timestamp: time.Now()}},
		})
	}))
	defer node3.Close()

	var mu sync.Mutex
	var delibs []model.DeliberateRequest
	node2 := httptest.NewServer(echoWorking(t, &delibs, &mu))
	defer node2.Close()

	orch := NewOrchestrator(client.Config{
		SensoryURL: node3.URL, WorkingURL: node2.URL, KnowledgeURL: node1.URL,
		DefaultTimeout: 5 * time.Second, DeliberateTimeout: 5 * time.Second,
	})

	thoughts := make(map[string]int)
	for cycle, topic := range isolationTopics {
		sentence := fmt.Sprintf("In this part of the conversation the speaker explains %s, %s and %s in detail. ", topic[0], topic[1], topic[2])
		repeat := 3
		if cycle == 4 {
			repeat = 1200 // ~100 KB: forces context budget truncation mid-run
		}
		res, err := orch.RunCycle(context.Background(), model.OrchestrateRequest{
			RawInput:      strings.Repeat(sentence, repeat),
			TaskDirective: "Answer the user's question using the conversation so far",
			SessionID:     "sess-beam-armc",
			Anchors:       []string{"#run:beam-armc"},
			MaxTokens:     256,
		}, fmt.Sprintf("trc-cycle-%d", cycle))
		if err != nil {
			t.Fatalf("cycle %d: %v", cycle, err)
		}

		// Distinct thoughts, and nothing from any other cycle reached Node 2 or its thought.
		if prev, dup := thoughts[res.FinalThought]; dup {
			t.Errorf("cycle %d repeated the thought of cycle %d: %q", cycle, prev, res.FinalThought)
		}
		thoughts[res.FinalThought] = cycle
		sent := delibs[len(delibs)-1]
		for other, otherTopic := range isolationTopics {
			if other == cycle {
				continue
			}
			for _, term := range otherTopic {
				if strings.Contains(res.FinalThought, term) || strings.Contains(strings.Join(sent.LongTermContext, " "), term) {
					t.Errorf("cycle %d leaked %q from cycle %d: %q", cycle, term, other, res.FinalThought)
				}
			}
		}
		if !strings.Contains(res.FinalThought, topic[0]+" background") {
			t.Errorf("cycle %d dropped its own relevant fact: %q", cycle, res.FinalThought)
		}

		// stages[1] records every dropped node with its reason.
		gate := res.Stages[1].RelevanceGate
		if gate == nil {
			t.Fatalf("cycle %d: stages[1].relevance_gate missing", cycle)
		}
		dropped := map[string]string{}
		for _, d := range gate.Dropped {
			dropped[d.ID] = d.Reason
		}
		for prior := range cycle {
			if dropped[fmt.Sprintf("concept-cycle-%d", prior)] != reasonNoTermOverlap {
				t.Errorf("cycle %d: prior cycle %d concept not dropped for term overlap: %v", cycle, prior, dropped)
			}
		}
		if dropped["unrelated-episode"] != reasonBelowSimFloor || dropped["hop-neighbour"] != reasonMissingSim {
			t.Errorf("cycle %d: unrelated nodes not dropped with reasons: %v", cycle, dropped)
		}
		if len(res.Recall.Nodes) != gate.NodesKept {
			t.Errorf("cycle %d: recall output still lists dropped nodes", cycle)
		}

		// Node 2 prompt stays inside the deliberation budget, by estimate and by reported tokens.
		budget := res.Stages[2].ContextBudget
		if budget == nil || !budget.WithinBudget || budget.EstimatedPromptTokens > budget.PromptBudgetTokens ||
			budget.ActualPromptTokens > budget.PromptLimitTokens || budget.ActualPromptTokens == 0 {
			t.Errorf("cycle %d: budget violated: %+v", cycle, budget)
		}
		if cycle == 4 && (!budget.Truncated || budget.ChunksTruncated != 1) {
			t.Errorf("cycle 4: expected the 100 KB chunk to be truncated with details, got %+v", budget)
		}
	}
	if len(thoughts) != len(isolationTopics) {
		t.Errorf("expected %d distinct thoughts, got %d", len(isolationTopics), len(thoughts))
	}
}
