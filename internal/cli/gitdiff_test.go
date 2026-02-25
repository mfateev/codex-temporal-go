package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunGitDiff_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	result := runGitDiff(dir)
	assert.Equal(t, "Not in a git repository.", result)
}

func TestRunGitDiff_NoChanges(t *testing.T) {
	dir := initTestGitRepo(t)
	result := runGitDiff(dir)
	assert.Equal(t, "No changes detected.", result)
}

func TestRunGitDiff_TrackedChanges(t *testing.T) {
	dir := initTestGitRepo(t)

	// Modify the committed file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("modified\n"), 0644))

	result := runGitDiff(dir)
	assert.Contains(t, result, "modified")
	assert.NotEqual(t, "No changes detected.", result)
}

func TestRunGitDiff_StagedChanges(t *testing.T) {
	dir := initTestGitRepo(t)

	// Create and stage a new file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged content\n"), 0644))
	gitCmd(t, dir, "add", "staged.txt")

	result := runGitDiff(dir)
	assert.Contains(t, result, "staged content")
}

func TestRunGitDiff_UntrackedFiles(t *testing.T) {
	dir := initTestGitRepo(t)

	// Create an untracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new file\n"), 0644))

	result := runGitDiff(dir)
	assert.Contains(t, result, "untracked.txt")
}

// initTestGitRepo creates a temporary git repo with one committed file.
func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "config", "user.name", "Test")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("initial\n"), 0644))
	gitCmd(t, dir, "add", "file.txt")
	gitCmd(t, dir, "commit", "-m", "initial")
	return dir
}

// gitCmd runs a git command in the given directory.
func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}
