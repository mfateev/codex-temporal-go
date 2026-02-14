// Package shell provides user-shell detection and command argument derivation.
//
// Maps to: codex-rs/core/src/shell.rs
// Linux-only (no PowerShell/Cmd support needed).
package shell

import (
	"os"
	"path/filepath"
)

// ShellType enumerates the supported shell flavours.
//
// Maps to: codex-rs/core/src/shell.rs Shell enum
type ShellType int

const (
	ShellTypeBash ShellType = iota
	ShellTypeZsh
	ShellTypeSh
)

// Shell represents a detected shell with its binary path.
//
// Maps to: codex-rs/core/src/shell.rs Shell struct
type Shell struct {
	Type ShellType
	Path string
}

// Name returns the short name of the shell ("bash", "zsh", "sh").
func (s *Shell) Name() string {
	switch s.Type {
	case ShellTypeBash:
		return "bash"
	case ShellTypeZsh:
		return "zsh"
	case ShellTypeSh:
		return "sh"
	default:
		return "sh"
	}
}

// DeriveExecArgs builds the argument vector used to execute a command string
// through this shell. When useLoginShell is true the shell is invoked with -lc
// (login + command); otherwise with -c only.
//
// Maps to: codex-rs/core/src/shell.rs Shell::derive_exec_args
func (s *Shell) DeriveExecArgs(command string, useLoginShell bool) []string {
	if useLoginShell {
		return []string{s.Path, "-lc", command}
	}
	return []string{s.Path, "-c", command}
}

// DetectShellType maps a shell binary path (or bare name) to a ShellType.
// Returns the type and true on success, or (0, false) for unknown shells.
//
// Maps to: codex-rs/core/src/shell.rs detect_shell_type
func DetectShellType(shellPath string) (ShellType, bool) {
	base := filepath.Base(shellPath)
	switch base {
	case "bash":
		return ShellTypeBash, true
	case "zsh":
		return ShellTypeZsh, true
	case "sh":
		return ShellTypeSh, true
	default:
		return 0, false
	}
}

// DetectUserShell returns the user's default shell by reading $SHELL.
// Falls back to bash, then sh if $SHELL is unset or unrecognised.
//
// Maps to: codex-rs/core/src/shell.rs detect_user_shell
func DetectUserShell() *Shell {
	shellEnv := os.Getenv("SHELL")
	if shellEnv != "" {
		if st, ok := DetectShellType(shellEnv); ok {
			return &Shell{Type: st, Path: shellEnv}
		}
	}

	// Fallback: try bash, then sh.
	for _, candidate := range []struct {
		name string
		st   ShellType
	}{
		{"bash", ShellTypeBash},
		{"sh", ShellTypeSh},
	} {
		if p, err := lookPath(candidate.name); err == nil {
			return &Shell{Type: candidate.st, Path: p}
		}
	}

	// Ultimate fallback — assume /bin/sh exists.
	return &Shell{Type: ShellTypeSh, Path: "/bin/sh"}
}

// lookPath is a thin wrapper around exec.LookPath, declared as a var so tests
// can override it without touching the filesystem.
var lookPath = defaultLookPath

func defaultLookPath(name string) (string, error) {
	// Inline a minimal lookup to avoid importing os/exec in this package.
	// We search $PATH for an executable.
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		pathEnv = "/usr/local/bin:/usr/bin:/bin"
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		candidate := filepath.Join(dir, name)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}
