package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"

	"github.com/mfateev/temporal-agent-harness/internal/llm"
	"github.com/mfateev/temporal-agent-harness/internal/memories"
	"github.com/mfateev/temporal-agent-harness/internal/models"
	"github.com/mfateev/temporal-agent-harness/internal/tools"
)

// MemoryActivities contains memory-related Temporal activities.
type MemoryActivities struct {
	llmClient      llm.LLMClient
	db             *memories.MemoryDB
	temporalClient client.Client
	toolRegistry   *tools.ToolRegistry
}

// NewMemoryActivities creates a new MemoryActivities instance.
func NewMemoryActivities(
	llmClient llm.LLMClient,
	db *memories.MemoryDB,
	temporalClient client.Client,
	toolRegistry *tools.ToolRegistry,
) *MemoryActivities {
	return &MemoryActivities{
		llmClient:      llmClient,
		db:             db,
		temporalClient: temporalClient,
		toolRegistry:   toolRegistry,
	}
}

// --- Phase 1: Extraction ---

// Phase1Input is the input for the ExtractPhase1 activity.
type Phase1Input struct {
	History     []models.ConversationItem `json:"history"`
	Cwd         string                    `json:"cwd"`
	WorkflowID  string                    `json:"workflow_id"`
	ModelConfig models.ModelConfig        `json:"model_config"`
}

// Phase1Output is the result of phase-1 memory extraction.
type Phase1Output struct {
	RawMemory      string `json:"raw_memory"`
	RolloutSummary string `json:"rollout_summary"`
	RolloutSlug    string `json:"rollout_slug"`
	IsNoOp         bool   `json:"is_no_op"`
}

// ExtractPhase1 serializes conversation history and calls the LLM to extract
// a raw memory and rollout summary.
// Maps to: codex-rs/core/src/memories/phase1.rs run_stage1_job
func (a *MemoryActivities) ExtractPhase1(ctx context.Context, input Phase1Input) (Phase1Output, error) {
	// Serialize conversation history for the LLM
	serialized, err := memories.SerializeConversationForMemory(input.History)
	if err != nil {
		return Phase1Output{}, fmt.Errorf("memories: serialize history: %w", err)
	}

	// Truncate to ~70% of context window
	contextLimit := int(float64(input.ModelConfig.ContextWindow) * memories.ContextWindowPercentForRollout)
	if contextLimit <= 0 {
		contextLimit = memories.DefaultRolloutTokenLimit
	}
	serialized = memories.TruncateToTokenLimit(serialized, contextLimit)

	// Build the prompt
	userMessage := memories.StageOneInputTemplate(input.WorkflowID, input.Cwd, serialized)

	// Call LLM
	request := llm.LLMRequest{
		History: []models.ConversationItem{
			{
				Type:    models.ItemTypeUserMessage,
				Content: userMessage,
			},
		},
		ModelConfig:      input.ModelConfig,
		BaseInstructions: memories.StageOneSystemPrompt,
	}

	response, err := a.llmClient.Call(ctx, request)
	if err != nil {
		return Phase1Output{}, fmt.Errorf("memories: phase1 LLM call: %w", err)
	}

	// Extract assistant response
	var responseText string
	for _, item := range response.Items {
		if item.Type == models.ItemTypeAssistantMessage && item.Content != "" {
			responseText = item.Content
			break
		}
	}

	if responseText == "" {
		return Phase1Output{IsNoOp: true}, nil
	}

	// Parse JSON response
	var parsed struct {
		RolloutSummary string  `json:"rollout_summary"`
		RolloutSlug    *string `json:"rollout_slug"`
		RawMemory      string  `json:"raw_memory"`
	}

	// Strip markdown code fences if present
	cleaned := strings.TrimSpace(responseText)
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.SplitN(cleaned, "\n", 2)
		if len(lines) > 1 {
			cleaned = lines[1]
		}
		if idx := strings.LastIndex(cleaned, "```"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
		cleaned = strings.TrimSpace(cleaned)
	}

	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return Phase1Output{}, fmt.Errorf("memories: parse phase1 response: %w (raw: %.200s)", err, cleaned)
	}

	slug := ""
	if parsed.RolloutSlug != nil {
		slug = *parsed.RolloutSlug
	}

	if parsed.RawMemory == "" && parsed.RolloutSummary == "" {
		return Phase1Output{IsNoOp: true}, nil
	}

	return Phase1Output{
		RawMemory:      parsed.RawMemory,
		RolloutSummary: parsed.RolloutSummary,
		RolloutSlug:    slug,
	}, nil
}

// --- Stage-1 DB operations ---

// UpsertStage1Input is the input for the UpsertStage1Output activity.
type UpsertStage1Input struct {
	WorkflowID      string `json:"workflow_id"`
	RawMemory       string `json:"raw_memory"`
	RolloutSummary  string `json:"rollout_summary"`
	RolloutSlug     string `json:"rollout_slug"`
	Cwd             string `json:"cwd"`
	SourceUpdatedAt int64  `json:"source_updated_at"`
}

// UpsertStage1Output writes a stage-1 output to the SQLite database.
func (a *MemoryActivities) UpsertStage1Output(ctx context.Context, input UpsertStage1Input) error {
	now := activity.GetInfo(ctx).StartedTime.Unix()
	return a.db.UpsertStage1Output(memories.Stage1Output{
		WorkflowID:      input.WorkflowID,
		SourceUpdatedAt: input.SourceUpdatedAt,
		RawMemory:       input.RawMemory,
		RolloutSummary:  input.RolloutSummary,
		RolloutSlug:     input.RolloutSlug,
		Cwd:             input.Cwd,
		GeneratedAt:     now,
	})
}

// --- Stage-1 listing ---

// ListStage1Input is the input for the ListStage1Outputs activity.
type ListStage1Input struct {
	MaxCount int `json:"max_count"`
}

// ListStage1Output is the result of the ListStage1Outputs activity.
type ListStage1Result struct {
	Outputs []memories.Stage1Output `json:"outputs"`
}

// ListStage1Outputs returns recent stage-1 outputs from the database.
func (a *MemoryActivities) ListStage1Outputs(ctx context.Context, input ListStage1Input) (ListStage1Result, error) {
	outputs, err := a.db.ListStage1OutputsForGlobal(input.MaxCount)
	if err != nil {
		return ListStage1Result{}, err
	}
	return ListStage1Result{Outputs: outputs}, nil
}

// --- File materialization ---

// MaterializeInput is the input for the MaterializeMemoryFiles activity.
type MaterializeInput struct {
	MemoryRoot string                  `json:"memory_root"`
	Outputs    []memories.Stage1Output `json:"outputs"`
	MaxCount   int                     `json:"max_count"`
}

// MaterializeMemoryFiles rebuilds disk files from stage-1 outputs.
// Maps to: codex-rs/core/src/memories/phase2.rs (file sync step)
func (a *MemoryActivities) MaterializeMemoryFiles(ctx context.Context, input MaterializeInput) error {
	if err := memories.SyncRolloutSummaries(input.MemoryRoot, input.Outputs, input.MaxCount); err != nil {
		return fmt.Errorf("memories: sync rollout summaries: %w", err)
	}
	if err := memories.RebuildRawMemoriesFile(input.MemoryRoot, input.Outputs, input.MaxCount); err != nil {
		return fmt.Errorf("memories: rebuild raw memories: %w", err)
	}
	return nil
}

// --- Consolidation agent ---

// ConsolidationAgentInput is the input for the RunConsolidationAgent activity.
type ConsolidationAgentInput struct {
	MemoryRoot  string            `json:"memory_root"`
	ModelConfig models.ModelConfig `json:"model_config"`
}

// RunConsolidationAgent runs an in-process agentic loop to consolidate memories.
// It creates a tool registry with file operations, builds the consolidation prompt,
// and loops until the LLM stops making tool calls.
// Maps to: codex-rs/core/src/memories/phase2.rs (spawn sub-agent)
func (a *MemoryActivities) RunConsolidationAgent(ctx context.Context, input ConsolidationAgentInput) error {
	prompt := memories.ConsolidationPrompt(input.MemoryRoot)

	// Build conversation with the consolidation prompt as the user message
	history := []models.ConversationItem{
		{
			Type:    models.ItemTypeUserMessage,
			Content: prompt,
		},
	}

	// Build tool specs for the consolidation agent (read, write, list, grep)
	toolSpecs := buildConsolidationToolSpecs()

	// Agentic loop: call LLM → execute tools → repeat
	for iteration := 0; iteration < 50; iteration++ {
		activity.RecordHeartbeat(ctx, fmt.Sprintf("consolidation iteration %d", iteration))

		request := llm.LLMRequest{
			History:     history,
			ModelConfig: input.ModelConfig,
			ToolSpecs:   toolSpecs,
		}

		response, err := a.llmClient.Call(ctx, request)
		if err != nil {
			return fmt.Errorf("memories: consolidation LLM call (iter %d): %w", iteration, err)
		}

		// Add response items to history
		history = append(history, response.Items...)

		// Extract tool calls from response
		var toolCalls []models.ConversationItem
		for _, item := range response.Items {
			if item.Type == models.ItemTypeFunctionCall {
				toolCalls = append(toolCalls, item)
			}
		}

		// If no tool calls, the agent is done
		if len(toolCalls) == 0 {
			break
		}

		// Execute tool calls
		for _, tc := range toolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				// Return error as tool output
				history = append(history, models.ConversationItem{
					Type:   models.ItemTypeFunctionCallOutput,
					CallID: tc.CallID,
					Output: &models.FunctionCallOutputPayload{
						Content: fmt.Sprintf("Error parsing arguments: %v", err),
					},
				})
				continue
			}

			handler, err := a.toolRegistry.GetHandler(tc.Name)
			if err != nil || handler == nil {
				errMsg := fmt.Sprintf("Unknown tool: %s", tc.Name)
				if err != nil {
					errMsg = fmt.Sprintf("Tool lookup error: %v", err)
				}
				history = append(history, models.ConversationItem{
					Type:   models.ItemTypeFunctionCallOutput,
					CallID: tc.CallID,
					Output: &models.FunctionCallOutputPayload{
						Content: errMsg,
					},
				})
				continue
			}

			invocation := &tools.ToolInvocation{
				CallID:    tc.CallID,
				ToolName:  tc.Name,
				Arguments: args,
				Cwd:       input.MemoryRoot,
			}

			output, err := handler.Handle(ctx, invocation)
			if err != nil {
				history = append(history, models.ConversationItem{
					Type:   models.ItemTypeFunctionCallOutput,
					CallID: tc.CallID,
					Output: &models.FunctionCallOutputPayload{
						Content: fmt.Sprintf("Tool error: %v", err),
					},
				})
				continue
			}

			history = append(history, models.ConversationItem{
				Type:   models.ItemTypeFunctionCallOutput,
				CallID: tc.CallID,
				Output: &models.FunctionCallOutputPayload{
					Content: output.Content,
					Success: output.Success,
				},
			})
		}
	}

	return nil
}

// buildConsolidationToolSpecs returns tool specs for the consolidation agent.
func buildConsolidationToolSpecs() []tools.ToolSpec {
	// Use the global spec registry to build specs for the consolidation tools
	return tools.BuildSpecs([]string{"read_file", "write_file", "list_dir", "grep_files"})
}

// --- Memory summary reading ---

// ReadMemorySummaryInput is the input for the ReadMemorySummary activity.
type ReadMemorySummaryInput struct {
	MemoryRoot string `json:"memory_root"`
	MaxTokens  int    `json:"max_tokens"`
}

// ReadMemorySummaryOutput is the result of the ReadMemorySummary activity.
type ReadMemorySummaryOutput struct {
	Summary string `json:"summary"`
}

// ReadMemorySummary reads the memory_summary.md file from disk.
func (a *MemoryActivities) ReadMemorySummary(ctx context.Context, input ReadMemorySummaryInput) (ReadMemorySummaryOutput, error) {
	maxTokens := input.MaxTokens
	if maxTokens <= 0 {
		maxTokens = memories.MemorySummaryMaxTokens
	}

	summary, err := memories.ReadMemorySummary(input.MemoryRoot, maxTokens)
	if err != nil {
		return ReadMemorySummaryOutput{}, err
	}
	return ReadMemorySummaryOutput{Summary: summary}, nil
}

// --- Signal consolidation ---

// SignalConsolidationInput is the input for the SignalConsolidation activity.
type SignalConsolidationInput struct {
	SessionWorkflowID string            `json:"session_workflow_id"`
	MemoryRoot        string            `json:"memory_root"`
	MemoryDbPath      string            `json:"memory_db_path"`
	ModelConfig       models.ModelConfig `json:"model_config"`
	MaxRawMemories    int               `json:"max_raw_memories"`
}

// SignalConsolidation uses SignalWithStartWorkflow to send a signal to the
// singleton consolidation workflow, starting it if it doesn't exist.
func (a *MemoryActivities) SignalConsolidation(ctx context.Context, input SignalConsolidationInput) error {
	if a.temporalClient == nil {
		return fmt.Errorf("memories: no temporal client configured for SignalConsolidation")
	}

	maxRaw := input.MaxRawMemories
	if maxRaw <= 0 {
		maxRaw = 1024
	}

	// ConsolidationState matches the workflow input type
	state := map[string]interface{}{
		"pending_sessions": []string{},
		"memory_root":      input.MemoryRoot,
		"memory_db_path":   input.MemoryDbPath,
		"model_config":     input.ModelConfig,
		"max_raw_memories":  maxRaw,
	}

	_, err := a.temporalClient.SignalWithStartWorkflow(
		ctx,
		memories.ConsolidationWorkflowID,
		memories.SignalNewRolloutSummary,
		input.SessionWorkflowID, // signal payload
		client.StartWorkflowOptions{
			ID:        memories.ConsolidationWorkflowID,
			TaskQueue: "temporal-agent-harness",
		},
		"ConsolidationWorkflow",
		state,
	)
	if err != nil {
		return fmt.Errorf("memories: signal consolidation: %w", err)
	}
	return nil
}
