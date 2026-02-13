package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/serviceerror"

	"github.com/mfateev/temporal-agent-harness/internal/workflow"
)

// --- Poll error classification tests ---

func TestClassifyPollError_NotFound(t *testing.T) {
	err := serviceerror.NewNotFound("workflow not found")
	assert.Equal(t, pollErrorCompleted, classifyPollError(err))
}

func TestClassifyPollError_WorkflowNotReady(t *testing.T) {
	err := &serviceerror.WorkflowNotReady{Message: "workflow not ready"}
	assert.Equal(t, pollErrorTransient, classifyPollError(err))
}

func TestClassifyPollError_QueryFailed(t *testing.T) {
	err := &serviceerror.QueryFailed{Message: "query rejected"}
	assert.Equal(t, pollErrorTransient, classifyPollError(err))
}

func TestClassifyPollError_AlreadyCompleted(t *testing.T) {
	err := fmt.Errorf("workflow execution already completed")
	assert.Equal(t, pollErrorCompleted, classifyPollError(err))
}

func TestClassifyPollError_UnknownError(t *testing.T) {
	err := fmt.Errorf("some unexpected error")
	assert.Equal(t, pollErrorFatal, classifyPollError(err))
}

func TestClassifyPollError_WrappedNotFound(t *testing.T) {
	inner := serviceerror.NewNotFound("workflow not found")
	err := fmt.Errorf("query failed: %w", inner)
	assert.Equal(t, pollErrorCompleted, classifyPollError(err))
}

// --- Approval input handling tests ---

func TestHandleApprovalInput_Yes(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
		{CallID: "c2", ToolName: "write_file"},
	}
	resp, autoApprove := HandleApprovalInput("y", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c1", "c2"}, resp.Approved)
	assert.Nil(t, resp.Denied)
	assert.False(t, autoApprove)
}

func TestHandleApprovalInput_YesFull(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
	}
	resp, _ := HandleApprovalInput("yes", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c1"}, resp.Approved)
}

func TestHandleApprovalInput_No(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
	}
	resp, _ := HandleApprovalInput("n", pending)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Approved)
	assert.Equal(t, []string{"c1"}, resp.Denied)
}

func TestHandleApprovalInput_NoFull(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
	}
	resp, _ := HandleApprovalInput("no", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c1"}, resp.Denied)
}

func TestHandleApprovalInput_Always(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
	}
	resp, autoApprove := HandleApprovalInput("a", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c1"}, resp.Approved)
	assert.True(t, autoApprove, "autoApprove should be set after 'always'")
}

func TestHandleApprovalInput_AlwaysFull(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
	}
	resp, autoApprove := HandleApprovalInput("always", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c1"}, resp.Approved)
	assert.True(t, autoApprove)
}

func TestHandleApprovalInput_Invalid(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
	}
	resp, _ := HandleApprovalInput("maybe", pending)
	assert.Nil(t, resp)
}

func TestHandleApprovalInput_CaseInsensitive(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
	}
	resp, _ := HandleApprovalInput("YES", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c1"}, resp.Approved)
}

func TestHandleApprovalInput_WithWhitespace(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
	}
	resp, _ := HandleApprovalInput("  y  ", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c1"}, resp.Approved)
}

func TestFormatApprovalInfo_Shell(t *testing.T) {
	info := formatApprovalInfo("shell", `{"command": "rm -rf /tmp"}`)
	assert.Equal(t, "Shell: rm -rf /tmp", info.Title)
	assert.Nil(t, info.Preview)
}

func TestFormatApprovalInfo_WriteFile(t *testing.T) {
	info := formatApprovalInfo("write_file", `{"file_path": "/home/user/test.txt", "content": "hello"}`)
	assert.Equal(t, "Write file: /home/user/test.txt", info.Title)
	require.NotNil(t, info.Preview)
	assert.Equal(t, []string{"hello"}, info.Preview)
}

func TestFormatApprovalInfo_WriteFilePathArg(t *testing.T) {
	info := formatApprovalInfo("write_file", `{"path": "/home/user/test.txt", "content": "hello"}`)
	assert.Equal(t, "Write file: /home/user/test.txt", info.Title)
	require.NotNil(t, info.Preview)
	assert.Equal(t, []string{"hello"}, info.Preview)
}

func TestFormatApprovalInfo_WriteFileNoContent(t *testing.T) {
	info := formatApprovalInfo("write_file", `{"file_path": "/home/user/test.txt"}`)
	assert.Equal(t, "Write file: /home/user/test.txt", info.Title)
	assert.Nil(t, info.Preview)
}

func TestFormatApprovalInfo_WriteFileMultiLine(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8"
	args := fmt.Sprintf(`{"file_path": "/home/user/test.go", "content": %q}`, content)
	info := formatApprovalInfo("write_file", args)
	assert.Equal(t, "Write file: /home/user/test.go", info.Title)
	require.NotNil(t, info.Preview)
	assert.LessOrEqual(t, len(info.Preview), 5)
	assert.Equal(t, "line1", info.Preview[0])
	// Middle lines should be truncated with "… +N lines"
	found := false
	for _, line := range info.Preview {
		if strings.HasPrefix(line, "…") {
			found = true
		}
	}
	assert.True(t, found, "expected middle truncation marker")
}

func TestFormatApprovalInfo_ApplyPatch(t *testing.T) {
	info := formatApprovalInfo("apply_patch", `{"file_path": "/home/user/test.txt"}`)
	assert.Equal(t, "Patch: /home/user/test.txt", info.Title)
	assert.Nil(t, info.Preview)
}

func TestFormatApprovalInfo_ApplyPatchWithInput(t *testing.T) {
	input := "*** Begin Patch\n*** Update File: test.go\n@@ line1 @@\n- old\n+ new\n*** End Patch"
	args := fmt.Sprintf(`{"input": %q}`, input)
	info := formatApprovalInfo("apply_patch", args)
	assert.Equal(t, "Patch", info.Title)
	require.NotNil(t, info.Preview)
	assert.Equal(t, "*** Begin Patch", info.Preview[0])
}

func TestFormatApprovalInfo_UnknownTool(t *testing.T) {
	info := formatApprovalInfo("custom_tool", `{"foo": "bar"}`)
	assert.Contains(t, info.Title, "custom_tool")
	assert.Nil(t, info.Preview)
}

func TestFormatApprovalInfo_BadJSON(t *testing.T) {
	info := formatApprovalInfo("shell", `{bad json`)
	assert.Contains(t, info.Title, "shell")
}

func TestFormatApprovalInfo_LongArgs(t *testing.T) {
	longArg := strings.Repeat("x", 400)
	info := formatApprovalInfo("custom_tool", longArg)
	assert.Contains(t, info.Title, "...")
	assert.LessOrEqual(t, len(info.Title), 320) // "custom_tool: " + 300 + "..."
}

// --- Index-based approval tests ---

func TestHandleApprovalInput_IndexSingle(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
		{CallID: "c2", ToolName: "write_file"},
		{CallID: "c3", ToolName: "apply_patch"},
	}
	resp, _ := HandleApprovalInput("2", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c2"}, resp.Approved)
	assert.Equal(t, []string{"c1", "c3"}, resp.Denied)
}

func TestHandleApprovalInput_IndexMultiple(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
		{CallID: "c2", ToolName: "write_file"},
		{CallID: "c3", ToolName: "apply_patch"},
	}
	resp, _ := HandleApprovalInput("1,3", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c1", "c3"}, resp.Approved)
	assert.Equal(t, []string{"c2"}, resp.Denied)
}

func TestHandleApprovalInput_IndexWithSpaces(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
		{CallID: "c2", ToolName: "write_file"},
	}
	resp, _ := HandleApprovalInput("1, 2", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c1", "c2"}, resp.Approved)
	assert.Empty(t, resp.Denied)
}

func TestHandleApprovalInput_IndexOutOfRange(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
	}
	resp, _ := HandleApprovalInput("5", pending)
	assert.Nil(t, resp)
}

func TestHandleApprovalInput_IndexZero(t *testing.T) {
	pending := []workflow.PendingApproval{
		{CallID: "c1", ToolName: "shell"},
	}
	resp, _ := HandleApprovalInput("0", pending)
	assert.Nil(t, resp)
}

func TestParseApprovalIndices_Valid(t *testing.T) {
	assert.Equal(t, []int{1, 3}, parseApprovalIndices("1,3", 3))
	assert.Equal(t, []int{2}, parseApprovalIndices("2", 3))
	assert.Equal(t, []int{1, 2, 3}, parseApprovalIndices("1,2,3", 3))
}

func TestParseApprovalIndices_WithSpaces(t *testing.T) {
	assert.Equal(t, []int{1, 2}, parseApprovalIndices("1, 2", 3))
}

func TestParseApprovalIndices_Dedup(t *testing.T) {
	indices := parseApprovalIndices("1,1,2", 3)
	assert.Equal(t, []int{1, 2}, indices)
}

func TestParseApprovalIndices_Invalid(t *testing.T) {
	assert.Nil(t, parseApprovalIndices("abc", 3))
	assert.Nil(t, parseApprovalIndices("0", 3))
	assert.Nil(t, parseApprovalIndices("4", 3))
	assert.Nil(t, parseApprovalIndices("", 3))
	assert.Nil(t, parseApprovalIndices("-1", 3))
}

// --- Escalation input tests ---

func TestHandleEscalationInput_Yes(t *testing.T) {
	pending := []workflow.EscalationRequest{
		{CallID: "c1", ToolName: "shell"},
	}
	resp := HandleEscalationInput("y", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c1"}, resp.Approved)
}

func TestHandleEscalationInput_No(t *testing.T) {
	pending := []workflow.EscalationRequest{
		{CallID: "c1", ToolName: "shell"},
	}
	resp := HandleEscalationInput("n", pending)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"c1"}, resp.Denied)
}

func TestHandleEscalationInput_Invalid(t *testing.T) {
	pending := []workflow.EscalationRequest{
		{CallID: "c1", ToolName: "shell"},
	}
	resp := HandleEscalationInput("maybe", pending)
	assert.Nil(t, resp)
}
