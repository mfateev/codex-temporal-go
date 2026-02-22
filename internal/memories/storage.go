package memories

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// EnsureLayout creates the memory folder structure if it doesn't exist.
// Maps to: codex-rs/core/src/memories/storage.rs ensure_layout
func EnsureLayout(root string) error {
	dirs := []string{
		root,
		filepath.Join(root, RolloutSummariesSubdir),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("memories: ensure layout: %w", err)
		}
	}
	return nil
}

// RebuildRawMemoriesFile writes the merged raw_memories.md file.
// Maps to: codex-rs/core/src/memories/storage.rs rebuild_raw_memories_file_from_memories
func RebuildRawMemoriesFile(root string, memories []Stage1Output, maxCount int) error {
	if err := EnsureLayout(root); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# Raw Memories\n\nMerged stage-1 raw memories (latest first):\n\n")

	count := 0
	for _, m := range memories {
		if count >= maxCount {
			break
		}
		if strings.TrimSpace(m.RawMemory) == "" {
			continue
		}
		updatedAt := time.Unix(m.SourceUpdatedAt, 0).UTC().Format(time.RFC3339)
		fmt.Fprintf(&b, "## Thread `%s`\nupdated_at: %s\ncwd: %s\n\n%s\n\n",
			m.WorkflowID, updatedAt, m.Cwd, strings.TrimSpace(m.RawMemory))
		count++
	}

	return os.WriteFile(filepath.Join(root, RawMemoriesFilename), []byte(b.String()), 0o644)
}

// SyncRolloutSummaries writes per-rollout summary files and prunes stale ones.
// Maps to: codex-rs/core/src/memories/storage.rs sync_rollout_summaries_from_memories
func SyncRolloutSummaries(root string, memories []Stage1Output, maxCount int) error {
	if err := EnsureLayout(root); err != nil {
		return err
	}

	summariesDir := filepath.Join(root, RolloutSummariesSubdir)
	keep := make(map[string]bool)

	count := 0
	for _, m := range memories {
		if count >= maxCount {
			break
		}
		if strings.TrimSpace(m.RolloutSummary) == "" {
			continue
		}
		if err := WriteRolloutSummaryForThread(root, m); err != nil {
			return err
		}
		filename := RolloutSummaryFileStem(m) + ".md"
		keep[filename] = true
		count++
	}

	return PruneRolloutSummaries(summariesDir, keep)
}

// WriteRolloutSummaryForThread writes a single rollout summary file.
// Maps to: codex-rs/core/src/memories/storage.rs (inline in sync)
func WriteRolloutSummaryForThread(root string, memory Stage1Output) error {
	summariesDir := filepath.Join(root, RolloutSummariesSubdir)
	filename := RolloutSummaryFileStem(memory) + ".md"

	updatedAt := time.Unix(memory.SourceUpdatedAt, 0).UTC().Format(time.RFC3339)
	content := fmt.Sprintf("workflow_id: %s\nupdated_at: %s\ncwd: %s\n\n%s\n",
		memory.WorkflowID, updatedAt, memory.Cwd,
		strings.TrimSpace(memory.RolloutSummary))

	return os.WriteFile(filepath.Join(summariesDir, filename), []byte(content), 0o644)
}

// slugRegex matches characters that are NOT lowercase alphanumeric or underscore.
var slugRegex = regexp.MustCompile(`[^a-z0-9_]`)

// RolloutSummaryFileStem returns the filename stem for a rollout summary.
// Format: {workflow_id}[-{sanitized_slug}]
// Maps to: codex-rs/core/src/memories/storage.rs
func RolloutSummaryFileStem(memory Stage1Output) string {
	// Sanitize workflow ID for filesystem use (replace / and other chars)
	sanitizedID := slugRegex.ReplaceAllString(strings.ToLower(memory.WorkflowID), "_")
	if len(sanitizedID) > 60 {
		sanitizedID = sanitizedID[:60]
	}

	if memory.RolloutSlug == "" {
		return sanitizedID
	}

	// Sanitize slug: lowercase, only alphanumeric + underscore, max 20 chars
	slug := strings.ToLower(memory.RolloutSlug)
	slug = strings.ReplaceAll(slug, "-", "_")
	slug = slugRegex.ReplaceAllString(slug, "")
	slug = strings.TrimRight(slug, "_")
	if len(slug) > 20 {
		slug = slug[:20]
	}
	slug = strings.TrimRight(slug, "_")

	if slug == "" {
		return sanitizedID
	}
	return sanitizedID + "-" + slug
}

// PruneRolloutSummaries removes files in the summaries directory that are not
// in the keep set.
// Maps to: codex-rs/core/src/memories/storage.rs (prune stale)
func PruneRolloutSummaries(summariesDir string, keep map[string]bool) error {
	entries, err := os.ReadDir(summariesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("memories: read summaries dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !keep[entry.Name()] {
			_ = os.Remove(filepath.Join(summariesDir, entry.Name()))
		}
	}
	return nil
}

// ReadMemorySummary reads the memory_summary.md file and truncates to maxTokens.
// Maps to: codex-rs/core/src/memories/storage.rs
func ReadMemorySummary(root string, maxTokens int) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, MemorySummaryFilename))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("memories: read memory_summary.md: %w", err)
	}

	content := string(data)
	if maxTokens > 0 {
		content = TruncateToCharLimit(content, maxTokens*4) // ~4 chars per token
	}
	return content, nil
}

// TruncateToCharLimit truncates text to maxChars, keeping head and tail.
func TruncateToCharLimit(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}
	if maxChars <= 0 {
		return ""
	}

	// Keep 80% head, 20% tail
	headSize := maxChars * 4 / 5
	tailSize := maxChars - headSize - 40 // reserve space for truncation marker
	if tailSize < 0 {
		tailSize = 0
	}

	head := text[:headSize]
	tail := ""
	if tailSize > 0 && tailSize < len(text) {
		tail = text[len(text)-tailSize:]
	}

	return head + "\n\n... [truncated] ...\n\n" + tail
}
