package client

import (
	"context"
	"fmt"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/telemetry"
)

// WorkingClient interacts with Node 2 (:8083) for working memory scratchpad deliberation.
type WorkingClient struct {
	baseURL string
	client  *BaseClient
}

// NewWorkingClient initialises a client for Node 2 with inference-aware timeout handling and optional TLS.
func NewWorkingClient(baseURL string, timeout time.Duration, tlsArgs ...interface{}) *WorkingClient {
	return &WorkingClient{
		baseURL: baseURL,
		client:  NewBaseClient(timeout, "", tlsArgs...),
	}
}

// Deliberate dispatches active context to Node 2 for stateless reasoning and action synthesis.
func (c *WorkingClient) Deliberate(ctx context.Context, req model.DeliberateRequest, traceID string) (*model.DeliberateResponse, error) {
	url := fmt.Sprintf("%s/api/v1/working/deliberate", c.baseURL)

	telemetry.LogStep(traceID, "WorkingMemory", fmt.Sprintf("Dispatching deliberation to Node 2 (Goal: %s)", req.Objective))

	var resp model.DeliberateResponse
	if err := c.client.PostJSON(ctx, url, req, &resp, traceID); err != nil {
		return nil, fmt.Errorf("node 2 deliberate failed: %w", err)
	}

	telemetry.LogStep(traceID, "WorkingMemory", fmt.Sprintf("Deliberation complete: Step %d (Action: %s, Complete: %t)",
		resp.StepIndex, resp.ProposedAction, resp.IsComplete))

	return &resp, nil
}

// GetHealth checks Node 2 service status and llama-server reachability.
func (c *WorkingClient) GetHealth(ctx context.Context, traceID string) (*model.WorkingHealthResponse, error) {
	url := fmt.Sprintf("%s/api/v1/working/health", c.baseURL)
	var resp model.WorkingHealthResponse
	if err := c.client.GetJSON(ctx, url, &resp, traceID); err != nil {
		return nil, fmt.Errorf("node 2 health query failed: %w", err)
	}
	return &resp, nil
}

