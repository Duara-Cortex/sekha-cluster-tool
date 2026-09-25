package client

import (
	"context"
	"fmt"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/telemetry"
)

// KnowledgeClient interacts with Node 1 (:8084) for associative recall and episodic consolidation.
type KnowledgeClient struct {
	baseURL string
	client  *BaseClient
}

// NewKnowledgeClient initialises a client for Node 1 with optional TLS.
func NewKnowledgeClient(baseURL string, timeout time.Duration, apiKey string, tlsArgs ...interface{}) *KnowledgeClient {
	return &KnowledgeClient{
		baseURL: baseURL,
		client:  NewBaseClient(timeout, apiKey, tlsArgs...),
	}
}

// Recall queries the Node 1 relational knowledge graph for relevant subgraphs.
func (c *KnowledgeClient) Recall(ctx context.Context, req model.RecallRequest, traceID string) (*model.RecallResponse, error) {
	url := fmt.Sprintf("%s/api/v1/memory/recall", c.baseURL)
	if req.IncludeEmbeddings {
		url += "?include_embeddings=true"
	}

	details := fmt.Sprintf("Query: %s, TopK: %d", req.Query, req.TopK)
	if len(req.Anchors) > 0 {
		mode := req.AnchorMode
		if mode == "" {
			mode = "boost"
		}
		details += fmt.Sprintf(", Anchors: %v, Mode: %s", req.Anchors, mode)
	}
	if req.IncludeEmbeddings {
		details += ", Embeddings: true"
	}
	telemetry.LogStep(traceID, "LongTermRecall", fmt.Sprintf("Querying Node 1 knowledge store (%s)", details))

	var resp model.RecallResponse
	if err := c.client.PostJSON(ctx, url, req, &resp, traceID); err != nil {
		return nil, fmt.Errorf("node 1 recall failed: %w", err)
	}

	telemetry.LogStep(traceID, "LongTermRecall", fmt.Sprintf("Recall complete: %d nodes, %d edges retrieved in %.2fms",
		len(resp.Nodes), len(resp.Edges), resp.QueryLatencyMS))

	return &resp, nil
}

// Consolidate commits a completed deliberation trace to Node 1 for background consolidation and decay.
func (c *KnowledgeClient) Consolidate(ctx context.Context, req model.ConsolidateRequest, traceID string) (*model.ConsolidateResponse, error) {
	url := fmt.Sprintf("%s/api/v1/memory/consolidate", c.baseURL)

	details := fmt.Sprintf("Session: %s, Sync: %t", req.SessionID, req.Synchronous)
	if len(req.Anchors) > 0 {
		details += fmt.Sprintf(", Anchors: %v", req.Anchors)
	}
	telemetry.LogStep(traceID, "Consolidation", fmt.Sprintf("Committing episodic trace to Node 1 (%s)", details))

	var resp model.ConsolidateResponse
	if err := c.client.PostJSON(ctx, url, req, &resp, traceID); err != nil {
		return nil, fmt.Errorf("node 1 consolidate failed: %w", err)
	}

	telemetry.LogStep(traceID, "Consolidation", fmt.Sprintf("Consolidation queued: Status: %s, Trace: %s, Extracted: %d, Fused: %d",
		resp.Status, resp.TraceID, resp.EntitiesExtracted, resp.NodesFused))

	return &resp, nil
}

// GetHealth checks Node 1 service status and knowledge graph scale.
func (c *KnowledgeClient) GetHealth(ctx context.Context, traceID string) (*model.KnowledgeHealthResponse, error) {
	url := fmt.Sprintf("%s/api/v1/memory/health", c.baseURL)
	var resp model.KnowledgeHealthResponse
	if err := c.client.GetJSON(ctx, url, &resp, traceID); err != nil {
		return nil, fmt.Errorf("node 1 health query failed: %w", err)
	}
	return &resp, nil
}
