package model

// OrchestrateRequest defines the input configuration for a full closed-loop cognitive turn.
type OrchestrateRequest struct {
	RawInput               string  `json:"raw_input"`
	TaskDirective          string  `json:"task_directive,omitempty"`
	FilterThreshold        float64 `json:"filter_threshold,omitempty"`
	RecallTopK             int     `json:"recall_top_k,omitempty"`
	MaxTokens              int      `json:"max_tokens,omitempty"`
	SessionID              string   `json:"session_id,omitempty"`
	SynchronousConsolidate bool     `json:"synchronous_consolidate,omitempty"`
	Anchors                []string `json:"anchors,omitempty"`
	AnchorMode             string   `json:"anchor_mode,omitempty"`
	IncludeEmbeddings      bool     `json:"include_embeddings,omitempty"`
}

// StageTelemetry tracks execution duration and status for an individual stage in the cognitive loop.
type StageTelemetry struct {
	StageName  string  `json:"stage_name"`
	Node       string  `json:"node"`
	Endpoint   string  `json:"endpoint"`
	DurationMS float64 `json:"duration_ms"`
	Status     string  `json:"status"`
	Error      string  `json:"error,omitempty"`
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
