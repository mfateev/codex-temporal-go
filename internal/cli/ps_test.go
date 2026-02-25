package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mfateev/temporal-agent-harness/internal/workflow"
)

func TestFormatExecSessionsDisplay_Empty(t *testing.T) {
	result := formatExecSessionsDisplay(nil)
	assert.Contains(t, result, "No exec sessions.")
}

func TestFormatExecSessionsDisplay_WithSessions(t *testing.T) {
	sessions := []workflow.ExecSessionSummary{
		{
			ProcessID: "1001",
			Command:   "bash",
			StartedAt: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			Exited:    false,
		},
		{
			ProcessID: "1002",
			Command:   "python script.py",
			StartedAt: time.Date(2025, 1, 15, 10, 45, 0, 0, time.UTC),
			Exited:    true,
			ExitCode:  0,
		},
	}

	result := formatExecSessionsDisplay(sessions)
	assert.Contains(t, result, "Exec Sessions (2)")
	assert.Contains(t, result, "1001")
	assert.Contains(t, result, "bash")
	assert.Contains(t, result, "running")
	assert.Contains(t, result, "1002")
	assert.Contains(t, result, "python script.py")
	assert.Contains(t, result, "exit(0)")
}
