package model

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
type OrchestrateResponse struct {
	TraceID         string              `json:"trace_id"`
	SessionID       string              `json:"session_id"`
	Status          string              `json:"status"`
	FinalThought    string              `json:"final_thought,omitempty"`
	ProposedAction  string              `json:"proposed_action,omitempty"`
	IsComplete      bool                `json:"is_complete"`
	Sensory         FilterResponse      `json:"sensory"`
	Recall          RecallResponse      `json:"recall"`
	Deliberation    DeliberateResponse  `json:"deliberation"`
	Consolidation   ConsolidateResponse `json:"consolidation"`
	Stages          []StageTelemetry    `json:"stages"`
	TotalDurationMS float64             `json:"total_duration_ms"`
}
