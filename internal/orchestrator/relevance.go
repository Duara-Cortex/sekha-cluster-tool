package orchestrator

import (
	"strings"
	"unicode"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

const (
	// DefaultRecallMinSim is the semantic similarity floor a recalled node must meet to reach Node 2.
	// It is compared against sim_score, never the blended score, because recency and anchor boosts
	// are exactly what promote the episode a previous cycle just consolidated.
	DefaultRecallMinSim = 0.50
	// DefaultMinTermOverlap is the number of distinctive terms a recalled node must share with the
	// current input (excluding directive terms, which every cycle of a run shares).
	DefaultMinTermOverlap = 2
	// DefaultMinTermCoverage is the fraction of a node's own distinctive terms that must appear in
	// the current input. A raw overlap count alone is too easy to satisfy against a 90 KB input.
	DefaultMinTermCoverage = 0.15
	// recallQueryExcerptRunes bounds how much of the top salient chunk is appended to the recall query.
	recallQueryExcerptRunes = 512
	// minTermLength ignores short tokens that carry little topical signal.
	minTermLength = 4
)

// Drop reasons recorded in stages[1].relevance_gate.dropped[].reason.
const (
	reasonKept           = "relevant"
	reasonMissingSim     = "missing_sim_score"
	reasonBelowSimFloor  = "below_sim_floor"
	reasonAnchorMismatch = "anchor_mismatch"
	reasonNoTermOverlap  = "insufficient_term_overlap"
)

var stopwords = map[string]bool{
	"about": true, "after": true, "again": true, "also": true, "been": true, "before": true,
	"being": true, "between": true, "both": true, "could": true, "does": true, "doing": true,
	"down": true, "during": true, "each": true, "from": true, "further": true, "have": true,
	"having": true, "here": true, "into": true, "just": true, "more": true, "most": true,
	"only": true, "other": true, "over": true, "same": true, "should": true, "some": true,
	"such": true, "than": true, "that": true, "their": true, "them": true, "then": true,
	"there": true, "these": true, "they": true, "this": true, "those": true, "through": true,
	"under": true, "until": true, "very": true, "were": true, "what": true, "when": true,
	"where": true, "which": true, "while": true, "will": true, "with": true, "would": true,
	"your": true, "yours": true, "user": true, "assistant": true, "please": true, "thanks": true,
	"like": true, "know": true, "want": true, "need": true, "make": true, "using": true,
	"used": true, "task": true, "input": true, "context": true, "session": true, "episode": true,
	"cycle": true, "config": true, "store": true, "configuration": true,
}

// terms returns the set of distinctive lowercase terms in s.
func terms(s string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(w)) >= minTermLength && !stopwords[w] {
			set[w] = true
		}
	}
	return set
}

// relevanceReference builds the term set recalled nodes are judged against. When the cycle has
// sensory content, directive terms are removed: the directive is typically identical across
// every cycle of a run and is written into each consolidated episode, so matching on it is
// what lets a previous cycle's episode leak into the current one.
func relevanceReference(directive string, chunks []model.SensoryChunk) (map[string]bool, string) {
	var sb strings.Builder
	for _, c := range chunks {
		sb.WriteString(c.Text)
		sb.WriteByte('\n')
	}
	content := terms(sb.String())
	if len(content) == 0 {
		return terms(directive), "task_directive"
	}
	for t := range terms(directive) {
		delete(content, t)
	}
	return content, "salient_input_minus_directive"
}

// gateSettings holds the resolved relevance gate parameters for one cycle.
type gateSettings struct {
	minSim          float64
	minTermOverlap  int
	minTermCoverage float64
	anchors         []string
}

// gateRecall filters recalled nodes down to those relevant to the current cycle. A node must
// carry a sim_score at or above the floor, match a requested anchor when it lists anchors, and
// share enough distinctive terms with the current input. A node without a sim_score (Node 1's
// lean schema omits it for anchor-only hits) is admitted only if it matches a requested anchor
// and still passes the term check. Anchor matching is required but never
// sufficient, because every cycle of a benchmark run usually shares the same anchor. Term
// overlap requires min(minTermOverlap, node terms) shared terms and minTermCoverage of the
// node's terms present in the input.
func gateRecall(nodes []model.ScoredNode, reference map[string]bool, referenceSource string, s gateSettings) ([]model.ScoredNode, *model.RelevanceGateReport) {
	report := &model.RelevanceGateReport{
		MinSimScore:     s.minSim,
		MinTermOverlap:  s.minTermOverlap,
		MinTermCoverage: s.minTermCoverage,
		AnchorsRequired: s.anchors,
		ReferenceSource: referenceSource,
		NodesIn:         len(nodes),
		Kept:            []model.RecallNodeDecision{},
		Dropped:         []model.RecallNodeDecision{},
	}
	wantAnchors := make(map[string]bool, len(s.anchors))
	for _, a := range s.anchors {
		wantAnchors[model.NormalizeAnchor(a)] = true
	}

	kept := make([]model.ScoredNode, 0, len(nodes))
	for _, n := range nodes {
		nodeTerms := terms(n.Label + " " + n.Summary)
		overlap := 0
		for t := range nodeTerms {
			if reference[t] {
				overlap++
			}
		}
		coverage := 0.0
		if len(nodeTerms) > 0 {
			coverage = float64(overlap) / float64(len(nodeTerms))
		}
		decision := model.RecallNodeDecision{
			ID:           n.ID,
			Label:        n.Label,
			EntityType:   n.EntityType,
			Score:        n.Score,
			SimScore:     n.SimScore,
			HopDistance:  n.HopDistance,
			TermOverlap:  overlap,
			TermCoverage: coverage,
		}

		anchorMatch := len(wantAnchors) > 0 && anyAnchorMatches(n.Anchors, wantAnchors)
		switch {
		case n.SimScore <= 0 && !anchorMatch:
			decision.Reason = reasonMissingSim
		case n.SimScore > 0 && n.SimScore < s.minSim:
			decision.Reason = reasonBelowSimFloor
		case len(wantAnchors) > 0 && len(n.Anchors) > 0 && !anyAnchorMatches(n.Anchors, wantAnchors):
			decision.Reason = reasonAnchorMismatch
		case overlap == 0 || overlap < min(s.minTermOverlap, len(nodeTerms)) || coverage < s.minTermCoverage:
			decision.Reason = reasonNoTermOverlap
		default:
			decision.Reason = reasonKept
		}

		if decision.Reason == reasonKept {
			kept = append(kept, n)
			report.Kept = append(report.Kept, decision)
		} else {
			report.Dropped = append(report.Dropped, decision)
		}
	}
	report.NodesKept = len(report.Kept)
	report.NodesDropped = len(report.Dropped)
	return kept, report
}

func anyAnchorMatches(nodeAnchors []string, want map[string]bool) bool {
	for _, a := range nodeAnchors {
		if want[model.NormalizeAnchor(a)] {
			return true
		}
	}
	return false
}

// buildRecallQuery combines the directive with a bounded excerpt of the most salient chunk so
// recall is steered by this cycle's content rather than by a directive every cycle shares.
func buildRecallQuery(directive string, chunks []model.SensoryChunk) (string, bool) {
	var top *model.SensoryChunk
	for i := range chunks {
		if top == nil || chunks[i].Salience > top.Salience {
			top = &chunks[i]
		}
	}
	if top == nil || strings.TrimSpace(top.Text) == "" {
		return directive, false
	}
	excerpt := strings.TrimSpace(top.Text)
	truncated := false
	if runes := []rune(excerpt); len(runes) > recallQueryExcerptRunes {
		excerpt = string(runes[:recallQueryExcerptRunes])
		truncated = true
	}
	if directive == "" {
		return excerpt, truncated
	}
	return directive + "\n" + excerpt, truncated
}

// filterRecallResponse restricts the recall response to the kept nodes and the edges between them,
// so the orchestrate output does not surface dropped episodes to callers.
func filterRecallResponse(resp *model.RecallResponse, kept []model.ScoredNode) {
	keptIDs := make(map[string]bool, len(kept))
	for _, n := range kept {
		keptIDs[n.ID] = true
	}
	if len(resp.ProjectedNodes) == len(resp.Nodes) {
		projected := make([]map[string]any, 0, len(kept))
		for i, n := range resp.Nodes {
			if keptIDs[n.ID] {
				projected = append(projected, resp.ProjectedNodes[i])
			}
		}
		resp.ProjectedNodes = projected
	} else {
		resp.ProjectedNodes = nil
	}
	resp.Nodes = kept
	edges := make([]model.Edge, 0, len(resp.Edges))
	for _, e := range resp.Edges {
		if keptIDs[e.SourceID] && keptIDs[e.TargetID] {
			edges = append(edges, e)
		}
	}
	resp.Edges = edges
}
