package memories

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// MemoryDB wraps a SQLite database for stage-1 output persistence.
// Maps to: codex-rs/state/src/runtime/memories.rs
type MemoryDB struct {
	db *sql.DB
}

// migration creates the stage1_outputs table and index.
const migration = `
CREATE TABLE IF NOT EXISTS stage1_outputs (
    workflow_id TEXT PRIMARY KEY,
    source_updated_at INTEGER NOT NULL,
    raw_memory TEXT NOT NULL,
    rollout_summary TEXT NOT NULL,
    rollout_slug TEXT,
    cwd TEXT NOT NULL DEFAULT '',
    generated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_stage1_source_updated
    ON stage1_outputs(source_updated_at DESC, workflow_id DESC);
`

// OpenMemoryDB opens (or creates) the SQLite database at path and runs
// migrations. The parent directory is created if it does not exist.
func OpenMemoryDB(path string) (*MemoryDB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("memories: create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("memories: open sqlite: %w", err)
	}

	// Enable WAL mode for better concurrency.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("memories: set WAL mode: %w", err)
	}

	if _, err := db.Exec(migration); err != nil {
		db.Close()
		return nil, fmt.Errorf("memories: run migration: %w", err)
	}

	return &MemoryDB{db: db}, nil
}

// Close closes the underlying database connection.
func (m *MemoryDB) Close() error {
	return m.db.Close()
}

// UpsertStage1Output inserts or replaces a stage-1 output.
// The replacement only happens if the new source_updated_at is >= the existing one.
// Maps to: codex-rs/state/src/runtime/memories.rs mark_stage1_job_succeeded
func (m *MemoryDB) UpsertStage1Output(output Stage1Output) error {
	_, err := m.db.Exec(`
		INSERT INTO stage1_outputs (workflow_id, source_updated_at, raw_memory, rollout_summary, rollout_slug, cwd, generated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workflow_id) DO UPDATE SET
			source_updated_at = excluded.source_updated_at,
			raw_memory = excluded.raw_memory,
			rollout_summary = excluded.rollout_summary,
			rollout_slug = excluded.rollout_slug,
			cwd = excluded.cwd,
			generated_at = excluded.generated_at
		WHERE excluded.source_updated_at >= stage1_outputs.source_updated_at
	`,
		output.WorkflowID,
		output.SourceUpdatedAt,
		output.RawMemory,
		output.RolloutSummary,
		output.RolloutSlug,
		output.Cwd,
		output.GeneratedAt,
	)
	if err != nil {
		return fmt.Errorf("memories: upsert stage1_output: %w", err)
	}
	return nil
}

// ListStage1OutputsForGlobal returns the most recent stage-1 outputs,
// ordered by source_updated_at DESC.
// Maps to: codex-rs/state/src/runtime/memories.rs list_stage1_outputs_for_global
func (m *MemoryDB) ListStage1OutputsForGlobal(limit int) ([]Stage1Output, error) {
	rows, err := m.db.Query(`
		SELECT workflow_id, source_updated_at, raw_memory, rollout_summary, rollout_slug, cwd, generated_at
		FROM stage1_outputs
		ORDER BY source_updated_at DESC, workflow_id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("memories: list stage1_outputs: %w", err)
	}
	defer rows.Close()

	var results []Stage1Output
	for rows.Next() {
		var o Stage1Output
		var slug sql.NullString
		if err := rows.Scan(&o.WorkflowID, &o.SourceUpdatedAt, &o.RawMemory, &o.RolloutSummary, &slug, &o.Cwd, &o.GeneratedAt); err != nil {
			return nil, fmt.Errorf("memories: scan stage1_output: %w", err)
		}
		if slug.Valid {
			o.RolloutSlug = slug.String
		}
		results = append(results, o)
	}
	return results, rows.Err()
}

// GetStage1Output returns a single stage-1 output by workflow ID, or nil if not found.
func (m *MemoryDB) GetStage1Output(workflowID string) (*Stage1Output, error) {
	var o Stage1Output
	var slug sql.NullString
	err := m.db.QueryRow(`
		SELECT workflow_id, source_updated_at, raw_memory, rollout_summary, rollout_slug, cwd, generated_at
		FROM stage1_outputs
		WHERE workflow_id = ?
	`, workflowID).Scan(&o.WorkflowID, &o.SourceUpdatedAt, &o.RawMemory, &o.RolloutSummary, &slug, &o.Cwd, &o.GeneratedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memories: get stage1_output: %w", err)
	}
	if slug.Valid {
		o.RolloutSlug = slug.String
	}
	return &o, nil
}
