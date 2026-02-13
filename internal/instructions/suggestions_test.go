package instructions

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildSuggestionInput_BasicFormat(t *testing.T) {
	result := BuildSuggestionInput("create a hello world file", "Done! I created hello.go", nil)

	assert.Contains(t, result, "User said: create a hello world file")
	assert.Contains(t, result, "Assistant responded: Done! I created hello.go")
	assert.NotContains(t, result, "Tools called:")
}

func TestBuildSuggestionInput_WithToolSummaries(t *testing.T) {
	tools := []string{"write_file", "shell (failed)"}
	result := BuildSuggestionInput("create a file", "Done!", tools)

	assert.Contains(t, result, "Tools called: write_file, shell (failed)")
}

func TestBuildSuggestionInput_TruncatesUserMessage(t *testing.T) {
	longMsg := strings.Repeat("a", 300)
	result := BuildSuggestionInput(longMsg, "short", nil)

	// Should be truncated to maxUserMsgLen + "..."
	assert.Contains(t, result, strings.Repeat("a", maxUserMsgLen)+"...")
	assert.NotContains(t, result, strings.Repeat("a", 300))
}

func TestBuildSuggestionInput_TruncatesAssistantMessage(t *testing.T) {
	longMsg := strings.Repeat("b", 600)
	result := BuildSuggestionInput("hi", longMsg, nil)

	assert.Contains(t, result, strings.Repeat("b", maxAssistantMsgLen)+"...")
	assert.NotContains(t, result, strings.Repeat("b", 600))
}

func TestBuildSuggestionInput_ShortMessagesNotTruncated(t *testing.T) {
	result := BuildSuggestionInput("hello", "world", nil)

	assert.Contains(t, result, "User said: hello")
	assert.Contains(t, result, "Assistant responded: world")
	assert.NotContains(t, result, "...")
}

func TestParseSuggestionResponse_ValidSuggestion(t *testing.T) {
	assert.Equal(t, "run the tests", ParseSuggestionResponse("run the tests"))
}

func TestParseSuggestionResponse_None(t *testing.T) {
	assert.Equal(t, "", ParseSuggestionResponse("NONE"))
	assert.Equal(t, "", ParseSuggestionResponse("none"))
	assert.Equal(t, "", ParseSuggestionResponse("None"))
	assert.Equal(t, "", ParseSuggestionResponse("  NONE  "))
}

func TestParseSuggestionResponse_Empty(t *testing.T) {
	assert.Equal(t, "", ParseSuggestionResponse(""))
	assert.Equal(t, "", ParseSuggestionResponse("   "))
}

func TestParseSuggestionResponse_StripsQuotes(t *testing.T) {
	assert.Equal(t, "run the tests", ParseSuggestionResponse(`"run the tests"`))
}

func TestParseSuggestionResponse_RejectsMultiLine(t *testing.T) {
	assert.Equal(t, "", ParseSuggestionResponse("line one\nline two"))
}

func TestParseSuggestionResponse_RejectsTooLong(t *testing.T) {
	assert.Equal(t, "", ParseSuggestionResponse(strings.Repeat("x", 101)))
}

func TestParseSuggestionResponse_AcceptsMaxLength(t *testing.T) {
	s := strings.Repeat("x", 100)
	assert.Equal(t, s, ParseSuggestionResponse(s))
}

func TestFormatToolSummary(t *testing.T) {
	assert.Equal(t, "shell", FormatToolSummary("shell", true))
	assert.Equal(t, "shell (failed)", FormatToolSummary("shell", false))
}

func TestSuggestionModelForProvider(t *testing.T) {
	tests := []struct {
		provider         string
		expectedModel    string
		expectedProvider string
	}{
		{"openai", "gpt-4o-mini", "openai"},
		{"anthropic", "claude-haiku-4-5-20251001", "anthropic"},
		{"google", "gpt-4o-mini", "openai"}, // falls back to openai
		{"", "gpt-4o-mini", "openai"},        // default
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			model, prov := SuggestionModelForProvider(tt.provider)
			assert.Equal(t, tt.expectedModel, model)
			assert.Equal(t, tt.expectedProvider, prov)
		})
	}
}
