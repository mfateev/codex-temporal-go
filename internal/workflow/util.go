// Package workflow contains Temporal workflow definitions.
//
// util.go contains small utility functions used across the workflow package.
package workflow

import (
	"crypto/sha256"
	"fmt"
	"sort"

	"go.temporal.io/sdk/workflow"

	"github.com/mfateev/temporal-agent-harness/internal/models"
)

// generateTurnID generates a unique turn ID using Temporal's SideEffect.
func generateTurnID(ctx workflow.Context) string {
	var nanos int64
	encoded := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		return workflow.Now(ctx).UnixNano()
	})
	_ = encoded.Get(&nanos)
	return fmt.Sprintf("turn-%d", nanos)
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
