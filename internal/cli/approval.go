package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.temporal.io/api/serviceerror"

	"github.com/mfateev/codex-temporal-go/internal/workflow"
)

// HandleApprovalInput parses the user's response to an approval prompt.
// Returns (response, setAutoApprove). Response is nil if input is not recognized.
//
// Supports:
//   - "y"/"yes" — approve all
//   - "n"/"no" — deny all
//   - "a"/"always" — approve all + set auto-approve flag
//   - "1,3" — approve indices 1 and 3, deny the rest
func HandleApprovalInput(line string, pending []workflow.PendingApproval) (*workflow.ApprovalResponse, bool) {
	line = strings.ToLower(strings.TrimSpace(line))

	allCallIDs := make([]string, len(pending))
	for i, ap := range pending {
		allCallIDs[i] = ap.CallID
	}

	switch line {
	case "y", "yes":
		return &workflow.ApprovalResponse{Approved: allCallIDs}, false
	case "n", "no":
		return &workflow.ApprovalResponse{Denied: allCallIDs}, false
	case "a", "always":
		return &workflow.ApprovalResponse{Approved: allCallIDs}, true
	}

	// Try index-based selection
	indices := parseApprovalIndices(line, len(pending))
	if indices == nil {
		return nil, false
	}

	approvedSet := make(map[int]bool, len(indices))
	for _, idx := range indices {
		approvedSet[idx] = true
	}

	var approved, denied []string
	for i, callID := range allCallIDs {
		if approvedSet[i+1] {
			approved = append(approved, callID)
		} else {
			denied = append(denied, callID)
		}
	}

	return &workflow.ApprovalResponse{Approved: approved, Denied: denied}, false
}

// HandleEscalationInput parses the user's response to an escalation prompt.
// Returns nil if the input is not recognized.
func HandleEscalationInput(line string, pending []workflow.EscalationRequest) *workflow.EscalationResponse {
	line = strings.ToLower(strings.TrimSpace(line))

	allCallIDs := make([]string, len(pending))
	for i, esc := range pending {
		allCallIDs[i] = esc.CallID
	}

	switch line {
	case "y", "yes":
		return &workflow.EscalationResponse{Approved: allCallIDs}
	case "n", "no":
		return &workflow.EscalationResponse{Denied: allCallIDs}
	}

	indices := parseApprovalIndices(line, len(pending))
	if indices == nil {
		return nil
	}

	approvedSet := make(map[int]bool, len(indices))
	for _, idx := range indices {
		approvedSet[idx] = true
	}

	var approved, denied []string
	for i, callID := range allCallIDs {
		if approvedSet[i+1] {
			approved = append(approved, callID)
		} else {
			denied = append(denied, callID)
		}
	}

	return &workflow.EscalationResponse{Approved: approved, Denied: denied}
}

// parseApprovalIndices parses a comma-separated list of 1-based indices.
// Returns nil if the input is not valid.
func parseApprovalIndices(input string, maxIndex int) []int {
	parts := strings.Split(input, ",")
	var indices []int
	seen := make(map[int]bool)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var idx int
		n, err := fmt.Sscanf(part, "%d", &idx)
		if err != nil || n != 1 || idx < 1 || idx > maxIndex {
			return nil
		}
		if !seen[idx] {
			seen[idx] = true
			indices = append(indices, idx)
		}
	}

	if len(indices) == 0 {
		return nil
	}
	return indices
}

// ApprovalSelectionToResponse maps a selector index to an ApprovalResponse.
// Options: 0=approve all, 1=deny all, 2=always approve, 3=select individually (returns nil).
func ApprovalSelectionToResponse(selected int, pending []workflow.PendingApproval) (*workflow.ApprovalResponse, bool) {
	allCallIDs := make([]string, len(pending))
	for i, ap := range pending {
		allCallIDs[i] = ap.CallID
	}

	switch selected {
	case 0: // Yes, allow
		return &workflow.ApprovalResponse{Approved: allCallIDs}, false
	case 1: // No, deny
		return &workflow.ApprovalResponse{Denied: allCallIDs}, false
	case 2: // Always allow
		return &workflow.ApprovalResponse{Approved: allCallIDs}, true
	case 3: // Select individually (multi-tool only) - fall back to textarea
		return nil, false
	default:
		return nil, false
	}
}

// EscalationSelectionToResponse maps a selector index to an EscalationResponse.
// Options: 0=approve (re-run without sandbox), 1=deny.
func EscalationSelectionToResponse(selected int, pending []workflow.EscalationRequest) *workflow.EscalationResponse {
	allCallIDs := make([]string, len(pending))
	for i, esc := range pending {
		allCallIDs[i] = esc.CallID
	}

	switch selected {
	case 0: // Yes, re-run
		return &workflow.EscalationResponse{Approved: allCallIDs}
	case 1: // No, deny
		return &workflow.EscalationResponse{Denied: allCallIDs}
	default:
		return nil
	}
}

// approvalInfo holds structured information extracted from tool arguments
// for rendering in approval prompts.
type approvalInfo struct {
	Title   string   // e.g. "Write file: /path/to/file.go" or "Shell: rm -rf /tmp"
	Preview []string // optional content preview lines (nil = no preview box)
}

// formatApprovalInfo extracts structured approval information from tool arguments.
func formatApprovalInfo(toolName, arguments string) approvalInfo {
	var args map[string]interface{}
	if json.Unmarshal([]byte(arguments), &args) == nil {
		switch toolName {
		case "shell":
			if cmd, ok := args["command"].(string); ok {
				return approvalInfo{Title: "Shell: " + cmd}
			}
		case "write_file":
			if path := stringArg(args, "file_path", "path"); path != "" {
				info := approvalInfo{Title: "Write file: " + path}
				if content, ok := args["content"].(string); ok && content != "" {
					info.Preview = contentPreview(content, 5)
				}
				return info
			}
		case "apply_patch":
			info := approvalInfo{Title: "Patch"}
			if path := stringArg(args, "file_path"); path != "" {
				info.Title = "Patch: " + path
			}
			if input, ok := args["input"].(string); ok && input != "" {
				info.Preview = contentPreview(input, 5)
			}
			return info
		case "read_file":
			if path := stringArg(args, "file_path", "path"); path != "" {
				return approvalInfo{Title: "Read: " + path}
			}
		case "list_dir":
			if path := stringArg(args, "dir_path", "path"); path != "" {
				return approvalInfo{Title: "List: " + path}
			}
		case "grep_files":
			if pat, ok := args["pattern"].(string); ok {
				title := "Search: " + pat
				if dir, ok := args["path"].(string); ok {
					title += " in " + dir
				}
				return approvalInfo{Title: title}
			}
		}
	}
	display := arguments
	if len(display) > 300 {
		display = display[:300] + "..."
	}
	return approvalInfo{Title: toolName + ": " + display}
}

// contentPreview splits content into lines and returns at most maxLines,
// using middle truncation if the content exceeds the limit.
func contentPreview(content string, maxLines int) []string {
	lines := strings.Split(content, "\n")
	truncated, _ := truncateMiddle(lines, maxLines)
	return truncated
}

// stringArg returns the first non-empty string value found among the given keys.
func stringArg(args map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := args[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// pollErrorKind classifies errors from workflow queries.
type pollErrorKind int

const (
	pollErrorTransient pollErrorKind = iota
	pollErrorCompleted
	pollErrorFatal
)

// classifyPollError categorizes a poll error using Temporal SDK typed errors.
func classifyPollError(err error) pollErrorKind {
	var notFoundErr *serviceerror.NotFound
	if errors.As(err, &notFoundErr) {
		return pollErrorCompleted
	}

	var notReadyErr *serviceerror.WorkflowNotReady
	if errors.As(err, &notReadyErr) {
		return pollErrorTransient
	}

	var queryFailedErr *serviceerror.QueryFailed
	if errors.As(err, &queryFailedErr) {
		return pollErrorTransient
	}

	if strings.Contains(err.Error(), "workflow execution already completed") {
		return pollErrorCompleted
	}

	return pollErrorFatal
}
