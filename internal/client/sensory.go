package client

import (
	"context"
	"fmt"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/telemetry"
)

// SensoryClient interacts with Node 3 (:8081) for text buffering and salience gating.
type SensoryClient struct {
	baseURL string
	client  *BaseClient
}

// NewSensoryClient initialises a client for Node 3.
func NewSensoryClient(baseURL string, timeout time.Duration) *SensoryClient {
	return &SensoryClient{
		baseURL: baseURL,
		client:  NewBaseClient(timeout),
	}
}

// Filter dispatches raw text or ring-buffer queries to Node 3 attention classifier.
func (c *SensoryClient) Filter(ctx context.Context, req model.FilterRequest, traceID string) (*model.FilterResponse, error) {
	url := fmt.Sprintf("%s/api/v1/sensory/filter", c.baseURL)
	if req.FromBuffer {
		url += "?from_buffer=true"
	}

	telemetry.LogStep(traceID, "SensoryFilter", fmt.Sprintf("Dispatching filter request to Node 3 (%s)", url))

	var resp model.FilterResponse
	if err := c.client.PostJSON(ctx, url, req, &resp, traceID); err != nil {
		return nil, fmt.Errorf("node 3 filter failed: %w", err)
	}

	telemetry.LogStep(traceID, "SensoryFilter", fmt.Sprintf("Filter complete: %d/%d chunks retained (%.1f%% noise dropped)",
		resp.SalientChunks, resp.TotalChunks, resp.ReductionRate*100))

	return &resp, nil
}

// GetStats queries the Node 3 ring buffer health and throughput statistics.
func (c *SensoryClient) GetStats(ctx context.Context, traceID string) (*model.SensoryStatsResponse, error) {
	url := fmt.Sprintf("%s/api/v1/sensory/stats", c.baseURL)
	var resp model.SensoryStatsResponse
	if err := c.client.GetJSON(ctx, url, &resp, traceID); err != nil {
		return nil, fmt.Errorf("node 3 stats query failed: %w", err)
	}
	return &resp, nil
}
