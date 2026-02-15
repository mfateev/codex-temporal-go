package handlers

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mfateev/temporal-agent-harness/internal/execsession"
	"github.com/mfateev/temporal-agent-harness/internal/tools"
)

func newExecInvocation(args map[string]interface{}) *tools.ToolInvocation {
	return &tools.ToolInvocation{
		CallID:    "test-call",
		ToolName:  "exec_command",
		Arguments: args,
		Cwd:       "/tmp",
	}
}

func newStdinInvocation(args map[string]interface{}) *tools.ToolInvocation {
	return &tools.ToolInvocation{
		CallID:    "test-call",
		ToolName:  "write_stdin",
		Arguments: args,
		Cwd:       "/tmp",
	}
}

// ---------------------------------------------------------------------------
// exec_command tests
// ---------------------------------------------------------------------------

func TestExecCommand_ShortLivedCommand(t *testing.T) {
	store := execsession.NewStore()
	handler := NewExecCommandHandler(store)
	ctx := context.Background()

	inv := newExecInvocation(map[string]interface{}{
		"cmd":           "echo hello from exec",
		"yield_time_ms": float64(5000),
	})

	output, err := handler.Handle(ctx, inv)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.Contains(t, output.Content, "hello from exec")
	assert.Contains(t, output.Content, "Exit code: 0")
	assert.NotContains(t, output.Content, "Session ID:")
	assert.True(t, *output.Success)
	assert.Equal(t, 0, store.Count(), "short-lived process should not be stored")
}

func TestExecCommand_LongRunningCommand(t *testing.T) {
	store := execsession.NewStore()
	handler := NewExecCommandHandler(store)
	ctx := context.Background()

	inv := newExecInvocation(map[string]interface{}{
		"cmd":           "sh -c 'echo starting; sleep 60'",
		"yield_time_ms": float64(1000), // Short yield to test session persistence
	})

	output, err := handler.Handle(ctx, inv)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.Contains(t, output.Content, "starting")
	assert.Contains(t, output.Content, "Session ID:")
	assert.NotContains(t, output.Content, "Exit code:")
	assert.True(t, *output.Success)
	assert.Equal(t, 1, store.Count(), "long-running process should be stored")

	// Clean up.
	// Extract session ID from output for cleanup.
	for _, line := range strings.Split(output.Content, "\n") {
		if strings.HasPrefix(line, "--- Session ID:") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				sess, err := store.Get(parts[3])
				if err == nil {
					sess.Close()
				}
				store.Remove(parts[3])
			}
		}
	}
}

func TestExecCommand_NonZeroExit(t *testing.T) {
	store := execsession.NewStore()
	handler := NewExecCommandHandler(store)
	ctx := context.Background()

	inv := newExecInvocation(map[string]interface{}{
		"cmd":           "sh -c 'echo oops; exit 1'",
		"yield_time_ms": float64(5000),
	})

	output, err := handler.Handle(ctx, inv)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.Contains(t, output.Content, "oops")
	assert.Contains(t, output.Content, "Exit code: 1")
	assert.False(t, *output.Success)
}

func TestExecCommand_MissingCmd(t *testing.T) {
	store := execsession.NewStore()
	handler := NewExecCommandHandler(store)
	ctx := context.Background()

	inv := newExecInvocation(map[string]interface{}{})

	_, err := handler.Handle(ctx, inv)
	assert.Error(t, err)
}

func TestExecCommand_TTYMode(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("PTY tests require Linux or macOS")
	}

	store := execsession.NewStore()
	handler := NewExecCommandHandler(store)
	ctx := context.Background()

	inv := newExecInvocation(map[string]interface{}{
		"cmd":           "echo pty test",
		"tty":           true,
		"yield_time_ms": float64(5000),
	})

	output, err := handler.Handle(ctx, inv)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.Contains(t, output.Content, "pty test")
}

func TestExecCommand_IsMutating_SafeCommand(t *testing.T) {
	store := execsession.NewStore()
	handler := NewExecCommandHandler(store)

	inv := newExecInvocation(map[string]interface{}{
		"cmd": "ls -la",
	})
	assert.False(t, handler.IsMutating(inv), "ls should be safe")
}

func TestExecCommand_IsMutating_UnsafeCommand(t *testing.T) {
	store := execsession.NewStore()
	handler := NewExecCommandHandler(store)

	inv := newExecInvocation(map[string]interface{}{
		"cmd": "rm -rf /",
	})
	assert.True(t, handler.IsMutating(inv), "rm should be mutating")
}

func TestExecCommand_IsMutating_EmptyCmd(t *testing.T) {
	store := execsession.NewStore()
	handler := NewExecCommandHandler(store)

	inv := newExecInvocation(map[string]interface{}{})
	assert.True(t, handler.IsMutating(inv), "empty cmd should be mutating")
}

// ---------------------------------------------------------------------------
// write_stdin tests
// ---------------------------------------------------------------------------

func TestWriteStdin_UnknownSession(t *testing.T) {
	store := execsession.NewStore()
	handler := NewWriteStdinHandler(store)
	ctx := context.Background()

	inv := newStdinInvocation(map[string]interface{}{
		"session_id": float64(9999),
		"chars":      "hello\n",
	})

	output, err := handler.Handle(ctx, inv)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.Contains(t, output.Content, "Unknown session ID")
	assert.False(t, *output.Success)
}

func TestWriteStdin_MissingSessionID(t *testing.T) {
	store := execsession.NewStore()
	handler := NewWriteStdinHandler(store)
	ctx := context.Background()

	inv := newStdinInvocation(map[string]interface{}{
		"chars": "hello\n",
	})

	_, err := handler.Handle(ctx, inv)
	assert.Error(t, err)
}

func TestWriteStdin_IsMutating_AlwaysFalse(t *testing.T) {
	store := execsession.NewStore()
	handler := NewWriteStdinHandler(store)

	inv := newStdinInvocation(map[string]interface{}{
		"session_id": float64(1234),
		"chars":      "rm -rf /\n",
	})
	assert.False(t, handler.IsMutating(inv), "write_stdin should never be mutating")
}

// ---------------------------------------------------------------------------
// exec_command + write_stdin integration
// ---------------------------------------------------------------------------

func TestExecThenWriteStdin_PTY(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("PTY tests require Linux or macOS")
	}

	store := execsession.NewStore()
	execHandler := NewExecCommandHandler(store)
	stdinHandler := NewWriteStdinHandler(store)
	ctx := context.Background()

	// Start cat in PTY mode (persists as a session).
	inv := newExecInvocation(map[string]interface{}{
		"cmd":           "cat",
		"tty":           true,
		"yield_time_ms": float64(1000),
	})

	execOut, err := execHandler.Handle(ctx, inv)
	require.NoError(t, err)
	require.NotNil(t, execOut)
	assert.Contains(t, execOut.Content, "Session ID:")
	assert.Equal(t, 1, store.Count())

	// Extract session ID.
	var sessionID string
	for _, line := range strings.Split(execOut.Content, "\n") {
		if strings.HasPrefix(line, "--- Session ID:") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				sessionID = parts[3]
			}
		}
	}
	require.NotEmpty(t, sessionID, "should have a session ID")

	// Write to the session.
	stdinInv := newStdinInvocation(map[string]interface{}{
		"session_id":    parseSessionIDForTest(sessionID),
		"chars":         "hello from stdin\n",
		"yield_time_ms": float64(2000),
	})

	stdinOut, err := stdinHandler.Handle(ctx, stdinInv)
	require.NoError(t, err)
	require.NotNil(t, stdinOut)
	assert.Contains(t, stdinOut.Content, "hello from stdin")

	// Clean up: close the session.
	sess, err := store.Get(sessionID)
	require.NoError(t, err)
	sess.Close()
	store.Remove(sessionID)
}

// ---------------------------------------------------------------------------
// Yield time clamping tests
// ---------------------------------------------------------------------------

func TestClampYieldTime(t *testing.T) {
	tests := []struct {
		name     string
		ms       int
		min, max int
		want     int
	}{
		{"below min", 100, 250, 30000, 250},
		{"at min", 250, 250, 30000, 250},
		{"in range", 5000, 250, 30000, 5000},
		{"at max", 30000, 250, 30000, 30000},
		{"above max", 60000, 250, 30000, 30000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clampYieldTime(tt.ms, tt.min, tt.max))
		})
	}
}

func TestFormatExecResponse_ShortLived(t *testing.T) {
	exitCode := 0
	resp := formatExecResponse([]byte("hello\n"), 1234*time.Millisecond, &exitCode, "")

	assert.Contains(t, resp.Content, "Wall time: 1.234s")
	assert.Contains(t, resp.Content, "Exit code: 0")
	assert.NotContains(t, resp.Content, "Session ID:")
	assert.Contains(t, resp.Content, "hello\n")
	assert.True(t, *resp.Success)
}

func TestFormatExecResponse_LongRunning(t *testing.T) {
	resp := formatExecResponse([]byte("output\n"), 500*time.Millisecond, nil, "12345")

	assert.Contains(t, resp.Content, "Wall time: 0.500s")
	assert.NotContains(t, resp.Content, "Exit code:")
	assert.Contains(t, resp.Content, "Session ID: 12345")
	assert.Contains(t, resp.Content, "output\n")
	assert.True(t, *resp.Success)
}

func TestFormatExecResponse_FailedExit(t *testing.T) {
	exitCode := 1
	resp := formatExecResponse([]byte("error\n"), 100*time.Millisecond, &exitCode, "")

	assert.Contains(t, resp.Content, "Exit code: 1")
	assert.False(t, *resp.Success)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseSessionIDForTest(id string) float64 {
	// JSON numbers come as float64 from LLM.
	var f float64
	for _, c := range id {
		f = f*10 + float64(c-'0')
	}
	return f
}
