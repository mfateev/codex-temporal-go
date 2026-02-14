package handlers

import (
	"context"
	"testing"

	"github.com/mfateev/temporal-agent-harness/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ShellCommandHandler tests (string-based, replaces legacy ShellTool tests)
// ---------------------------------------------------------------------------

func TestShellCommandHandler_IsMutating_SafeCommand(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{"command": "ls -la"},
	}
	assert.False(t, tool.IsMutating(invocation), "ls should be classified as non-mutating")
}

func TestShellCommandHandler_IsMutating_UnsafeCommand(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{"command": "rm -rf /tmp/test"},
	}
	assert.True(t, tool.IsMutating(invocation), "rm should be classified as mutating")
}

func TestShellCommandHandler_IsMutating_MissingCommand(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{},
	}
	assert.True(t, tool.IsMutating(invocation), "missing command should be classified as mutating")
}

func TestShellCommandHandler_IsMutating_GitStatus(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{"command": "git status"},
	}
	assert.False(t, tool.IsMutating(invocation), "git status should be classified as non-mutating")
}

func TestShellCommandHandler_IsMutating_GitPushForce(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{"command": "git push --force"},
	}
	assert.True(t, tool.IsMutating(invocation), "git push --force should be classified as mutating")
}

func TestShellCommandHandler_Handle_Success(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{"command": "echo hello"},
	}
	output, err := tool.Handle(context.Background(), invocation)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "hello\n", output.Content)
	require.NotNil(t, output.Success)
	assert.True(t, *output.Success)
}

func TestShellCommandHandler_Handle_Failure(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{"command": "exit 1"},
	}
	output, err := tool.Handle(context.Background(), invocation)
	require.NoError(t, err) // Non-zero exit is not a Go error
	require.NotNil(t, output)
	require.NotNil(t, output.Success)
	assert.False(t, *output.Success)
}

func TestShellCommandHandler_Handle_StderrCaptured(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{"command": "echo out && echo err >&2"},
	}
	output, err := tool.Handle(context.Background(), invocation)
	require.NoError(t, err)
	require.NotNil(t, output)
	// AggregateOutput concatenates stdout then stderr when under cap
	assert.Contains(t, output.Content, "out")
	assert.Contains(t, output.Content, "err")
}

func TestShellCommandHandler_Handle_MissingCommand(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{},
	}
	_, err := tool.Handle(context.Background(), invocation)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
}

func TestShellCommandHandler_Handle_EmptyCommand(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{"command": ""},
	}
	_, err := tool.Handle(context.Background(), invocation)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
}

func TestShellCommandHandler_LoginTrue(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{"command": "echo hello", "login": true},
	}
	output, err := tool.Handle(context.Background(), invocation)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Contains(t, output.Content, "hello")
	require.NotNil(t, output.Success)
	assert.True(t, *output.Success)
}

func TestShellCommandHandler_LoginFalse(t *testing.T) {
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{"command": "echo hello", "login": false},
	}
	output, err := tool.Handle(context.Background(), invocation)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Contains(t, output.Content, "hello")
	require.NotNil(t, output.Success)
	assert.True(t, *output.Success)
}

func TestShellCommandHandler_LoginDefault(t *testing.T) {
	// When login is not specified, it defaults to true (login shell)
	tool := NewShellCommandHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{"command": "echo default"},
	}
	output, err := tool.Handle(context.Background(), invocation)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Contains(t, output.Content, "default")
}

func TestShellCommandHandler_Name(t *testing.T) {
	tool := NewShellCommandHandler()
	assert.Equal(t, "shell_command", tool.Name())
}

// ---------------------------------------------------------------------------
// ShellHandler tests (array-based)
// ---------------------------------------------------------------------------

func TestShellHandler_Name(t *testing.T) {
	tool := NewShellHandler()
	assert.Equal(t, "shell", tool.Name())
}

func TestShellHandler_IsMutating_SafeCommand(t *testing.T) {
	tool := NewShellHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{
			"command": []interface{}{"ls", "-la"},
		},
	}
	assert.False(t, tool.IsMutating(invocation), "ls should be classified as non-mutating")
}

func TestShellHandler_IsMutating_UnsafeCommand(t *testing.T) {
	tool := NewShellHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{
			"command": []interface{}{"rm", "-rf", "/tmp/test"},
		},
	}
	assert.True(t, tool.IsMutating(invocation), "rm should be classified as mutating")
}

func TestShellHandler_IsMutating_MissingCommand(t *testing.T) {
	tool := NewShellHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{},
	}
	assert.True(t, tool.IsMutating(invocation), "missing command should be classified as mutating")
}

func TestShellHandler_IsMutating_EmptyArray(t *testing.T) {
	tool := NewShellHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{
			"command": []interface{}{},
		},
	}
	assert.True(t, tool.IsMutating(invocation), "empty array should be classified as mutating")
}

func TestShellHandler_IsMutating_StringInsteadOfArray(t *testing.T) {
	tool := NewShellHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{
			"command": "ls -la",
		},
	}
	assert.True(t, tool.IsMutating(invocation), "string instead of array should be classified as mutating")
}

func TestShellHandler_Handle_Success(t *testing.T) {
	tool := NewShellHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{
			"command": []interface{}{"echo", "hello"},
		},
	}
	output, err := tool.Handle(context.Background(), invocation)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "hello\n", output.Content)
	require.NotNil(t, output.Success)
	assert.True(t, *output.Success)
}

func TestShellHandler_Handle_BashWrapped(t *testing.T) {
	tool := NewShellHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{
			"command": []interface{}{"bash", "-c", "echo wrapped"},
		},
	}
	output, err := tool.Handle(context.Background(), invocation)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Contains(t, output.Content, "wrapped")
}

func TestShellHandler_Handle_MissingCommand(t *testing.T) {
	tool := NewShellHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{},
	}
	_, err := tool.Handle(context.Background(), invocation)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
}

func TestShellHandler_Handle_EmptyArray(t *testing.T) {
	tool := NewShellHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{
			"command": []interface{}{},
		},
	}
	_, err := tool.Handle(context.Background(), invocation)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
}

func TestShellHandler_Handle_NonStringElement(t *testing.T) {
	tool := NewShellHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{
			"command": []interface{}{"echo", 42},
		},
	}
	_, err := tool.Handle(context.Background(), invocation)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
}

func TestShellHandler_Handle_Failure(t *testing.T) {
	tool := NewShellHandler()
	invocation := &tools.ToolInvocation{
		Arguments: map[string]interface{}{
			"command": []interface{}{"bash", "-c", "exit 1"},
		},
	}
	output, err := tool.Handle(context.Background(), invocation)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotNil(t, output.Success)
	assert.False(t, *output.Success)
}

// ---------------------------------------------------------------------------
// Legacy ShellTool alias tests
// ---------------------------------------------------------------------------

func TestShellTool_Alias_IsShellCommandHandler(t *testing.T) {
	tool := NewShellTool()
	assert.Equal(t, "shell_command", tool.Name(), "NewShellTool should create a ShellCommandHandler")
}

// ---------------------------------------------------------------------------
// parseLoginArg
// ---------------------------------------------------------------------------

func TestParseLoginArg_Default(t *testing.T) {
	assert.True(t, parseLoginArg(map[string]interface{}{}), "default should be true")
}

func TestParseLoginArg_True(t *testing.T) {
	assert.True(t, parseLoginArg(map[string]interface{}{"login": true}))
}

func TestParseLoginArg_False(t *testing.T) {
	assert.False(t, parseLoginArg(map[string]interface{}{"login": false}))
}

func TestParseLoginArg_InvalidType(t *testing.T) {
	assert.True(t, parseLoginArg(map[string]interface{}{"login": "yes"}), "non-bool should default to true")
}
