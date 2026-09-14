package model

import "time"

// Node represents a vertex in the relational knowledge graph on Node 1.
type Node struct {
	ID               string     `json:"id"`
	EntityType       string     `json:"entity_type"`
	Label            string     `json:"label"`
	Summary          string     `json:"summary"`
	Embedding        []float32  `json:"embedding,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	LastAccessedAt   time.Time  `json:"last_accessed_at"`
	LastReinforcedAt time.Time  `json:"last_reinforced_at,omitempty"`
	ArchivedAt       *time.Time `json:"archived_at,omitempty"`
	AccessCount      int64      `json:"access_count"`
	StabilityScore   float64    `json:"stability_score"`
	IsArchived       bool       `json:"is_archived"`
}

// Edge represents a directed, weighted relationship between two knowledge nodes.
type Edge struct {
	SourceID         string    `json:"source_id"`
	TargetID         string    `json:"target_id"`
	RelationType     string    `json:"relation_type"`
	Weight           float64   `json:"weight"`
	CreatedAt        time.Time `json:"created_at"`
	LastReinforcedAt time.Time `json:"last_reinforced_at,omitempty"`
}

// ScoredNode wraps a Node with its associative recall score breakdown.
type ScoredNode struct {
	Node
	Score          float64 `json:"score"`
	SimScore       float64 `json:"sim_score"`
	FrequencyScore float64 `json:"frequency_score"`
	RecencyScore   float64 `json:"recency_score"`
	HopDistance    int     `json:"hop_distance"`
}

// RecallRequest defines the payload for POST /api/v1/memory/recall.
type RecallRequest struct {
	Query      string    `json:"query,omitempty"`
	Embedding  []float32 `json:"embedding,omitempty"`
	EntityID   string    `json:"entity_id,omitempty"`
	TopK       int       `json:"top_k,omitempty"`
	Alpha      float64   `json:"alpha,omitempty"`       // Weight for semantic similarity (default 0.6)
	Beta       float64   `json:"beta,omitempty"`        // Weight for access count frequency (default 0.2)
	Gamma      float64   `json:"gamma,omitempty"`       // Weight for recency decay (default 0.2)
	ExpandHops int       `json:"expand_hops,omitempty"` // Graph expansion depth: 0 or 1 (default 1)
}

// RecallResponse returns ranked contextual nodes and their relational subgraph.
type RecallResponse struct {
	Nodes          []ScoredNode `json:"nodes"`
	Edges          []Edge       `json:"edges"`
	QueryLatencyMS float64      `json:"query_latency_ms"`
}

// KnowledgeHealthResponse reports Node 1 operational status and graph sizing.
type KnowledgeHealthResponse struct {
	Status        string `json:"status"`
	Node          string `json:"node"`
	Port          int    `json:"port"`
	Service       string `json:"service"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	NodeCount     int64  `json:"node_count"`
	EdgeCount     int64  `json:"edge_count"`
}
