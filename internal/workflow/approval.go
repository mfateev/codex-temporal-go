// Package workflow contains Temporal workflow definitions.
//
// approval.go encapsulates tool approval classification and decision logic.
//
// Maps to: Codex AskForApproval policy check before tool dispatch
package workflow

import (
	"encoding/json"
	"fmt"

	"github.com/mfateev/temporal-agent-harness/internal/execpolicy"
	"github.com/mfateev/temporal-agent-harness/internal/models"
	"github.com/mfateev/temporal-agent-harness/internal/shell"
	"github.com/mfateev/temporal-agent-harness/internal/tools"
)

// ApprovalGate encapsulates tool approval classification and decision logic.
type ApprovalGate struct {
	mode        models.ApprovalMode
	policyRules string
}

// NewApprovalGate creates an ApprovalGate with the given approval mode and policy rules.
func NewApprovalGate(mode models.ApprovalMode, policyRules string) *ApprovalGate {
	return &ApprovalGate{mode: mode, policyRules: policyRules}
}

// Classify determines which tools need approval vs are forbidden.
// Delegates to classifyToolsForApproval.
func (g *ApprovalGate) Classify(calls []models.ConversationItem) ([]PendingApproval, []models.ConversationItem) {
	return classifyToolsForApproval(calls, g.mode, g.policyRules)
}

// ApplyDecision filters calls based on user's approval response.
// Delegates to applyApprovalDecision.
func (g *ApprovalGate) ApplyDecision(calls []models.ConversationItem, resp *ApprovalResponse) (approved, denied []models.ConversationItem) {
	return applyApprovalDecision(calls, resp)
}

// classifyToolsForApproval determines which tool calls need user approval.
// Uses the exec policy engine when available, falling back to heuristic classification.
//
// Returns:
//   - pending: tools needing approval (shown to user)
//   - forbidden: tools that are forbidden (denied immediately)
//
// Maps to: Codex AskForApproval policy check before tool dispatch
func classifyToolsForApproval(
	functionCalls []models.ConversationItem,
	mode models.ApprovalMode,
	policyRules string,
) (pending []PendingApproval, forbidden []models.ConversationItem) {
	// Empty/unset mode or "never" -> auto-approve all (backward compat)
	if mode == "" || mode == models.ApprovalNever {
		return nil, nil
	}

	// Build exec policy manager from serialized rules
	var policyMgr *execpolicy.ExecPolicyManager
	if policyRules != "" {
		mgr, err := execpolicy.LoadExecPolicyFromSource(policyRules)
		if err == nil {
			policyMgr = mgr
		}
	}

	for _, fc := range functionCalls {
		req, reason := evaluateToolApproval(fc.Name, fc.Arguments, policyMgr, mode)
		switch req {
		case tools.ApprovalSkip:
			continue // auto-approved
		case tools.ApprovalNeeded:
			pending = append(pending, PendingApproval{
				CallID:    fc.CallID,
				ToolName:  fc.Name,
				Arguments: fc.Arguments,
				Reason:    reason,
			})
		case tools.ApprovalForbidden:
			falseVal := false
			msg := "This command is forbidden by exec policy."
			if reason != "" {
				msg = fmt.Sprintf("Forbidden: %s", reason)
			}
			forbidden = append(forbidden, models.ConversationItem{
				Type:   models.ItemTypeFunctionCallOutput,
				CallID: fc.CallID,
				Output: &models.FunctionCallOutputPayload{
					Content: msg,
					Success: &falseVal,
				},
			})
		}
	}
	return pending, forbidden
}

// evaluateToolApproval determines the approval requirement for a single tool call.
// Returns the requirement and a human-readable reason.
func evaluateToolApproval(
	toolName, arguments string,
	policyMgr *execpolicy.ExecPolicyManager,
	mode models.ApprovalMode,
) (tools.ExecApprovalRequirement, string) {
	// Collab tools are workflow-intercepted and always safe
	if isCollabToolCall(toolName) {
		return tools.ApprovalSkip, ""
	}

	switch toolName {
	case "read_file", "list_dir", "grep_files", "request_user_input", "update_plan":
		return tools.ApprovalSkip, "" // Read-only / workflow-intercepted tools always safe

	case "shell":
		return evaluateShellArrayApproval(arguments, policyMgr, mode)

	case "shell_command":
		return evaluateShellCommandApproval(arguments, policyMgr, mode)

	case "write_file", "apply_patch":
		if mode == models.ApprovalNever {
			return tools.ApprovalSkip, ""
		}
		return tools.ApprovalNeeded, "mutating file operation"

	default:
		if mode == models.ApprovalNever {
			return tools.ApprovalSkip, ""
		}
		return tools.ApprovalNeeded, "unknown tool"
	}
}

// evaluateShellArrayApproval evaluates the array-based "shell" tool call
// through the exec policy engine. The command argument is []interface{} → []string.
func evaluateShellArrayApproval(
	arguments string,
	policyMgr *execpolicy.ExecPolicyManager,
	mode models.ApprovalMode,
) (tools.ExecApprovalRequirement, string) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return tools.ApprovalNeeded, "cannot parse arguments"
	}

	cmdArr, ok := args["command"].([]interface{})
	if !ok || len(cmdArr) == 0 {
		return tools.ApprovalNeeded, "missing command"
	}
	cmdVec := make([]string, len(cmdArr))
	for i, v := range cmdArr {
		s, ok := v.(string)
		if !ok {
			return tools.ApprovalNeeded, "command array contains non-string element"
		}
		cmdVec[i] = s
	}

	return evaluateCommandVecApproval(cmdVec, policyMgr, mode)
}

// evaluateShellCommandApproval evaluates the string-based "shell_command" tool
// call through the exec policy engine. The command string is wrapped via the
// user's detected shell (with optional login flag).
func evaluateShellCommandApproval(
	arguments string,
	policyMgr *execpolicy.ExecPolicyManager,
	mode models.ApprovalMode,
) (tools.ExecApprovalRequirement, string) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return tools.ApprovalNeeded, "cannot parse arguments"
	}
	cmd, ok := args["command"].(string)
	if !ok || cmd == "" {
		return tools.ApprovalNeeded, "missing command"
	}

	// Derive exec args through the user's detected shell.
	login := true
	if loginVal, ok := args["login"].(bool); ok {
		login = loginVal
	}
	userShell := shell.DetectUserShell()
	cmdVec := userShell.DeriveExecArgs(cmd, login)

	return evaluateCommandVecApproval(cmdVec, policyMgr, mode)
}

// evaluateCommandVecApproval is the shared approval path for a fully-resolved
// command vector (used by both shell and shell_command approval).
func evaluateCommandVecApproval(
	cmdVec []string,
	policyMgr *execpolicy.ExecPolicyManager,
	mode models.ApprovalMode,
) (tools.ExecApprovalRequirement, string) {
	// Use exec policy if available
	if policyMgr != nil {
		eval := policyMgr.GetEvaluation(cmdVec, string(mode))
		req := decisionToApprovalReq(eval.Decision)
		return req, eval.Justification
	}

	// Fallback to heuristic
	if mode == models.ApprovalNever || mode == "" {
		return tools.ApprovalSkip, ""
	}
	if mode == models.ApprovalOnFailure {
		return tools.ApprovalSkip, "" // runs in sandbox
	}
	// unless-trusted: use EvaluateCommand which handles both
	// ["bash", "-lc", "cmd"] and direct ["git", "status"] formats.
	mgr := execpolicy.NewExecPolicyManager(execpolicy.NewPolicy())
	return mgr.EvaluateCommand(cmdVec, string(mode)), ""
}

// decisionToApprovalReq maps a policy Decision to ExecApprovalRequirement.
func decisionToApprovalReq(d execpolicy.Decision) tools.ExecApprovalRequirement {
	switch d {
	case execpolicy.DecisionAllow:
		return tools.ApprovalSkip
	case execpolicy.DecisionPrompt:
		return tools.ApprovalNeeded
	case execpolicy.DecisionForbidden:
		return tools.ApprovalForbidden
	default:
		return tools.ApprovalNeeded
	}
}

// applyApprovalDecision filters function calls based on the approval response.
// Returns approved function calls and denied result items for history.
func applyApprovalDecision(functionCalls []models.ConversationItem, resp *ApprovalResponse) ([]models.ConversationItem, []models.ConversationItem) {
	if resp == nil {
		return functionCalls, nil
	}

	deniedSet := make(map[string]bool, len(resp.Denied))
	for _, id := range resp.Denied {
		deniedSet[id] = true
	}

	var approved []models.ConversationItem
	var denied []models.ConversationItem

	for _, fc := range functionCalls {
		if deniedSet[fc.CallID] {
			falseVal := false
			denied = append(denied, models.ConversationItem{
				Type:   models.ItemTypeFunctionCallOutput,
				CallID: fc.CallID,
				Output: &models.FunctionCallOutputPayload{
					Content: "User denied execution of this tool call.",
					Success: &falseVal,
				},
			})
		} else {
			approved = append(approved, fc)
		}
	}

	return approved, denied
}
