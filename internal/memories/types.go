// Package memories implements cross-session learning via LLM-powered memory
// extraction and consolidation.
//
// Maps to: codex-rs/core/src/memories/ + codex-rs/state/src/model/memories.rs
package memories

// Stage1Output is a single phase-1 extraction result (one per session/workflow).
// Maps to: codex-rs/state/src/model/memories.rs Stage1Output
type Stage1Output struct {
	WorkflowID      string `json:"workflow_id"`
	SourceUpdatedAt int64  `json:"source_updated_at"` // epoch seconds
	RawMemory       string `json:"raw_memory"`
	RolloutSummary  string `json:"rollout_summary"`
	RolloutSlug     string `json:"rollout_slug,omitempty"`
	Cwd             string `json:"cwd"`
	GeneratedAt     int64  `json:"generated_at"` // epoch seconds
}

// Subdirectory and file name constants for the memory folder layout.
// Maps to: codex-rs/core/src/memories/mod.rs
const (
	// RolloutSummariesSubdir is the subdirectory for per-rollout summary files.
	RolloutSummariesSubdir = "rollout_summaries"

	// RawMemoriesFilename is the merged raw memories file.
	RawMemoriesFilename = "raw_memories.md"

	// MemoryMDFilename is the clustered handbook.
	MemoryMDFilename = "MEMORY.md"

	// MemorySummaryFilename is the compact navigational summary.
	MemorySummaryFilename = "memory_summary.md"

	// DefaultRolloutTokenLimit is the max tokens for a rollout sent to phase-1.
	DefaultRolloutTokenLimit = 150000

	// MemorySummaryMaxTokens is the default max tokens for memory_summary.md
	// when injected into developer instructions.
	MemorySummaryMaxTokens = 5000

	// ContextWindowPercentForRollout is the fraction of context window used.
	ContextWindowPercentForRollout = 0.70

	// ConsolidationWorkflowID is the fixed workflow ID for the singleton
	// consolidation workflow.
	ConsolidationWorkflowID = "memory-consolidation"

	// SignalNewRolloutSummary is the signal name sent to the consolidation workflow.
	SignalNewRolloutSummary = "new_rollout_summary"
)
