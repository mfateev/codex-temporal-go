// Package workflow contains Temporal workflow definitions.
//
// user_input.go handles interception and processing of request_user_input tool calls.
//
// Maps to: codex-rs/protocol/src/request_user_input.rs
package workflow

import (
	"encoding/json"
	"fmt"

	"go.temporal.io/sdk/workflow"

	"github.com/mfateev/temporal-agent-harness/internal/models"
)

// handleRequestUserInput intercepts a request_user_input tool call, parses the
// arguments, sets the pending phase, waits for the user's response, and returns
// a FunctionCallOutput item with the user's answers as JSON.
//
// Maps to: codex-rs/protocol/src/request_user_input.rs
func (s *SessionState) handleRequestUserInput(ctx workflow.Context, fc models.ConversationItem) (models.ConversationItem, error) {
	logger := workflow.GetLogger(ctx)

	// Parse and validate the arguments
	questions, err := parseRequestUserInputArgs(fc.Arguments)
	if err != nil {
		logger.Warn("Invalid request_user_input args", "error", err)
		falseVal := false
		return models.ConversationItem{
			Type:   models.ItemTypeFunctionCallOutput,
			CallID: fc.CallID,
			Output: &models.FunctionCallOutputPayload{
				Content: fmt.Sprintf("Invalid request_user_input arguments: %v", err),
				Success: &falseVal,
			},
		}, nil
	}

	// Set pending state
	s.Phase = PhaseUserInputPending
	s.PendingUserInputReq = &PendingUserInputRequest{
		CallID:    fc.CallID,
		Questions: questions,
	}
	s.UserInputQReceived = false
	s.UserInputQResponse = nil

	logger.Info("Waiting for user input response", "question_count", len(questions))

	// Wait for user response or interrupt
	err = workflow.Await(ctx, func() bool {
		return s.UserInputQReceived || s.Interrupted || s.ShutdownRequested
	})
	if err != nil {
		return models.ConversationItem{}, fmt.Errorf("user input await failed: %w", err)
	}

	s.PendingUserInputReq = nil

	if s.Interrupted || s.ShutdownRequested {
		logger.Info("User input wait interrupted")
		falseVal := false
		return models.ConversationItem{
			Type:   models.ItemTypeFunctionCallOutput,
			CallID: fc.CallID,
			Output: &models.FunctionCallOutputPayload{
				Content: "User input request was interrupted.",
				Success: &falseVal,
			},
		}, nil
	}

	// Build the response JSON
	responseJSON, err := json.Marshal(s.UserInputQResponse)
	if err != nil {
		return models.ConversationItem{}, fmt.Errorf("failed to marshal user input response: %w", err)
	}

	trueVal := true
	return models.ConversationItem{
		Type:   models.ItemTypeFunctionCallOutput,
		CallID: fc.CallID,
		Output: &models.FunctionCallOutputPayload{
			Content: string(responseJSON),
			Success: &trueVal,
		},
	}, nil
}

// parseRequestUserInputArgs validates and parses the request_user_input arguments.
// Returns parsed questions or an error if the args are invalid.
func parseRequestUserInputArgs(argsJSON string) ([]RequestUserInputQuestion, error) {
	var args struct {
		Questions []struct {
			ID       string `json:"id"`
			Header   string `json:"header,omitempty"`
			Question string `json:"question"`
			IsOther  bool   `json:"is_other,omitempty"`
			Options  []struct {
				Label       string `json:"label"`
				Description string `json:"description,omitempty"`
			} `json:"options"`
		} `json:"questions"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if len(args.Questions) == 0 {
		return nil, fmt.Errorf("questions array must not be empty")
	}
	if len(args.Questions) > 4 {
		return nil, fmt.Errorf("at most 4 questions allowed, got %d", len(args.Questions))
	}

	questions := make([]RequestUserInputQuestion, len(args.Questions))
	for i, q := range args.Questions {
		if q.ID == "" {
			return nil, fmt.Errorf("question %d: id is required", i+1)
		}
		if q.Question == "" {
			return nil, fmt.Errorf("question %d: question text is required", i+1)
		}
		if len(q.Options) == 0 {
			return nil, fmt.Errorf("question %d: options must not be empty", i+1)
		}

		options := make([]RequestUserInputQuestionOption, len(q.Options))
		for j, opt := range q.Options {
			if opt.Label == "" {
				return nil, fmt.Errorf("question %d, option %d: label is required", i+1, j+1)
			}
			options[j] = RequestUserInputQuestionOption{
				Label:       opt.Label,
				Description: opt.Description,
			}
		}

		questions[i] = RequestUserInputQuestion{
			ID:       q.ID,
			Header:   q.Header,
			Question: q.Question,
			IsOther:  q.IsOther,
			Options:  options,
		}
	}

	return questions, nil
}
