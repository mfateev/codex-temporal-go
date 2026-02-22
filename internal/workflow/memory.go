package workflow

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/mfateev/temporal-agent-harness/internal/activities"
	"github.com/mfateev/temporal-agent-harness/internal/memories"
	"github.com/mfateev/temporal-agent-harness/internal/models"
)

// ConsolidationState is the durable state for the consolidation workflow.
// Passed through ContinueAsNew.
type ConsolidationState struct {
	PendingSessions []string          `json:"pending_sessions"`
	MemoryRoot      string            `json:"memory_root"`
	MemoryDbPath    string            `json:"memory_db_path"`
	ModelConfig     models.ModelConfig `json:"model_config"`
	MaxRawMemories  int               `json:"max_raw_memories"`
}

// ConsolidationWorkflow is a singleton workflow that consolidates memories.
// It receives signals when new rollout summaries are available, then runs
// the consolidation pipeline: list outputs → materialize files → run agent.
//
// Workflow ID: "memory-consolidation" (fixed)
// Maps to: codex-rs/core/src/memories/phase2.rs
func ConsolidationWorkflow(ctx workflow.Context, state ConsolidationState) error {
	logger := workflow.GetLogger(ctx)

	if state.MaxRawMemories <= 0 {
		state.MaxRawMemories = 1024
	}

	// Register signal handler for new rollout summaries
	signalCh := workflow.GetSignalChannel(ctx, memories.SignalNewRolloutSummary)

	// Drain any pending signals (including the one from SignalWithStartWorkflow)
	for {
		var sessionID string
		ok := signalCh.ReceiveAsync(&sessionID)
		if !ok {
			break
		}
		if sessionID != "" {
			state.PendingSessions = append(state.PendingSessions, sessionID)
		}
	}

	// Main loop: wait for signals, then consolidate
	for {
		// Wait until we have pending sessions
		if len(state.PendingSessions) == 0 {
			logger.Info("Consolidation workflow waiting for signals")
			ok, err := workflow.AwaitWithTimeout(ctx, 24*time.Hour, func() bool {
				// Check for new signals
				for {
					var sessionID string
					gotSignal := signalCh.ReceiveAsync(&sessionID)
					if !gotSignal {
						break
					}
					if sessionID != "" {
						state.PendingSessions = append(state.PendingSessions, sessionID)
					}
				}
				return len(state.PendingSessions) > 0
			})
			if err != nil {
				return err
			}
			if !ok {
				// Timed out with no signals — ContinueAsNew to keep history bounded
				logger.Info("Consolidation workflow idle timeout, ContinueAsNew")
				return workflow.NewContinueAsNewError(ctx, ConsolidationWorkflow, state)
			}
		}

		logger.Info("Consolidation workflow processing", "pending", len(state.PendingSessions))

		// Activity options for DB/file operations
		shortActCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy: &temporal.RetryPolicy{
				MaximumAttempts: 3,
			},
		})

		// 1. List stage-1 outputs from DB
		var listResult activities.ListStage1Result
		err := workflow.ExecuteActivity(shortActCtx, "ListStage1Outputs",
			activities.ListStage1Input{MaxCount: state.MaxRawMemories},
		).Get(ctx, &listResult)
		if err != nil {
			logger.Warn("Failed to list stage1 outputs", "error", err)
			state.PendingSessions = nil
			continue
		}

		if len(listResult.Outputs) == 0 {
			logger.Info("No stage1 outputs found, clearing pending")
			state.PendingSessions = nil
			continue
		}

		// 2. Materialize files on disk
		err = workflow.ExecuteActivity(shortActCtx, "MaterializeMemoryFiles",
			activities.MaterializeInput{
				MemoryRoot: state.MemoryRoot,
				Outputs:    listResult.Outputs,
				MaxCount:   state.MaxRawMemories,
			},
		).Get(ctx, nil)
		if err != nil {
			logger.Warn("Failed to materialize memory files", "error", err)
			state.PendingSessions = nil
			continue
		}

		// 3. Run consolidation agent (long-running)
		longActCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 15 * time.Minute,
			HeartbeatTimeout:    2 * time.Minute,
			RetryPolicy: &temporal.RetryPolicy{
				MaximumAttempts: 2,
			},
		})

		err = workflow.ExecuteActivity(longActCtx, "RunConsolidationAgent",
			activities.ConsolidationAgentInput{
				MemoryRoot:  state.MemoryRoot,
				ModelConfig: state.ModelConfig,
			},
		).Get(ctx, nil)
		if err != nil {
			logger.Warn("Consolidation agent failed", "error", err)
		} else {
			logger.Info("Consolidation completed successfully")
		}

		// Clear pending sessions
		state.PendingSessions = nil

		// ContinueAsNew to keep history bounded
		return workflow.NewContinueAsNewError(ctx, ConsolidationWorkflow, state)
	}
}
