package memories

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureLayout(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "memories")

	err := EnsureLayout(root)
	require.NoError(t, err)

	// Verify directories were created
	info, err := os.Stat(root)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	info, err = os.Stat(filepath.Join(root, RolloutSummariesSubdir))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestRolloutSummaryFileStem(t *testing.T) {
	tests := []struct {
		name     string
		memory   Stage1Output
		contains string
	}{
		{
			name:     "no slug",
			memory:   Stage1Output{WorkflowID: "wf-123"},
			contains: "wf_123",
		},
		{
			name:     "with slug",
			memory:   Stage1Output{WorkflowID: "wf-1", RolloutSlug: "fix-auth-bug"},
			contains: "fix_auth_bug",
		},
		{
			name:     "long slug truncated",
			memory:   Stage1Output{WorkflowID: "wf-1", RolloutSlug: "this-is-a-very-long-slug-that-should-be-truncated"},
			contains: "this_is_a_very_long",
		},
		{
			name:     "special chars stripped",
			memory:   Stage1Output{WorkflowID: "wf-1", RolloutSlug: "Fix-Auth-Bug"},
			contains: "fix_auth_bug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stem := RolloutSummaryFileStem(tt.memory)
			assert.Contains(t, stem, tt.contains)
		})
	}
}

func TestRebuildRawMemoriesFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memories")

	memories := []Stage1Output{
		{
			WorkflowID:      "wf-1",
			SourceUpdatedAt: 1000,
			RawMemory:       "memory one",
			Cwd:             "/home/dev",
		},
		{
			WorkflowID:      "wf-2",
			SourceUpdatedAt: 2000,
			RawMemory:       "memory two",
			Cwd:             "/tmp",
		},
	}

	err := RebuildRawMemoriesFile(root, memories, 10)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(root, RawMemoriesFilename))
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "# Raw Memories")
	assert.Contains(t, content, "memory one")
	assert.Contains(t, content, "memory two")
	assert.Contains(t, content, "wf-1")
	assert.Contains(t, content, "wf-2")
}

func TestRebuildRawMemoriesFileMaxCount(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memories")

	mems := []Stage1Output{
		{WorkflowID: "wf-1", SourceUpdatedAt: 1000, RawMemory: "mem1", Cwd: "/tmp"},
		{WorkflowID: "wf-2", SourceUpdatedAt: 2000, RawMemory: "mem2", Cwd: "/tmp"},
		{WorkflowID: "wf-3", SourceUpdatedAt: 3000, RawMemory: "mem3", Cwd: "/tmp"},
	}

	err := RebuildRawMemoriesFile(root, mems, 2)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(root, RawMemoriesFilename))
	require.NoError(t, err)
	content := string(data)

	// Only first 2 should be included
	assert.Contains(t, content, "mem1")
	assert.Contains(t, content, "mem2")
	assert.NotContains(t, content, "mem3")
}

func TestRebuildSkipsEmptyMemories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memories")

	mems := []Stage1Output{
		{WorkflowID: "wf-1", SourceUpdatedAt: 1000, RawMemory: "real memory", Cwd: "/tmp"},
		{WorkflowID: "wf-2", SourceUpdatedAt: 2000, RawMemory: "   ", Cwd: "/tmp"},
		{WorkflowID: "wf-3", SourceUpdatedAt: 3000, RawMemory: "", Cwd: "/tmp"},
	}

	err := RebuildRawMemoriesFile(root, mems, 10)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(root, RawMemoriesFilename))
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "real memory")
	assert.Equal(t, 1, strings.Count(content, "## Thread"))
}

func TestSyncRolloutSummaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memories")

	mems := []Stage1Output{
		{WorkflowID: "wf-1", SourceUpdatedAt: 1000, RolloutSummary: "summary 1", Cwd: "/tmp"},
		{WorkflowID: "wf-2", SourceUpdatedAt: 2000, RolloutSummary: "summary 2", Cwd: "/home"},
	}

	err := SyncRolloutSummaries(root, mems, 10)
	require.NoError(t, err)

	// Verify files were created
	entries, err := os.ReadDir(filepath.Join(root, RolloutSummariesSubdir))
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	// Verify content of one
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(root, RolloutSummariesSubdir, entry.Name()))
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "workflow_id:")
		assert.Contains(t, content, "updated_at:")
	}
}

func TestSyncRolloutSummariesPrunesOld(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memories")
	require.NoError(t, EnsureLayout(root))

	// Create a stale file
	summariesDir := filepath.Join(root, RolloutSummariesSubdir)
	require.NoError(t, os.WriteFile(filepath.Join(summariesDir, "old-file.md"), []byte("stale"), 0o644))

	// Sync with new data
	mems := []Stage1Output{
		{WorkflowID: "wf-new", SourceUpdatedAt: 1000, RolloutSummary: "new summary", Cwd: "/tmp"},
	}

	err := SyncRolloutSummaries(root, mems, 10)
	require.NoError(t, err)

	// Stale file should be pruned
	entries, err := os.ReadDir(summariesDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotEqual(t, "old-file.md", e.Name(), "stale file should be pruned")
	}
}

func TestReadMemorySummary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memories")
	require.NoError(t, os.MkdirAll(root, 0o755))

	// No file — should return empty
	summary, err := ReadMemorySummary(root, 1000)
	require.NoError(t, err)
	assert.Equal(t, "", summary)

	// Write a file
	content := "## User Profile\nTest user."
	require.NoError(t, os.WriteFile(filepath.Join(root, MemorySummaryFilename), []byte(content), 0o644))

	summary, err = ReadMemorySummary(root, 1000)
	require.NoError(t, err)
	assert.Equal(t, content, summary)
}

func TestReadMemorySummaryTruncation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memories")
	require.NoError(t, os.MkdirAll(root, 0o755))

	// Write a large file
	content := strings.Repeat("x", 100000)
	require.NoError(t, os.WriteFile(filepath.Join(root, MemorySummaryFilename), []byte(content), 0o644))

	summary, err := ReadMemorySummary(root, 100) // 100 tokens ≈ 400 chars
	require.NoError(t, err)
	assert.Less(t, len(summary), 100000)
	assert.Contains(t, summary, "[truncated]")
}

func TestTruncateToCharLimit(t *testing.T) {
	// Short text: no truncation
	assert.Equal(t, "hello", TruncateToCharLimit("hello", 100))

	// Long text: truncated
	long := strings.Repeat("a", 1000)
	result := TruncateToCharLimit(long, 200)
	assert.Less(t, len(result), 1000)
	assert.Contains(t, result, "[truncated]")

	// Zero limit
	assert.Equal(t, "", TruncateToCharLimit("hello", 0))
}
