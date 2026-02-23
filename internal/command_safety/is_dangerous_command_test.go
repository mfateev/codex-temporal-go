package command_safety

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Maps to: codex-rs/shell-command/src/command_safety/is_dangerous_command.rs tests
// NOTE: Git commands removed from dangerous checks per Codex PR #11510.

func TestRmRfIsDangerous(t *testing.T) {
	assert.True(t, CommandMightBeDangerous([]string{"rm", "-rf", "/"}))
}

func TestRmFIsDangerous(t *testing.T) {
	assert.True(t, CommandMightBeDangerous([]string{"rm", "-f", "/"}))
}

func TestSudoRmIsDangerous(t *testing.T) {
	assert.True(t, CommandMightBeDangerous([]string{"sudo", "rm", "-rf", "/"}))
}

// Git commands are no longer considered dangerous (PR #11510).

func TestGitResetIsNotDangerous(t *testing.T) {
	assert.False(t, CommandMightBeDangerous([]string{"git", "reset"}))
	assert.False(t, CommandMightBeDangerous([]string{"git", "reset", "--hard"}))
}

func TestGitPushForceIsNotDangerous(t *testing.T) {
	assert.False(t, CommandMightBeDangerous([]string{"git", "push", "--force", "origin", "main"}))
	assert.False(t, CommandMightBeDangerous([]string{"git", "push", "-f", "origin", "main"}))
	assert.False(t, CommandMightBeDangerous([]string{"git", "push", "--force-with-lease", "origin", "main"}))
}

func TestGitPushDeleteIsNotDangerous(t *testing.T) {
	assert.False(t, CommandMightBeDangerous([]string{"git", "push", "--delete", "origin", "feature"}))
	assert.False(t, CommandMightBeDangerous([]string{"git", "push", "origin", ":feature"}))
	assert.False(t, CommandMightBeDangerous([]string{"git", "push", "origin", "+main"}))
}

func TestGitBranchDeleteIsNotDangerous(t *testing.T) {
	assert.False(t, CommandMightBeDangerous([]string{"git", "branch", "-d", "feature"}))
	assert.False(t, CommandMightBeDangerous([]string{"git", "branch", "-D", "feature"}))
}

func TestGitCleanIsNotDangerous(t *testing.T) {
	assert.False(t, CommandMightBeDangerous([]string{"git", "clean", "-fdx"}))
	assert.False(t, CommandMightBeDangerous([]string{"git", "clean", "--force"}))
}

func TestBashGitResetIsNotDangerous(t *testing.T) {
	assert.False(t, CommandMightBeDangerous([]string{"bash", "-lc", "git reset --hard"}))
}

func TestGitStatusIsNotDangerous(t *testing.T) {
	assert.False(t, CommandMightBeDangerous([]string{"git", "status"}))
}

func TestGitPushWithoutForceIsNotDangerous(t *testing.T) {
	assert.False(t, CommandMightBeDangerous([]string{"git", "push", "origin", "main"}))
}
