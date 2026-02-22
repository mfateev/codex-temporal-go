package memories

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mfateev/temporal-agent-harness/internal/models"
)

func TestSerializeConversationForMemory(t *testing.T) {
	items := []models.ConversationItem{
		{Type: models.ItemTypeUserMessage, Content: "hello"},
		{Type: models.ItemTypeAssistantMessage, Content: "hi there"},
		{Type: models.ItemTypeFunctionCall, Name: "read_file", Arguments: `{"path":"/tmp/test"}`, CallID: "call-1"},
		{Type: models.ItemTypeFunctionCallOutput, CallID: "call-1", Output: &models.FunctionCallOutputPayload{Content: "file content"}},
	}

	result, err := SerializeConversationForMemory(items)
	require.NoError(t, err)

	// Verify it's valid JSON
	var parsed []serializableItem
	err = json.Unmarshal([]byte(result), &parsed)
	require.NoError(t, err)
	assert.Len(t, parsed, 4)

	assert.Equal(t, "user_message", parsed[0].Type)
	assert.Equal(t, "hello", parsed[0].Content)
	assert.Equal(t, "assistant_message", parsed[1].Type)
	assert.Equal(t, "function_call", parsed[2].Type)
	assert.Equal(t, "read_file", parsed[2].Name)
	assert.Equal(t, "function_call_output", parsed[3].Type)
	assert.Equal(t, "file content", parsed[3].Output)
}

func TestSerializeFiltersInternalItems(t *testing.T) {
	items := []models.ConversationItem{
		{Type: models.ItemTypeUserMessage, Content: "hello"},
		{Type: models.ItemTypeTurnStarted, TurnID: "turn-1"},
		{Type: models.ItemTypeAssistantMessage, Content: "hi"},
		{Type: models.ItemTypeTurnComplete, TurnID: "turn-1"},
		{Type: models.ItemTypeCompaction, Content: "compacted"},
		{Type: models.ItemTypeModelSwitch, Content: "switched to gpt-4"},
	}

	result, err := SerializeConversationForMemory(items)
	require.NoError(t, err)

	var parsed []serializableItem
	err = json.Unmarshal([]byte(result), &parsed)
	require.NoError(t, err)

	// Only user_message and assistant_message should be included
	assert.Len(t, parsed, 2)
	assert.Equal(t, "user_message", parsed[0].Type)
	assert.Equal(t, "assistant_message", parsed[1].Type)
}

func TestSerializeEmptyHistory(t *testing.T) {
	result, err := SerializeConversationForMemory(nil)
	require.NoError(t, err)
	assert.Equal(t, "null", result)
}

func TestTruncateToTokenLimit(t *testing.T) {
	// Short text: no truncation
	result := TruncateToTokenLimit("hello", 100)
	assert.Equal(t, "hello", result)

	// Long text: truncated
	long := strings.Repeat("word ", 100000) // ~500k chars
	result = TruncateToTokenLimit(long, 1000) // ~4000 chars
	assert.Less(t, len(result), 100000)
}

func TestShouldIncludeForMemory(t *testing.T) {
	tests := []struct {
		itemType models.ConversationItemType
		included bool
	}{
		{models.ItemTypeUserMessage, true},
		{models.ItemTypeAssistantMessage, true},
		{models.ItemTypeFunctionCall, true},
		{models.ItemTypeFunctionCallOutput, true},
		{models.ItemTypeWebSearchCall, true},
		{models.ItemTypeTurnStarted, false},
		{models.ItemTypeTurnComplete, false},
		{models.ItemTypeCompaction, false},
		{models.ItemTypeModelSwitch, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.itemType), func(t *testing.T) {
			item := models.ConversationItem{Type: tt.itemType}
			assert.Equal(t, tt.included, shouldIncludeForMemory(item))
		})
	}
}
