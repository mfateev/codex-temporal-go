// Package handlers contains built-in tool handler implementations.
//
// Corresponds to: codex-rs/core/src/tools/handlers/
package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/mfateev/temporal-agent-harness/internal/command_safety"
	execpkg "github.com/mfateev/temporal-agent-harness/internal/exec"
	"github.com/mfateev/temporal-agent-harness/internal/execenv"
	"github.com/mfateev/temporal-agent-harness/internal/sandbox"
	"github.com/mfateev/temporal-agent-harness/internal/shell"
	"github.com/mfateev/temporal-agent-harness/internal/tools"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// resolveWorkdir extracts a working directory from the invocation.
// It checks the "workdir" argument first, then falls back to invocation.Cwd.
func resolveWorkdir(invocation *tools.ToolInvocation) string {
	cwd := invocation.Cwd
	if workdirArg, ok := invocation.Arguments["workdir"]; ok {
		if wd, ok := workdirArg.(string); ok && wd != "" {
			cwd = wd
		}
	}
	return cwd
}

// executeCommand runs a command spec through the sandbox/env pipeline and
// returns the aggregated output. This is the shared execution path for both
// ShellHandler and ShellCommandHandler.
func executeCommand(
	ctx context.Context,
	spec sandbox.CommandSpec,
	invocation *tools.ToolInvocation,
	sandboxMgr sandbox.SandboxManager,
) (*tools.ToolOutput, error) {
	execEnv, err := resolveExecEnv(spec, invocation.SandboxPolicy, sandboxMgr)
	if err != nil {
		return nil, tools.NewValidationError("sandbox setup failed: " + err.Error())
	}

	cmd := exec.CommandContext(ctx, execEnv.Command[0], execEnv.Command[1:]...)
	if execEnv.Cwd != "" {
		cmd.Dir = execEnv.Cwd
	}

	// Apply environment variable filtering if an env policy is set.
	if invocation.EnvPolicy != nil {
		filteredEnv := resolveFilteredEnv(invocation.EnvPolicy)
		cmd.Env = execenv.EnvMapToSlice(filteredEnv)
	}

	// Apply sandbox environment variables (merged on top of any filtered env)
	if len(execEnv.Env) > 0 {
		if cmd.Env == nil {
			cmd.Env = os.Environ()
		}
		cmd.Env = appendEnvMap(cmd.Env, execEnv.Env)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()

	output := execpkg.AggregateOutput(stdoutBuf.Bytes(), stderrBuf.Bytes())

	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		success := false
		return &tools.ToolOutput{
			Content: string(output),
			Success: &success,
		}, nil
	}

	success := true
	return &tools.ToolOutput{
		Content: string(output),
		Success: &success,
	}, nil
}

// resolveExecEnv applies sandbox wrapping if a policy is set.
func resolveExecEnv(spec sandbox.CommandSpec, policyRef *tools.SandboxPolicyRef, sandboxMgr sandbox.SandboxManager) (*sandbox.ExecEnv, error) {
	if policyRef == nil || sandboxMgr == nil {
		return &sandbox.ExecEnv{
			Command: append([]string{spec.Program}, spec.Args...),
			Cwd:     spec.Cwd,
		}, nil
	}

	policy := sandboxPolicyRefToPolicy(policyRef)
	return sandboxMgr.Transform(spec, policy)
}

// sandboxPolicyRefToPolicy converts the serializable ref to a sandbox.SandboxPolicy.
func sandboxPolicyRefToPolicy(ref *tools.SandboxPolicyRef) *sandbox.SandboxPolicy {
	if ref == nil {
		return nil
	}
	roots := make([]sandbox.WritableRoot, len(ref.WritableRoots))
	for i, r := range ref.WritableRoots {
		roots[i] = sandbox.WritableRoot(r)
	}
	return &sandbox.SandboxPolicy{
		Mode:          sandbox.SandboxMode(ref.Mode),
		WritableRoots: roots,
		NetworkAccess: ref.NetworkAccess,
	}
}

// resolveFilteredEnv converts an EnvPolicyRef to a filtered environment map.
func resolveFilteredEnv(ref *tools.EnvPolicyRef) map[string]string {
	if ref == nil {
		return nil
	}
	policy := &execenv.ShellEnvironmentPolicy{
		Inherit:               execenv.Inherit(ref.Inherit),
		IgnoreDefaultExcludes: ref.IgnoreDefaultExcludes,
		Exclude:               ref.Exclude,
		Set:                   ref.Set,
		IncludeOnly:           ref.IncludeOnly,
	}
	return execenv.CreateEnv(policy)
}

// appendEnvMap appends key=value pairs from a map to an env slice.
func appendEnvMap(base []string, envMap map[string]string) []string {
	for k, v := range envMap {
		base = append(base, k+"="+v)
	}
	return base
}

// parseCommandArray parses the "command" argument as a []string from the
// JSON-decoded []interface{} that LLMs provide for the array-based shell tool.
func parseCommandArray(commandArg interface{}) ([]string, error) {
	arr, ok := commandArg.([]interface{})
	if !ok {
		return nil, fmt.Errorf("command must be an array of strings")
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("command array cannot be empty")
	}
	result := make([]string, len(arr))
	for i, v := range arr {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("command array element %d must be a string", i)
		}
		result[i] = s
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// ShellHandler — array-based "shell" tool (direct execvp)
// ---------------------------------------------------------------------------

// ShellHandler executes commands from an array of strings (direct execvp).
//
// Maps to: codex-rs/core/src/tools/handlers/shell.rs (shell variant)
type ShellHandler struct {
	sandboxMgr sandbox.SandboxManager
}

// NewShellHandler creates a new array-based shell handler.
func NewShellHandler() *ShellHandler {
	return &ShellHandler{sandboxMgr: sandbox.NewNoopSandboxManager()}
}

// NewShellHandlerWithSandbox creates an array-based shell handler with a sandbox manager.
func NewShellHandlerWithSandbox(mgr sandbox.SandboxManager) *ShellHandler {
	return &ShellHandler{sandboxMgr: mgr}
}

// Name returns "shell".
func (h *ShellHandler) Name() string { return "shell" }

// Kind returns ToolKindFunction.
func (h *ShellHandler) Kind() tools.ToolKind { return tools.ToolKindFunction }

// IsMutating parses the command array and classifies via IsKnownSafeCommand.
func (h *ShellHandler) IsMutating(invocation *tools.ToolInvocation) bool {
	commandArg, ok := invocation.Arguments["command"]
	if !ok {
		return true
	}
	cmdVec, err := parseCommandArray(commandArg)
	if err != nil || len(cmdVec) == 0 {
		return true
	}
	return !command_safety.IsKnownSafeCommand(cmdVec)
}

// Handle parses the command array and executes it via execvp (no shell wrapping).
func (h *ShellHandler) Handle(ctx context.Context, invocation *tools.ToolInvocation) (*tools.ToolOutput, error) {
	commandArg, ok := invocation.Arguments["command"]
	if !ok {
		return nil, tools.NewValidationError("missing required argument: command")
	}

	cmdVec, err := parseCommandArray(commandArg)
	if err != nil {
		return nil, tools.NewValidationError(err.Error())
	}

	cwd := resolveWorkdir(invocation)

	spec := sandbox.CommandSpec{
		Program: cmdVec[0],
		Args:    cmdVec[1:],
		Cwd:     cwd,
	}

	return executeCommand(ctx, spec, invocation, h.sandboxMgr)
}

// ---------------------------------------------------------------------------
// ShellCommandHandler — string-based "shell_command" tool (user's shell)
// ---------------------------------------------------------------------------

// ShellCommandHandler executes a command string through the user's login shell.
//
// Maps to: codex-rs/core/src/tools/handlers/shell.rs (shell_command variant)
type ShellCommandHandler struct {
	sandboxMgr sandbox.SandboxManager
}

// NewShellCommandHandler creates a new string-based shell command handler.
func NewShellCommandHandler() *ShellCommandHandler {
	return &ShellCommandHandler{sandboxMgr: sandbox.NewNoopSandboxManager()}
}

// NewShellCommandHandlerWithSandbox creates a string-based shell command handler
// with a sandbox manager.
func NewShellCommandHandlerWithSandbox(mgr sandbox.SandboxManager) *ShellCommandHandler {
	return &ShellCommandHandler{sandboxMgr: mgr}
}

// Name returns "shell_command".
func (h *ShellCommandHandler) Name() string { return "shell_command" }

// Kind returns ToolKindFunction.
func (h *ShellCommandHandler) Kind() tools.ToolKind { return tools.ToolKindFunction }

// IsMutating derives exec args via the user's shell and classifies via IsKnownSafeCommand.
func (h *ShellCommandHandler) IsMutating(invocation *tools.ToolInvocation) bool {
	commandArg, ok := invocation.Arguments["command"]
	if !ok {
		return true
	}
	command, ok := commandArg.(string)
	if !ok || command == "" {
		return true
	}

	login := parseLoginArg(invocation.Arguments)
	userShell := shell.DetectUserShell()
	cmdVec := userShell.DeriveExecArgs(command, login)
	return !command_safety.IsKnownSafeCommand(cmdVec)
}

// Handle executes a command string through the user's detected shell.
func (h *ShellCommandHandler) Handle(ctx context.Context, invocation *tools.ToolInvocation) (*tools.ToolOutput, error) {
	commandArg, ok := invocation.Arguments["command"]
	if !ok {
		return nil, tools.NewValidationError("missing required argument: command")
	}

	command, ok := commandArg.(string)
	if !ok {
		return nil, tools.NewValidationError("command must be a string")
	}

	if command == "" {
		return nil, tools.NewValidationError("command cannot be empty")
	}

	login := parseLoginArg(invocation.Arguments)
	cwd := resolveWorkdir(invocation)

	userShell := shell.DetectUserShell()
	execArgs := userShell.DeriveExecArgs(command, login)

	spec := sandbox.CommandSpec{
		Program: execArgs[0],
		Args:    execArgs[1:],
		Cwd:     cwd,
	}

	return executeCommand(ctx, spec, invocation, h.sandboxMgr)
}

// parseLoginArg extracts the "login" boolean from arguments, defaulting to true.
func parseLoginArg(args map[string]interface{}) bool {
	loginArg, ok := args["login"]
	if !ok {
		return true // default to login shell
	}
	b, ok := loginArg.(bool)
	if !ok {
		return true
	}
	return b
}

// ---------------------------------------------------------------------------
// Legacy compatibility aliases
// ---------------------------------------------------------------------------

// ShellTool is the legacy shell tool name. It is now an alias for
// ShellCommandHandler to preserve backward compatibility with existing
// handler registrations.
//
// Deprecated: Use NewShellHandler or NewShellCommandHandler directly.
type ShellTool = ShellCommandHandler

// NewShellTool creates a ShellCommandHandler (backward compat alias).
func NewShellTool() *ShellCommandHandler {
	return NewShellCommandHandler()
}

// NewShellToolWithSandbox creates a ShellCommandHandler with sandbox (backward compat alias).
func NewShellToolWithSandbox(mgr sandbox.SandboxManager) *ShellCommandHandler {
	return NewShellCommandHandlerWithSandbox(mgr)
}
