package orchestrator

import (
	"fmt"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

const (
	// DefaultContextTokens is the Node 2 llama-server context window on the test hardware (n_ctx).
	DefaultContextTokens = 4096
	// DefaultPromptReserveTokens covers the Node 2 prompt template, role markers and JSON framing.
	DefaultPromptReserveTokens = 384
	// perItemOverheadTokens accounts for the separators Node 2 adds around each chunk or fact.
	perItemOverheadTokens = 8
	// recallShareDivisor caps recalled facts at 1/4 of the prompt budget so input chunks keep priority.
	recallShareDivisor = 4
	// minTruncatedChunkTokens is the smallest partial chunk worth sending.
	minTruncatedChunkTokens = 32
	// maxPackingDecisions bounds the per-item detail list in stages[] telemetry.
	maxPackingDecisions = 64

	estimatorName = "conservative-heuristic-v1"
)

// EstimateTokens returns a deliberately high token estimate for s without a tokenizer.
// ASCII letter runs count one token per 3 characters, every digit, punctuation mark and
// symbol counts as one token, whitespace runs longer than one character count as one token,
// and each non-ASCII rune counts as one token per 2 UTF-8 bytes (rounded up).
func EstimateTokens(s string) int {
	tokens := 0
	letterRun := 0
	spaceRun := 0
	flushLetters := func() {
		if letterRun > 0 {
			tokens += (letterRun + 2) / 3
			letterRun = 0
		}
	}
	flushSpaces := func() {
		if spaceRun > 1 {
			tokens++
		}
		spaceRun = 0
	}
	for _, r := range s {
		switch {
		case r < utf8.RuneSelf && (unicode.IsLetter(r) || r == '_'):
			flushSpaces()
			letterRun++
		case r < utf8.RuneSelf && unicode.IsSpace(r):
			flushLetters()
			spaceRun++
		case r < utf8.RuneSelf:
			flushLetters()
			flushSpaces()
			tokens++
		default:
			flushLetters()
			flushSpaces()
			tokens += (utf8.RuneLen(r) + 1) / 2
		}
	}
	flushLetters()
	flushSpaces()
	return tokens
}

// truncateToTokens returns the longest rune-aligned prefix of s whose estimate fits within limit.
// EstimateTokens never decreases as a prefix grows, so a binary search over rune offsets is exact.
func truncateToTokens(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if EstimateTokens(s) <= limit {
		return s
	}
	offsets := make([]int, 0, utf8.RuneCountInString(s)+1)
	for i := range s {
		offsets = append(offsets, i)
	}
	offsets = append(offsets, len(s))
	lo, hi := 0, len(offsets)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if EstimateTokens(s[:offsets[mid]]) <= limit {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return s[:offsets[lo]]
}

// budgetSettings holds the resolved Node 2 context window parameters for one cycle.
type budgetSettings struct {
	contextTokens int
	maxTokens     int
	reserveTokens int
}

// packedContext is the budget-compliant Stage 3 payload plus its telemetry.
type packedContext struct {
	chunks []model.SensoryChunk
	facts  []string
	report *model.ContextBudgetReport
}

// packContext ranks sensory chunks by salience and recalled facts by recall score, then packs
// the highest ranked items into the Node 2 prompt budget. Low-ranked items are truncated or
// dropped, and every such decision is recorded. It returns an error when not even the
// objective fits, in which case nothing may be sent to Node 2.
func packContext(objective string, chunks []model.SensoryChunk, facts []rankedFact, s budgetSettings) (*packedContext, error) {
	report := &model.ContextBudgetReport{
		Estimator:             estimatorName,
		ContextWindowTokens:   s.contextTokens,
		MaxCompletionTokens:   s.maxTokens,
		PromptLimitTokens:     s.contextTokens - s.maxTokens,
		TemplateReserveTokens: s.reserveTokens,
		PromptBudgetTokens:    s.contextTokens - s.maxTokens - s.reserveTokens,
		ChunksIn:              len(chunks),
		FactsIn:               len(facts),
	}

	objectiveTokens := EstimateTokens(objective)
	report.CandidateTokens = objectiveTokens
	for _, c := range chunks {
		report.CandidateTokens += EstimateTokens(c.Text) + perItemOverheadTokens
	}
	for _, f := range facts {
		report.CandidateTokens += EstimateTokens(f.text) + perItemOverheadTokens
	}

	if report.PromptBudgetTokens <= objectiveTokens {
		return &packedContext{report: report}, fmt.Errorf(
			"node 2 prompt budget exhausted before packing: context window %d - max_tokens %d - template reserve %d leaves %d tokens, objective alone needs ~%d; lower --max-tokens or raise --context-tokens",
			s.contextTokens, s.maxTokens, s.reserveTokens, report.PromptBudgetTokens, objectiveTokens)
	}

	remaining := report.PromptBudgetTokens - objectiveTokens
	used := objectiveTokens
	record := func(d model.PackingDecision) {
		if len(report.Decisions) < maxPackingDecisions {
			report.Decisions = append(report.Decisions, d)
		} else {
			report.DecisionsOmitted++
		}
	}

	// Recalled facts: whole facts only, highest score first, capped at a share of the budget.
	recallCap := report.PromptBudgetTokens / recallShareDivisor
	sortedFacts := append([]rankedFact(nil), facts...)
	sort.SliceStable(sortedFacts, func(i, j int) bool { return sortedFacts[i].score > sortedFacts[j].score })
	keptFacts := make(map[int]bool, len(facts))
	recallUsed := 0
	for _, f := range sortedFacts {
		cost := EstimateTokens(f.text) + perItemOverheadTokens
		if recallUsed+cost <= recallCap && cost <= remaining {
			keptFacts[f.order] = true
			recallUsed += cost
			remaining -= cost
			used += cost
			continue
		}
		report.FactsDropped++
		record(model.PackingDecision{ID: f.id, Kind: "recall_fact", Rank: f.score, OriginalTokens: cost - perItemOverheadTokens, Action: "dropped"})
	}

	// Sensory chunks: highest salience first; the first chunk that does not fit is truncated,
	// everything ranked below it is dropped.
	type rankedChunk struct {
		order int
		chunk model.SensoryChunk
	}
	ranked := make([]rankedChunk, len(chunks))
	for i, c := range chunks {
		ranked[i] = rankedChunk{order: i, chunk: c}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].chunk.Salience > ranked[j].chunk.Salience })

	keptChunks := make(map[int]model.SensoryChunk, len(chunks))
	exhausted := false
	for _, rc := range ranked {
		id := rc.chunk.ID
		if id == "" {
			id = fmt.Sprintf("chunk[%d]", rc.order)
		}
		textTokens := EstimateTokens(rc.chunk.Text)
		cost := textTokens + perItemOverheadTokens
		if !exhausted && cost <= remaining {
			keptChunks[rc.order] = rc.chunk
			remaining -= cost
			used += cost
			continue
		}
		if !exhausted && remaining-perItemOverheadTokens >= minTruncatedChunkTokens {
			partial := truncateToTokens(rc.chunk.Text, remaining-perItemOverheadTokens)
			partialTokens := EstimateTokens(partial)
			if partial != "" {
				truncated := rc.chunk
				truncated.Text = partial
				keptChunks[rc.order] = truncated
				remaining -= partialTokens + perItemOverheadTokens
				used += partialTokens + perItemOverheadTokens
				report.ChunksTruncated++
				record(model.PackingDecision{ID: id, Kind: "sensory_chunk", Rank: rc.chunk.Salience, OriginalTokens: textTokens, KeptTokens: partialTokens, Action: "truncated"})
				exhausted = true
				continue
			}
		}
		exhausted = true
		report.ChunksDropped++
		record(model.PackingDecision{ID: id, Kind: "sensory_chunk", Rank: rc.chunk.Salience, OriginalTokens: textTokens, Action: "dropped"})
	}

	// Restore original order among the kept items so Node 2 reads them in sequence.
	packed := &packedContext{report: report}
	for i := range chunks {
		if c, ok := keptChunks[i]; ok {
			packed.chunks = append(packed.chunks, c)
		}
	}
	for _, f := range facts {
		if keptFacts[f.order] {
			packed.facts = append(packed.facts, f.text)
		}
	}

	report.ChunksPacked = len(packed.chunks)
	report.FactsPacked = len(packed.facts)
	report.EstimatedPromptTokens = used
	report.Truncated = report.ChunksTruncated > 0 || report.ChunksDropped > 0 || report.FactsDropped > 0
	report.WithinBudget = used <= report.PromptBudgetTokens
	return packed, nil
}

// rankedFact is a recalled fact string with the Node 1 score used to rank it.
type rankedFact struct {
	order int
	id    string
	text  string
	score float64
}
