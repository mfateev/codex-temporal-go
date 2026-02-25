package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/mfateev/temporal-agent-harness/internal/workflow"
)

// formatExecSessionsDisplay formats exec session summaries as a table for display.
func formatExecSessionsDisplay(sessions []workflow.ExecSessionSummary) string {
	if len(sessions) == 0 {
		return "No exec sessions.\n"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Exec Sessions (%d)\n", len(sessions)))
	b.WriteString("──────────────────\n")
	b.WriteString(fmt.Sprintf("  %-10s %-30s %-10s %s\n", "PID", "Command", "Status", "Started"))
	b.WriteString(fmt.Sprintf("  %-10s %-30s %-10s %s\n", "───", "───────", "──────", "───────"))

	for _, s := range sessions {
		cmd := s.Command
		if len(cmd) > 30 {
			cmd = cmd[:27] + "..."
		}

		status := "running"
		if s.Exited {
			status = fmt.Sprintf("exit(%d)", s.ExitCode)
		}

		started := s.StartedAt.Format(time.Kitchen)
		b.WriteString(fmt.Sprintf("  %-10s %-30s %-10s %s\n", s.ProcessID, cmd, status, started))
	}

	return b.String()
}
