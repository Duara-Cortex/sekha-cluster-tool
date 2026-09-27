package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

func TestEstimateTokens_ErrsHigh(t *testing.T) {
	cases := []struct {
		text    string
		atLeast int
	}{
		{"", 0},
		{"hello world", 4},              // letter runs at 3 chars/token
		{"2026-09-27T10:11:12Z", 20},    // every digit and symbol counts
		{"温度警告", 8},                     // 3-byte runes count 2 each
		{"a\n\n\nb", 3},                 // whitespace run counts
		{strings.Repeat("x", 300), 100}, // long runs never fall below 3 chars/token
	}
	for _, tc := range cases {
		if got := EstimateTokens(tc.text); got < tc.atLeast {
			t.Errorf("EstimateTokens(%q) = %d, want >= %d", tc.text, got, tc.atLeast)
		}
	}
}

func TestEstimateTokens_CountsNewlinesAndCaseSplits(t *testing.T) {
	if got := EstimateTokens("a\nb"); got != 3 {
		t.Errorf("a newline is a token in Node 2's tokenizer; got %d, want 3", got)
	}
	if got, whole := EstimateTokens("aBcDeF"), EstimateTokens("abcdef"); got <= whole {
		t.Errorf("camelCase should cost more than one run: %d vs %d", got, whole)
	}
}

func TestEstimateTokens_MonotonicInPrefix(t *testing.T) {
	text := "[Session 12] User: Please remember that on 2024-03-05 MiraKestrel moved to Denver.\n\n  Ada's greyhound — 温度 42%; see https://ex.org/a_b?c=1"
	prev := 0
	for i := range text {
		if got := EstimateTokens(text[:i]); got < prev {
			t.Fatalf("estimate fell from %d to %d at byte %d (%q)", prev, got, i, text[:i])
		} else {
			prev = got
		}
	}
}

// calibrationFile holds Node 2 tokenizer counts for real BEAM chunks, collected live.
const calibrationFile = "testdata/token_calibration.json"

type calibrationSample struct {
	ID           string `json:"id"`
	Text         string `json:"text"`
	ActualTokens int    `json:"actual_tokens"`
}

// TestEstimateTokens_NeverUnderestimatesCalibration checks EstimateTokens against Node 2's real
// token counts: it must never be below them, and in aggregate it should stay within ~1.3x.
func TestEstimateTokens_NeverUnderestimatesCalibration(t *testing.T) {
	data, err := os.ReadFile(calibrationFile)
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("no calibration fixtures in " + calibrationFile)
	}
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Samples []calibrationSample `json:"samples"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Samples) == 0 {
		t.Fatal("calibration file has no samples")
	}
	est, actual := 0, 0
	for _, s := range fixture.Samples {
		e := EstimateTokens(s.Text)
		if e < s.ActualTokens {
			t.Errorf("%s: estimate %d < actual %d", s.ID, e, s.ActualTokens)
		}
		est += e
		actual += s.ActualTokens
	}
	ratio := float64(est) / float64(actual)
	t.Logf("%d samples: estimated %d / actual %d = %.3f", len(fixture.Samples), est, actual, ratio)
	if ratio > maxCalibrationRatio {
		t.Errorf("estimator runs %.3fx high on real text, want <= %.2f", ratio, maxCalibrationRatio)
	}
}

// maxCalibrationRatio is the release target for estimated/actual on the calibration set.
const maxCalibrationRatio = 1.3

func TestTruncateToTokens_RuneAlignedAndMaximal(t *testing.T) {
	text := strings.Repeat("温度 alert 42; ", 200)
	for _, limit := range []int{1, 7, 50, 333} {
		got := truncateToTokens(text, limit)
		if !utf8.ValidString(got) || !strings.HasPrefix(text, got) {
			t.Fatalf("limit %d: truncation broke a rune or is not a prefix", limit)
		}
		if EstimateTokens(got) > limit {
			t.Errorf("limit %d: kept %d tokens", limit, EstimateTokens(got))
		}
		if next := text[:len(got)+utf8.RuneLen([]rune(text[len(got):])[0])]; EstimateTokens(next) <= limit {
			t.Errorf("limit %d: prefix is not maximal", limit)
		}
	}
}

func chunk(id, text string, salience float64) model.SensoryChunk {
	return model.SensoryChunk{ID: id, Text: text, Salience: salience}
}

func TestPackContext_RanksTruncatesAndRestoresOrder(t *testing.T) {
	big := strings.Repeat("orchard ledger entry ", 600) // ~4.2K tokens each, larger than the whole budget
	chunks := []model.SensoryChunk{
		chunk("low", big, 0.2),
		chunk("high", strings.Repeat("glacier melt ", 100), 0.9),
		chunk("mid", big, 0.6),
		chunk("tiny-low", "short note", 0.1),
	}
	facts := []rankedFact{
		{order: 0, id: "f-weak", text: strings.Repeat("weak fact ", 1000), score: 0.55},
		{order: 1, id: "f-strong", text: "[policy: Melt] glacier melt thresholds", score: 0.9},
	}
	packed, err := packContext("Assess melt", chunks, facts, budgetSettings{contextTokens: 4096, maxTokens: 256, outputReserveTokens: 512, reserveTokens: 256})
	if err != nil {
		t.Fatal(err)
	}
	r := packed.report
	// The output side is Node 2's 512 reserve, since it exceeds max_tokens 256.
	if r.PromptBudgetTokens != 4096-512-256 || r.PromptLimitTokens != 4096-512 || r.OutputReserveTokens != 512 {
		t.Errorf("unexpected budget arithmetic: %+v", r)
	}
	if r.EstimatedPromptTokens > r.PromptBudgetTokens || !r.WithinBudget {
		t.Errorf("packed prompt over budget: %d > %d", r.EstimatedPromptTokens, r.PromptBudgetTokens)
	}
	var ids []string
	for _, c := range packed.chunks {
		ids = append(ids, c.ID)
	}
	// "high" fits whole, "mid" is truncated to the remainder, "low" and "tiny-low" are dropped;
	// kept chunks come back in their original order.
	if got := strings.Join(ids, ","); got != "high,mid" {
		t.Errorf("packed chunk order = %s, want high,mid", got)
	}
	if r.ChunksTruncated != 1 || r.ChunksDropped != 2 || !r.Truncated {
		t.Errorf("unexpected truncation counts: %+v", r)
	}
	if len(packed.facts) != 1 || !strings.Contains(packed.facts[0], "glacier") || r.FactsDropped != 1 {
		t.Errorf("expected only the strong fact within the recall share, got %v", packed.facts)
	}
	actions := map[string]string{}
	for _, d := range r.Decisions {
		actions[d.ID] = d.Action
		if d.Action == "truncated" && (d.KeptTokens <= 0 || d.KeptTokens >= d.OriginalTokens) {
			t.Errorf("truncation detail lacks token counts: %+v", d)
		}
	}
	if actions["mid"] != "truncated" || actions["low"] != "dropped" || actions["f-weak"] != "dropped" {
		t.Errorf("decisions not recorded: %v", actions)
	}
}

func TestPackContext_RefusesWhenObjectiveCannotFit(t *testing.T) {
	_, err := packContext(strings.Repeat("goal ", 2000), nil, nil, budgetSettings{contextTokens: 1024, maxTokens: 512, outputReserveTokens: 512, reserveTokens: 256})
	if err == nil || !strings.Contains(err.Error(), "prompt budget exhausted") {
		t.Fatalf("expected budget error, got %v", err)
	}
}

func TestPackContext_CapsDecisionList(t *testing.T) {
	var chunks []model.SensoryChunk
	for i := range 200 {
		chunks = append(chunks, chunk(fmt.Sprintf("c%d", i), strings.Repeat("word ", 200), 0.5))
	}
	packed, _ := packContext("x", chunks, nil, budgetSettings{contextTokens: 4096, maxTokens: 256, outputReserveTokens: 512, reserveTokens: 256})
	r := packed.report
	if len(r.Decisions) != maxPackingDecisions || r.DecisionsOmitted != r.ChunksDropped+r.ChunksTruncated-maxPackingDecisions {
		t.Errorf("decision list not capped consistently: %d listed, %d omitted, %d dropped", len(r.Decisions), r.DecisionsOmitted, r.ChunksDropped)
	}
}

func TestGateRecall_Reasons(t *testing.T) {
	node := func(id string, sim float64, anchors []string, summary string) model.ScoredNode {
		return model.ScoredNode{Node: model.Node{ID: id, Label: id, Summary: summary, Anchors: anchors}, Score: 0.99, SimScore: sim}
	}
	reference, source := relevanceReference("Answer the benchmark question", []model.SensoryChunk{
		chunk("c1", "The glacier meltwater volume rose after the heatwave in Svalbard", 0.9),
	})
	if source != "salient_input_minus_directive" || reference["benchmark"] {
		t.Fatalf("directive terms must be excluded from the reference: %v", reference)
	}
	nodes := []model.ScoredNode{
		node("keep", 0.8, []string{"#run:beam"}, "Svalbard glacier meltwater records"),
		node("no-sim", 0, nil, "Svalbard glacier meltwater records"),
		node("anchored-no-sim", 0, []string{"#run:beam"}, "Svalbard glacier meltwater records"),
		node("anchored-no-sim-off-topic", 0, []string{"#run:beam"}, "Violin varnish recipes"),
		node("low-sim", 0.3, nil, "Svalbard glacier meltwater records"),
		node("wrong-anchor", 0.9, []string{"#run:other"}, "Svalbard glacier meltwater records"),
		node("prior-episode", 0.92, []string{"#run:beam"}, "Answer the benchmark question about violin varnish"),
	}
	kept, report := gateRecall(nodes, reference, source, gateSettings{minSim: 0.5, minTermOverlap: DefaultMinTermOverlap, minTermCoverage: DefaultMinTermCoverage, anchors: []string{"#run:beam"}})
	if len(kept) != 2 || kept[0].ID != "keep" || kept[1].ID != "anchored-no-sim" {
		t.Fatalf("expected 'keep' and 'anchored-no-sim', got %+v", kept)
	}
	want := map[string]string{
		"no-sim":        reasonMissingSim,
		"low-sim":       reasonBelowSimFloor,
		"wrong-anchor":  reasonAnchorMismatch,
		"prior-episode": reasonNoTermOverlap,
		// An anchor match admits a node without sim_score, but never on its own.
		"anchored-no-sim-off-topic": reasonNoTermOverlap,
	}
	for _, d := range report.Dropped {
		if want[d.ID] != d.Reason {
			t.Errorf("%s dropped for %q, want %q", d.ID, d.Reason, want[d.ID])
		}
	}
	if report.NodesIn != 7 || report.NodesKept != 2 || report.NodesDropped != 5 || report.MinSimScore != 0.5 {
		t.Errorf("unexpected report counts: %+v", report)
	}
}

func TestBuildRecallQuery_UsesTopChunkExcerpt(t *testing.T) {
	q, truncated := buildRecallQuery("Answer", []model.SensoryChunk{
		chunk("a", "minor", 0.1),
		chunk("b", strings.Repeat("é", 600), 0.9),
	})
	if !truncated || !strings.HasPrefix(q, "Answer\n") || utf8.RuneCountInString(q) != len("Answer\n")+recallQueryExcerptRunes {
		t.Errorf("unexpected recall query (%d runes, truncated=%v)", utf8.RuneCountInString(q), truncated)
	}
	if q, _ := buildRecallQuery("Only directive", nil); q != "Only directive" {
		t.Errorf("expected directive-only query, got %q", q)
	}
}
