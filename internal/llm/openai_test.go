package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mfateev/temporal-agent-harness/internal/models"
	"github.com/mfateev/temporal-agent-harness/internal/tools"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Tests for buildInput ---

// TestBuildInput_UserMessage verifies user messages are converted to EasyInputMessageParam.
func TestBuildInput_UserMessage(t *testing.T) {
	client := &OpenAIClient{}
	history := []models.ConversationItem{
		{Type: models.ItemTypeUserMessage, Content: "hello"},
	}

	items := client.buildInput(history)

	require.Len(t, items, 1)
	require.NotNil(t, items[0].OfMessage, "should be an EasyInputMessageParam")
	assert.Equal(t, responses.EasyInputMessageRoleUser, items[0].OfMessage.Role)

	// Verify content is set as string via param.Opt
	assert.True(t, items[0].OfMessage.Content.OfString.Valid())
	assert.Equal(t, "hello", items[0].OfMessage.Content.OfString.Value)
}

// TestBuildInput_AssistantMessage verifies assistant messages are converted to
// ResponseOutputMessageParam (fed back as input to maintain conversation state).
func TestBuildInput_AssistantMessage(t *testing.T) {
	client := &OpenAIClient{}
	history := []models.ConversationItem{
		{Type: models.ItemTypeAssistantMessage, Content: "I'll help you"},
	}

	items := client.buildInput(history)

	require.Len(t, items, 1)
	require.NotNil(t, items[0].OfOutputMessage, "should be ResponseOutputMessageParam")
	require.Len(t, items[0].OfOutputMessage.Content, 1)
	require.NotNil(t, items[0].OfOutputMessage.Content[0].OfOutputText)
	assert.Equal(t, "I'll help you", items[0].OfOutputMessage.Content[0].OfOutputText.Text)
	assert.Equal(t, responses.ResponseOutputMessageStatusCompleted, items[0].OfOutputMessage.Status)
}

// TestBuildInput_FunctionCall verifies function calls are converted to ResponseFunctionToolCallParam.
func TestBuildInput_FunctionCall(t *testing.T) {
	client := &OpenAIClient{}
	history := []models.ConversationItem{
		{Type: models.ItemTypeFunctionCall, CallID: "call_123", Name: "shell", Arguments: `{"command":"ls"}`},
	}

	items := client.buildInput(history)

	require.Len(t, items, 1)
	require.NotNil(t, items[0].OfFunctionCall, "should be ResponseFunctionToolCallParam")
	assert.Equal(t, "call_123", items[0].OfFunctionCall.CallID)
	assert.Equal(t, "shell", items[0].OfFunctionCall.Name)
	assert.Equal(t, `{"command":"ls"}`, items[0].OfFunctionCall.Arguments)
}

// TestBuildInput_FunctionCallOutput verifies function call outputs are converted
// to ResponseInputItemFunctionCallOutputParam.
func TestBuildInput_FunctionCallOutput(t *testing.T) {
	client := &OpenAIClient{}
	history := []models.ConversationItem{
		{
			Type:   models.ItemTypeFunctionCallOutput,
			CallID: "call_123",
			Output: &models.FunctionCallOutputPayload{Content: "file.txt\ndir/"},
		},
	}

	items := client.buildInput(history)

	require.Len(t, items, 1)
	require.NotNil(t, items[0].OfFunctionCallOutput, "should be ResponseInputItemFunctionCallOutputParam")
	assert.Equal(t, "call_123", items[0].OfFunctionCallOutput.CallID)
	assert.True(t, items[0].OfFunctionCallOutput.Output.OfString.Valid())
	assert.Equal(t, "file.txt\ndir/", items[0].OfFunctionCallOutput.Output.OfString.Value)
}

// TestBuildInput_FunctionCallOutput_NilOutput verifies nil output payload produces empty content.
func TestBuildInput_FunctionCallOutput_NilOutput(t *testing.T) {
	client := &OpenAIClient{}
	history := []models.ConversationItem{
		{Type: models.ItemTypeFunctionCallOutput, CallID: "call_456", Output: nil},
	}

	items := client.buildInput(history)

	require.Len(t, items, 1)
	require.NotNil(t, items[0].OfFunctionCallOutput)
	assert.True(t, items[0].OfFunctionCallOutput.Output.OfString.Valid())
	assert.Equal(t, "", items[0].OfFunctionCallOutput.Output.OfString.Value)
}

// TestBuildInput_SkipsTurnMarkers verifies that turn_started and turn_complete
// markers are filtered out (they are internal workflow markers, not sent to API).
func TestBuildInput_SkipsTurnMarkers(t *testing.T) {
	client := &OpenAIClient{}
	history := []models.ConversationItem{
		{Type: models.ItemTypeTurnStarted, TurnID: "turn-1"},
		{Type: models.ItemTypeUserMessage, Content: "hello"},
		{Type: models.ItemTypeTurnComplete, TurnID: "turn-1"},
	}

	items := client.buildInput(history)

	require.Len(t, items, 1, "only the user message should remain")
	require.NotNil(t, items[0].OfMessage)
}

// TestBuildInput_MixedHistory verifies a full conversation roundtrip with all item types.
func TestBuildInput_MixedHistory(t *testing.T) {
	client := &OpenAIClient{}
	history := []models.ConversationItem{
		{Type: models.ItemTypeTurnStarted, TurnID: "turn-1"},
		{Type: models.ItemTypeUserMessage, Content: "list files"},
		{Type: models.ItemTypeAssistantMessage, Content: "I'll run ls"},
		{Type: models.ItemTypeFunctionCall, CallID: "call_1", Name: "shell", Arguments: `{"command":"ls"}`},
		{Type: models.ItemTypeFunctionCallOutput, CallID: "call_1", Output: &models.FunctionCallOutputPayload{Content: "file.txt"}},
		{Type: models.ItemTypeAssistantMessage, Content: "Here are the files"},
		{Type: models.ItemTypeTurnComplete, TurnID: "turn-1"},
	}

	items := client.buildInput(history)

	// Should have 5 items (turn markers filtered out)
	require.Len(t, items, 5)

	// user message
	require.NotNil(t, items[0].OfMessage)
	assert.Equal(t, responses.EasyInputMessageRoleUser, items[0].OfMessage.Role)

	// assistant message
	require.NotNil(t, items[1].OfOutputMessage)

	// function call
	require.NotNil(t, items[2].OfFunctionCall)
	assert.Equal(t, "call_1", items[2].OfFunctionCall.CallID)

	// function call output
	require.NotNil(t, items[3].OfFunctionCallOutput)
	assert.Equal(t, "call_1", items[3].OfFunctionCallOutput.CallID)

	// second assistant message
	require.NotNil(t, items[4].OfOutputMessage)
}

// --- Tests for buildToolDefinitions ---

// TestBuildToolDefinitions verifies ToolSpec → FunctionToolParam conversion.
func TestBuildToolDefinitions(t *testing.T) {
	client := &OpenAIClient{}
	specs := []tools.ToolSpec{
		{
			Name:        "shell",
			Description: "Execute a shell command",
			Parameters: []tools.ToolParameter{
				{Name: "command", Type: "string", Description: "The command to run", Required: true},
				{Name: "timeout_ms", Type: "integer", Description: "Timeout in ms", Required: false},
			},
		},
	}

	defs := client.buildToolDefinitions(specs, "")

	require.Len(t, defs, 1)
	require.NotNil(t, defs[0].OfFunction)
	assert.Equal(t, "shell", defs[0].OfFunction.Name)
	assert.True(t, defs[0].OfFunction.Description.Valid())
	assert.Equal(t, "Execute a shell command", defs[0].OfFunction.Description.Value)

	params, ok := defs[0].OfFunction.Parameters["properties"].(map[string]interface{})
	require.True(t, ok)

	cmdProp, ok := params["command"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "string", cmdProp["type"])

	required, ok := defs[0].OfFunction.Parameters["required"].([]string)
	require.True(t, ok)
	assert.Contains(t, required, "command")
	assert.NotContains(t, required, "timeout_ms")
}

// --- Tests for buildInstructions ---

// TestBuildInstructions_BaseOnly verifies base instructions alone.
func TestBuildInstructions_BaseOnly(t *testing.T) {
	client := &OpenAIClient{}
	request := LLMRequest{
		BaseInstructions: "You are a helpful assistant.",
	}

	instructions := client.buildInstructions(request)
	assert.Equal(t, "You are a helpful assistant.", instructions)
}

// TestBuildInstructions_UserOnly verifies user instructions alone.
func TestBuildInstructions_UserOnly(t *testing.T) {
	client := &OpenAIClient{}
	request := LLMRequest{
		UserInstructions: "be nice",
	}

	instructions := client.buildInstructions(request)
	assert.Equal(t, "be nice", instructions)
}

// TestBuildInstructions_BaseAndUser verifies base + user are combined.
func TestBuildInstructions_BaseAndUser(t *testing.T) {
	client := &OpenAIClient{}
	request := LLMRequest{
		BaseInstructions: "system base",
		UserInstructions: "user docs",
	}

	instructions := client.buildInstructions(request)
	assert.Contains(t, instructions, "system base")
	assert.Contains(t, instructions, "user docs")
}

// TestBuildInstructions_AllThree verifies base + user + developer are all included.
func TestBuildInstructions_AllThree(t *testing.T) {
	client := &OpenAIClient{}
	request := LLMRequest{
		BaseInstructions:      "base prompt",
		DeveloperInstructions: "be useful",
		UserInstructions:      "be nice",
	}

	instructions := client.buildInstructions(request)
	assert.Contains(t, instructions, "base prompt")
	assert.Contains(t, instructions, "be nice")
	assert.Contains(t, instructions, "be useful")
}

// TestBuildInstructions_Empty verifies empty instructions produce empty string.
func TestBuildInstructions_Empty(t *testing.T) {
	client := &OpenAIClient{}
	request := LLMRequest{}

	instructions := client.buildInstructions(request)
	assert.Equal(t, "", instructions)
}

// TestBuildInstructions_DeveloperOnly verifies developer instructions alone.
func TestBuildInstructions_DeveloperOnly(t *testing.T) {
	client := &OpenAIClient{}
	request := LLMRequest{
		DeveloperInstructions: "dev only",
	}

	instructions := client.buildInstructions(request)
	assert.Equal(t, "dev only", instructions)
}

// --- Tests for parseOutput ---

// TestParseOutput_Message verifies ResponseOutputMessage → ConversationItem.
func TestParseOutput_Message(t *testing.T) {
	client := &OpenAIClient{}
	resp := &responses.Response{
		ID: "resp_123",
		Output: []responses.ResponseOutputItemUnion{
			{
				Type: "message",
				Content: []responses.ResponseOutputMessageContentUnion{
					{Type: "output_text", Text: "Hello!"},
				},
			},
		},
	}

	items, finishReason := client.parseOutput(resp)

	require.Len(t, items, 1)
	assert.Equal(t, models.ItemTypeAssistantMessage, items[0].Type)
	assert.Equal(t, "Hello!", items[0].Content)
	assert.Equal(t, models.FinishReasonStop, finishReason)
}

// TestParseOutput_FunctionCalls verifies ResponseFunctionToolCall → ConversationItem.
func TestParseOutput_FunctionCalls(t *testing.T) {
	client := &OpenAIClient{}
	resp := &responses.Response{
		ID: "resp_456",
		Output: []responses.ResponseOutputItemUnion{
			{
				Type:      "function_call",
				CallID:    "call_1",
				Name:      "shell",
				Arguments: `{"command":"ls"}`,
			},
		},
	}

	items, finishReason := client.parseOutput(resp)

	require.Len(t, items, 1)
	assert.Equal(t, models.ItemTypeFunctionCall, items[0].Type)
	assert.Equal(t, "call_1", items[0].CallID)
	assert.Equal(t, "shell", items[0].Name)
	assert.Equal(t, `{"command":"ls"}`, items[0].Arguments)
	assert.Equal(t, models.FinishReasonToolCalls, finishReason)
}

// TestParseOutput_Mixed verifies multiple output items (message + function calls).
func TestParseOutput_Mixed(t *testing.T) {
	client := &OpenAIClient{}
	resp := &responses.Response{
		ID: "resp_789",
		Output: []responses.ResponseOutputItemUnion{
			{
				Type: "message",
				Content: []responses.ResponseOutputMessageContentUnion{
					{Type: "output_text", Text: "Let me check"},
				},
			},
			{
				Type:      "function_call",
				CallID:    "call_1",
				Name:      "shell",
				Arguments: `{"command":"ls"}`,
			},
			{
				Type:      "function_call",
				CallID:    "call_2",
				Name:      "read_file",
				Arguments: `{"path":"test.txt"}`,
			},
		},
	}

	items, finishReason := client.parseOutput(resp)

	require.Len(t, items, 3)
	assert.Equal(t, models.ItemTypeAssistantMessage, items[0].Type)
	assert.Equal(t, "Let me check", items[0].Content)
	assert.Equal(t, models.ItemTypeFunctionCall, items[1].Type)
	assert.Equal(t, "call_1", items[1].CallID)
	assert.Equal(t, models.ItemTypeFunctionCall, items[2].Type)
	assert.Equal(t, "call_2", items[2].CallID)
	assert.Equal(t, models.FinishReasonToolCalls, finishReason)
}

// TestParseOutput_Empty verifies empty output produces default empty assistant message.
func TestParseOutput_Empty(t *testing.T) {
	client := &OpenAIClient{}
	resp := &responses.Response{
		ID:     "resp_empty",
		Output: []responses.ResponseOutputItemUnion{},
	}

	items, finishReason := client.parseOutput(resp)

	require.Len(t, items, 1)
	assert.Equal(t, models.ItemTypeAssistantMessage, items[0].Type)
	assert.Equal(t, "", items[0].Content)
	assert.Equal(t, models.FinishReasonStop, finishReason)
}

// --- Tests for classifyByStatusCode ---

func TestClassifyByStatusCode_400_Fatal(t *testing.T) {
	err := classifyByStatusCode(http.StatusBadRequest, fmt.Errorf("bad request"))
	assert.Equal(t, models.ErrorTypeFatal, err.Type)
	assert.False(t, err.Retryable)
}

func TestClassifyByStatusCode_401_Fatal(t *testing.T) {
	err := classifyByStatusCode(http.StatusUnauthorized, fmt.Errorf("unauthorized"))
	assert.Equal(t, models.ErrorTypeFatal, err.Type)
	assert.False(t, err.Retryable)
}

func TestClassifyByStatusCode_403_Fatal(t *testing.T) {
	err := classifyByStatusCode(http.StatusForbidden, fmt.Errorf("forbidden"))
	assert.Equal(t, models.ErrorTypeFatal, err.Type)
	assert.False(t, err.Retryable)
}

func TestClassifyByStatusCode_404_Fatal(t *testing.T) {
	err := classifyByStatusCode(http.StatusNotFound, fmt.Errorf("not found"))
	assert.Equal(t, models.ErrorTypeFatal, err.Type)
	assert.False(t, err.Retryable)
}

func TestClassifyByStatusCode_422_Fatal(t *testing.T) {
	err := classifyByStatusCode(http.StatusUnprocessableEntity, fmt.Errorf("unprocessable"))
	assert.Equal(t, models.ErrorTypeFatal, err.Type)
	assert.False(t, err.Retryable)
}

func TestClassifyByStatusCode_408_Transient(t *testing.T) {
	err := classifyByStatusCode(http.StatusRequestTimeout, fmt.Errorf("timeout"))
	assert.Equal(t, models.ErrorTypeTransient, err.Type)
	assert.True(t, err.Retryable)
}

func TestClassifyByStatusCode_409_Transient(t *testing.T) {
	err := classifyByStatusCode(http.StatusConflict, fmt.Errorf("conflict"))
	assert.Equal(t, models.ErrorTypeTransient, err.Type)
	assert.True(t, err.Retryable)
}

func TestClassifyByStatusCode_429_APILimit(t *testing.T) {
	err := classifyByStatusCode(http.StatusTooManyRequests, fmt.Errorf("rate limited"))
	assert.Equal(t, models.ErrorTypeAPILimit, err.Type)
	assert.True(t, err.Retryable)
}

func TestClassifyByStatusCode_500_Transient(t *testing.T) {
	err := classifyByStatusCode(http.StatusInternalServerError, fmt.Errorf("server error"))
	assert.Equal(t, models.ErrorTypeTransient, err.Type)
	assert.True(t, err.Retryable)
}

func TestClassifyByStatusCode_502_Transient(t *testing.T) {
	err := classifyByStatusCode(http.StatusBadGateway, fmt.Errorf("bad gateway"))
	assert.Equal(t, models.ErrorTypeTransient, err.Type)
	assert.True(t, err.Retryable)
}

func TestClassifyByStatusCode_503_Transient(t *testing.T) {
	err := classifyByStatusCode(http.StatusServiceUnavailable, fmt.Errorf("unavailable"))
	assert.Equal(t, models.ErrorTypeTransient, err.Type)
	assert.True(t, err.Retryable)
}

// --- Tests for classifyError (OpenAI) ---

// newOpenAIError creates an openai.Error with required Request/Response fields.
func newOpenAIError(statusCode int) *openai.Error {
	req := httptest.NewRequest("POST", "https://api.openai.com/v1/responses", nil)
	resp := &http.Response{StatusCode: statusCode, Request: req}
	return &openai.Error{
		StatusCode: statusCode,
		Request:    req,
		Response:   resp,
	}
}

func TestClassifyError_OpenAI_400_NonRetryable(t *testing.T) {
	result := classifyError(newOpenAIError(400))
	var actErr *models.ActivityError
	require.ErrorAs(t, result, &actErr)
	assert.Equal(t, models.ErrorTypeFatal, actErr.Type)
	assert.False(t, actErr.Retryable)
}

func TestClassifyError_OpenAI_429_RateLimit(t *testing.T) {
	result := classifyError(newOpenAIError(429))
	var actErr *models.ActivityError
	require.ErrorAs(t, result, &actErr)
	assert.Equal(t, models.ErrorTypeAPILimit, actErr.Type)
	assert.True(t, actErr.Retryable)
}

func TestClassifyError_OpenAI_500_Retryable(t *testing.T) {
	result := classifyError(newOpenAIError(500))
	var actErr *models.ActivityError
	require.ErrorAs(t, result, &actErr)
	assert.Equal(t, models.ErrorTypeTransient, actErr.Type)
	assert.True(t, actErr.Retryable)
}

func TestClassifyError_ContextLengthExceeded(t *testing.T) {
	err := fmt.Errorf("maximum context length exceeded")
	result := classifyError(err)
	var actErr *models.ActivityError
	require.ErrorAs(t, result, &actErr)
	assert.Equal(t, models.ErrorTypeContextOverflow, actErr.Type)
	assert.False(t, actErr.Retryable)
}

func TestClassifyError_NetworkError_Transient(t *testing.T) {
	err := fmt.Errorf("dial tcp: connection refused")
	result := classifyError(err)
	var actErr *models.ActivityError
	require.ErrorAs(t, result, &actErr)
	assert.Equal(t, models.ErrorTypeTransient, actErr.Type)
	assert.True(t, actErr.Retryable)
}

// --- Tests for Call() request construction via HTTP mock ---

// fakeResponsesAPIResponse returns a minimal valid Responses API JSON response.
func fakeResponsesAPIResponse() string {
	return `{
		"id": "resp_test123",
		"object": "response",
		"created_at": 1700000000,
		"model": "gpt-4o-mini",
		"status": "completed",
		"output": [{
			"type": "message",
			"id": "msg_1",
			"role": "assistant",
			"status": "completed",
			"content": [{"type": "output_text", "text": "Hello!", "annotations": []}]
		}],
		"usage": {"input_tokens": 10, "output_tokens": 5, "total_tokens": 15, "input_tokens_details": {"cached_tokens": 0}, "output_tokens_details": {"reasoning_tokens": 0}},
		"parallel_tool_calls": true,
		"temperature": 1.0,
		"top_p": 1.0,
		"tool_choice": "auto",
		"tools": [],
		"text": {"format": {"type": "text"}}
	}`
}

// TestCall_ModelParameterSent verifies that the model parameter from ModelConfig
// is included in the HTTP request body sent to the OpenAI API.
func TestCall_ModelParameterSent(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, fakeResponsesAPIResponse())
	}))
	defer server.Close()

	client := &OpenAIClient{
		client: openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	request := LLMRequest{
		History: []models.ConversationItem{
			{Type: models.ItemTypeUserMessage, Content: "hello"},
		},
		ModelConfig: models.ModelConfig{
			Model:       "gpt-4o-mini",
			Temperature: 0.7,
			MaxTokens:   4096,
		},
	}

	_, err := client.Call(context.Background(), request)
	require.NoError(t, err)

	assert.Equal(t, "gpt-4o-mini", capturedBody["model"], "model parameter must be present in API request")
}

// TestCall_TemperatureAndMaxTokensSent verifies that Temperature and MaxTokens
// from ModelConfig are included in the HTTP request body.
func TestCall_TemperatureAndMaxTokensSent(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, fakeResponsesAPIResponse())
	}))
	defer server.Close()

	client := &OpenAIClient{
		client: openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	request := LLMRequest{
		History: []models.ConversationItem{
			{Type: models.ItemTypeUserMessage, Content: "hello"},
		},
		ModelConfig: models.ModelConfig{
			Model:       "gpt-4o-mini",
			Temperature: 0.7,
			MaxTokens:   4096,
		},
	}

	_, err := client.Call(context.Background(), request)
	require.NoError(t, err)

	assert.InDelta(t, 0.7, capturedBody["temperature"], 0.01, "temperature must be sent")
	assert.EqualValues(t, 4096, capturedBody["max_output_tokens"], "max_output_tokens must be sent")
}

// TestCall_ZeroTemperatureAndMaxTokensOmitted verifies that zero values
// for Temperature and MaxTokens are not sent to the API.
func TestCall_ZeroTemperatureAndMaxTokensOmitted(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, fakeResponsesAPIResponse())
	}))
	defer server.Close()

	client := &OpenAIClient{
		client: openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	request := LLMRequest{
		History: []models.ConversationItem{
			{Type: models.ItemTypeUserMessage, Content: "hello"},
		},
		ModelConfig: models.ModelConfig{
			Model:       "gpt-4o-mini",
			Temperature: 0,
			MaxTokens:   0,
		},
	}

	_, err := client.Call(context.Background(), request)
	require.NoError(t, err)

	_, hasTemp := capturedBody["temperature"]
	_, hasMax := capturedBody["max_output_tokens"]
	assert.False(t, hasTemp, "zero temperature should not be sent")
	assert.False(t, hasMax, "zero max_output_tokens should not be sent")
}

// TestCall_ToolDefinitionsSent verifies that tool specs are included
// in the HTTP request body when provided.
func TestCall_ToolDefinitionsSent(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, fakeResponsesAPIResponse())
	}))
	defer server.Close()

	client := &OpenAIClient{
		client: openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	request := LLMRequest{
		History: []models.ConversationItem{
			{Type: models.ItemTypeUserMessage, Content: "hello"},
		},
		ModelConfig: models.DefaultModelConfig(),
		ToolSpecs: []tools.ToolSpec{
			{
				Name:        "shell",
				Description: "Execute a shell command",
				Parameters: []tools.ToolParameter{
					{Name: "command", Type: "string", Description: "The command to run", Required: true},
				},
			},
		},
	}

	_, err := client.Call(context.Background(), request)
	require.NoError(t, err)

	toolsRaw, hasTools := capturedBody["tools"]
	assert.True(t, hasTools, "tools must be present when tool specs are provided")
	toolsList, ok := toolsRaw.([]interface{})
	require.True(t, ok)
	assert.Len(t, toolsList, 1)
}

// TestCall_PreviousResponseIDSent verifies that PreviousResponseID is included
// in the HTTP request when provided.
func TestCall_PreviousResponseIDSent(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, fakeResponsesAPIResponse())
	}))
	defer server.Close()

	client := &OpenAIClient{
		client: openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	request := LLMRequest{
		History: []models.ConversationItem{
			{Type: models.ItemTypeUserMessage, Content: "hello"},
		},
		ModelConfig:        models.DefaultModelConfig(),
		PreviousResponseID: "resp_prev_123",
	}

	_, err := client.Call(context.Background(), request)
	require.NoError(t, err)

	assert.Equal(t, "resp_prev_123", capturedBody["previous_response_id"],
		"previous_response_id must be sent when provided")
}

// TestCall_StoreEnabled verifies that store=true is sent in requests.
func TestCall_StoreEnabled(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, fakeResponsesAPIResponse())
	}))
	defer server.Close()

	client := &OpenAIClient{
		client: openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	request := LLMRequest{
		History: []models.ConversationItem{
			{Type: models.ItemTypeUserMessage, Content: "hello"},
		},
		ModelConfig: models.DefaultModelConfig(),
	}

	_, err := client.Call(context.Background(), request)
	require.NoError(t, err)

	assert.Equal(t, true, capturedBody["store"], "store must be true")
}

// TestCall_ResponseIDReturned verifies that the response ID is captured from the API response.
func TestCall_ResponseIDReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, fakeResponsesAPIResponse())
	}))
	defer server.Close()

	client := &OpenAIClient{
		client: openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	request := LLMRequest{
		History: []models.ConversationItem{
			{Type: models.ItemTypeUserMessage, Content: "hello"},
		},
		ModelConfig: models.DefaultModelConfig(),
	}

	resp, err := client.Call(context.Background(), request)
	require.NoError(t, err)

	assert.Equal(t, "resp_test123", resp.ResponseID, "response ID must be captured")
}

// TestCall_InstructionsSent verifies that combined instructions are sent in the request.
func TestCall_InstructionsSent(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, fakeResponsesAPIResponse())
	}))
	defer server.Close()

	client := &OpenAIClient{
		client: openai.NewClient(
			option.WithBaseURL(server.URL),
			option.WithAPIKey("test-key"),
		),
	}

	request := LLMRequest{
		History: []models.ConversationItem{
			{Type: models.ItemTypeUserMessage, Content: "hello"},
		},
		ModelConfig:      models.DefaultModelConfig(),
		BaseInstructions: "test base",
		UserInstructions: "test user",
	}

	_, err := client.Call(context.Background(), request)
	require.NoError(t, err)

	instructions, ok := capturedBody["instructions"].(string)
	require.True(t, ok, "instructions must be a string")
	assert.Contains(t, instructions, "test base")
	assert.Contains(t, instructions, "test user")
}

// --- Tests for web search parity ---

// TestParseOutput_WebSearchCall verifies web_search_call → ConversationItem mapping.
func TestParseOutput_WebSearchCall(t *testing.T) {
	client := &OpenAIClient{}
	resp := &responses.Response{
		ID: "resp_ws",
		Output: []responses.ResponseOutputItemUnion{
			{
				Type:   "web_search_call",
				ID:     "ws_123",
				Status: "completed",
				Action: responses.ResponseOutputItemUnionAction{
					Type:  "search",
					Query: "Go generics tutorial",
				},
			},
		},
	}

	items, finishReason := client.parseOutput(resp)

	require.Len(t, items, 1)
	assert.Equal(t, models.ItemTypeWebSearchCall, items[0].Type)
	assert.Equal(t, "ws_123", items[0].CallID)
	assert.Equal(t, "search", items[0].WebSearchAction)
	assert.Equal(t, "completed", items[0].WebSearchStatus)
	assert.Equal(t, "Go generics tutorial", items[0].Content)
	assert.Equal(t, models.FinishReasonStop, finishReason)
}

// TestParseOutput_WebSearchCall_OpenPage verifies open_page action.
func TestParseOutput_WebSearchCall_OpenPage(t *testing.T) {
	client := &OpenAIClient{}
	resp := &responses.Response{
		ID: "resp_ws2",
		Output: []responses.ResponseOutputItemUnion{
			{
				Type:   "web_search_call",
				ID:     "ws_456",
				Status: "completed",
				Action: responses.ResponseOutputItemUnionAction{
					Type: "open_page",
					URL:  "https://example.com/page",
				},
			},
		},
	}

	items, _ := client.parseOutput(resp)

	require.Len(t, items, 1)
	assert.Equal(t, "open_page", items[0].WebSearchAction)
	assert.Equal(t, "https://example.com/page", items[0].Content)
	assert.Equal(t, "https://example.com/page", items[0].WebSearchURL)
}

// TestParseOutput_WebSearchCall_FindInPage verifies find_in_page action.
func TestParseOutput_WebSearchCall_FindInPage(t *testing.T) {
	client := &OpenAIClient{}
	resp := &responses.Response{
		ID: "resp_ws3",
		Output: []responses.ResponseOutputItemUnion{
			{
				Type:   "web_search_call",
				ID:     "ws_789",
				Status: "completed",
				Action: responses.ResponseOutputItemUnionAction{
					Type:    "find_in_page",
					Pattern: "installation",
					URL:     "https://example.com/docs",
				},
			},
		},
	}

	items, _ := client.parseOutput(resp)

	require.Len(t, items, 1)
	assert.Equal(t, "find_in_page", items[0].WebSearchAction)
	assert.Equal(t, "'installation' in https://example.com/docs", items[0].Content)
	assert.Equal(t, "https://example.com/docs", items[0].WebSearchURL)
}

// TestParseOutput_MixedWithWebSearch verifies message + web_search + function_call output.
func TestParseOutput_MixedWithWebSearch(t *testing.T) {
	client := &OpenAIClient{}
	resp := &responses.Response{
		ID: "resp_mixed_ws",
		Output: []responses.ResponseOutputItemUnion{
			{
				Type:   "web_search_call",
				ID:     "ws_1",
				Status: "completed",
				Action: responses.ResponseOutputItemUnionAction{Type: "search", Query: "weather NYC"},
			},
			{
				Type: "message",
				Content: []responses.ResponseOutputMessageContentUnion{
					{Type: "output_text", Text: "The weather is sunny."},
				},
			},
			{
				Type:      "function_call",
				CallID:    "call_1",
				Name:      "shell",
				Arguments: `{"command":"date"}`,
			},
		},
	}

	items, finishReason := client.parseOutput(resp)

	require.Len(t, items, 3)
	assert.Equal(t, models.ItemTypeWebSearchCall, items[0].Type)
	assert.Equal(t, models.ItemTypeAssistantMessage, items[1].Type)
	assert.Equal(t, models.ItemTypeFunctionCall, items[2].Type)
	assert.Equal(t, models.FinishReasonToolCalls, finishReason)
}

// TestFormatWebSearchDetail_Search verifies search action formatting.
func TestFormatWebSearchDetail_Search(t *testing.T) {
	action := responses.ResponseOutputItemUnionAction{Type: "search", Query: "Go 1.22 release"}
	detail := formatWebSearchDetail("search", action)
	assert.Equal(t, "Go 1.22 release", detail)
}

// TestFormatWebSearchDetail_SearchMultipleQueries verifies multiple queries.
func TestFormatWebSearchDetail_SearchMultipleQueries(t *testing.T) {
	action := responses.ResponseOutputItemUnionAction{
		Type:    "search",
		Queries: []string{"first query", "second query"},
	}
	detail := formatWebSearchDetail("search", action)
	assert.Equal(t, "first query ...", detail)
}

// TestFormatWebSearchDetail_OpenPage verifies open_page formatting.
func TestFormatWebSearchDetail_OpenPage(t *testing.T) {
	action := responses.ResponseOutputItemUnionAction{Type: "open_page", URL: "https://example.com"}
	detail := formatWebSearchDetail("open_page", action)
	assert.Equal(t, "https://example.com", detail)
}

// TestFormatWebSearchDetail_FindInPage verifies find_in_page formatting.
func TestFormatWebSearchDetail_FindInPage(t *testing.T) {
	action := responses.ResponseOutputItemUnionAction{
		Type:    "find_in_page",
		Pattern: "installation",
		URL:     "https://example.com/docs",
	}
	detail := formatWebSearchDetail("find_in_page", action)
	assert.Equal(t, "'installation' in https://example.com/docs", detail)
}

// TestFormatWebSearchDetail_FindInPage_PatternOnly verifies find without URL.
func TestFormatWebSearchDetail_FindInPage_PatternOnly(t *testing.T) {
	action := responses.ResponseOutputItemUnionAction{Type: "find_in_page", Pattern: "TODO"}
	detail := formatWebSearchDetail("find_in_page", action)
	assert.Equal(t, "'TODO'", detail)
}

// TestFormatWebSearchDetail_Unknown verifies unknown action produces empty detail.
func TestFormatWebSearchDetail_Unknown(t *testing.T) {
	action := responses.ResponseOutputItemUnionAction{Type: "something_new"}
	detail := formatWebSearchDetail("something_new", action)
	assert.Equal(t, "", detail)
}

// TestBuildToolDefinitions_WebSearchCached verifies cached mode adds web search tool.
func TestBuildToolDefinitions_WebSearchCached(t *testing.T) {
	client := &OpenAIClient{}
	defs := client.buildToolDefinitions(nil, models.WebSearchCached)

	require.Len(t, defs, 1)
	require.NotNil(t, defs[0].OfWebSearch, "should be a WebSearchToolParam")
	assert.Equal(t, responses.WebSearchToolSearchContextSizeLow, defs[0].OfWebSearch.SearchContextSize)
}

// TestBuildToolDefinitions_WebSearchLive verifies live mode adds web search tool.
func TestBuildToolDefinitions_WebSearchLive(t *testing.T) {
	client := &OpenAIClient{}
	defs := client.buildToolDefinitions(nil, models.WebSearchLive)

	require.Len(t, defs, 1)
	require.NotNil(t, defs[0].OfWebSearch, "should be a WebSearchToolParam")
	assert.Equal(t, responses.WebSearchToolSearchContextSizeMedium, defs[0].OfWebSearch.SearchContextSize)
}

// TestBuildToolDefinitions_WebSearchDisabled verifies disabled mode adds no web search.
func TestBuildToolDefinitions_WebSearchDisabled(t *testing.T) {
	client := &OpenAIClient{}
	defs := client.buildToolDefinitions(nil, models.WebSearchDisabled)
	assert.Empty(t, defs)
}

// TestBuildToolDefinitions_FunctionPlusWebSearch verifies both function tools and web search.
func TestBuildToolDefinitions_FunctionPlusWebSearch(t *testing.T) {
	client := &OpenAIClient{}
	specs := []tools.ToolSpec{
		{Name: "shell", Description: "Run command", Parameters: []tools.ToolParameter{
			{Name: "command", Type: "string", Description: "cmd", Required: true},
		}},
	}
	defs := client.buildToolDefinitions(specs, models.WebSearchLive)

	require.Len(t, defs, 2)
	assert.NotNil(t, defs[0].OfFunction, "first should be function tool")
	assert.NotNil(t, defs[1].OfWebSearch, "second should be web search tool")
}

// TestBuildInput_WebSearchCall verifies web_search_call items are fed back via OfWebSearchCall.
func TestBuildInput_WebSearchCall(t *testing.T) {
	client := &OpenAIClient{}
	history := []models.ConversationItem{
		{
			Type:            models.ItemTypeWebSearchCall,
			CallID:          "ws_123",
			WebSearchAction: "search",
			WebSearchStatus: "completed",
			Content:         "Go generics",
		},
	}

	items := client.buildInput(history)

	require.Len(t, items, 1)
	require.NotNil(t, items[0].OfWebSearchCall, "should be OfWebSearchCall")
	assert.Equal(t, "ws_123", items[0].OfWebSearchCall.ID)
	assert.Equal(t, responses.ResponseFunctionWebSearchStatus("completed"), items[0].OfWebSearchCall.Status)
}
