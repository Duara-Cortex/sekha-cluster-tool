package model

import "time"

// ReasoningStep captures a single deliberate reasoning step in the trajectory.
type ReasoningStep struct {
	StepIndex   int       `json:"step_index"`
	Thought     string    `json:"thought"`
	Action      string    `json:"action,omitempty"`
	Observation string    `json:"observation,omitempty"`
	Status      string    `json:"status"`
	Timestamp   time.Time `json:"timestamp"`
}

// CandidateAction represents an uncommitted proposed action or tool invocation.
type CandidateAction struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	Committed bool                   `json:"committed"`
	CreatedAt time.Time              `json:"created_at"`
}

// WorkingMemoryState is the active deliberation state managed on Node 2.
type WorkingMemoryState struct {
	SessionID        string            `json:"session_id"`
	ActiveGoal       string            `json:"active_goal"`
	SensoryContext   []SensoryChunk    `json:"sensory_context"`
	LongTermContext  []string          `json:"long_term_context"`
	Trajectory       []ReasoningStep   `json:"trajectory"`
	CandidateActions []CandidateAction `json:"candidate_actions"`
	Status           string            `json:"status"`
	TokenEstimate    int               `json:"token_estimate"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// DeliberateRequest defines the input payload for POST /api/v1/working/deliberate.
type DeliberateRequest struct {
	Objective       string         `json:"objective"`
	SensoryChunks   []SensoryChunk `json:"sensory_chunks,omitempty"`
	LongTermContext []string       `json:"long_term_context,omitempty"`
	Observation     string         `json:"observation,omitempty"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
	Temperature     float64        `json:"temperature,omitempty"`
	// Prepacked tells Node 2 the caller already packed SensoryChunks and LongTermContext into
	// PromptBudgetTokens, so Node 2 should not cut them again.
	Prepacked          bool `json:"prepacked,omitempty"`
	PromptBudgetTokens int  `json:"prompt_budget_tokens,omitempty"`
}

// ContextUsage is Node 2's account of what it actually put in the prompt. Older Node 2 builds
// do not send it.
type ContextUsage struct {
	SensoryReceived       int `json:"sensory_received"`
	SensoryKept           int `json:"sensory_kept"`
	SensoryDropped        int `json:"sensory_dropped"`
	SensoryTruncated      int `json:"sensory_truncated"`
	FactsReceived         int `json:"facts_received"`
	FactsKept             int `json:"facts_kept"`
	EstimatedPromptTokens int `json:"estimated_prompt_tokens"`
	ActualPromptTokens    int `json:"actual_prompt_tokens"`
	PromptWindowTokens    int `json:"prompt_window_tokens"`
}

// DeliberateResponse is returned upon step completion by Node 2.
type DeliberateResponse struct {
	Status           string            `json:"status"`
	StepIndex        int               `json:"step_index"`
	Thought          string            `json:"thought"`
	ProposedAction   string            `json:"proposed_action,omitempty"`
	IsComplete       bool              `json:"is_complete"`
	CandidateActions []CandidateAction `json:"candidate_actions,omitempty"`
	PromptTokens     int               `json:"prompt_tokens"`
	CompletionTokens int               `json:"completion_tokens"`
	TotalTokens      int               `json:"total_tokens"`
	EvaluationRate   float64           `json:"prompt_eval_rate_tps,omitempty"`
	GenerationRate   float64           `json:"generation_rate_tps,omitempty"`
	ContextUsage     *ContextUsage     `json:"context_usage,omitempty"`
	ActiveGoal       string            `json:"active_goal"`
	TrajectoryLength int               `json:"trajectory_length"`
	Timestamp        time.Time         `json:"timestamp"`
}

// WorkingHealthResponse reports Node 2 scratchpad and llama-server operational status.
type WorkingHealthResponse struct {
	Status         string    `json:"status"`
	Node           string    `json:"node"`
	Port           int       `json:"port"`
	Service        string    `json:"service"`
	LlamaInference string    `json:"llama_inference"`
	UptimeSeconds  int64     `json:"uptime_seconds"`
	Timestamp      time.Time `json:"timestamp"`
}
