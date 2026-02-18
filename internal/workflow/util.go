// Package workflow contains Temporal workflow definitions.
//
// util.go contains small utility functions used across the workflow package.
package workflow

import (
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/mfateev/temporal-agent-harness/internal/models"
)

// nextTurnID increments the session turn counter and returns a unique turn ID.
// Using a counter rather than a side-effect keeps determinism without Temporal overhead.
func (s *SessionState) nextTurnID() string {
	s.TurnCounter++
	return fmt.Sprintf("turn-%d", s.TurnCounter)
}

// truncate returns s truncated to n bytes with "..." appended if it was longer.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// toInt64 converts a JSON-decoded number (float64) to int64.
func toInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

// toolCallsKey produces a deterministic hash for a batch of tool calls
// based on tool names and arguments, used for repeat detection.
func toolCallsKey(calls []models.ConversationItem) string {
	// Build a sorted list of "name:args" strings for deterministic ordering.
	parts := make([]string, len(calls))
	for i, c := range calls {
		parts[i] = c.Name + ":" + c.Arguments
	}
	sort.Strings(parts)
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// extractFunctionCalls filters items to return only FunctionCall items.
func extractFunctionCalls(items []models.ConversationItem) []models.ConversationItem {
	var calls []models.ConversationItem
	for _, item := range items {
		if item.Type == models.ItemTypeFunctionCall {
			calls = append(calls, item)
		}
	}
	return calls
}
