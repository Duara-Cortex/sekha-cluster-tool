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
func NewOrchestrator(cfg client.Config) *Orchestrator {
	return &Orchestrator{
		cfg:       cfg,
		sensory:   client.NewSensoryClient(cfg.SensoryURL, cfg.DefaultTimeout),
		working:   client.NewWorkingClient(cfg.WorkingURL, cfg.DeliberateTimeout),
		knowledge: client.NewKnowledgeClient(cfg.KnowledgeURL, cfg.DefaultTimeout),
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

	sensoryResp, err := o.sensory.Filter(ctx, filterReq, traceID)
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

	recallQuery := req.TaskDirective
	if recallQuery == "" && len(sensoryResp.Chunks) > 0 {
		recallQuery = sensoryResp.Chunks[0].Text
	}

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

	recallResp, err := o.knowledge.Recall(ctx, recallReq, traceID)
	stage2Duration := float64(time.Since(stage2Start).Microseconds()) / 1000.0

	knowledgeNodeName := fmt.Sprintf("Knowledge Layer (%s)", o.cfg.KnowledgeURL)
	longTermFacts := make([]string, 0)
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
		resp.Stages = append(resp.Stages, model.StageTelemetry{
			StageName:  "2_long_term_recall",
			Node:       knowledgeNodeName,
			Endpoint:   "/api/v1/memory/recall",
			DurationMS: stage2Duration,
			Status:     "success",
		})
		for _, node := range recallResp.Nodes {
			fact := fmt.Sprintf("[%s: %s] %s", node.EntityType, node.Label, node.Summary)
			longTermFacts = append(longTermFacts, fact)
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

	delibReq := model.DeliberateRequest{
		Objective:       req.TaskDirective,
		SensoryChunks:   sensoryResp.Chunks,
		LongTermContext: longTermFacts,
		MaxTokens:       maxTokens,
		Temperature:     0.2,
	}
	if delibReq.Objective == "" {
		delibReq.Objective = "Process salient sensory inputs and formulate next action"
	}

	delibResp, err := o.working.Deliberate(ctx, delibReq, traceID)
	stage3Duration := float64(time.Since(stage3Start).Microseconds()) / 1000.0

	workingNodeName := fmt.Sprintf("Working Memory Layer (%s)", o.cfg.WorkingURL)
	if err != nil {
		resp.Stages = append(resp.Stages, model.StageTelemetry{
			StageName:  "3_working_deliberate",
			Node:       workingNodeName,
			Endpoint:   "/api/v1/working/deliberate",
			DurationMS: stage3Duration,
			Status:     "failed",
			Error:      err.Error(),
		})
		telemetry.LogStep(traceID, "WorkingMemory", fmt.Sprintf("Deliberation stage error: %v", err))
		delibResp = &model.DeliberateResponse{
			Status:         "failed",
			Thought:        "Deliberation service unreachable; fallback to direct response.",
			ProposedAction: "AWAIT_STABILISATION",
			IsComplete:     false,
		}
	} else {
		resp.Stages = append(resp.Stages, model.StageTelemetry{
			StageName:  "3_working_deliberate",
			Node:       workingNodeName,
			Endpoint:   "/api/v1/working/deliberate",
			DurationMS: stage3Duration,
			Status:     "success",
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

	consolidateResp, err := o.knowledge.Consolidate(ctx, consolidateReq, traceID)
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
			Status:  "failed",
			TraceID: traceID,
			Message: err.Error(),
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
	resp.Status = "completed"

	telemetry.LogStep(traceID, "Orchestrator", fmt.Sprintf("Cognitive cycle completed in %.2fms", resp.TotalDurationMS))

	return resp, nil
}
