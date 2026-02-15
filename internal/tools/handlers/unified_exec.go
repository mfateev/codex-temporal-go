package handlers

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/mfateev/temporal-agent-harness/internal/command_safety"
	"github.com/mfateev/temporal-agent-harness/internal/execsession"
	"github.com/mfateev/temporal-agent-harness/internal/shell"
	"github.com/mfateev/temporal-agent-harness/internal/tools"
)

// Yield time constants matching Codex.
// Maps to: codex-rs/core/src/unified_exec/mod.rs
const (
	MinYieldTimeMs      = 250
	MaxYieldTimeMs      = 30_000
	MinEmptyYieldTimeMs = 5_000 // Minimum for empty polls (prevent rapid polling).
	DefaultExecYieldMs  = 10_000
	DefaultStdinYieldMs = 250
)

// Unified exec environment variables set for all exec sessions.
// Ensures consistent, non-colored output for LLM consumption.
// Maps to: codex-rs/core/src/unified_exec/process_manager.rs UNIFIED_EXEC_ENV
var unifiedExecEnv = map[string]string{
	"NO_COLOR":  "1",
	"TERM":      "dumb",
	"LANG":      "C.UTF-8",
	"LC_CTYPE":  "C.UTF-8",
	"LC_ALL":    "C.UTF-8",
	"COLORTERM": "",
	"PAGER":     "cat",
	"GIT_PAGER": "cat",
	"GH_PAGER":  "cat",
}

// UnifiedExecHandler implements the shared logic for exec_command and write_stdin.
//
// Maps to: codex-rs/core/src/tools/handlers/unified_exec.rs UnifiedExecHandler
type UnifiedExecHandler struct {
	store *execsession.Store
}

// NewUnifiedExecHandler creates a handler backed by the given session store.
func NewUnifiedExecHandler(store *execsession.Store) *UnifiedExecHandler {
	return &UnifiedExecHandler{store: store}
}

// ExecCommandHandler is the ToolHandler wrapper for exec_command.
type ExecCommandHandler struct {
	h *UnifiedExecHandler
}

// NewExecCommandHandler creates an exec_command handler.
func NewExecCommandHandler(store *execsession.Store) *ExecCommandHandler {
	return &ExecCommandHandler{h: NewUnifiedExecHandler(store)}
}

func (h *ExecCommandHandler) Name() string                    { return "exec_command" }
func (h *ExecCommandHandler) Kind() tools.ToolKind            { return tools.ToolKindFunction }
func (h *ExecCommandHandler) IsMutating(inv *tools.ToolInvocation) bool { return h.h.isMutatingExecCommand(inv) }
func (h *ExecCommandHandler) Handle(ctx context.Context, inv *tools.ToolInvocation) (*tools.ToolOutput, error) {
	return h.h.handleExecCommand(ctx, inv)
}

// WriteStdinHandler is the ToolHandler wrapper for write_stdin.
type WriteStdinHandler struct {
	h *UnifiedExecHandler
}

// NewWriteStdinHandler creates a write_stdin handler.
func NewWriteStdinHandler(store *execsession.Store) *WriteStdinHandler {
	return &WriteStdinHandler{h: NewUnifiedExecHandler(store)}
}

func (h *WriteStdinHandler) Name() string                    { return "write_stdin" }
func (h *WriteStdinHandler) Kind() tools.ToolKind            { return tools.ToolKindFunction }
func (h *WriteStdinHandler) IsMutating(_ *tools.ToolInvocation) bool { return false }
func (h *WriteStdinHandler) Handle(ctx context.Context, inv *tools.ToolInvocation) (*tools.ToolOutput, error) {
	return h.h.handleWriteStdin(ctx, inv)
}

// ---------------------------------------------------------------------------
// exec_command implementation
// ---------------------------------------------------------------------------

func (h *UnifiedExecHandler) isMutatingExecCommand(inv *tools.ToolInvocation) bool {
	cmdStr, ok := inv.Arguments["cmd"].(string)
	if !ok || cmdStr == "" {
		return true
	}
	login := parseBoolArg(inv.Arguments, "login", true)
	userShell := shell.DetectUserShell()
	cmdVec := userShell.DeriveExecArgs(cmdStr, login)
	return !command_safety.IsKnownSafeCommand(cmdVec)
}

func (h *UnifiedExecHandler) handleExecCommand(ctx context.Context, inv *tools.ToolInvocation) (*tools.ToolOutput, error) {
	cmdStr, ok := inv.Arguments["cmd"].(string)
	if !ok || cmdStr == "" {
		return nil, tools.NewValidationError("missing required argument: cmd")
	}

	tty := parseBoolArg(inv.Arguments, "tty", false)
	login := parseBoolArg(inv.Arguments, "login", true)
	yieldMs := parseNumberArg(inv.Arguments, "yield_time_ms", DefaultExecYieldMs)
	yieldMs = clampYieldTime(yieldMs, MinYieldTimeMs, MaxYieldTimeMs)

	cwd := resolveWorkdir(inv)

	// Resolve shell.
	shellBin := ""
	if s, ok := inv.Arguments["shell"].(string); ok && s != "" {
		shellBin = s
	}

	// Build command via user's shell.
	var cmdVec []string
	if shellBin != "" {
		if login {
			cmdVec = []string{shellBin, "-lc", cmdStr}
		} else {
			cmdVec = []string{shellBin, "-c", cmdStr}
		}
	} else {
		userShell := shell.DetectUserShell()
		cmdVec = userShell.DeriveExecArgs(cmdStr, login)
	}

	// Build environment: inherit + unified exec env.
	env := buildExecEnv(inv)

	// Allocate process ID.
	processID := h.store.AllocateID()

	startTime := time.Now()

	sess, err := execsession.StartSession(execsession.SessionOpts{
		ProcessID: processID,
		Command:   cmdVec,
		Cwd:       cwd,
		Env:       env,
		TTY:       tty,
	})
	if err != nil {
		h.store.ReleaseID(processID)
		return nil, tools.NewValidationError(fmt.Sprintf("failed to start command: %v", err))
	}

	// Collect output up to yield_time deadline.
	deadline := time.Now().Add(time.Duration(yieldMs) * time.Millisecond)
	output := sess.CollectOutput(deadline, inv.Heartbeat)
	wallTime := time.Since(startTime)

	// Check if process exited during collection.
	if sess.HasExited() {
		h.store.ReleaseID(processID)
		return formatExecResponse(output, wallTime, sess.ExitCode(), ""), nil
	}

	// Long-running: store the session.
	h.store.Store(sess)
	return formatExecResponse(output, wallTime, nil, processID), nil
}

// ---------------------------------------------------------------------------
// write_stdin implementation
// ---------------------------------------------------------------------------

func (h *UnifiedExecHandler) handleWriteStdin(ctx context.Context, inv *tools.ToolInvocation) (*tools.ToolOutput, error) {
	sessionIDRaw, ok := inv.Arguments["session_id"]
	if !ok {
		return nil, tools.NewValidationError("missing required argument: session_id")
	}
	sessionID := fmt.Sprintf("%d", int(parseNumberArg(inv.Arguments, "session_id", 0)))
	if sessionID == "0" {
		// Try parsing as float (JSON numbers are float64).
		if f, ok := sessionIDRaw.(float64); ok {
			sessionID = fmt.Sprintf("%d", int(f))
		} else {
			return nil, tools.NewValidationError("session_id must be a number")
		}
	}

	chars, _ := inv.Arguments["chars"].(string)
	yieldMs := parseNumberArg(inv.Arguments, "yield_time_ms", DefaultStdinYieldMs)

	// Clamp yield time: empty writes get longer minimum.
	if chars == "" {
		yieldMs = clampYieldTime(yieldMs, MinEmptyYieldTimeMs, MaxYieldTimeMs)
	} else {
		yieldMs = clampYieldTime(yieldMs, MinYieldTimeMs, MaxYieldTimeMs)
	}

	sess, err := h.store.Get(sessionID)
	if err != nil {
		success := false
		return &tools.ToolOutput{
			Content: fmt.Sprintf("Unknown session ID: %s. The process may have already exited.", sessionID),
			Success: &success,
		}, nil
	}

	startTime := time.Now()

	// Write input if non-empty.
	if chars != "" {
		if err := sess.WriteStdin([]byte(chars)); err != nil {
			success := false
			return &tools.ToolOutput{
				Content: fmt.Sprintf("Failed to write to stdin: %v", err),
				Success: &success,
			}, nil
		}
		// Brief pause for process to react.
		time.Sleep(100 * time.Millisecond)
	}

	// Collect new output.
	deadline := time.Now().Add(time.Duration(yieldMs) * time.Millisecond)
	output := sess.CollectOutput(deadline, inv.Heartbeat)
	wallTime := time.Since(startTime)

	// Check if process exited.
	if sess.HasExited() {
		h.store.Remove(sessionID)
		return formatExecResponse(output, wallTime, sess.ExitCode(), ""), nil
	}

	return formatExecResponse(output, wallTime, nil, sessionID), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// formatExecResponse formats the tool response matching Codex's format_response.
// Maps to: codex-rs/core/src/tools/handlers/unified_exec.rs format_response
func formatExecResponse(output []byte, wallTime time.Duration, exitCode *int, sessionID string) *tools.ToolOutput {
	var result string
	result += fmt.Sprintf("--- Wall time: %.3fs ---\n", wallTime.Seconds())
	if exitCode != nil {
		result += fmt.Sprintf("--- Exit code: %d ---\n", *exitCode)
	}
	if sessionID != "" {
		result += fmt.Sprintf("--- Session ID: %s ---\n", sessionID)
	}
	result += "--- Output ---\n"
	if len(output) > 0 {
		result += string(output)
	}

	success := exitCode == nil || *exitCode == 0
	return &tools.ToolOutput{
		Content: result,
		Success: &success,
	}
}

// buildExecEnv creates the environment for exec sessions:
// base OS environment + unified exec vars overlaid.
func buildExecEnv(inv *tools.ToolInvocation) []string {
	env := os.Environ()
	for k, v := range unifiedExecEnv {
		env = append(env, k+"="+v)
	}
	return env
}

// parseBoolArg extracts a boolean argument with a default value.
func parseBoolArg(args map[string]interface{}, key string, defaultVal bool) bool {
	v, ok := args[key]
	if !ok {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// parseNumberArg extracts a numeric argument with a default value.
func parseNumberArg(args map[string]interface{}, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return defaultVal
	}
}

// clampYieldTime constrains yield time to [min, max].
func clampYieldTime(ms, minMs, maxMs int) int {
	return int(math.Max(float64(minMs), math.Min(float64(ms), float64(maxMs))))
}
