package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/mfateev/temporal-agent-harness/internal/models"
	"github.com/mfateev/temporal-agent-harness/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Unit tests for cache_control placement in request builders ---

// TestBuildSystemBlocks_CacheControl verifies that base and user instruction blocks
// each carry a cache_control breakpoint with type "ephemeral".
func TestBuildSystemBlocks_CacheControl(t *testing.T) {
	c := &AnthropicClient{}
	req := LLMRequest{
		BaseInstructions: "You are a helpful assistant.",
		UserInstructions: "Be concise.",
	}

	blocks := c.buildSystemBlocks(req)

	require.Len(t, blocks, 2)
	for i, block := range blocks {
		cc := block.CacheControl
		assert.Equal(t, "ephemeral", string(cc.Type),
			"system block %d must have cache_control.type=ephemeral", i)
	}
}

// TestBuildSystemBlocks_CacheControl_BaseOnly verifies a single base instruction block
// still gets a cache breakpoint.
func TestBuildSystemBlocks_CacheControl_BaseOnly(t *testing.T) {
	c := &AnthropicClient{}
	req := LLMRequest{BaseInstructions: "base only"}

	blocks := c.buildSystemBlocks(req)

	require.Len(t, blocks, 1)
	assert.Equal(t, "ephemeral", string(blocks[0].CacheControl.Type))
}

// TestBuildSystemBlocks_NoCacheControl_Empty verifies no blocks are returned
// for an empty request (nothing to cache).
func TestBuildSystemBlocks_NoCacheControl_Empty(t *testing.T) {
	c := &AnthropicClient{}
	blocks := c.buildSystemBlocks(LLMRequest{})
	assert.Empty(t, blocks)
}

// TestBuildToolDefinitions_CacheControl verifies that the last tool definition
// has a cache_control breakpoint, while earlier tools do not.
func TestBuildToolDefinitions_CacheControl(t *testing.T) {
	c := &AnthropicClient{}
	specs := []tools.ToolSpec{
		{
			Name:        "shell",
			Description: "Run a shell command",
			Parameters: []tools.ToolParameter{
				{Name: "command", Type: "string", Description: "The command", Required: true},
			},
		},
		{
			Name:        "read_file",
			Description: "Read a file",
			Parameters: []tools.ToolParameter{
				{Name: "path", Type: "string", Description: "The path", Required: true},
			},
		},
	}

	defs := c.buildToolDefinitions(specs)

	require.Len(t, defs, 2)

	// First tool: no cache_control (zero value)
	firstTool := defs[0].OfTool
	require.NotNil(t, firstTool)
	assert.Equal(t, "", string(firstTool.CacheControl.Type),
		"non-last tools must not have cache_control")

	// Last tool: cache_control set to ephemeral
	lastTool := defs[len(defs)-1].OfTool
	require.NotNil(t, lastTool)
	assert.Equal(t, "ephemeral", string(lastTool.CacheControl.Type),
		"last tool must have cache_control.type=ephemeral")
}

// TestBuildToolDefinitions_CacheControl_SingleTool verifies a single tool also
// gets the cache breakpoint.
func TestBuildToolDefinitions_CacheControl_SingleTool(t *testing.T) {
	c := &AnthropicClient{}
	specs := []tools.ToolSpec{
		{Name: "shell", Description: "Run shell", Parameters: []tools.ToolParameter{
			{Name: "command", Type: "string", Description: "cmd", Required: true},
		}},
	}

	defs := c.buildToolDefinitions(specs)

	require.Len(t, defs, 1)
	require.NotNil(t, defs[0].OfTool)
	assert.Equal(t, "ephemeral", string(defs[0].OfTool.CacheControl.Type))
}

// TestBuildToolDefinitions_NoTools verifies that an empty tool list does not panic.
func TestBuildToolDefinitions_NoTools(t *testing.T) {
	c := &AnthropicClient{}
	defs := c.buildToolDefinitions(nil)
	assert.Empty(t, defs)
}

// TestBuildMessages_CacheBreakpointOnPenultimate verifies that after converting
// history, the last content block of the second-to-last message carries a
// cache_control breakpoint.
func TestBuildMessages_CacheBreakpointOnPenultimate(t *testing.T) {
	c := &AnthropicClient{}
	req := LLMRequest{
		// Developer instructions become messages[0].
		DeveloperInstructions: "you are an agent",
		// History produces messages[1] (assistant) and messages[2] (current user).
		History: []models.ConversationItem{
			{Type: models.ItemTypeAssistantMessage, Content: "I will help."},
			{Type: models.ItemTypeUserMessage, Content: "Do the thing."},
		},
	}

	messages, err := c.buildMessages(req)
	require.NoError(t, err)

	// Expect: [devInstructions, assistant, currentUser] = 3 messages.
	require.GreaterOrEqual(t, len(messages), 2, "need at least 2 messages for cache breakpoint")

	penultimate := messages[len(messages)-2]
	require.NotEmpty(t, penultimate.Content)

	lastBlock := penultimate.Content[len(penultimate.Content)-1]
	cc := lastBlock.GetCacheControl()
	require.NotNil(t, cc, "penultimate message's last content block must have a CacheControl pointer")
	assert.Equal(t, "ephemeral", string(cc.Type),
		"penultimate message cache_control.type must be ephemeral")
}

// TestBuildMessages_NoCacheBreakpoint_SingleMessage verifies that a single-message
// history (no prior context to cache) does not get a cache breakpoint.
func TestBuildMessages_NoCacheBreakpoint_SingleMessage(t *testing.T) {
	c := &AnthropicClient{}
	req := LLMRequest{
		History: []models.ConversationItem{
			{Type: models.ItemTypeUserMessage, Content: "hello"},
		},
	}

	messages, err := c.buildMessages(req)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	// Only one message — no penultimate, so no breakpoint added. Nothing to assert beyond no panic.
}

// --- HTTP interception test: verifies cache_control appears in the wire request ---

// fakeAnthropicResponse returns a minimal valid Anthropic Messages API JSON response.
func fakeAnthropicResponse() string {
	return `{
		"id": "msg_test123",
		"type": "message",
		"role": "assistant",
		"model": "claude-haiku-4-5-20251001",
		"content": [{"type": "text", "text": "Hello!"}],
		"stop_reason": "end_turn",
		"stop_sequence": null,
		"usage": {
			"input_tokens": 100,
			"output_tokens": 10,
			"cache_creation_input_tokens": 80,
			"cache_read_input_tokens": 0,
			"cache_creation": {
				"ephemeral_5m_input_tokens": 80,
				"ephemeral_1h_input_tokens": 0
			}
		}
	}`
}

// TestCall_CacheControlSentInSystemBlocks verifies that the system blocks in
// the wire request contain cache_control with type "ephemeral".
func TestCall_CacheControlSentInSystemBlocks(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, fakeAnthropicResponse())
	}))
	defer server.Close()

	c := &AnthropicClient{
		client: anthropic.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	_, err := c.Call(context.Background(), LLMRequest{
		ModelConfig:      models.ModelConfig{Model: "claude-haiku-4-5-20251001", MaxTokens: 1024},
		BaseInstructions: "You are helpful.",
		UserInstructions: "Be concise.",
		History:          []models.ConversationItem{{Type: models.ItemTypeUserMessage, Content: "hi"}},
	})
	require.NoError(t, err)

	systemRaw, ok := capturedBody["system"]
	require.True(t, ok, "system field must be present in request")
	systemBlocks, ok := systemRaw.([]interface{})
	require.True(t, ok)
	require.Len(t, systemBlocks, 2)

	for i, blockRaw := range systemBlocks {
		block, ok := blockRaw.(map[string]interface{})
		require.True(t, ok)
		cc, ok := block["cache_control"].(map[string]interface{})
		require.True(t, ok, "system block %d must have cache_control", i)
		assert.Equal(t, "ephemeral", cc["type"], "system block %d cache_control.type must be ephemeral", i)
	}
}

// TestCall_CacheControlSentOnLastTool verifies that the last tool definition in
// the wire request carries cache_control with type "ephemeral".
func TestCall_CacheControlSentOnLastTool(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, fakeAnthropicResponse())
	}))
	defer server.Close()

	c := &AnthropicClient{
		client: anthropic.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	_, err := c.Call(context.Background(), LLMRequest{
		ModelConfig: models.ModelConfig{Model: "claude-haiku-4-5-20251001", MaxTokens: 1024},
		History:     []models.ConversationItem{{Type: models.ItemTypeUserMessage, Content: "hi"}},
		ToolSpecs: []tools.ToolSpec{
			{Name: "shell", Description: "Run shell", Parameters: []tools.ToolParameter{
				{Name: "command", Type: "string", Description: "cmd", Required: true},
			}},
			{Name: "read_file", Description: "Read a file", Parameters: []tools.ToolParameter{
				{Name: "path", Type: "string", Description: "path", Required: true},
			}},
		},
	})
	require.NoError(t, err)

	toolsRaw, ok := capturedBody["tools"]
	require.True(t, ok, "tools field must be present")
	toolsList, ok := toolsRaw.([]interface{})
	require.True(t, ok)
	require.Len(t, toolsList, 2)

	// First tool: no cache_control
	firstTool, ok := toolsList[0].(map[string]interface{})
	require.True(t, ok)
	_, hasCC := firstTool["cache_control"]
	assert.False(t, hasCC, "non-last tools must not have cache_control")

	// Last tool: cache_control with type "ephemeral"
	lastTool, ok := toolsList[len(toolsList)-1].(map[string]interface{})
	require.True(t, ok)
	cc, ok := lastTool["cache_control"].(map[string]interface{})
	require.True(t, ok, "last tool must have cache_control in wire request")
	assert.Equal(t, "ephemeral", cc["type"])
}

// TestCall_CacheControlSentOnPenultimateMessage verifies that in a multi-turn
// request, the penultimate message's last content block carries cache_control.
func TestCall_CacheControlSentOnPenultimateMessage(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, fakeAnthropicResponse())
	}))
	defer server.Close()

	c := &AnthropicClient{
		client: anthropic.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	_, err := c.Call(context.Background(), LLMRequest{
		ModelConfig: models.ModelConfig{Model: "claude-haiku-4-5-20251001", MaxTokens: 1024},
		History: []models.ConversationItem{
			{Type: models.ItemTypeUserMessage, Content: "first turn"},
			{Type: models.ItemTypeAssistantMessage, Content: "I'll help."},
			{Type: models.ItemTypeUserMessage, Content: "second turn"},
		},
	})
	require.NoError(t, err)

	messagesRaw, ok := capturedBody["messages"]
	require.True(t, ok, "messages field must be present")
	messagesList, ok := messagesRaw.([]interface{})
	require.True(t, ok)
	require.GreaterOrEqual(t, len(messagesList), 2, "need at least 2 messages")

	penultimate, ok := messagesList[len(messagesList)-2].(map[string]interface{})
	require.True(t, ok)
	contentRaw, ok := penultimate["content"].([]interface{})
	require.True(t, ok, "penultimate message must have content")
	require.NotEmpty(t, contentRaw)

	lastContent, ok := contentRaw[len(contentRaw)-1].(map[string]interface{})
	require.True(t, ok)
	cc, ok := lastContent["cache_control"].(map[string]interface{})
	require.True(t, ok, "penultimate message's last content block must have cache_control in wire request")
	assert.Equal(t, "ephemeral", cc["type"])
}

// TestCall_CachedTokensReported verifies that cache_read_input_tokens from the
// API response is captured in TokenUsage.CachedTokens.
func TestCall_CachedTokensReported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Return a response with cache_read_input_tokens set.
		fmt.Fprint(w, `{
			"id": "msg_cached",
			"type": "message",
			"role": "assistant",
			"model": "claude-haiku-4-5-20251001",
			"content": [{"type": "text", "text": "cached response"}],
			"stop_reason": "end_turn",
			"stop_sequence": null,
			"usage": {
				"input_tokens": 20,
				"output_tokens": 5,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens": 80,
				"cache_creation": {
					"ephemeral_5m_input_tokens": 0,
					"ephemeral_1h_input_tokens": 0
				}
			}
		}`)
	}))
	defer server.Close()

	c := &AnthropicClient{
		client: anthropic.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	resp, err := c.Call(context.Background(), LLMRequest{
		ModelConfig: models.ModelConfig{Model: "claude-haiku-4-5-20251001", MaxTokens: 1024},
		History:     []models.ConversationItem{{Type: models.ItemTypeUserMessage, Content: "hi"}},
	})
	require.NoError(t, err)

	assert.Equal(t, 80, resp.TokenUsage.CachedTokens, "cache_read_input_tokens must be reported in CachedTokens")
	assert.Equal(t, 20, resp.TokenUsage.PromptTokens)
	assert.Equal(t, 5, resp.TokenUsage.CompletionTokens)
}
