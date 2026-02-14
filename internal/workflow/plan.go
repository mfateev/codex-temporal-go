// Package workflow contains Temporal workflow definitions.
//
// plan.go handles interception and processing of update_plan tool calls.
//
// Maps to: Codex update_plan tool (codex-rs/core/src/tools/)
package workflow

import (
	"encoding/json"
	"fmt"

	"go.temporal.io/sdk/workflow"

	"github.com/mfateev/temporal-agent-harness/internal/models"
)

// handleUpdatePlan intercepts an update_plan tool call, parses the arguments,
// validates the plan, updates the session plan state, and returns a
// FunctionCallOutput item confirming the update.
//
// Unlike handleRequestUserInput, this does not block waiting for user response.
// The plan is stored in SessionState and exposed via the get_turn_status query.
//
// Maps to: Codex update_plan tool handler
func (s *SessionState) handleUpdatePlan(ctx workflow.Context, fc models.ConversationItem) (models.ConversationItem, error) {
	logger := workflow.GetLogger(ctx)

	planState, err := parseUpdatePlanArgs(fc.Arguments)
	if err != nil {
		logger.Warn("Invalid update_plan args", "error", err)
		falseVal := false
		return models.ConversationItem{
			Type:   models.ItemTypeFunctionCallOutput,
			CallID: fc.CallID,
			Output: &models.FunctionCallOutputPayload{
				Content: fmt.Sprintf("Invalid update_plan arguments: %v", err),
				Success: &falseVal,
			},
		}, nil
	}

	// Update session plan state (persists across ContinueAsNew)
	s.Plan = planState

	logger.Info("Plan updated", "steps", len(planState.Steps))

	trueVal := true
	return models.ConversationItem{
		Type:   models.ItemTypeFunctionCallOutput,
		CallID: fc.CallID,
		Output: &models.FunctionCallOutputPayload{
			Content: "Plan updated.",
			Success: &trueVal,
		},
	}, nil
}

// parseUpdatePlanArgs validates and parses the update_plan arguments.
// Returns a PlanState or an error if the args are invalid.
func parseUpdatePlanArgs(argsJSON string) (*PlanState, error) {
	var args struct {
		Explanation string `json:"explanation,omitempty"`
		Plan        []struct {
			Step   string `json:"step"`
			Status string `json:"status"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if len(args.Plan) == 0 {
		return nil, fmt.Errorf("plan array must not be empty")
	}

	inProgressCount := 0
	steps := make([]PlanStep, len(args.Plan))
	for i, s := range args.Plan {
		if s.Step == "" {
			return nil, fmt.Errorf("step %d: step description must not be empty", i+1)
		}

		status := PlanStepStatus(s.Status)
		switch status {
		case PlanStepPending, PlanStepInProgress, PlanStepCompleted:
			// valid
		default:
			return nil, fmt.Errorf("step %d: invalid status %q (must be pending, in_progress, or completed)", i+1, s.Status)
		}

		if status == PlanStepInProgress {
			inProgressCount++
		}

		steps[i] = PlanStep{
			Step:   s.Step,
			Status: status,
		}
	}

	if inProgressCount > 1 {
		return nil, fmt.Errorf("at most one step can be in_progress, got %d", inProgressCount)
	}

	return &PlanState{
		Explanation: args.Explanation,
		Steps:       steps,
	}, nil
}
