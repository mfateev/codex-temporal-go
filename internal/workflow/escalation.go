// Package workflow contains Temporal workflow definitions.
//
// escalation.go implements on-failure escalation logic and sandbox denial detection.
//
// Maps to: codex-rs/core/src/exec.rs sandbox denial handling
package workflow

import (
	"fmt"
	"strings"

	"go.temporal.io/sdk/workflow"

	"github.com/mfateev/temporal-agent-harness/internal/activities"
	"github.com/mfateev/temporal-agent-harness/internal/models"
)

// sandboxDenialKeywords are output strings that indicate a sandbox/permission
// denial rather than a normal command failure.
// Matches Codex: codex-rs/core/src/exec.rs SANDBOX_DENIED_KEYWORDS
var sandboxDenialKeywords = []string{
	"operation not permitted",
	"permission denied",
	"read-only file system",
	"seccomp",
	"sandbox",
	"landlock",
	"failed to write file",
}

// isLikelySandboxDenial checks whether a failed tool result looks like it was
// blocked by a sandbox rather than failing for an ordinary reason (file not
// found, invalid args, etc.).
func isLikelySandboxDenial(output string) bool {
	lower := strings.ToLower(output)
	for _, kw := range sandboxDenialKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// handleOnFailureEscalation checks for failed tools in on-failure mode.
// For failed tools that look like sandbox denials, prompts the user to
// re-execute without sandbox. Normal failures are passed through to the LLM.
// Returns updated tool results (may include re-executed results).
func (s *SessionState) handleOnFailureEscalation(
	ctx workflow.Context,
	functionCalls []models.ConversationItem,
	toolResults []activities.ToolActivityOutput,
) ([]activities.ToolActivityOutput, error) {
	logger := workflow.GetLogger(ctx)

	// Find failed tools
	var escalations []EscalationRequest
	failedIndices := make(map[int]bool)

	for i, result := range toolResults {
		if result.Success != nil && !*result.Success {
			if isLikelySandboxDenial(result.Content) {
				// Looks like sandbox blocked it — escalate to user
				failedIndices[i] = true
				escalations = append(escalations, EscalationRequest{
					CallID:    result.CallID,
					ToolName:  functionCalls[i].Name,
					Arguments: functionCalls[i].Arguments,
					Output:    result.Content,
					Reason:    "command failed in sandbox",
				})
			} else {
				// Normal failure (file not found, bad args, etc.) — let LLM see it
				logger.Info("Tool failed but not sandbox-related, returning to LLM",
					"tool", functionCalls[i].Name, "output_prefix", truncate(result.Content, 100))
			}
		}
	}

	if len(escalations) == 0 {
		return toolResults, nil // No failures
	}

	// Enter escalation pending state
	s.Phase = PhaseEscalationPending
	s.PendingEscalations = escalations
	s.EscalationReceived = false
	s.EscalationResponse = nil

	logger.Info("Waiting for escalation decision", "failed_count", len(escalations))

	// Wait for escalation response
	err := workflow.Await(ctx, func() bool {
		return s.EscalationReceived || s.Interrupted || s.ShutdownRequested
	})
	if err != nil {
		return nil, fmt.Errorf("escalation await failed: %w", err)
	}

	s.PendingEscalations = nil

	if s.Interrupted || s.ShutdownRequested {
		logger.Info("Escalation wait interrupted")
		return toolResults, nil // Return original results
	}

	if s.EscalationResponse == nil {
		return toolResults, nil
	}

	// Re-execute approved tools without sandbox
	approvedSet := make(map[string]bool, len(s.EscalationResponse.Approved))
	for _, id := range s.EscalationResponse.Approved {
		approvedSet[id] = true
	}

	for i, result := range toolResults {
		if !failedIndices[i] || !approvedSet[result.CallID] {
			continue
		}

		logger.Info("Re-executing tool without sandbox", "tool", functionCalls[i].Name)

		// Re-execute without sandbox (no SandboxPolicy)
		reResults, err := executeToolsInParallel(
			ctx,
			[]models.ConversationItem{functionCalls[i]},
			s.ToolSpecs, s.Config.Cwd, s.Config.SessionTaskQueue,
		)
		if err != nil {
			continue // Keep original failed result
		}
		if len(reResults) > 0 {
			toolResults[i] = reResults[0]
		}
	}

	return toolResults, nil
}
