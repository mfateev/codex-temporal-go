package handlers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mfateev/codex-temporal-go/internal/tools"
)

// Tests ported from: codex-rs/core/src/tools/handlers/grep_files.rs mod tests
// All 6 Rust unit tests are ported below, plus additional validation tests.

func rgAvailable() bool {
	cmd := exec.Command("rg", "--version")
	return cmd.Run() == nil
}

func skipIfNoRg(t *testing.T) {
	t.Helper()
	if !rgAvailable() {
		t.Skip("rg not available in PATH; skipping test")
	}
}

func newGrepInvocation(args map[string]interface{}) *tools.ToolInvocation {
	return &tools.ToolInvocation{
		CallID:    "test-call",
		ToolName:  "grep_files",
		Arguments: args,
	}
}

// Port of: parses_basic_results
func TestGrepFiles_ParsesBasicResults(t *testing.T) {
	stdout := []byte("/tmp/file_a.rs\n/tmp/file_b.rs\n")
	parsed := parseResults(stdout, 10)
	assert.Equal(t, []string{"/tmp/file_a.rs", "/tmp/file_b.rs"}, parsed)
}

// Port of: parse_truncates_after_limit
func TestGrepFiles_ParseTruncatesAfterLimit(t *testing.T) {
	stdout := []byte("/tmp/file_a.rs\n/tmp/file_b.rs\n/tmp/file_c.rs\n")
	parsed := parseResults(stdout, 2)
	assert.Equal(t, []string{"/tmp/file_a.rs", "/tmp/file_b.rs"}, parsed)
}

// Port of: run_search_returns_results
func TestGrepFiles_RunSearchReturnsResults(t *testing.T) {
	skipIfNoRg(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "match_one.txt"), []byte("alpha beta gamma"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "match_two.txt"), []byte("alpha delta"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte("omega"), 0o644))

	results, err := runRgSearch(context.Background(), "alpha", "", dir, 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	joined := joinResults(results)
	assert.Contains(t, joined, "match_one.txt")
	assert.Contains(t, joined, "match_two.txt")
}

// Port of: run_search_with_glob_filter
func TestGrepFiles_RunSearchWithGlobFilter(t *testing.T) {
	skipIfNoRg(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "match_one.rs"), []byte("alpha beta gamma"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "match_two.txt"), []byte("alpha delta"), 0o644))

	results, err := runRgSearch(context.Background(), "alpha", "*.rs", dir, 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	joined := joinResults(results)
	assert.Contains(t, joined, "match_one.rs")
}

// Port of: run_search_respects_limit
func TestGrepFiles_RunSearchRespectsLimit(t *testing.T) {
	skipIfNoRg(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "one.txt"), []byte("alpha one"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "two.txt"), []byte("alpha two"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "three.txt"), []byte("alpha three"), 0o644))

	results, err := runRgSearch(context.Background(), "alpha", "", dir, 2)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

// Port of: run_search_handles_no_matches
func TestGrepFiles_RunSearchHandlesNoMatches(t *testing.T) {
	skipIfNoRg(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "one.txt"), []byte("omega"), 0o644))

	results, err := runRgSearch(context.Background(), "alpha", "", dir, 5)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// Additional validation tests for the Handle method.

func TestGrepFiles_MissingPattern(t *testing.T) {
	tool := NewGrepFilesTool()
	inv := newGrepInvocation(map[string]interface{}{})

	_, err := tool.Handle(context.Background(), inv)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
	assert.Contains(t, err.Error(), "missing required argument: pattern")
}

func TestGrepFiles_PatternWrongType(t *testing.T) {
	tool := NewGrepFilesTool()
	inv := newGrepInvocation(map[string]interface{}{
		"pattern": 123,
	})

	_, err := tool.Handle(context.Background(), inv)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
	assert.Contains(t, err.Error(), "pattern must be a string")
}

func TestGrepFiles_EmptyPattern(t *testing.T) {
	tool := NewGrepFilesTool()
	inv := newGrepInvocation(map[string]interface{}{
		"pattern": "  ",
	})

	_, err := tool.Handle(context.Background(), inv)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
	assert.Contains(t, err.Error(), "pattern must not be empty")
}

func TestGrepFiles_ZeroLimit(t *testing.T) {
	tool := NewGrepFilesTool()
	inv := newGrepInvocation(map[string]interface{}{
		"pattern": "test",
		"limit":   float64(0),
	})

	_, err := tool.Handle(context.Background(), inv)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
	assert.Contains(t, err.Error(), "limit must be greater than zero")
}

func TestGrepFiles_NonexistentPath(t *testing.T) {
	skipIfNoRg(t)

	tool := NewGrepFilesTool()
	inv := newGrepInvocation(map[string]interface{}{
		"pattern": "test",
		"path":    "/tmp/nonexistent-grep-dir-" + t.Name(),
	})

	output, err := tool.Handle(context.Background(), inv)
	require.NoError(t, err) // filesystem errors are tool output, not Go errors
	require.NotNil(t, output.Success)
	assert.False(t, *output.Success)
	assert.Contains(t, output.Content, "unable to access")
}

func TestGrepFiles_NoMatchesReturnsSuccessFalse(t *testing.T) {
	skipIfNoRg(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("omega"), 0o644))

	tool := NewGrepFilesTool()
	inv := newGrepInvocation(map[string]interface{}{
		"pattern": "nonexistent_pattern_xyz",
		"path":    dir,
	})

	output, err := tool.Handle(context.Background(), inv)
	require.NoError(t, err)
	require.NotNil(t, output.Success)
	assert.False(t, *output.Success)
	assert.Equal(t, "No matches found.", output.Content)
}

func TestGrepFiles_HandleReturnsMatchingFiles(t *testing.T) {
	skipIfNoRg(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "match.txt"), []byte("needle in haystack"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "miss.txt"), []byte("no match here"), 0o644))

	tool := NewGrepFilesTool()
	inv := newGrepInvocation(map[string]interface{}{
		"pattern": "needle",
		"path":    dir,
	})

	output, err := tool.Handle(context.Background(), inv)
	require.NoError(t, err)
	require.NotNil(t, output.Success)
	assert.True(t, *output.Success)
	assert.Contains(t, output.Content, "match.txt")
	assert.NotContains(t, output.Content, "miss.txt")
}

func TestGrepFiles_ToolMetadata(t *testing.T) {
	tool := NewGrepFilesTool()
	assert.Equal(t, "grep_files", tool.Name())
	assert.Equal(t, tools.ToolKindFunction, tool.Kind())
	assert.False(t, tool.IsMutating(nil))
}

// joinResults concatenates results for substring assertions.
func joinResults(results []string) string {
	result := ""
	for _, r := range results {
		result += r + "\n"
	}
	return result
}
