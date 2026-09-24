package client

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
)

// NodeProbeResult captures the health probe result of a single cluster layer.
type NodeProbeResult struct {
	Name     string      `json:"name"`
	Role     string      `json:"role"`
	Key      string      `json:"key"` // "sensory", "working", "knowledge"
	URL      string      `json:"url"`
	Status   string      `json:"status"` // "healthy", "unreachable", "unconfigured"
	Duration float64     `json:"ping_ms"`
	Details  interface{} `json:"details,omitempty"`
	Error    string      `json:"error,omitempty"`
}

// ClusterProbeResult aggregates concurrent probe results across all three layers.
type ClusterProbeResult struct {
	ClusterStatus string            `json:"cluster_status"` // "all_nodes_healthy" or "degraded_or_partially_offline"
	AllHealthy    bool              `json:"all_healthy"`
	OnlineCount   int               `json:"online_count"`
	TotalCount    int               `json:"total_count"`
	EnvFileLoaded string            `json:"env_file_loaded"`
	TraceID       string            `json:"trace_id"`
	Timestamp     string            `json:"timestamp"`
	Nodes         []NodeProbeResult `json:"nodes"`
}

func (p ClusterProbeResult) findNode(key string) *NodeProbeResult {
	for i := range p.Nodes {
		if p.Nodes[i].Key == key {
			return &p.Nodes[i]
		}
	}
	return nil
}

// ProbeClusterHealth concurrently probes Node 3, Node 2, and Node 1 with bounded timeouts.
func ProbeClusterHealth(ctx context.Context, cfg Config, timeout time.Duration, traceID string) ClusterProbeResult {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}

	results := make([]NodeProbeResult, 3)
	var wg sync.WaitGroup
	wg.Add(3)

	// 1. Probe Sensory Layer (Node 3)
	go func() {
		defer wg.Done()
		if cfg.SensoryURL == "" {
			results[0] = NodeProbeResult{
				Name:   "Sensory Layer",
				Role:   "Sensory Buffer & Attention Filter",
				Key:    "sensory",
				URL:    "(not configured)",
				Status: "unconfigured",
				Error:  "Endpoint address not set in .env, environment, flags, or build",
			}
			return
		}
		sClient := NewSensoryClient(cfg.SensoryURL, timeout)
		t0 := time.Now()
		cCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		sStats, err := sClient.GetStats(cCtx, traceID)
		d := float64(time.Since(t0).Microseconds()) / 1000.0
		if err != nil {
			results[0] = NodeProbeResult{
				Name:     "Sensory Layer",
				Role:     "Sensory Buffer & Attention Filter",
				Key:      "sensory",
				URL:      cfg.SensoryURL,
				Status:   "unreachable",
				Duration: d,
				Error:    err.Error(),
			}
		} else {
			results[0] = NodeProbeResult{
				Name:     "Sensory Layer",
				Role:     "Sensory Buffer & Attention Filter",
				Key:      "sensory",
				URL:      cfg.SensoryURL,
				Status:   "healthy",
				Duration: d,
				Details:  sStats,
			}
		}
	}()

	// 2. Probe Working Memory Layer (Node 2)
	go func() {
		defer wg.Done()
		if cfg.WorkingURL == "" {
			results[1] = NodeProbeResult{
				Name:   "Working Memory Layer",
				Role:   "Working Memory & Inference Engine",
				Key:    "working",
				URL:    "(not configured)",
				Status: "unconfigured",
				Error:  "Endpoint address not set in .env, environment, flags, or build",
			}
			return
		}
		wClient := NewWorkingClient(cfg.WorkingURL, timeout)
		t0 := time.Now()
		cCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		wHealth, err := wClient.GetHealth(cCtx, traceID)
		d := float64(time.Since(t0).Microseconds()) / 1000.0
		if err != nil {
			results[1] = NodeProbeResult{
				Name:     "Working Memory Layer",
				Role:     "Working Memory & Inference Engine",
				Key:      "working",
				URL:      cfg.WorkingURL,
				Status:   "unreachable",
				Duration: d,
				Error:    err.Error(),
			}
		} else {
			results[1] = NodeProbeResult{
				Name:     "Working Memory Layer",
				Role:     "Working Memory & Inference Engine",
				Key:      "working",
				URL:      cfg.WorkingURL,
				Status:   "healthy",
				Duration: d,
				Details:  wHealth,
			}
		}
	}()

	// 3. Probe Knowledge Layer (Node 1)
	go func() {
		defer wg.Done()
		if cfg.KnowledgeURL == "" {
			results[2] = NodeProbeResult{
				Name:   "Knowledge Layer",
				Role:   "Knowledge Graph & Consolidation Store",
				Key:    "knowledge",
				URL:    "(not configured)",
				Status: "unconfigured",
				Error:  "Endpoint address not set in .env, environment, flags, or build",
			}
			return
		}
		kClient := NewKnowledgeClient(cfg.KnowledgeURL, timeout, cfg.APIKey)
		t0 := time.Now()
		cCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		kHealth, err := kClient.GetHealth(cCtx, traceID)
		d := float64(time.Since(t0).Microseconds()) / 1000.0
		if err != nil {
			results[2] = NodeProbeResult{
				Name:     "Knowledge Layer",
				Role:     "Knowledge Graph & Consolidation Store",
				Key:      "knowledge",
				URL:      cfg.KnowledgeURL,
				Status:   "unreachable",
				Duration: d,
				Error:    err.Error(),
			}
		} else {
			results[2] = NodeProbeResult{
				Name:     "Knowledge Layer",
				Role:     "Knowledge Graph & Consolidation Store",
				Key:      "knowledge",
				URL:      cfg.KnowledgeURL,
				Status:   "healthy",
				Duration: d,
				Details:  kHealth,
			}
		}
	}()

	wg.Wait()

	allHealthy := true
	onlineCount := 0
	for _, r := range results {
		if r.Status == "healthy" {
			onlineCount++
		} else {
			allHealthy = false
		}
	}

	clusterStatus := "all_nodes_healthy"
	if !allHealthy {
		clusterStatus = "degraded_or_partially_offline"
	}

	return ClusterProbeResult{
		ClusterStatus: clusterStatus,
		AllHealthy:    allHealthy,
		OnlineCount:   onlineCount,
		TotalCount:    len(results),
		EnvFileLoaded: cfg.EnvFileLoaded,
		TraceID:       traceID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Nodes:         results,
	}
}

// FormatDashboard renders the human-readable ASCII terminal status dashboard.
func (p ClusterProbeResult) FormatDashboard() string {
	var sb strings.Builder
	sb.WriteString("======================================================================\n")
	sb.WriteString("              Sekha Tri-Node Edge Cognitive Cluster Status            \n")
	sb.WriteString("======================================================================\n")

	stateText := "ALL NODES HEALTHY"
	if !p.AllHealthy {
		if p.OnlineCount > 0 {
			stateText = "DEGRADED"
		} else {
			stateText = "OFFLINE"
		}
	}
	sb.WriteString(fmt.Sprintf("Cluster State: %s (%d/%d Online) | Trace ID: %s\n", stateText, p.OnlineCount, p.TotalCount, p.TraceID))

	configSource := p.EnvFileLoaded
	if configSource == "" {
		configSource = "(environment variables / defaults)"
	}
	sb.WriteString(fmt.Sprintf("Config Source: %s\n", configSource))

	// Node 3: Sensory Layer
	sb.WriteString("\n")
	p.renderSensoryNode(&sb)

	// Node 2: Working Memory Layer
	sb.WriteString("\n")
	p.renderWorkingNode(&sb)

	// Node 1: Knowledge Layer
	sb.WriteString("\n")
	p.renderKnowledgeNode(&sb)

	sb.WriteString("======================================================================\n")
	return sb.String()
}

func (p ClusterProbeResult) renderSensoryNode(sb *strings.Builder) {
	node := p.findNode("sensory")
	if node == nil {
		return
	}
	switch node.Status {
	case "healthy":
		sb.WriteString(fmt.Sprintf("[●] Node 3: Sensory Layer (%s)\n", node.URL))
		sb.WriteString(fmt.Sprintf("    • Status:       HEALTHY (%.1f ms)\n", node.Duration))
		if sStats, ok := node.Details.(*model.SensoryStatsResponse); ok && sStats != nil {
			usedMB := sStats.BufferUsageMB
			if usedMB <= 0 && sStats.UsedBytes > 0 {
				usedMB = float64(sStats.UsedBytes) / (1024 * 1024)
			}
			capMB := sStats.BufferCapacityMB
			if capMB <= 0 && sStats.CapacityBytes > 0 {
				capMB = float64(sStats.CapacityBytes) / (1024 * 1024)
			}
			if capMB <= 0 {
				capMB = 64.0
			}
			fill := sStats.FillPercent
			if fill <= 0 {
				fill = sStats.BufferUsagePct
			}
			sb.WriteString(fmt.Sprintf("    • Buffer Usage: %.1f MB / %.1f MB (%.1f%% fill)\n", usedMB, capMB, fill))

			ingested := sStats.TotalIngested
			if ingested == 0 && sStats.TotalIngestedCount > 0 {
				ingested = sStats.TotalIngestedCount
			}
			sb.WriteString(fmt.Sprintf("    • Throughput:   %s ingested | %s dropped\n", formatNumber(ingested), formatNumber(sStats.DroppedPackets)))

			if sStats.ClassifierTelemetry != nil {
				ratioPct := sStats.ClassifierTelemetry.NoiseReductionRatio * 100
				sb.WriteString(fmt.Sprintf("    • Gating:       %s salient / %s evaluated (%.1f%% noise reduced)\n",
					formatNumber(int64(sStats.ClassifierTelemetry.TotalSalient)),
					formatNumber(int64(sStats.ClassifierTelemetry.TotalEvaluated)),
					ratioPct))
			} else {
				sb.WriteString("    • Gating:       N/A\n")
			}
		} else {
			sb.WriteString("    • Buffer Usage: N/A\n")
			sb.WriteString("    • Throughput:   N/A\n")
			sb.WriteString("    • Gating:       N/A\n")
		}
	case "unreachable":
		sb.WriteString(fmt.Sprintf("[✗] Node 3: Sensory Layer (%s)\n", node.URL))
		sb.WriteString(fmt.Sprintf("    • Status:       OFFLINE (%.1f ms)\n", node.Duration))
		sb.WriteString(fmt.Sprintf("    • Error:        %s\n", node.Error))
	case "unconfigured":
		sb.WriteString("[!] Node 3: Sensory Layer ((not configured))\n")
		sb.WriteString("    • Status:       UNCONFIGURED\n")
		sb.WriteString(fmt.Sprintf("    • Error:        %s\n", node.Error))
	}
}

func (p ClusterProbeResult) renderWorkingNode(sb *strings.Builder) {
	node := p.findNode("working")
	if node == nil {
		return
	}
	switch node.Status {
	case "healthy":
		sb.WriteString(fmt.Sprintf("[●] Node 2: Working Memory Layer (%s)\n", node.URL))
		sb.WriteString(fmt.Sprintf("    • Status:       HEALTHY (%.1f ms)\n", node.Duration))
		if wHealth, ok := node.Details.(*model.WorkingHealthResponse); ok && wHealth != nil {
			slmEngine := "REACHABLE (llama-server :8082)"
			if wHealth.LlamaInference != "" && !strings.EqualFold(wHealth.LlamaInference, "reachable") {
				slmEngine = fmt.Sprintf("%s (llama-server :8082)", strings.ToUpper(wHealth.LlamaInference))
			}
			sb.WriteString(fmt.Sprintf("    • SLM Engine:   %s\n", slmEngine))

			svc := wHealth.Service
			if svc == "" {
				svc = "sekha-working-scratchpad"
			}
			sb.WriteString(fmt.Sprintf("    • Service:      %s (uptime: %s)\n", svc, formatUptime(wHealth.UptimeSeconds)))
		} else {
			sb.WriteString("    • SLM Engine:   N/A\n")
			sb.WriteString("    • Service:      N/A\n")
		}
	case "unreachable":
		sb.WriteString(fmt.Sprintf("[✗] Node 2: Working Memory Layer (%s)\n", node.URL))
		sb.WriteString(fmt.Sprintf("    • Status:       OFFLINE (%.1f ms)\n", node.Duration))
		sb.WriteString(fmt.Sprintf("    • Error:        %s\n", node.Error))
	case "unconfigured":
		sb.WriteString("[!] Node 2: Working Memory Layer ((not configured))\n")
		sb.WriteString("    • Status:       UNCONFIGURED\n")
		sb.WriteString(fmt.Sprintf("    • Error:        %s\n", node.Error))
	}
}

func (p ClusterProbeResult) renderKnowledgeNode(sb *strings.Builder) {
	node := p.findNode("knowledge")
	if node == nil {
		return
	}
	switch node.Status {
	case "healthy":
		sb.WriteString(fmt.Sprintf("[●] Node 1: Long-Term Knowledge Layer (%s)\n", node.URL))
		sb.WriteString(fmt.Sprintf("    • Status:       HEALTHY (%.1f ms)\n", node.Duration))
		if kHealth, ok := node.Details.(*model.KnowledgeHealthResponse); ok && kHealth != nil {
			sb.WriteString(fmt.Sprintf("    • Graph Scale:  %s nodes | %s relational edges\n", formatNumber(kHealth.NodeCount), formatNumber(kHealth.EdgeCount)))

			embedder := "REACHABLE (384-D :8086)"
			if kHealth.EmbeddingEngine != nil {
				port := ":8086"
				if u, err := url.Parse(kHealth.EmbeddingEngine.URL); err == nil && u.Port() != "" {
					port = ":" + u.Port()
				}
				dim := 384
				if kHealth.EmbeddingEngine.Dimension > 0 {
					dim = kHealth.EmbeddingEngine.Dimension
				}
				embedder = fmt.Sprintf("%s (%d-D %s)", strings.ToUpper(kHealth.EmbeddingEngine.Status), dim, port)
			}
			sb.WriteString(fmt.Sprintf("    • Embedder:     %s\n", embedder))

			svc := kHealth.Service
			if svc == "" {
				svc = "sekha-knowledge-store"
			}
			sb.WriteString(fmt.Sprintf("    • Service:      %s (uptime: %s)\n", svc, formatUptime(kHealth.UptimeSeconds)))
		} else {
			sb.WriteString("    • Graph Scale:  N/A\n")
			sb.WriteString("    • Embedder:     N/A\n")
			sb.WriteString("    • Service:      N/A\n")
		}
	case "unreachable":
		sb.WriteString(fmt.Sprintf("[✗] Node 1: Long-Term Knowledge Layer (%s)\n", node.URL))
		sb.WriteString(fmt.Sprintf("    • Status:       OFFLINE (%.1f ms)\n", node.Duration))
		sb.WriteString(fmt.Sprintf("    • Error:        %s\n", node.Error))
	case "unconfigured":
		sb.WriteString("[!] Node 1: Long-Term Knowledge Layer ((not configured))\n")
		sb.WriteString("    • Status:       UNCONFIGURED\n")
		sb.WriteString(fmt.Sprintf("    • Error:        %s\n", node.Error))
	}
}

// FormatPing renders the lightweight preflight terminal connectivity output.
func (p ClusterProbeResult) FormatPing() string {
	var sb strings.Builder
	nodeLabels := map[string]string{
		"sensory":   "Node 3 (Sensory)",
		"working":   "Node 2 (Working)",
		"knowledge": "Node 1 (Knowledge)",
	}

	for _, key := range []string{"sensory", "working", "knowledge"} {
		node := p.findNode(key)
		if node == nil {
			continue
		}
		label := nodeLabels[key]
		durStr := fmt.Sprintf("%.1fms", node.Duration)
		if node.Status == "healthy" {
			sb.WriteString(fmt.Sprintf("[OK] %-19s - %-7s (%s)\n", label, durStr, node.URL))
		} else if node.Status == "unconfigured" {
			sb.WriteString(fmt.Sprintf("[FAIL] %-17s - unconfigured ((not configured)) - %s\n", label, node.Error))
		} else {
			errStr := node.Error
			if errStr != "" {
				errStr = " - " + errStr
			}
			sb.WriteString(fmt.Sprintf("[FAIL] %-17s - %-7s (%s)%s\n", label, durStr, node.URL, errStr))
		}
	}

	if p.AllHealthy {
		sb.WriteString(fmt.Sprintf("Cluster: HEALTHY (%d/%d nodes online)\n", p.OnlineCount, p.TotalCount))
	} else if p.OnlineCount > 0 {
		sb.WriteString(fmt.Sprintf("Cluster: DEGRADED (%d/%d nodes online)\n", p.OnlineCount, p.TotalCount))
	} else {
		sb.WriteString(fmt.Sprintf("Cluster: OFFLINE (%d/%d nodes online)\n", p.OnlineCount, p.TotalCount))
	}

	return sb.String()
}

// PingNodeSummary describes the reachability of a single node in ping JSON output.
type PingNodeSummary struct {
	Reachable bool    `json:"reachable"`
	PingMS    float64 `json:"ping_ms"`
	URL       string  `json:"url"`
	Error     string  `json:"error,omitempty"`
}

// PingResponse represents the structured JSON output for the ping subcommand.
type PingResponse struct {
	AllHealthy bool                       `json:"all_healthy"`
	Nodes      map[string]PingNodeSummary `json:"nodes"`
	Timestamp  string                     `json:"timestamp"`
}

// ToPingJSON converts the cluster probe result to the ping JSON response schema.
func (p ClusterProbeResult) ToPingJSON() PingResponse {
	nodes := make(map[string]PingNodeSummary)
	for _, n := range p.Nodes {
		ms := math.Round(n.Duration*10) / 10
		nodes[n.Key] = PingNodeSummary{
			Reachable: n.Status == "healthy",
			PingMS:    ms,
			URL:       n.URL,
			Error:     n.Error,
		}
	}
	return PingResponse{
		AllHealthy: p.AllHealthy,
		Nodes:      nodes,
		Timestamp:  p.Timestamp,
	}
}

// StatusNodeOutput defines the structured JSON format of a probed node for status --json.
type StatusNodeOutput struct {
	Name     string      `json:"name"`
	Role     string      `json:"role"`
	URL      string      `json:"url"`
	Status   string      `json:"status"`
	Duration float64     `json:"ping_ms"`
	Details  interface{} `json:"details,omitempty"`
	Error    string      `json:"error,omitempty"`
}

// StatusResponse represents the structured JSON output for the status subcommand.
type StatusResponse struct {
	ClusterStatus string             `json:"cluster_status"`
	EnvFileLoaded string             `json:"env_file_loaded"`
	TraceID       string             `json:"trace_id"`
	Timestamp     string             `json:"timestamp"`
	Nodes         []StatusNodeOutput `json:"nodes"`
}

// ToStatusJSON converts the probe result to the status JSON response schema.
func (p ClusterProbeResult) ToStatusJSON() StatusResponse {
	nodes := make([]StatusNodeOutput, len(p.Nodes))
	for i, n := range p.Nodes {
		nodes[i] = StatusNodeOutput{
			Name:     n.Name,
			Role:     n.Role,
			URL:      n.URL,
			Status:   n.Status,
			Duration: n.Duration,
			Details:  n.Details,
			Error:    n.Error,
		}
	}
	return StatusResponse{
		ClusterStatus: p.ClusterStatus,
		EnvFileLoaded: p.EnvFileLoaded,
		TraceID:       p.TraceID,
		Timestamp:     p.Timestamp,
		Nodes:         nodes,
	}
}

func formatNumber(n int64) string {
	in := strconv.FormatInt(n, 10)
	var out []byte
	l := len(in)
	for i, c := range in {
		if i > 0 && (l-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

func formatUptime(seconds int64) string {
	if seconds < 0 {
		return "0s"
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%dd %02dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %02dm", hours, mins)
	}
	return fmt.Sprintf("%dm %02ds", mins, seconds%60)
}
