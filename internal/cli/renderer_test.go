package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mfateev/codex-temporal-go/internal/models"
	"github.com/mfateev/codex-temporal-go/internal/workflow"
)

func TestRenderer_RenderAssistantMessage(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true, true) // noColor=true for testing

	rendered := r.RenderItem(models.ConversationItem{
		Type:    models.ItemTypeAssistantMessage,
		Content: "Hello, world!",
	})

	assert.True(t, rendered)
	assert.Contains(t, buf.String(), "Hello, world!")
}

func TestRenderer_RenderFunctionCall(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true, true)

	rendered := r.RenderItem(models.ConversationItem{
		Type:      models.ItemTypeFunctionCall,
		Name:      "shell",
		Arguments: `{"command": "echo hello"}`,
	})

	assert.True(t, rendered)
	assert.Contains(t, buf.String(), "shell")
	assert.Contains(t, buf.String(), "echo hello")
}

func TestRenderer_RenderFunctionCallOutput_Success(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true, true)

	success := true
	rendered := r.RenderItem(models.ConversationItem{
		Type:   models.ItemTypeFunctionCallOutput,
		CallID: "call-1",
		Output: &models.FunctionCallOutputPayload{
			Content: "hello\n",
			Success: &success,
		},
	})

	assert.True(t, rendered)
	assert.Contains(t, buf.String(), "hello")
}

func TestRenderer_RenderFunctionCallOutput_Failure(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true, true)

	failure := false
	rendered := r.RenderItem(models.ConversationItem{
		Type:   models.ItemTypeFunctionCallOutput,
		CallID: "call-1",
		Output: &models.FunctionCallOutputPayload{
			Content: "command not found",
			Success: &failure,
		},
	})

	assert.True(t, rendered)
	assert.Contains(t, buf.String(), "command not found")
}

func TestRenderer_RenderTurnStarted(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true, true)

	rendered := r.RenderItem(models.ConversationItem{
		Type:   models.ItemTypeTurnStarted,
		TurnID: "turn-123",
	})

	assert.True(t, rendered)
	assert.Contains(t, buf.String(), "turn-123")
}

func TestRenderer_TurnCompleteNotRendered(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true, true)

	rendered := r.RenderItem(models.ConversationItem{
		Type:   models.ItemTypeTurnComplete,
		TurnID: "turn-123",
	})

	assert.False(t, rendered)
	assert.Empty(t, buf.String())
}

func TestRenderer_UserMessageNotRendered(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true, true)

	rendered := r.RenderItem(models.ConversationItem{
		Type:    models.ItemTypeUserMessage,
		Content: "Hello",
	})

	assert.False(t, rendered, "User messages should not be rendered during live conversation (readline echoes them)")
	assert.Empty(t, buf.String())
}

func TestRenderer_RenderItemForResume_ShowsUserMessages(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true, true)

	r.RenderItemForResume(models.ConversationItem{
		Type:    models.ItemTypeUserMessage,
		Content: "Hello from resume",
	})

	assert.Contains(t, buf.String(), "Hello from resume")
}

func TestRenderer_RenderStatusLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true, true)

	r.RenderStatusLine("gpt-4o-mini", 1234, 3)

	assert.Contains(t, buf.String(), "gpt-4o-mini")
	assert.Contains(t, buf.String(), "1,234")
	assert.Contains(t, buf.String(), "turn 3")
}

func TestRenderer_LongOutputTruncated(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true, true)

	// Create output with 25 lines
	longContent := ""
	for i := 0; i < 25; i++ {
		longContent += "line\n"
	}

	success := true
	r.RenderItem(models.ConversationItem{
		Type:   models.ItemTypeFunctionCallOutput,
		CallID: "call-1",
		Output: &models.FunctionCallOutputPayload{
			Content: longContent,
			Success: &success,
		},
	})

	assert.Contains(t, buf.String(), "more lines")
}

func TestRenderer_ColorDisabled(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true, true) // noColor=true

	r.RenderItem(models.ConversationItem{
		Type:      models.ItemTypeFunctionCall,
		Name:      "shell",
		Arguments: `{"command": "ls"}`,
	})

	// Should not contain ANSI escape codes
	assert.NotContains(t, buf.String(), "\033[")
}

func TestRenderer_ColorEnabled(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, false, false) // noColor=false

	r.RenderItem(models.ConversationItem{
		Type:      models.ItemTypeFunctionCall,
		Name:      "shell",
		Arguments: `{"command": "ls"}`,
	})

	// Should contain ANSI escape codes
	assert.Contains(t, buf.String(), "\033[")
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1234, "1,234"},
		{12345, "12,345"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, formatTokens(tt.input))
	}
}

func TestPhaseMessage(t *testing.T) {
	tests := []struct {
		phase         string
		toolsInFlight []string
		expected      string
	}{
		{"llm_calling", nil, "Thinking..."},
		{"tool_executing", []string{"shell"}, "Running shell..."},
		{"tool_executing", nil, "Running tool..."},
		{"waiting_for_input", nil, "Working..."},
	}

	for _, tt := range tests {
		result := PhaseMessage(workflow.TurnPhase(tt.phase), tt.toolsInFlight)
		assert.Equal(t, tt.expected, result)
	}
}
