package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Node represents a vertex in the relational knowledge graph on Node 1.
type Node struct {
	ID               string     `json:"id,omitempty"`
	EntityType       string     `json:"entity_type,omitempty"`
	Label            string     `json:"label,omitempty"`
	Summary          string     `json:"summary,omitempty"`
	Embedding        []float32  `json:"embedding,omitempty"`
	Anchors          []string   `json:"anchors,omitempty"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`
	LastAccessedAt   *time.Time `json:"last_accessed_at,omitempty"`
	LastReinforcedAt *time.Time `json:"last_reinforced_at,omitempty"`
	ArchivedAt       *time.Time `json:"archived_at,omitempty"`
	AccessCount      int64      `json:"access_count,omitempty"`
	StabilityScore   float64    `json:"stability_score,omitempty"`
	IsArchived       bool       `json:"is_archived,omitempty"`
}

// NormalizeAnchor ensures anchor tags are lowercase, trimmed, and prefixed with '#'
// (e.g. "project:kestrel" -> "#project:kestrel", " #PROJECT:KESTREL " -> "#project:kestrel").
func NormalizeAnchor(anchor string) string {
	a := strings.ToLower(strings.TrimSpace(anchor))
	if a == "" || a == "#" {
		return ""
	}
	if !strings.HasPrefix(a, "#") {
		a = "#" + a
	}
	return a
}

// ParseAnchors parses repeatable and/or comma-delimited anchor tags into a normalized, deduplicated slice.
func ParseAnchors(raw ...string) []string {
	var result []string
	seen := make(map[string]bool)
	for _, item := range raw {
		for _, part := range strings.Split(item, ",") {
			norm := NormalizeAnchor(part)
			if norm != "" && !seen[norm] {
				seen[norm] = true
				result = append(result, norm)
			}
		}
	}
	return result
}

// Edge represents a directed, weighted relationship between two knowledge nodes.
type Edge struct {
	SourceID         string     `json:"source_id"`
	TargetID         string     `json:"target_id"`
	RelationType     string     `json:"relation_type"`
	Weight           float64    `json:"weight"`
	CreatedAt        time.Time  `json:"created_at"`
	LastReinforcedAt *time.Time `json:"last_reinforced_at,omitempty"`
}

// ScoredNode wraps a Node with its associative recall score breakdown.
type ScoredNode struct {
	Node
	Score          float64 `json:"score"`
	SimScore       float64 `json:"sim_score,omitempty"`
	FrequencyScore float64 `json:"frequency_score,omitempty"`
	RecencyScore   float64 `json:"recency_score,omitempty"`
	AnchorScore    float64 `json:"anchor_score,omitempty"`
	HopDistance    int     `json:"hop_distance,omitempty"`
}

// RecallRequest defines the payload for POST /api/v1/memory/recall.
type RecallRequest struct {
	Query             string    `json:"query,omitempty"`
	Embedding         []float32 `json:"embedding,omitempty"`
	EntityID          string    `json:"entity_id,omitempty"`
	TopK              int       `json:"top_k,omitempty"`
	Alpha             float64   `json:"alpha,omitempty"`         // Weight for semantic similarity (default 0.6)
	Beta              float64   `json:"beta,omitempty"`          // Weight for access count frequency (default 0.2)
	Gamma             float64   `json:"gamma,omitempty"`         // Weight for recency decay (default 0.2)
	ExpandHops        int       `json:"expand_hops,omitempty"`   // Graph expansion depth: 0 or 1 (default 1)
	Anchors           []string  `json:"anchors,omitempty"`       // Target anchor tags (e.g. ["#project:kestrel"])
	AnchorMode        string    `json:"anchor_mode,omitempty"`   // "boost" | "filter" (default: "boost")
	AnchorWeight      float64   `json:"anchor_weight,omitempty"` // Weight for anchor bonus w_anc (default 1.0)
	IncludeEmbeddings bool      `json:"include_embeddings,omitempty"`
	Fields            []string  `json:"fields,omitempty"`
}

// RecallResponse returns ranked contextual nodes and their relational subgraph.
type RecallResponse struct {
	Nodes          []ScoredNode     `json:"nodes"`
	Edges          []Edge           `json:"edges"`
	QueryLatencyMS float64          `json:"query_latency_ms"`
	ProjectedNodes []map[string]any `json:"-"`
}

// UnmarshalJSON customises JSON deserialisation for RecallResponse to support both lean schemas and field projections.
func (r *RecallResponse) UnmarshalJSON(data []byte) error {
	type Alias RecallResponse
	aux := struct {
		Nodes []json.RawMessage `json:"nodes"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	r.Nodes = make([]ScoredNode, 0, len(aux.Nodes))
	r.ProjectedNodes = make([]map[string]any, 0, len(aux.Nodes))
	for _, rawNode := range aux.Nodes {
		var sn ScoredNode
		if err := json.Unmarshal(rawNode, &sn); err == nil {
			r.Nodes = append(r.Nodes, sn)
		}
		var m map[string]any
		if err := json.Unmarshal(rawNode, &m); err == nil {
			r.ProjectedNodes = append(r.ProjectedNodes, m)
		}
	}
	return nil
}

// MarshalJSON customises JSON serialisation when field projection is active or lean schema is preserved.
func (r RecallResponse) MarshalJSON() ([]byte, error) {
	edges := r.Edges
	if edges == nil {
		edges = []Edge{}
	}
	if len(r.ProjectedNodes) > 0 && len(r.ProjectedNodes) == len(r.Nodes) {
		return json.Marshal(&struct {
			Nodes          []map[string]any `json:"nodes"`
			Edges          []Edge           `json:"edges"`
			QueryLatencyMS float64          `json:"query_latency_ms"`
		}{
			Nodes:          r.ProjectedNodes,
			Edges:          edges,
			QueryLatencyMS: r.QueryLatencyMS,
		})
	}
	nodes := r.Nodes
	if nodes == nil {
		nodes = []ScoredNode{}
	}
	return json.Marshal(&struct {
		Nodes          []ScoredNode `json:"nodes"`
		Edges          []Edge       `json:"edges"`
		QueryLatencyMS float64      `json:"query_latency_ms"`
	}{
		Nodes:          nodes,
		Edges:          edges,
		QueryLatencyMS: r.QueryLatencyMS,
	})
}

// FormatMarkdown renders a clean, token-efficient Markdown view of the recall results.
func (r *RecallResponse) FormatMarkdown() string {
	if r == nil || len(r.Nodes) == 0 {
		return "No associative nodes matched the query."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Recall Results (%d nodes, %.2fms)\n", len(r.Nodes), r.QueryLatencyMS))

	for _, node := range r.Nodes {
		sb.WriteString(fmt.Sprintf("- [%s] **%s** (`%s`, score: %.2f, sim: %.2f)\n",
			node.ID, node.Label, node.EntityType, node.Score, node.SimScore))
		sb.WriteString(fmt.Sprintf("  Summary: %s\n", node.Summary))
		if len(node.Anchors) > 0 {
			sb.WriteString(fmt.Sprintf("  Anchors: %s\n", strings.Join(node.Anchors, ", ")))
		} else {
			sb.WriteString("  Anchors: none\n")
		}
	}

	if len(r.Edges) > 0 {
		sb.WriteString(fmt.Sprintf("\n## Relational Subgraph (%d edges)\n", len(r.Edges)))
		for _, edge := range r.Edges {
			sb.WriteString(fmt.Sprintf("- `%s` --(%s, weight: %.2f)--> `%s`\n",
				edge.SourceID, edge.RelationType, edge.Weight, edge.TargetID))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// FormatConcise is an alias of FormatMarkdown for concise output formatting.
func (r *RecallResponse) FormatConcise() string {
	return r.FormatMarkdown()
}

// RenderMarkdown is an alias of FormatMarkdown.
func (r *RecallResponse) RenderMarkdown() string {
	return r.FormatMarkdown()
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
