package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"github.com/mfateev/temporal-agent-harness/internal/activities"
	"github.com/mfateev/temporal-agent-harness/internal/memories"
	"github.com/mfateev/temporal-agent-harness/internal/models"
)

// Stub activity functions for the memory test environment.
// These are never called directly — OnActivity mocks override them —
// but they must be registered so the test env recognises the activity names.

func ListStage1Outputs(_ context.Context, _ activities.ListStage1Input) (activities.ListStage1Result, error) {
	panic("stub: should be mocked")
}

func MaterializeMemoryFiles(_ context.Context, _ activities.MaterializeInput) error {
	panic("stub: should be mocked")
}

func RunConsolidationAgent(_ context.Context, _ activities.ConsolidationAgentInput) error {
	panic("stub: should be mocked")
}

func TestConsolidationWorkflow_EmptyPending(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(ConsolidationWorkflow)
	env.RegisterActivity(ListStage1Outputs)
	env.RegisterActivity(MaterializeMemoryFiles)
	env.RegisterActivity(RunConsolidationAgent)

	state := ConsolidationState{
		PendingSessions: []string{"session-1"},
		MemoryRoot:      "/tmp/memories",
		MemoryDbPath:    "/tmp/state.sqlite",
		ModelConfig:     models.DefaultModelConfig(),
		MaxRawMemories:  1024,
	}

	env.OnActivity("ListStage1Outputs", mock.Anything, activities.ListStage1Input{MaxCount: 1024}).
		Return(activities.ListStage1Result{Outputs: nil}, nil)

	env.ExecuteWorkflow(ConsolidationWorkflow, state)
	require.True(t, env.IsWorkflowCompleted())

	// Should ContinueAsNew after clearing pending
	err := env.GetWorkflowError()
	require.Error(t, err)
	var canErr *workflow.ContinueAsNewError
	require.ErrorAs(t, err, &canErr)
}

func TestConsolidationWorkflow_WithOutputs(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(ConsolidationWorkflow)
	env.RegisterActivity(ListStage1Outputs)
	env.RegisterActivity(MaterializeMemoryFiles)
	env.RegisterActivity(RunConsolidationAgent)

	outputs := []memories.Stage1Output{
		{
			WorkflowID:      "wf-1",
			SourceUpdatedAt: 1000,
			RawMemory:       "memory 1",
			RolloutSummary:  "summary 1",
			Cwd:             "/tmp",
			GeneratedAt:     2000,
		},
	}

	state := ConsolidationState{
		PendingSessions: []string{"session-1"},
		MemoryRoot:      "/tmp/memories",
		MemoryDbPath:    "/tmp/state.sqlite",
		ModelConfig:     models.DefaultModelConfig(),
		MaxRawMemories:  1024,
	}

	env.OnActivity("ListStage1Outputs", mock.Anything, activities.ListStage1Input{MaxCount: 1024}).
		Return(activities.ListStage1Result{Outputs: outputs}, nil)

	env.OnActivity("MaterializeMemoryFiles", mock.Anything, mock.Anything).Return(nil)

	env.OnActivity("RunConsolidationAgent", mock.Anything, mock.Anything).Return(nil)

	env.ExecuteWorkflow(ConsolidationWorkflow, state)
	require.True(t, env.IsWorkflowCompleted())

	// Should ContinueAsNew after successful consolidation
	err := env.GetWorkflowError()
	require.Error(t, err)
	var canErr *workflow.ContinueAsNewError
	require.ErrorAs(t, err, &canErr)
}

func TestConsolidationWorkflow_DefaultMaxRawMemories(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(ConsolidationWorkflow)
	env.RegisterActivity(ListStage1Outputs)
	env.RegisterActivity(MaterializeMemoryFiles)
	env.RegisterActivity(RunConsolidationAgent)

	// State with MaxRawMemories = 0 should default to 1024
	state := ConsolidationState{
		PendingSessions: []string{"session-1"},
		MemoryRoot:      "/tmp/memories",
		MaxRawMemories:  0,
	}

	env.OnActivity("ListStage1Outputs", mock.Anything, activities.ListStage1Input{MaxCount: 1024}).
		Return(activities.ListStage1Result{Outputs: nil}, nil)

	env.ExecuteWorkflow(ConsolidationWorkflow, state)
	require.True(t, env.IsWorkflowCompleted())
}
