// Package instructions contains prompt construction for LLM calls.
//
// suggestions.go provides the system prompt and input builder for the
// post-turn suggestion feature. After an agentic turn completes, a cheap/fast
// LLM call generates a single ghost-text suggestion shown in the CLI input.
package instructions

import (
	"fmt"
	"strings"
)

// SuggestionSystemPrompt is the system prompt used for the lightweight
// suggestion LLM call that runs after each agentic turn completes.
const SuggestionSystemPrompt = `Suggest what the user would naturally type next into this coding assistant.

Look at the user's request and the assistant's response. Predict what THEY would type —
not what you think they should do. The test: would they think "I was just about to type that"?

Guidelines:
- After code was written → "run the tests" or "try it out"
- After a fix → "verify it works"
- After the assistant offers options → suggest the likely pick
- After the assistant asks to continue → "yes" or "go ahead"
- Task complete with obvious follow-up → "commit this" or "push it"
- After error or misunderstanding → say nothing (let them assess)

Be specific when possible: "run the tests" beats "continue".

NEVER suggest:
- Evaluative ("looks good", "thanks")
- Questions ("what about...?")
- Assistant-voice ("Let me...", "I'll...")
- New ideas they didn't mention
- Multiple sentences

2-12 words, match the user's style. Or nothing if the next step isn't obvious.

Reply with ONLY the suggestion text, no quotes or explanation. If nothing fits, reply with
exactly the word NONE.`

// maxUserMsgLen is the maximum character length for the user message excerpt
// sent to the suggestion model.
const maxUserMsgLen = 200

// maxAssistantMsgLen is the maximum character length for the assistant message
// excerpt sent to the suggestion model.
const maxAssistantMsgLen = 500

// BuildSuggestionInput constructs the user message for the suggestion LLM call.
// It includes the last user message (truncated), last assistant message (truncated),
// and a summary of tool calls made during the turn.
func BuildSuggestionInput(userMsg, assistantMsg string, toolSummaries []string) string {
	var b strings.Builder

	b.WriteString("User said: ")
	b.WriteString(truncateString(userMsg, maxUserMsgLen))
	b.WriteString("\n\n")

	b.WriteString("Assistant responded: ")
	b.WriteString(truncateString(assistantMsg, maxAssistantMsgLen))

	if len(toolSummaries) > 0 {
		b.WriteString("\n\nTools called: ")
		b.WriteString(strings.Join(toolSummaries, ", "))
	}

	return b.String()
}

// truncateString truncates s to maxLen characters, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// SuggestionModelForProvider returns the cheap/fast model name to use for
// suggestion generation based on the user's primary provider.
func SuggestionModelForProvider(provider string) (model string, resolvedProvider string) {
	switch strings.ToLower(provider) {
	case "anthropic":
		return "claude-haiku-4-5-20251001", "anthropic"
	default:
		return "gpt-4o-mini", "openai"
	}
}

// ParseSuggestionResponse extracts the suggestion text from the LLM response.
// Returns empty string if the response is "NONE", empty, or invalid.
func ParseSuggestionResponse(response string) string {
	s := strings.TrimSpace(response)
	if s == "" || strings.EqualFold(s, "NONE") {
		return ""
	}
	// Strip surrounding quotes if present
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	// Sanity check: reject multi-line or overly long responses
	if strings.Contains(s, "\n") || len(s) > 100 {
		return ""
	}
	return s
}

// FormatToolSummary formats a tool name and success status into a summary string.
func FormatToolSummary(toolName string, success bool) string {
	if success {
		return toolName
	}
	return fmt.Sprintf("%s (failed)", toolName)
}
