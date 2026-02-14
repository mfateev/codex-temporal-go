package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Unit tests for parseUpdatePlanArgs
// ---------------------------------------------------------------------------

func TestParseUpdatePlanArgs_Valid(t *testing.T) {
	args := `{
		"explanation": "Starting the migration",
		"plan": [
			{"step": "Read existing code", "status": "completed"},
			{"step": "Write migration script", "status": "in_progress"},
			{"step": "Run tests", "status": "pending"}
		]
	}`
	state, err := parseUpdatePlanArgs(args)
	require.NoError(t, err)
	assert.Equal(t, "Starting the migration", state.Explanation)
	require.Len(t, state.Steps, 3)
	assert.Equal(t, "Read existing code", state.Steps[0].Step)
	assert.Equal(t, PlanStepCompleted, state.Steps[0].Status)
	assert.Equal(t, "Write migration script", state.Steps[1].Step)
	assert.Equal(t, PlanStepInProgress, state.Steps[1].Status)
	assert.Equal(t, "Run tests", state.Steps[2].Step)
	assert.Equal(t, PlanStepPending, state.Steps[2].Status)
}

func TestParseUpdatePlanArgs_NoExplanation(t *testing.T) {
	args := `{
		"plan": [
			{"step": "Do the thing", "status": "pending"}
		]
	}`
	state, err := parseUpdatePlanArgs(args)
	require.NoError(t, err)
	assert.Empty(t, state.Explanation)
	require.Len(t, state.Steps, 1)
	assert.Equal(t, "Do the thing", state.Steps[0].Step)
	assert.Equal(t, PlanStepPending, state.Steps[0].Status)
}

func TestParseUpdatePlanArgs_InvalidJSON(t *testing.T) {
	_, err := parseUpdatePlanArgs(`{invalid json`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestParseUpdatePlanArgs_EmptyPlan(t *testing.T) {
	_, err := parseUpdatePlanArgs(`{"plan": []}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestParseUpdatePlanArgs_MissingStep(t *testing.T) {
	args := `{"plan": [{"step": "", "status": "pending"}]}`
	_, err := parseUpdatePlanArgs(args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step description must not be empty")
}

func TestParseUpdatePlanArgs_InvalidStatus(t *testing.T) {
	args := `{"plan": [{"step": "Do something", "status": "running"}]}`
	_, err := parseUpdatePlanArgs(args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
	assert.Contains(t, err.Error(), "running")
}

func TestParseUpdatePlanArgs_MultipleInProgress(t *testing.T) {
	args := `{
		"plan": [
			{"step": "Step A", "status": "in_progress"},
			{"step": "Step B", "status": "in_progress"}
		]
	}`
	_, err := parseUpdatePlanArgs(args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most one step can be in_progress")
}

func TestParseUpdatePlanArgs_AllCompleted(t *testing.T) {
	args := `{
		"plan": [
			{"step": "Step 1", "status": "completed"},
			{"step": "Step 2", "status": "completed"},
			{"step": "Step 3", "status": "completed"}
		]
	}`
	state, err := parseUpdatePlanArgs(args)
	require.NoError(t, err)
	require.Len(t, state.Steps, 3)
	for _, step := range state.Steps {
		assert.Equal(t, PlanStepCompleted, step.Status)
	}
}
