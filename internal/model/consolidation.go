package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

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
	IsSecret         bool              `json:"is_secret,omitempty"`
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

// BuildPositionalConsolidation constructs a ConsolidateRequest from positional shortcut arguments.
func BuildPositionalConsolidation(label, summary, sessionID, goal, outcome string, anchors []string, sync bool, traceID string) ConsolidateRequest {
	now := time.Now().UTC()
	trimmedLabel := strings.TrimSpace(label)
	trimmedSummary := strings.TrimSpace(summary)

	text := fmt.Sprintf("%s config: %s", trimmedLabel, trimmedSummary)
	if strings.HasPrefix(trimmedSummary, trimmedLabel) || strings.HasPrefix(strings.ToLower(trimmedSummary), strings.ToLower(trimmedLabel)) {
		text = trimmedSummary
	}

	if goal == "" {
		goal = "Store " + trimmedLabel + " configuration"
	}
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess-memorise-%d", now.Unix())
	}
	if outcome == "" {
		outcome = "success"
	}

	return ConsolidateRequest{
		TraceID:     traceID,
		SessionID:   sessionID,
		TaskGoal:    goal,
		Outcome:     outcome,
		Status:      "completed",
		SensoryContext: []SensoryItem{
			{
				ID:        "fact-01",
				Text:      text,
				Salience:  1.0,
				Source:    "user",
				Timestamp: now,
			},
		},
		Trajectory: []TrajectoryStep{
			{
				StepIndex: 0,
				Thought:   fmt.Sprintf("Committed %s configuration to long-term memory", trimmedLabel),
				Status:    "completed",
				Timestamp: now,
			},
		},
		Anchors:     ParseAnchors(anchors...),
		Synchronous: sync,
	}
}

// SecretPattern defines a compiled regular expression for identifying raw credentials.
type SecretPattern struct {
	Name    string
	Pattern *regexp.Regexp
}

// DefaultSecretPatterns provides heuristic regex patterns to identify private keys and high-entropy API tokens.
var DefaultSecretPatterns = []SecretPattern{
	{
		Name:    "Private Key",
		Pattern: regexp.MustCompile(`-----BEGIN (?:[A-Za-z0-9_-]+ )?PRIVATE KEY-----`),
	},
	{
		Name:    "OpenAI API Key",
		Pattern: regexp.MustCompile(`\bsk-(?:proj-)?[a-zA-Z0-9_-]{20,}\b`),
	},
	{
		Name:    "GitHub Token",
		Pattern: regexp.MustCompile(`\bgh[pousr]_[a-zA-Z0-9]{36,}\b`),
	},
	{
		Name:    "AWS Access Key",
		Pattern: regexp.MustCompile(`\b(A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}\b`),
	},
	{
		Name:    "Bearer Token",
		Pattern: regexp.MustCompile(`(?i)\bBearer\s+[a-zA-Z0-9_\-\.]{20,}\b`),
	},
	{
		Name:    "Slack Token",
		Pattern: regexp.MustCompile(`\bxox[baprs]-[0-9a-zA-Z]{10,}\b`),
	},
	{
		Name:    "API Key / Secret Token",
		Pattern: regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret[_-]?key|access[_-]?token)\s*[:=]\s*['"]?[a-zA-Z0-9_\-]{20,}`),
	},
}

// DetectRawSecrets scans text for raw credentials (private keys, API keys, tokens),
// while ignoring external references like vault:// or env://.
func DetectRawSecrets(content string) (bool, string) {
	if content == "" {
		return false, ""
	}

	for _, sp := range DefaultSecretPatterns {
		locs := sp.Pattern.FindAllStringIndex(content, -1)
		for _, loc := range locs {
			start := loc[0]
			// Check if immediately preceded by external reference schemas: vault:// or env://
			prefix := ""
			if start >= 8 {
				prefix = content[start-8 : start]
			} else {
				prefix = content[:start]
			}
			if strings.HasSuffix(prefix, "vault://") || strings.HasSuffix(prefix, "env://") {
				continue
			}
			return true, sp.Name
		}
	}
	return false, ""
}

// DetectRawSecretsInStrings scans multiple strings for raw credentials.
func DetectRawSecretsInStrings(texts ...string) (bool, string) {
	for _, text := range texts {
		if detected, secretType := DetectRawSecrets(text); detected {
			return true, secretType
		}
	}
	return false, ""
}

// DetectSecrets scans all fields of a ConsolidateRequest for raw credentials.
func (r *ConsolidateRequest) DetectSecrets() (bool, string) {
	if detected, typ := DetectRawSecretsInStrings(r.TaskGoal, r.ActiveGoal, r.Outcome, r.Status); detected {
		return true, typ
	}
	for _, s := range r.SensoryContext {
		if detected, typ := DetectRawSecrets(s.Text); detected {
			return true, typ
		}
	}
	for _, step := range r.Trajectory {
		if detected, typ := DetectRawSecretsInStrings(step.Thought, step.Action, step.Observation); detected {
			return true, typ
		}
	}
	for _, a := range r.CandidateActions {
		if detected, typ := DetectRawSecrets(a.Type); detected {
			return true, typ
		}
		if len(a.Payload) > 0 {
			if b, err := json.Marshal(a.Payload); err == nil {
				if detected, typ := DetectRawSecrets(string(b)); detected {
					return true, typ
				}
			}
		}
	}
	return false, ""
}

