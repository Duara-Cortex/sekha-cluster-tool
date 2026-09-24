package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  NewBaseClient(timeout, ""),
	}
}

// Filter dispatches raw text or ring-buffer queries to Node 3 attention classifier.
func (c *SensoryClient) Filter(ctx context.Context, req model.FilterRequest, traceID string) (*model.FilterResponse, error) {
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid sensory base URL: %w", err)
	}

	filterURL := baseURL.ResolveReference(&url.URL{Path: baseURL.Path + "/api/v1/sensory/filter"})
	q := filterURL.Query()

	if req.FromBuffer {
		q.Set("from_buffer", "true")
	}
	if req.Threshold > 0 {
		q.Set("threshold", fmt.Sprintf("%g", req.Threshold))
	}
	if req.TaskDirective != "" {
		q.Set("task", req.TaskDirective)
	}
	filterURL.RawQuery = q.Encode()
	urlStr := filterURL.String()

	telemetry.LogStep(traceID, "SensoryFilter", fmt.Sprintf("Dispatching filter request to Node 3 (%s)", urlStr))

	var bodyReader io.Reader
	contentType := ""
	if !req.FromBuffer {
		bodyReader = strings.NewReader(req.Text)
		contentType = "text/plain"
	}

	startTime := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to initialise HTTP request: %w", err)
	}

	if bodyReader != nil && contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	httpReq.Header.Set("Accept", "application/json")
	telemetry.InjectTraceID(httpReq, traceID)

	resp, err := c.client.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("node 3 filter failed: %w", err)
	}
	defer resp.Body.Close()

	durationMS := float64(time.Since(startTime).Microseconds()) / 1000.0

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from %s: %w", urlStr, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("node 3 filter failed: server returned error HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	type node3ChunkItem struct {
		ID        string    `json:"id"`
		Seq       uint64    `json:"seq"`
		Timestamp time.Time `json:"timestamp"`
		Origin    string    `json:"origin"`
		Data      string    `json:"data"`
		Text      string    `json:"text"`
		SizeBytes int64     `json:"size_bytes"`
	}

	type node3Metrics struct {
		Entropy           float64 `json:"entropy"`
		EntropyNorm       float64 `json:"entropy_norm"`
		LexicalDensity    float64 `json:"lexical_density"`
		EntityDensity     float64 `json:"entity_density"`
		TaskRelevance     float64 `json:"task_relevance"`
		RepetitionPenalty float64 `json:"repetition_penalty"`
		SalienceScore     float64 `json:"salience_score"`
		IsSalient         bool    `json:"is_salient"`
	}

	type node3SalientChunk struct {
		Chunk    node3ChunkItem `json:"chunk"`
		Metrics  node3Metrics   `json:"metrics"`
		ID       string         `json:"id"`
		Text     string         `json:"text"`
		Salience float64        `json:"salience"`
		Source   string         `json:"source"`
	}

	type intermediateFilterResponse struct {
		// Standard orchestrator fields
		Chunks         []model.SensoryChunk `json:"chunks"`
		TotalChunks    int                  `json:"total_chunks"`
		SalientChunks  json.RawMessage      `json:"salient_chunks"`
		NoiseDiscarded int                  `json:"noise_discarded"`
		ReductionRate  float64              `json:"reduction_rate"`
		LatencyMS      float64              `json:"latency_ms"`

		// Node 3 native fields
		TotalEvaluated      int     `json:"total_evaluated"`
		SalientCount        int     `json:"salient_count"`
		DiscardedCount      int     `json:"discarded_count"`
		NoiseReductionRatio float64 `json:"noise_reduction_ratio"`
		Threshold           float64 `json:"threshold"`
	}

	var inter intermediateFilterResponse
	if err := json.Unmarshal(respBytes, &inter); err != nil {
		return nil, fmt.Errorf("failed to decode JSON response from %s: %w (body: %s)", urlStr, err, string(respBytes))
	}

	finalResp := &model.FilterResponse{
		Chunks:    make([]model.SensoryChunk, 0),
		LatencyMS: durationMS,
	}
	if inter.LatencyMS > 0 {
		finalResp.LatencyMS = inter.LatencyMS
	}

	// 1. Process Chunks if already in standard orchestrator format
	if len(inter.Chunks) > 0 {
		finalResp.Chunks = inter.Chunks
	}

	// 2. Process salient_chunks if present
	if len(inter.SalientChunks) > 0 {
		trimmed := bytes.TrimSpace(inter.SalientChunks)
		if len(trimmed) > 0 {
			if trimmed[0] == '[' {
				// Node 3 format: array of chunks
				var rawChunks []node3SalientChunk
				if err := json.Unmarshal(trimmed, &rawChunks); err == nil && len(finalResp.Chunks) == 0 {
					for idx, sc := range rawChunks {
						text := sc.Text
						if text == "" {
							text = sc.Chunk.Data
						}
						if text == "" {
							text = sc.Chunk.Text
						}

						// If text is a stringified JSON (from legacy format), unwrap it
						trimmedText := strings.TrimSpace(text)
						if strings.HasPrefix(trimmedText, "{") && strings.HasSuffix(trimmedText, "}") {
							var inner struct {
								Text string `json:"text"`
								Data string `json:"data"`
							}
							if err := json.Unmarshal([]byte(trimmedText), &inner); err == nil {
								if inner.Text != "" {
									text = inner.Text
								} else if inner.Data != "" {
									text = inner.Data
								}
							}
						}

						id := sc.ID
						if id == "" {
							id = sc.Chunk.ID
						}
						if id == "" {
							seq := sc.Chunk.Seq
							if seq == 0 {
								seq = uint64(idx + 1)
							}
							id = fmt.Sprintf("chnk-%03d", seq)
						}

						source := sc.Source
						if source == "" {
							source = sc.Chunk.Origin
						}
						if source == "" {
							source = "filter_stream"
						}

						ts := sc.Chunk.Timestamp
						if ts.IsZero() {
							ts = time.Now()
						}

						score := sc.Salience
						if score == 0 {
							score = sc.Metrics.SalienceScore
						}
						if score == 0 && sc.Metrics.IsSalient {
							score = 1.0
						}

						finalResp.Chunks = append(finalResp.Chunks, model.SensoryChunk{
							ID:        id,
							Text:      text,
							Salience:  math.Round(score*100) / 100,
							Source:    source,
							Timestamp: ts,
						})
					}
				}
			} else {
				// Orchestrator format: integer count
				var scInt int
				if err := json.Unmarshal(trimmed, &scInt); err == nil {
					finalResp.SalientChunks = scInt
				}
			}
		}
	}

	if inter.TotalChunks > 0 {
		finalResp.TotalChunks = inter.TotalChunks
	} else if inter.TotalEvaluated > 0 {
		finalResp.TotalChunks = inter.TotalEvaluated
	} else {
		finalResp.TotalChunks = len(finalResp.Chunks)
	}

	if finalResp.SalientChunks == 0 {
		if inter.SalientCount > 0 || inter.TotalEvaluated > 0 {
			finalResp.SalientChunks = inter.SalientCount
		} else {
			finalResp.SalientChunks = len(finalResp.Chunks)
		}
	}

	if inter.NoiseDiscarded > 0 || inter.TotalChunks > 0 {
		finalResp.NoiseDiscarded = inter.NoiseDiscarded
	} else {
		finalResp.NoiseDiscarded = inter.DiscardedCount
	}

	if inter.ReductionRate > 0 {
		finalResp.ReductionRate = inter.ReductionRate
	} else if inter.NoiseReductionRatio > 0 {
		finalResp.ReductionRate = math.Round(inter.NoiseReductionRatio*100) / 100
	} else if finalResp.TotalChunks > 0 {
		finalResp.ReductionRate = math.Round((float64(finalResp.NoiseDiscarded)/float64(finalResp.TotalChunks))*100) / 100
	}

	telemetry.LogStep(traceID, "SensoryFilter", fmt.Sprintf("Filter complete: %d/%d chunks retained (%.1f%% noise dropped)",
		finalResp.SalientChunks, finalResp.TotalChunks, finalResp.ReductionRate*100))

	return finalResp, nil
}

// GetStats queries the Node 3 ring buffer health and throughput statistics.
func (c *SensoryClient) GetStats(ctx context.Context, traceID string) (*model.SensoryStatsResponse, error) {
	urlStr := fmt.Sprintf("%s/api/v1/sensory/stats", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialise HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	telemetry.InjectTraceID(req, traceID)

	resp, err := c.client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("node 3 stats query failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from %s: %w", urlStr, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("server returned error HTTP %d from %s: %s", resp.StatusCode, urlStr, string(respBytes))
	}

	type rawSensoryStats struct {
		Status              string                     `json:"status"`
		Node                string                     `json:"node"`
		Port                int                        `json:"port"`
		Service             string                     `json:"service"`
		BufferCapacityMB    float64                    `json:"buffer_capacity_mb"`
		BufferUsageMB       float64                    `json:"buffer_usage_mb"`
		BufferUsagePct      float64                    `json:"buffer_usage_pct"`
		IngestionRateKBS    float64                    `json:"ingestion_rate_kbs"`
		TotalIngested       int64                      `json:"total_ingested"`
		CapacityBytes       int64                      `json:"capacity_bytes"`
		UsedBytes           int64                      `json:"used_bytes"`
		FillPercent         float64                    `json:"fill_percent"`
		CurrentItemCount    int64                      `json:"current_item_count"`
		TotalIngestedCount  int64                      `json:"total_ingested_count"`
		TotalIngestedBytes  int64                      `json:"total_ingested_bytes"`
		IngestionRateKBps   float64                    `json:"ingestion_rate_kbps"`
		DroppedPackets      int64                      `json:"dropped_packets"`
		DroppedBytes        int64                      `json:"dropped_bytes"`
		UptimeSeconds       json.Number                `json:"uptime_seconds"`
		ClassifierTelemetry *model.ClassifierTelemetry `json:"classifier_telemetry"`
	}

	var raw rawSensoryStats
	decoder := json.NewDecoder(bytes.NewReader(respBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode JSON response from %s: %w (body: %s)", urlStr, err, string(respBytes))
	}

	defaultNode := "sekha-node3"
	defaultPort := 8081
	if u, err := url.Parse(c.baseURL); err == nil {
		if host := u.Hostname(); host != "" {
			defaultNode = host
		}
		if p := u.Port(); p != "" {
			if pi, err := strconv.Atoi(p); err == nil {
				defaultPort = pi
			}
		}
	}

	status := raw.Status
	if status == "" {
		status = "healthy"
	}

	node := raw.Node
	if node == "" {
		node = defaultNode
	}

	port := raw.Port
	if port == 0 {
		port = defaultPort
	}

	service := raw.Service
	if service == "" {
		service = "sekha-sensory-buffer"
	}

	capMB := raw.BufferCapacityMB
	if capMB == 0 && raw.CapacityBytes > 0 {
		capMB = math.Round((float64(raw.CapacityBytes)/(1024*1024))*100) / 100
	}

	usageMB := raw.BufferUsageMB
	if usageMB == 0 && raw.UsedBytes > 0 {
		usageMB = math.Round((float64(raw.UsedBytes)/(1024*1024))*100) / 100
	}

	usagePct := raw.BufferUsagePct
	if usagePct == 0 && raw.FillPercent > 0 {
		usagePct = math.Round(raw.FillPercent*100) / 100
	}

	rateKBS := raw.IngestionRateKBS
	if rateKBS == 0 && raw.IngestionRateKBps > 0 {
		rateKBS = math.Round(raw.IngestionRateKBps*100) / 100
	}

	totalIngested := raw.TotalIngested
	if totalIngested == 0 && raw.TotalIngestedCount > 0 {
		totalIngested = raw.TotalIngestedCount
	}

	var uptimeSec int64
	if raw.UptimeSeconds != "" {
		if uf, err := raw.UptimeSeconds.Float64(); err == nil {
			uptimeSec = int64(math.Floor(uf))
		}
	}

	fillPercent := raw.FillPercent
	if fillPercent > 0 {
		fillPercent = math.Round(fillPercent*100) / 100
	} else {
		fillPercent = usagePct
	}

	itemCount := raw.CurrentItemCount
	if itemCount == 0 {
		itemCount = totalIngested
	}

	statsResp := &model.SensoryStatsResponse{
		Status:              status,
		Node:                node,
		Port:                port,
		Service:             service,
		BufferCapacityMB:    capMB,
		BufferUsageMB:       usageMB,
		BufferUsagePct:      usagePct,
		IngestionRateKBS:    rateKBS,
		TotalIngested:       totalIngested,
		DroppedPackets:      raw.DroppedPackets,
		UptimeSeconds:       uptimeSec,
		FillPercent:         fillPercent,
		CurrentItemCount:    itemCount,
		ClassifierTelemetry: raw.ClassifierTelemetry,
	}

	return statsResp, nil
}

