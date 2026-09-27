package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/client"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/telemetry"
)

// Orchestrator coordinates the 4-stage closed-loop cognitive cycle across cluster nodes.
type Orchestrator struct {
	cfg       client.Config
	sensory   *client.SensoryClient
	working   *client.WorkingClient
	knowledge *client.KnowledgeClient
}

// NewOrchestrator initialises the orchestration engine with cluster layer clients.
// The clients carry no fixed HTTP timeout: RunCycle sets each stage's deadline from config
// (sensory and consolidate timeouts, the default timeout for recall, and a token-sized deadline
// for deliberation).
func NewOrchestrator(cfg client.Config) *Orchestrator {
	return &Orchestrator{
		cfg:       cfg,
		sensory:   client.NewSensoryClient(cfg.SensoryURL, 0, cfg.TLSCACert, cfg.Insecure),
		working:   client.NewWorkingClient(cfg.WorkingURL, 0, cfg.TLSCACert, cfg.Insecure),
		knowledge: client.NewKnowledgeClient(cfg.KnowledgeURL, 0, cfg.APIKey, cfg.TLSCACert, cfg.Insecure),
	}
}

// RunCycle executes the deterministic 4-stage cognitive loop across Node 3, Node 1, and Node 2.
func (o *Orchestrator) RunCycle(ctx context.Context, req model.OrchestrateRequest, traceID string) (*model.OrchestrateResponse, error) {
	startTime := time.Now()

	if traceID == "" {
		traceID = telemetry.GenerateTraceID()
	}
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("session-%d", time.Now().Unix())
	}

	telemetry.LogStep(traceID, "Orchestrator", fmt.Sprintf("Beginning cognitive cycle (Session: %s, Directive: %s)", sessionID, req.TaskDirective))

	resp := &model.OrchestrateResponse{
		TraceID:   traceID,
		SessionID: sessionID,
		Status:    "in_progress",
		Stages:    make([]model.StageTelemetry, 0, 4),
	}

	// -------------------------------------------------------------------------
	// Stage 1: Sensory Gating (Node 3)
	// -------------------------------------------------------------------------
	stage1Start := time.Now()
	threshold := req.FilterThreshold
	if threshold <= 0 {
		threshold = 0.45
	}

	filterReq := model.FilterRequest{
		Text:          req.RawInput,
		TaskDirective: req.TaskDirective,
		Threshold:     threshold,
	}

	stage1Ctx, cancel1 := withDeadline(ctx, o.cfg.SensoryTimeout)
	sensoryResp, err := o.sensory.Filter(stage1Ctx, filterReq, traceID)
	err = client.WithDeadline(stage1Ctx, err, "sensory filter", o.cfg.SensoryTimeout, o.cfg.SensorySource, "CLUSTER_SENSORY_TIMEOUT_MS")
	cancel1()
	stage1Duration := float64(time.Since(stage1Start).Microseconds()) / 1000.0

	sensoryNodeName := fmt.Sprintf("Sensory Layer (%s)", o.cfg.SensoryURL)
	if err != nil {
		resp.Stages = append(resp.Stages, model.StageTelemetry{
			StageName:  "1_sensory_filter",
			Node:       sensoryNodeName,
			Endpoint:   "/api/v1/sensory/filter",
			DurationMS: stage1Duration,
			Status:     "failed",
			Error:      err.Error(),
		})
		telemetry.LogStep(traceID, "SensoryFilter", fmt.Sprintf("Sensory stage error: %v", err))
		// Fallback: create raw sensory chunk to prevent stalling downstream reasoning
		sensoryResp = &model.FilterResponse{
			Chunks: []model.SensoryChunk{
				{
					ID:        fmt.Sprintf("raw-%d", time.Now().UnixNano()),
					Text:      req.RawInput,
					Salience:  0.50,
					Source:    "fallback_unfiltered",
					Timestamp: time.Now(),
				},
			},
			TotalChunks:   1,
			SalientChunks: 1,
		}
	} else {
		resp.Stages = append(resp.Stages, model.StageTelemetry{
			StageName:  "1_sensory_filter",
			Node:       sensoryNodeName,
			Endpoint:   "/api/v1/sensory/filter",
			DurationMS: stage1Duration,
			Status:     "success",
		})
	}
	resp.Sensory = *sensoryResp

	// -------------------------------------------------------------------------
	// Stage 2: Long-Term Associative Recall (Node 1)
	// -------------------------------------------------------------------------
	stage2Start := time.Now()
	topK := req.RecallTopK
	if topK <= 0 {
		topK = 5
	}

	recallQuery, excerptTruncated := buildRecallQuery(req.TaskDirective, sensoryResp.Chunks)

	recallReq := model.RecallRequest{
		Query:             recallQuery,
		TopK:              topK,
		Alpha:             0.6,
		Beta:              0.2,
		Gamma:             0.2,
		ExpandHops:        1,
		Anchors:           req.Anchors,
		AnchorMode:        req.AnchorMode,
		IncludeEmbeddings: req.IncludeEmbeddings,
	}

	stage2Ctx, cancel2 := withDeadline(ctx, o.cfg.DefaultTimeout)
	recallResp, err := o.knowledge.Recall(stage2Ctx, recallReq, traceID)
	cancel2()
	stage2Duration := float64(time.Since(stage2Start).Microseconds()) / 1000.0

	knowledgeNodeName := fmt.Sprintf("Knowledge Layer (%s)", o.cfg.KnowledgeURL)
	var facts []rankedFact
	if err != nil {
		resp.Stages = append(resp.Stages, model.StageTelemetry{
			StageName:  "2_long_term_recall",
			Node:       knowledgeNodeName,
			Endpoint:   "/api/v1/memory/recall",
			DurationMS: stage2Duration,
			Status:     "failed",
			Error:      err.Error(),
		})
		telemetry.LogStep(traceID, "LongTermRecall", fmt.Sprintf("Recall stage error: %v", err))
		recallResp = &model.RecallResponse{}
	} else {
		reference, referenceSource := relevanceReference(req.TaskDirective, sensoryResp.Chunks)
		kept, gate := gateRecall(recallResp.Nodes, reference, referenceSource, gateSettings{
			minSim:          positiveOr(req.RecallMinSim, DefaultRecallMinSim),
			minTermOverlap:  DefaultMinTermOverlap,
			minTermCoverage: DefaultMinTermCoverage,
			anchors:         req.Anchors,
		})
		gate.RecallQueryBytes = len(recallQuery)
		gate.RecallQueryExcerpt = excerptTruncated
		filterRecallResponse(recallResp, kept)
		telemetry.LogStep(traceID, "RelevanceGate", fmt.Sprintf("Kept %d of %d recalled nodes (sim floor %.2f)", gate.NodesKept, gate.NodesIn, gate.MinSimScore))

		resp.Stages = append(resp.Stages, model.StageTelemetry{
			StageName:     "2_long_term_recall",
			Node:          knowledgeNodeName,
			Endpoint:      "/api/v1/memory/recall",
			DurationMS:    stage2Duration,
			Status:        "success",
			RelevanceGate: gate,
		})
		for i, node := range kept {
			facts = append(facts, rankedFact{
				order: i,
				id:    node.ID,
				text:  fmt.Sprintf("[%s: %s] %s", node.EntityType, node.Label, node.Summary),
				score: node.SimScore,
			})
		}
	}
	resp.Recall = *recallResp

	// -------------------------------------------------------------------------
	// Stage 3: Working Memory Scratchpad Deliberation (Node 2)
	// -------------------------------------------------------------------------
	stage3Start := time.Now()
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 256
	}

	objective := req.TaskDirective
	if objective == "" {
		objective = "Process salient sensory inputs and formulate next action"
	}

	workingNodeName := fmt.Sprintf("Working Memory Layer (%s)", o.cfg.WorkingURL)
	packed, packErr := packContext(objective, sensoryResp.Chunks, facts, budgetSettings{
		contextTokens:       positiveIntOr(req.ContextTokens, DefaultContextTokens),
		maxTokens:           maxTokens,
		outputReserveTokens: positiveIntOr(req.OutputReserveTokens, DefaultOutputReserveTokens),
		reserveTokens:       positiveIntOr(req.PromptReserveTokens, DefaultPromptReserveTokens),
	})

	var delibResp *model.DeliberateResponse
	if packErr != nil {
		err = packErr
	} else {
		delibReq := model.DeliberateRequest{
			Objective:          objective,
			SensoryChunks:      packed.chunks,
			LongTermContext:    packed.facts,
			MaxTokens:          maxTokens,
			Temperature:        0.2,
			Prepacked:          true,
			PromptBudgetTokens: packed.report.PromptBudgetTokens,
		}
		promptTokens := packed.report.EstimatedPromptTokens + packed.report.TemplateReserveTokens
		stage3Ctx, cancel3 := withDeadline(ctx, client.DeliberationTimeout(o.cfg.DeliberateTimeout, promptTokens, maxTokens))
		delibResp, err = o.working.Deliberate(stage3Ctx, delibReq, traceID)
		cancel3()
		if err == nil {
			packed.report.ApplyNode2Usage(delibResp)
		} else {
			packed.report.ApplyNode2Usage(nil)
		}
	}
	stage3Duration := float64(time.Since(stage3Start).Microseconds()) / 1000.0

	if err != nil {
		resp.Stages = append(resp.Stages, model.StageTelemetry{
			StageName:     "3_working_deliberate",
			Node:          workingNodeName,
			Endpoint:      "/api/v1/working/deliberate",
			DurationMS:    stage3Duration,
			Status:        "failed",
			Error:         err.Error(),
			ContextBudget: packed.report,
		})
		telemetry.LogStep(traceID, "WorkingMemory", fmt.Sprintf("Deliberation stage error: %v", err))
		fallbackThought := "Deliberation service unreachable; fallback to direct response."
		if packErr != nil {
			fallbackThought = "Deliberation skipped: prompt would exceed the Node 2 context budget."
		}
		delibResp = &model.DeliberateResponse{
			Status:           "failed",
			StepIndex:        1,
			TrajectoryLength: 1,
			Thought:          fallbackThought,
			ProposedAction:   "AWAIT_STABILISATION",
			IsComplete:       false,
		}
	} else {
		telemetry.LogStep(traceID, "ContextBudget", fmt.Sprintf("Packed %d/%d chunks and %d/%d facts, ~%d of %d prompt tokens (actual %d, node 2 usage %s)",
			packed.report.ChunksPacked, packed.report.ChunksIn, packed.report.FactsPacked, packed.report.FactsIn,
			packed.report.EstimatedPromptTokens, packed.report.PromptBudgetTokens, packed.report.ActualPromptTokens, packed.report.Node2Usage))
		if u := packed.report.Node2ContextUsage; packed.report.Node2SecondCut && u != nil {
			telemetry.LogStep(traceID, "ContextBudget", fmt.Sprintf("Node 2 cut the prepacked context again: kept %d of %d chunks (%d truncated), %d of %d facts",
				u.SensoryKept, u.SensoryReceived, u.SensoryTruncated, u.FactsKept, u.FactsReceived))
		}
		status := "success"
		if !packed.report.WithinBudget {
			status = "over_budget"
		}
		resp.Stages = append(resp.Stages, model.StageTelemetry{
			StageName:     "3_working_deliberate",
			Node:          workingNodeName,
			Endpoint:      "/api/v1/working/deliberate",
			DurationMS:    stage3Duration,
			Status:        status,
			ContextBudget: packed.report,
		})
	}
	resp.Deliberation = *delibResp
	resp.FinalThought = delibResp.Thought
	resp.ProposedAction = delibResp.ProposedAction
	resp.IsComplete = delibResp.IsComplete

	// -------------------------------------------------------------------------
	// Stage 4: Episodic Memory Consolidation (Node 1)
	// -------------------------------------------------------------------------
	stage4Start := time.Now()

	// Convert sensory chunks to consolidation sensory items
	sensoryItems := make([]model.SensoryItem, 0, len(sensoryResp.Chunks))
	for _, sc := range sensoryResp.Chunks {
		sensoryItems = append(sensoryItems, model.SensoryItem{
			ID:        sc.ID,
			Text:      sc.Text,
			Salience:  sc.Salience,
			Source:    sc.Source,
			Timestamp: sc.Timestamp,
		})
	}

	trajectorySteps := []model.TrajectoryStep{
		{
			StepIndex: delibResp.StepIndex,
			Thought:   delibResp.Thought,
			Action:    delibResp.ProposedAction,
			Status:    delibResp.Status,
			Timestamp: time.Now(),
		},
	}

	outcome := "success"
	if !delibResp.IsComplete && delibResp.Status == "failed" {
		outcome = "failure"
	}

	consolidateReq := model.ConsolidateRequest{
		TraceID:          traceID,
		SessionID:        sessionID,
		TaskGoal:         req.TaskDirective,
		Outcome:          outcome,
		SensoryContext:   sensoryItems,
		Trajectory:       trajectorySteps,
		CandidateActions: delibResp.CandidateActions,
		Anchors:          req.Anchors,
		Synchronous:      req.SynchronousConsolidate,
	}

	stage4Ctx, cancel4 := withDeadline(ctx, o.cfg.ConsolidateTimeout)
	consolidateResp, err := o.knowledge.Consolidate(stage4Ctx, consolidateReq, traceID)
	err = client.WithDeadline(stage4Ctx, err, "consolidate", o.cfg.ConsolidateTimeout, o.cfg.ConsolidateSource, "CLUSTER_CONSOLIDATE_TIMEOUT_MS")
	cancel4()
	stage4Duration := float64(time.Since(stage4Start).Microseconds()) / 1000.0

	if err != nil {
		resp.Stages = append(resp.Stages, model.StageTelemetry{
			StageName:  "4_memory_consolidate",
			Node:       knowledgeNodeName,
			Endpoint:   "/api/v1/memory/consolidate",
			DurationMS: stage4Duration,
			Status:     "failed",
			Error:      err.Error(),
		})
		telemetry.LogStep(traceID, "Consolidation", fmt.Sprintf("Consolidation stage error: %v", err))
		consolidateResp = &model.ConsolidateResponse{
			Status:      "failed",
			TraceID:     traceID,
			Message:     err.Error(),
			Synchronous: req.SynchronousConsolidate,
		}
	} else {
		resp.Stages = append(resp.Stages, model.StageTelemetry{
			StageName:  "4_memory_consolidate",
			Node:       knowledgeNodeName,
			Endpoint:   "/api/v1/memory/consolidate",
			DurationMS: stage4Duration,
			Status:     "success",
		})
	}
	resp.Consolidation = *consolidateResp

	// Calculate total execution metrics
	resp.TotalDurationMS = float64(time.Since(startTime).Microseconds()) / 1000.0
	resp.Status, resp.LoopComplete = cycleOutcome(resp.Stages)

	telemetry.LogStep(traceID, "Orchestrator", fmt.Sprintf("Cognitive cycle %s in %.2fms", resp.Status, resp.TotalDurationMS))

	return resp, nil
}

// cycleOutcome derives the top-level status from the stage results: "completed" when no stage
// failed, "partial" when some failed, "failed" when all did. loopComplete is true only when all
// four stages report "success" (an over_budget deliberation ran, but does not count).
func cycleOutcome(stages []model.StageTelemetry) (status string, loopComplete bool) {
	failed, succeeded := 0, 0
	for _, st := range stages {
		switch st.Status {
		case "failed":
			failed++
		case "success":
			succeeded++
		}
	}
	switch {
	case failed == 0:
		status = "completed"
	case failed == len(stages):
		status = "failed"
	default:
		status = "partial"
	}
	return status, len(stages) == 4 && succeeded == 4
}

// withDeadline applies d to ctx; a non-positive d means no deadline, as with the HTTP client.
func withDeadline(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, d)
}

func positiveOr(v, fallback float64) float64 {
	if v > 0 {
		return v
	}
	return fallback
}

func positiveIntOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}
