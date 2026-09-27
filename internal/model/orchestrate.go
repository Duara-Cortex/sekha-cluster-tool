package model

import (
	"fmt"
	"unicode/utf8"
)

// OrchestrateRequest defines the input configuration for a full closed-loop cognitive turn.
type OrchestrateRequest struct {
	RawInput               string   `json:"raw_input"`
	TaskDirective          string   `json:"task_directive,omitempty"`
	FilterThreshold        float64  `json:"filter_threshold,omitempty"`
	RecallTopK             int      `json:"recall_top_k,omitempty"`
	MaxTokens              int      `json:"max_tokens,omitempty"`
	SessionID              string   `json:"session_id,omitempty"`
	SynchronousConsolidate bool     `json:"synchronous_consolidate,omitempty"`
	Anchors                []string `json:"anchors,omitempty"`
	AnchorMode             string   `json:"anchor_mode,omitempty"`
	IncludeEmbeddings      bool     `json:"include_embeddings,omitempty"`
	RecallMinSim           float64  `json:"recall_min_sim,omitempty"`
	ContextTokens          int      `json:"context_tokens,omitempty"`
	PromptReserveTokens    int      `json:"prompt_reserve_tokens,omitempty"`
}

// StageTelemetry tracks execution duration and status for an individual stage in the cognitive loop.
type StageTelemetry struct {
	StageName     string               `json:"stage_name"`
	Node          string               `json:"node"`
	Endpoint      string               `json:"endpoint"`
	DurationMS    float64              `json:"duration_ms"`
	Status        string               `json:"status"`
	Error         string               `json:"error,omitempty"`
	RelevanceGate *RelevanceGateReport `json:"relevance_gate,omitempty"`
	ContextBudget *ContextBudgetReport `json:"context_budget,omitempty"`
}

// RecallNodeDecision records why a recalled Node 1 node was kept or dropped before deliberation.
type RecallNodeDecision struct {
	ID           string  `json:"id"`
	Label        string  `json:"label,omitempty"`
	EntityType   string  `json:"entity_type,omitempty"`
	Score        float64 `json:"score"`
	SimScore     float64 `json:"sim_score"`
	HopDistance  int     `json:"hop_distance,omitempty"`
	TermOverlap  int     `json:"term_overlap"`
	TermCoverage float64 `json:"term_coverage"`
	Reason       string  `json:"reason"`
}

// RelevanceGateReport summarises the Stage 2 -> Stage 3 relevance gate applied to recalled nodes.
type RelevanceGateReport struct {
	MinSimScore        float64              `json:"min_sim_score"`
	MinTermOverlap     int                  `json:"min_term_overlap"`
	MinTermCoverage    float64              `json:"min_term_coverage"`
	AnchorsRequired    []string             `json:"anchors_required,omitempty"`
	ReferenceSource    string               `json:"reference_source"`
	RecallQueryBytes   int                  `json:"recall_query_bytes"`
	RecallQueryExcerpt bool                 `json:"recall_query_excerpt_truncated,omitempty"`
	NodesIn            int                  `json:"nodes_in"`
	NodesKept          int                  `json:"nodes_kept"`
	NodesDropped       int                  `json:"nodes_dropped"`
	Kept               []RecallNodeDecision `json:"kept"`
	Dropped            []RecallNodeDecision `json:"dropped"`
}

// PackingDecision records a context item that was truncated or dropped to fit the Node 2 budget.
type PackingDecision struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	Rank           float64 `json:"rank_score"`
	OriginalTokens int     `json:"original_tokens"`
	KeptTokens     int     `json:"kept_tokens"`
	Action         string  `json:"action"`
}

// ContextBudgetReport describes how Stage 3 context was packed into the Node 2 context window.
type ContextBudgetReport struct {
	Estimator             string            `json:"estimator"`
	ContextWindowTokens   int               `json:"context_window_tokens"`
	MaxCompletionTokens   int               `json:"max_completion_tokens"`
	PromptLimitTokens     int               `json:"prompt_limit_tokens"`
	TemplateReserveTokens int               `json:"template_reserve_tokens"`
	PromptBudgetTokens    int               `json:"prompt_budget_tokens"`
	CandidateTokens       int               `json:"candidate_tokens"`
	EstimatedPromptTokens int               `json:"estimated_prompt_tokens"`
	ActualPromptTokens    int               `json:"actual_prompt_tokens,omitempty"`
	WithinBudget          bool              `json:"within_budget"`
	ChunksIn              int               `json:"chunks_in"`
	ChunksPacked          int               `json:"chunks_packed"`
	ChunksTruncated       int               `json:"chunks_truncated"`
	ChunksDropped         int               `json:"chunks_dropped"`
	FactsIn               int               `json:"facts_in"`
	FactsPacked           int               `json:"facts_packed"`
	FactsDropped          int               `json:"facts_dropped"`
	Truncated             bool              `json:"truncated"`
	Decisions             []PackingDecision `json:"decisions,omitempty"`
	DecisionsOmitted      int               `json:"decisions_omitted,omitempty"`
}

// OrchestrateResponse encapsulates the end-to-end outcome of the four-stage cognitive cycle.
// This is the full (--full) output; the default output is its Concise form.
//
// Status is "completed" when no stage failed, "partial" when some failed and "failed" when all
// did. IsComplete is Node 2's own flag from deliberation and says nothing about the loop;
// LoopComplete is true only when all four stages report "success".
type OrchestrateResponse struct {
	Status          string              `json:"status"`
	IsComplete      bool                `json:"is_complete"`
	Stages          []StageTelemetry    `json:"stages"`
	FinalThought    string              `json:"final_thought,omitempty"`
	ProposedAction  string              `json:"proposed_action,omitempty"`
	TraceID         string              `json:"trace_id"`
	SessionID       string              `json:"session_id"`
	TotalDurationMS float64             `json:"total_duration_ms"`
	LoopComplete    bool                `json:"loop_complete"`
	Sensory         FilterResponse      `json:"sensory"`
	Recall          RecallResponse      `json:"recall"`
	Deliberation    DeliberateResponse  `json:"deliberation"`
	Consolidation   ConsolidateResponse `json:"consolidation"`
}

// maxConciseErrorBytes caps error and message text in concise output, so a few failed stages
// carrying HTTP body snippets still keep stages[] near the top of the output.
const maxConciseErrorBytes = 300

// ConciseStage is a stages[] entry in the default orchestrate output.
type ConciseStage struct {
	StageName  string  `json:"stage_name"`
	Status     string  `json:"status"`
	Error      string  `json:"error,omitempty"`
	DurationMS float64 `json:"duration_ms"`
}

// SensorySummary gives Node 3 filter counts without chunk text.
type SensorySummary struct {
	TotalChunks    int     `json:"total_chunks"`
	SalientChunks  int     `json:"salient_chunks"`
	NoiseDiscarded int     `json:"noise_discarded"`
	ReductionRate  float64 `json:"reduction_rate"`
	LatencyMS      float64 `json:"latency_ms"`
}

// RelevanceGateSummary gives the Stage 2 -> Stage 3 gate counts and thresholds without the
// per-node kept/dropped lists.
type RelevanceGateSummary struct {
	MinSimScore     float64  `json:"min_sim_score"`
	MinTermOverlap  int      `json:"min_term_overlap"`
	MinTermCoverage float64  `json:"min_term_coverage"`
	AnchorsRequired []string `json:"anchors_required,omitempty"`
	NodesIn         int      `json:"nodes_in"`
	NodesKept       int      `json:"nodes_kept"`
	NodesDropped    int      `json:"nodes_dropped"`
}

// RecallSummary gives recall counts and latency without node content.
type RecallSummary struct {
	Nodes          int                   `json:"nodes"`
	Edges          int                   `json:"edges"`
	QueryLatencyMS float64               `json:"query_latency_ms"`
	RelevanceGate  *RelevanceGateSummary `json:"relevance_gate,omitempty"`
}

// ContextBudgetSummary gives the Stage 3 packing counts without per-item decisions.
type ContextBudgetSummary struct {
	ContextWindowTokens   int  `json:"context_window_tokens"`
	PromptBudgetTokens    int  `json:"prompt_budget_tokens"`
	EstimatedPromptTokens int  `json:"estimated_prompt_tokens"`
	ActualPromptTokens    int  `json:"actual_prompt_tokens,omitempty"`
	WithinBudget          bool `json:"within_budget"`
	ChunksIn              int  `json:"chunks_in"`
	ChunksPacked          int  `json:"chunks_packed"`
	ChunksTruncated       int  `json:"chunks_truncated"`
	ChunksDropped         int  `json:"chunks_dropped"`
	FactsIn               int  `json:"facts_in"`
	FactsPacked           int  `json:"facts_packed"`
	FactsDropped          int  `json:"facts_dropped"`
}

// DeliberationSummary gives Node 2 status, token counts and rates.
type DeliberationSummary struct {
	Status           string                `json:"status"`
	PromptTokens     int                   `json:"prompt_tokens"`
	CompletionTokens int                   `json:"completion_tokens"`
	TotalTokens      int                   `json:"total_tokens"`
	EvaluationRate   float64               `json:"prompt_eval_rate_tps,omitempty"`
	GenerationRate   float64               `json:"generation_rate_tps,omitempty"`
	ContextBudget    *ContextBudgetSummary `json:"context_budget,omitempty"`
}

// ConsolidationSummary gives the Node 1 consolidation receipt with a capped message.
type ConsolidationSummary struct {
	Status            string `json:"status"`
	Synchronous       bool   `json:"synchronous"`
	EntitiesExtracted int    `json:"entities_extracted"`
	NodesFused        int    `json:"nodes_fused"`
	EdgesReinforced   int    `json:"edges_reinforced"`
	Message           string `json:"message,omitempty"`
}

// OrchestrateSummary is the default orchestrate output: a few KB for any input size, with the
// stage outcomes near the top so a truncated preview still shows them.
type OrchestrateSummary struct {
	Status          string               `json:"status"`
	IsComplete      bool                 `json:"is_complete"`
	Stages          []ConciseStage       `json:"stages"`
	FinalThought    string               `json:"final_thought,omitempty"`
	ProposedAction  string               `json:"proposed_action,omitempty"`
	TraceID         string               `json:"trace_id"`
	SessionID       string               `json:"session_id"`
	TotalDurationMS float64              `json:"total_duration_ms"`
	LoopComplete    bool                 `json:"loop_complete"`
	Sensory         SensorySummary       `json:"sensory"`
	Recall          RecallSummary        `json:"recall"`
	Deliberation    DeliberationSummary  `json:"deliberation"`
	Consolidation   ConsolidationSummary `json:"consolidation"`
}

// Concise summarises r for default output, dropping chunk text, recalled nodes and per-item
// gate and packing decisions.
func (r *OrchestrateResponse) Concise() OrchestrateSummary {
	out := OrchestrateSummary{
		Status:          r.Status,
		IsComplete:      r.IsComplete,
		Stages:          make([]ConciseStage, 0, len(r.Stages)),
		FinalThought:    r.FinalThought,
		ProposedAction:  r.ProposedAction,
		TraceID:         r.TraceID,
		SessionID:       r.SessionID,
		TotalDurationMS: r.TotalDurationMS,
		LoopComplete:    r.LoopComplete,
		Sensory:         r.Sensory.Summary(),
		Recall: RecallSummary{
			Nodes:          len(r.Recall.Nodes),
			Edges:          len(r.Recall.Edges),
			QueryLatencyMS: r.Recall.QueryLatencyMS,
		},
		Deliberation: DeliberationSummary{
			Status:           r.Deliberation.Status,
			PromptTokens:     r.Deliberation.PromptTokens,
			CompletionTokens: r.Deliberation.CompletionTokens,
			TotalTokens:      r.Deliberation.TotalTokens,
			EvaluationRate:   r.Deliberation.EvaluationRate,
			GenerationRate:   r.Deliberation.GenerationRate,
		},
		Consolidation: r.Consolidation.Concise(),
	}
	for _, st := range r.Stages {
		out.Stages = append(out.Stages, ConciseStage{
			StageName:  st.StageName,
			Status:     st.Status,
			Error:      CapText(st.Error, maxConciseErrorBytes),
			DurationMS: st.DurationMS,
		})
		if g := st.RelevanceGate; g != nil {
			out.Recall.RelevanceGate = &RelevanceGateSummary{
				MinSimScore:     g.MinSimScore,
				MinTermOverlap:  g.MinTermOverlap,
				MinTermCoverage: g.MinTermCoverage,
				AnchorsRequired: g.AnchorsRequired,
				NodesIn:         g.NodesIn,
				NodesKept:       g.NodesKept,
				NodesDropped:    g.NodesDropped,
			}
		}
		if b := st.ContextBudget; b != nil {
			out.Deliberation.ContextBudget = &ContextBudgetSummary{
				ContextWindowTokens:   b.ContextWindowTokens,
				PromptBudgetTokens:    b.PromptBudgetTokens,
				EstimatedPromptTokens: b.EstimatedPromptTokens,
				ActualPromptTokens:    b.ActualPromptTokens,
				WithinBudget:          b.WithinBudget,
				ChunksIn:              b.ChunksIn,
				ChunksPacked:          b.ChunksPacked,
				ChunksTruncated:       b.ChunksTruncated,
				ChunksDropped:         b.ChunksDropped,
				FactsIn:               b.FactsIn,
				FactsPacked:           b.FactsPacked,
				FactsDropped:          b.FactsDropped,
			}
		}
	}
	return out
}

// Summary gives the filter counts without chunk text.
func (r FilterResponse) Summary() SensorySummary {
	return SensorySummary{
		TotalChunks:    r.TotalChunks,
		SalientChunks:  r.SalientChunks,
		NoiseDiscarded: r.NoiseDiscarded,
		ReductionRate:  r.ReductionRate,
		LatencyMS:      r.LatencyMS,
	}
}

// Concise gives the consolidation receipt with its message capped.
func (r ConsolidateResponse) Concise() ConsolidationSummary {
	return ConsolidationSummary{
		Status:            r.Status,
		Synchronous:       r.Synchronous,
		EntitiesExtracted: r.EntitiesExtracted,
		NodesFused:        r.NodesFused,
		EdgesReinforced:   r.EdgesReinforced,
		Message:           CapText(r.Message, maxConciseErrorBytes),
	}
}

// CapText shortens s to at most n bytes (on a rune boundary), noting the original length.
func CapText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return fmt.Sprintf("%s... (%d bytes total)", s[:cut], len(s))
}
