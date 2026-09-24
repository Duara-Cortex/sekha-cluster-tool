package model

import "time"

// SensoryChunk represents a discrete, scored text payload evaluated by the Node 3 attention gate.
type SensoryChunk struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Salience  float64   `json:"salience"`
	Source    string    `json:"source,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// FilterRequest defines the payload dispatched to Node 3 (POST /api/v1/sensory/filter).
type FilterRequest struct {
	Text          string  `json:"text,omitempty"`
	TaskDirective string  `json:"task_directive,omitempty"`
	Threshold     float64 `json:"threshold,omitempty"`
	FromBuffer    bool    `json:"from_buffer,omitempty"`
}

// FilterResponse encapsulates filtered salient chunks and stream reduction metrics from Node 3.
type FilterResponse struct {
	Chunks         []SensoryChunk `json:"chunks"`
	TotalChunks    int            `json:"total_chunks"`
	SalientChunks  int            `json:"salient_chunks"`
	NoiseDiscarded int            `json:"noise_discarded"`
	ReductionRate  float64        `json:"reduction_rate"`
	LatencyMS      float64        `json:"latency_ms"`
}

// ClassifierTelemetry tracks cumulative filtering activity and noise reduction metrics.
type ClassifierTelemetry struct {
	TotalEvaluated      uint64  `json:"total_evaluated"`
	TotalSalient        uint64  `json:"total_salient"`
	TotalDiscarded      uint64  `json:"total_discarded"`
	NoiseReductionRatio float64 `json:"noise_reduction_ratio"`
}

// SensoryStatsResponse provides operational telemetry from the Node 3 ring buffer daemon.
type SensoryStatsResponse struct {
	Status              string               `json:"status"`
	Node                string               `json:"node"`
	Port                int                  `json:"port"`
	Service             string               `json:"service"`
	BufferCapacityMB    float64              `json:"buffer_capacity_mb"`
	BufferUsageMB       float64              `json:"buffer_usage_mb"`
	BufferUsagePct      float64              `json:"buffer_usage_pct"`
	IngestionRateKBS    float64              `json:"ingestion_rate_kbs"`
	TotalIngested       int64                `json:"total_ingested"`
	DroppedPackets      int64                `json:"dropped_packets"`
	UptimeSeconds       int64                `json:"uptime_seconds"`
	FillPercent         float64              `json:"fill_percent,omitempty"`
	CurrentItemCount    int64                `json:"current_item_count,omitempty"`
	CapacityBytes       int64                `json:"capacity_bytes,omitempty"`
	UsedBytes           int64                `json:"used_bytes,omitempty"`
	TotalIngestedCount  int64                `json:"total_ingested_count,omitempty"`
	ClassifierTelemetry *ClassifierTelemetry `json:"classifier_telemetry,omitempty"`
}

