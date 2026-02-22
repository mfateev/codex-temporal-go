package memories

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tempDB(t *testing.T) *MemoryDB {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenMemoryDB(filepath.Join(dir, "test.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenMemoryDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "subdir", "test.sqlite")

	db, err := OpenMemoryDB(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Verify directory was created
	_, err = os.Stat(filepath.Dir(dbPath))
	assert.NoError(t, err)
}

func TestUpsertAndGet(t *testing.T) {
	db := tempDB(t)

	output := Stage1Output{
		WorkflowID:      "wf-1",
		SourceUpdatedAt: 1000,
		RawMemory:       "raw memory 1",
		RolloutSummary:  "summary 1",
		RolloutSlug:     "test-slug",
		Cwd:             "/tmp",
		GeneratedAt:     2000,
	}

	err := db.UpsertStage1Output(output)
	require.NoError(t, err)

	got, err := db.GetStage1Output("wf-1")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "wf-1", got.WorkflowID)
	assert.Equal(t, int64(1000), got.SourceUpdatedAt)
	assert.Equal(t, "raw memory 1", got.RawMemory)
	assert.Equal(t, "summary 1", got.RolloutSummary)
	assert.Equal(t, "test-slug", got.RolloutSlug)
	assert.Equal(t, "/tmp", got.Cwd)
	assert.Equal(t, int64(2000), got.GeneratedAt)
}

func TestUpsertTimestampGuard(t *testing.T) {
	db := tempDB(t)

	// Insert initial record
	err := db.UpsertStage1Output(Stage1Output{
		WorkflowID:      "wf-1",
		SourceUpdatedAt: 2000,
		RawMemory:       "new memory",
		RolloutSummary:  "new summary",
		Cwd:             "/tmp",
		GeneratedAt:     3000,
	})
	require.NoError(t, err)

	// Try to upsert with older timestamp — should not overwrite
	err = db.UpsertStage1Output(Stage1Output{
		WorkflowID:      "wf-1",
		SourceUpdatedAt: 1000,
		RawMemory:       "old memory",
		RolloutSummary:  "old summary",
		Cwd:             "/tmp",
		GeneratedAt:     1000,
	})
	require.NoError(t, err)

	got, err := db.GetStage1Output("wf-1")
	require.NoError(t, err)
	assert.Equal(t, "new memory", got.RawMemory)
	assert.Equal(t, int64(2000), got.SourceUpdatedAt)
}

func TestUpsertNewerTimestamp(t *testing.T) {
	db := tempDB(t)

	// Insert initial
	err := db.UpsertStage1Output(Stage1Output{
		WorkflowID:      "wf-1",
		SourceUpdatedAt: 1000,
		RawMemory:       "old memory",
		Cwd:             "/tmp",
		GeneratedAt:     1000,
	})
	require.NoError(t, err)

	// Upsert with newer timestamp — should overwrite
	err = db.UpsertStage1Output(Stage1Output{
		WorkflowID:      "wf-1",
		SourceUpdatedAt: 2000,
		RawMemory:       "new memory",
		RolloutSummary:  "new summary",
		Cwd:             "/home",
		GeneratedAt:     3000,
	})
	require.NoError(t, err)

	got, err := db.GetStage1Output("wf-1")
	require.NoError(t, err)
	assert.Equal(t, "new memory", got.RawMemory)
	assert.Equal(t, int64(2000), got.SourceUpdatedAt)
}

func TestGetNotFound(t *testing.T) {
	db := tempDB(t)

	got, err := db.GetStage1Output("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestListStage1OutputsForGlobal(t *testing.T) {
	db := tempDB(t)

	// Insert several outputs
	for i := 0; i < 5; i++ {
		err := db.UpsertStage1Output(Stage1Output{
			WorkflowID:      fmt.Sprintf("wf-%d", i),
			SourceUpdatedAt: int64(1000 + i*100),
			RawMemory:       fmt.Sprintf("memory %d", i),
			RolloutSummary:  fmt.Sprintf("summary %d", i),
			Cwd:             "/tmp",
			GeneratedAt:     int64(2000 + i),
		})
		require.NoError(t, err)
	}

	// List all
	results, err := db.ListStage1OutputsForGlobal(100)
	require.NoError(t, err)
	assert.Len(t, results, 5)
	// Should be ordered by source_updated_at DESC
	assert.Equal(t, "wf-4", results[0].WorkflowID)
	assert.Equal(t, "wf-0", results[4].WorkflowID)

	// List with limit
	results, err = db.ListStage1OutputsForGlobal(3)
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, "wf-4", results[0].WorkflowID)
}

func TestListEmpty(t *testing.T) {
	db := tempDB(t)

	results, err := db.ListStage1OutputsForGlobal(100)
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestNullSlug(t *testing.T) {
	db := tempDB(t)

	// Insert without slug
	err := db.UpsertStage1Output(Stage1Output{
		WorkflowID:      "wf-noslug",
		SourceUpdatedAt: 1000,
		RawMemory:       "memory",
		RolloutSummary:  "summary",
		Cwd:             "/tmp",
		GeneratedAt:     2000,
	})
	require.NoError(t, err)

	got, err := db.GetStage1Output("wf-noslug")
	require.NoError(t, err)
	assert.Equal(t, "", got.RolloutSlug)

	results, err := db.ListStage1OutputsForGlobal(10)
	require.NoError(t, err)
	assert.Equal(t, "", results[0].RolloutSlug)
}

func TestMultipleWorkflows(t *testing.T) {
	db := tempDB(t)

	// Insert records for different workflows
	for _, id := range []string{"wf-a", "wf-b", "wf-c"} {
		err := db.UpsertStage1Output(Stage1Output{
			WorkflowID:      id,
			SourceUpdatedAt: 1000,
			RawMemory:       "memory " + id,
			Cwd:             "/tmp",
			GeneratedAt:     2000,
		})
		require.NoError(t, err)
	}

	// Verify each exists
	for _, id := range []string{"wf-a", "wf-b", "wf-c"} {
		got, err := db.GetStage1Output(id)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "memory "+id, got.RawMemory)
	}
}
