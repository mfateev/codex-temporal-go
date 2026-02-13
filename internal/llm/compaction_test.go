package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mfateev/temporal-agent-harness/internal/models"
)

// --- collectRecentUserMessages tests ---

func TestCollectRecentUserMessages_WithinBudget(t *testing.T) {
	items := []models.ConversationItem{
		{Type: models.ItemTypeUserMessage, Content: "msg1"},
		{Type: models.ItemTypeAssistantMessage, Content: "reply1"},
		{Type: models.ItemTypeUserMessage, Content: "msg2"},
		{Type: models.ItemTypeAssistantMessage, Content: "reply2"},
	}

	// Large budget — all items should be collected
	result := collectRecentUserMessages(items, 100_000)
	assert.Len(t, result, 4)
	assert.Equal(t, "msg1", result[0].Content)
	assert.Equal(t, "reply2", result[3].Content)
}

func TestCollectRecentUserMessages_ExceedsBudget(t *testing.T) {
	items := []models.ConversationItem{
		{Type: models.ItemTypeUserMessage, Content: "old message that is quite long"},
		{Type: models.ItemTypeAssistantMessage, Content: "old reply that is also quite long"},
		{Type: models.ItemTypeUserMessage, Content: "new"},
		{Type: models.ItemTypeAssistantMessage, Content: "new reply"},
	}

	// Very small budget — should only get the last items
	// 5 tokens * 4 chars = 20 chars budget
	result := collectRecentUserMessages(items, 5)
	assert.True(t, len(result) < 4, "should not collect all items with tiny budget")
	assert.True(t, len(result) > 0, "should collect at least one item")
	// Last items should be the most recent
	assert.Equal(t, "new reply", result[len(result)-1].Content)
}

func TestCollectRecentUserMessages_Empty(t *testing.T) {
	result := collectRecentUserMessages(nil, 100_000)
	assert.Empty(t, result)
}

func TestCollectRecentUserMessages_SkipsMarkers(t *testing.T) {
	items := []models.ConversationItem{
		{Type: models.ItemTypeTurnStarted, TurnID: "t1"},
		{Type: models.ItemTypeUserMessage, Content: "msg1"},
		{Type: models.ItemTypeCompaction, Content: "compacted"},
		{Type: models.ItemTypeAssistantMessage, Content: "reply1"},
		{Type: models.ItemTypeTurnComplete, TurnID: "t1"},
	}

	result := collectRecentUserMessages(items, 100_000)
	// Should skip turn markers and compaction markers
	assert.Len(t, result, 2)
	for _, item := range result {
		assert.NotEqual(t, models.ItemTypeTurnStarted, item.Type)
		assert.NotEqual(t, models.ItemTypeTurnComplete, item.Type)
		assert.NotEqual(t, models.ItemTypeCompaction, item.Type)
	}
}

// --- buildCompactedHistory tests ---

func TestBuildCompactedHistory_CorrectStructure(t *testing.T) {
	recentItems := []models.ConversationItem{
		{Type: models.ItemTypeUserMessage, Content: "recent msg"},
		{Type: models.ItemTypeAssistantMessage, Content: "recent reply"},
	}

	result := buildCompactedHistory("This is the summary", recentItems)

	// Should be: compaction marker + summary + recent items
	assert.Len(t, result, 4)

	// First item: compaction marker
	assert.Equal(t, models.ItemTypeCompaction, result[0].Type)
	assert.Equal(t, "context_compacted", result[0].Content)

	// Second item: summary with prefix
	assert.Equal(t, models.ItemTypeAssistantMessage, result[1].Type)
	assert.Contains(t, result[1].Content, "Another language model started")
	assert.Contains(t, result[1].Content, "This is the summary")

	// Remaining items: recent items
	assert.Equal(t, models.ItemTypeUserMessage, result[2].Type)
	assert.Equal(t, "recent msg", result[2].Content)
	assert.Equal(t, models.ItemTypeAssistantMessage, result[3].Type)
	assert.Equal(t, "recent reply", result[3].Content)
}

func TestBuildCompactedHistory_EmptyRecentItems(t *testing.T) {
	result := buildCompactedHistory("Summary text", nil)

	assert.Len(t, result, 2)
	assert.Equal(t, models.ItemTypeCompaction, result[0].Type)
	assert.Equal(t, models.ItemTypeAssistantMessage, result[1].Type)
}

// --- extractLastAssistantMessage tests ---

func TestExtractLastAssistantMessage_FindsLast(t *testing.T) {
	items := []models.ConversationItem{
		{Type: models.ItemTypeAssistantMessage, Content: "first"},
		{Type: models.ItemTypeUserMessage, Content: "user msg"},
		{Type: models.ItemTypeAssistantMessage, Content: "second"},
		{Type: models.ItemTypeFunctionCall, Name: "shell"},
	}

	result := extractLastAssistantMessage(items)
	assert.Equal(t, "second", result)
}

func TestExtractLastAssistantMessage_HandlesEmpty(t *testing.T) {
	result := extractLastAssistantMessage(nil)
	assert.Equal(t, "", result)

	result = extractLastAssistantMessage([]models.ConversationItem{
		{Type: models.ItemTypeUserMessage, Content: "no assistant"},
	})
	assert.Equal(t, "", result)
}

func TestExtractLastAssistantMessage_SkipsEmptyContent(t *testing.T) {
	items := []models.ConversationItem{
		{Type: models.ItemTypeAssistantMessage, Content: "has content"},
		{Type: models.ItemTypeAssistantMessage, Content: ""},
	}

	result := extractLastAssistantMessage(items)
	assert.Equal(t, "has content", result)
}
