// Package workflow contains Temporal workflow definitions.
//
// init.go handles one-time session initialization: resolving instructions
// and loading exec policy rules from the worker filesystem.
package workflow

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/mfateev/temporal-agent-harness/internal/activities"
	"github.com/mfateev/temporal-agent-harness/internal/instructions"
)

// resolveInstructions loads worker-side AGENTS.md files and merges all
// instruction sources into the session configuration. Called once before
// the first turn. Non-fatal: falls back to CLI-provided docs on failure.
func (s *SessionState) resolveInstructions(ctx workflow.Context) {
	logger := workflow.GetLogger(ctx)

	// Load worker-side project docs via activity (runs on session task queue)
	var workerDocs string
	loadInput := activities.LoadWorkerInstructionsInput{
		Cwd: s.Config.Cwd,
	}

	actOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2,
		},
	}
	if s.Config.SessionTaskQueue != "" {
		actOpts.TaskQueue = s.Config.SessionTaskQueue
	}
	loadCtx := workflow.WithActivityOptions(ctx, actOpts)

	var loadResult activities.LoadWorkerInstructionsOutput
	err := workflow.ExecuteActivity(loadCtx, "LoadWorkerInstructions", loadInput).Get(ctx, &loadResult)
	if err != nil {
		logger.Warn("Failed to load worker instructions, using CLI fallback", "error", err)
	} else {
		workerDocs = loadResult.ProjectDocs
	}

	// Merge all instruction sources
	merged := instructions.MergeInstructions(instructions.MergeInput{
		BaseOverride:             s.Config.BaseInstructions,
		CLIProjectDocs:           s.Config.CLIProjectDocs,
		WorkerProjectDocs:        workerDocs,
		UserPersonalInstructions: s.Config.UserPersonalInstructions,
		ApprovalMode:             string(s.Config.ApprovalMode),
		Cwd:                      s.Config.Cwd,
	})

	// Store merged results in config (persists through ContinueAsNew)
	s.Config.BaseInstructions = merged.Base
	s.Config.DeveloperInstructions = merged.Developer
	s.Config.UserInstructions = merged.User

	logger.Info("Instructions resolved",
		"base_len", len(merged.Base),
		"developer_len", len(merged.Developer),
		"user_len", len(merged.User))
}

// loadExecPolicy loads exec policy rules from the worker filesystem.
// Non-fatal: falls back to empty policy on failure.
func (s *SessionState) loadExecPolicy(ctx workflow.Context) {
	logger := workflow.GetLogger(ctx)

	if s.Config.CodexHome == "" {
		return
	}

	loadInput := activities.LoadExecPolicyInput{
		CodexHome: s.Config.CodexHome,
	}

	actOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2,
		},
	}
	if s.Config.SessionTaskQueue != "" {
		actOpts.TaskQueue = s.Config.SessionTaskQueue
	}
	loadCtx := workflow.WithActivityOptions(ctx, actOpts)

	var loadResult activities.LoadExecPolicyOutput
	err := workflow.ExecuteActivity(loadCtx, "LoadExecPolicy", loadInput).Get(ctx, &loadResult)
	if err != nil {
		logger.Warn("Failed to load exec policy, using defaults", "error", err)
		return
	}

	s.ExecPolicyRules = loadResult.RulesSource
	logger.Info("Exec policy loaded", "rules_len", len(loadResult.RulesSource))
}
