package memories

import (
	"encoding/json"

	"github.com/mfateev/temporal-agent-harness/internal/models"
)

// serializableItem is a simplified view of a ConversationItem for memory extraction.
type serializableItem struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

// SerializeConversationForMemory converts conversation items into a JSON string
// suitable for memory extraction. Filters out internal-only item types.
// Maps to: codex-rs/core/src/memories/phase1.rs serialize_filtered_rollout_response_items
func SerializeConversationForMemory(items []models.ConversationItem) (string, error) {
	var filtered []serializableItem
	for _, item := range items {
		if !shouldIncludeForMemory(item) {
			continue
		}
		si := serializableItem{
			Type: string(item.Type),
		}
		switch item.Type {
		case models.ItemTypeUserMessage:
			si.Content = item.Content
		case models.ItemTypeAssistantMessage:
			si.Content = item.Content
		case models.ItemTypeFunctionCall:
			si.Name = item.Name
			si.Arguments = item.Arguments
		case models.ItemTypeFunctionCallOutput:
			if item.Output != nil {
				si.Output = item.Output.Content
			}
		case models.ItemTypeWebSearchCall:
			si.Content = item.WebSearchURL
		default:
			si.Content = item.Content
		}
		filtered = append(filtered, si)
	}

	data, err := json.Marshal(filtered)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// shouldIncludeForMemory returns true if the item should be included in memory extraction.
// Maps to: codex-rs/core/src/rollout/policy.rs should_persist_response_item_for_memories
func shouldIncludeForMemory(item models.ConversationItem) bool {
	switch item.Type {
	case models.ItemTypeUserMessage,
		models.ItemTypeAssistantMessage,
		models.ItemTypeFunctionCall,
		models.ItemTypeFunctionCallOutput,
		models.ItemTypeWebSearchCall:
		return true
	case models.ItemTypeTurnStarted,
		models.ItemTypeTurnComplete,
		models.ItemTypeCompaction,
		models.ItemTypeModelSwitch:
		return false
	default:
		return false
	}
}

// TruncateToTokenLimit keeps text within an approximate token limit.
// Uses head+tail strategy to preserve context from both ends.
func TruncateToTokenLimit(text string, limit int) string {
	// Rough estimate: ~4 chars per token
	charLimit := limit * 4
	return TruncateToCharLimit(text, charLimit)
}
