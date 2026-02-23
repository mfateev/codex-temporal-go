package command_safety

import (
	"path/filepath"
	"strings"
)

// CommandMightBeDangerous returns true if the command is potentially destructive.
//
// Maps to: codex-rs/core/src/command_safety/is_dangerous_command.rs command_might_be_dangerous
func CommandMightBeDangerous(command []string) bool {
	if isDangerousToCallWithExec(command) {
		return true
	}

	// Support `bash -lc "<script>"` where any part of the script might contain a dangerous command.
	if allCommands := ParseShellLcPlainCommands(command); allCommands != nil {
		for _, cmd := range allCommands {
			if isDangerousToCallWithExec(cmd) {
				return true
			}
		}
	}

	return false
}

// FindGitSubcommand finds the first matching git subcommand, skipping global options.
// Shared between safe and dangerous command checks.
//
// Maps to: codex-rs/core/src/command_safety/is_dangerous_command.rs find_git_subcommand
func FindGitSubcommand(command []string, subcommands []string) (idx int, name string, found bool) {
	if len(command) == 0 {
		return 0, "", false
	}

	cmd0 := command[0]
	base := filepath.Base(cmd0)
	if base != "git" {
		return 0, "", false
	}

	skipNext := false
	for i := 1; i < len(command); i++ {
		if skipNext {
			skipNext = false
			continue
		}

		arg := command[i]

		if isGitGlobalOptionWithInlineValue(arg) {
			continue
		}

		if isGitGlobalOptionWithValue(arg) {
			skipNext = true
			continue
		}

		if arg == "--" || strings.HasPrefix(arg, "-") {
			continue
		}

		for _, sub := range subcommands {
			if arg == sub {
				return i, arg, true
			}
		}

		// In git, the first non-option token is the subcommand. If it isn't
		// one of the subcommands we're looking for, we must stop scanning to
		// avoid misclassifying later positional args (e.g., branch names).
		return 0, "", false
	}

	return 0, "", false
}

func isGitGlobalOptionWithValue(arg string) bool {
	switch arg {
	case "-C", "-c", "--config-env", "--exec-path", "--git-dir", "--namespace", "--super-prefix", "--work-tree":
		return true
	}
	return false
}

func isGitGlobalOptionWithInlineValue(arg string) bool {
	if strings.HasPrefix(arg, "--config-env=") ||
		strings.HasPrefix(arg, "--exec-path=") ||
		strings.HasPrefix(arg, "--git-dir=") ||
		strings.HasPrefix(arg, "--namespace=") ||
		strings.HasPrefix(arg, "--super-prefix=") ||
		strings.HasPrefix(arg, "--work-tree=") {
		return true
	}
	// -C<value> or -c<value> (len > 2 means inline value)
	if (strings.HasPrefix(arg, "-C") || strings.HasPrefix(arg, "-c")) && len(arg) > 2 {
		return true
	}
	return false
}

// isDangerousToCallWithExec checks if a command is truly destructive.
//
// NOTE: Git commands were removed from dangerous checks per Codex PR #11510.
// Only truly destructive operations (rm -f, sudo) remain.
func isDangerousToCallWithExec(command []string) bool {
	if len(command) == 0 {
		return false
	}

	cmd0 := command[0]

	switch {
	case cmd0 == "rm":
		if len(command) > 1 {
			arg1 := command[1]
			if arg1 == "-f" || arg1 == "-rf" {
				return true
			}
		}
		return false

	case cmd0 == "sudo":
		if len(command) > 1 {
			return isDangerousToCallWithExec(command[1:])
		}
		return false

	default:
		return false
	}
}
