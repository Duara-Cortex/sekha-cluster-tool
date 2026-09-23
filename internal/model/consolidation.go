package model

import "time"

// SensoryItem captures a salient sensory chunk preserved in episodic context.
type SensoryItem struct {
	ID        string    `json:"id,omitempty"`
	Text      string    `json:"text"`
	Salience  float64   `json:"salience"`
	Source    string    `json:"source,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// TrajectoryStep captures a deliberate reasoning step within an episodic trace.
type TrajectoryStep struct {
	StepIndex   int       `json:"step_index"`
	Thought     string    `json:"thought"`
	Action      string    `json:"action,omitempty"`
	Observation string    `json:"observation,omitempty"`
	Status      string    `json:"status"`
	Timestamp   time.Time `json:"timestamp"`
}

// ConsolidateRequest defines the payload for POST /api/v1/memory/consolidate.
type ConsolidateRequest struct {
	TraceID          string            `json:"trace_id,omitempty"`
	SessionID        string            `json:"session_id"`
	TaskGoal         string            `json:"task_goal,omitempty"`
	ActiveGoal       string            `json:"active_goal,omitempty"`
	Outcome          string            `json:"outcome,omitempty"`
	Status           string            `json:"status,omitempty"`
	SensoryContext   []SensoryItem     `json:"sensory_context,omitempty"`
	Trajectory       []TrajectoryStep  `json:"trajectory,omitempty"`
	CandidateActions []CandidateAction `json:"candidate_actions,omitempty"`
	Anchors          []string          `json:"anchors,omitempty"`
	Synchronous      bool              `json:"synchronous,omitempty"`
}

// ConsolidateResponse returns ingestion receipts and consolidation statistics from Node 1.
type ConsolidateResponse struct {
	Status            string `json:"status"`
	TraceID           string `json:"trace_id"`
	Message           string `json:"message"`
	Synchronous       bool   `json:"synchronous"`
	EntitiesExtracted int    `json:"entities_extracted,omitempty"`
	NodesFused        int    `json:"nodes_fused,omitempty"`
	EdgesReinforced   int    `json:"edges_reinforced,omitempty"`
}
