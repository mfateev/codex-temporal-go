package cli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// runGitDiff collects unstaged, staged, and untracked file diffs from the
// working directory. Returns a human-readable summary or an error message.
func runGitDiff(cwd string) string {
	// Check if we're inside a git repository.
	check := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	check.Dir = cwd
	if err := check.Run(); err != nil {
		return "Not in a git repository."
	}

	var sections []string

	// 1. Unstaged changes to tracked files.
	unstaged := execGit(cwd, "diff")
	if unstaged != "" {
		sections = append(sections, unstaged)
	}

	// 2. Staged changes.
	staged := execGit(cwd, "diff", "--cached")
	if staged != "" {
		sections = append(sections, staged)
	}

	// 3. Untracked files — show their content as diffs.
	untracked := execGit(cwd, "ls-files", "--others", "--exclude-standard")
	if untracked != "" {
		for _, file := range strings.Split(strings.TrimSpace(untracked), "\n") {
			file = strings.TrimSpace(file)
			if file == "" {
				continue
			}
			// git diff --no-index exits 1 when files differ (normal).
			cmd := exec.Command("git", "diff", "--no-index", "--", "/dev/null", file)
			cmd.Dir = cwd
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			_ = cmd.Run()
			if s := strings.TrimSpace(out.String()); s != "" {
				sections = append(sections, s)
			}
		}
	}

	if len(sections) == 0 {
		return "No changes detected."
	}
	return strings.Join(sections, "\n")
}

// execGit runs a git command in cwd and returns trimmed stdout, or "" on error.
func execGit(cwd string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// runGitDiffCmd returns a tea.Cmd that runs git diff in a goroutine.
func runGitDiffCmd(cwd string) tea.Cmd {
	return func() tea.Msg {
		// Resolve to absolute path for safety.
		abs, err := filepath.Abs(cwd)
		if err != nil {
			abs = cwd
		}
		return DiffResultMsg{Output: runGitDiff(abs)}
	}
}
