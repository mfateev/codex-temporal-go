package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mfateev/temporal-agent-harness/internal/workflow"
)

func TestFormatMcpToolsDisplay_NoTools(t *testing.T) {
	result := formatMcpToolsDisplay(nil, DefaultStyles())
	assert.Contains(t, result, "No MCP tools registered.")
}

func TestFormatMcpToolsDisplay_SingleServer(t *testing.T) {
	tools := []workflow.McpToolSummary{
		{QualifiedName: "mcp__server1__tool_a", ServerName: "server1", ToolName: "tool_a"},
		{QualifiedName: "mcp__server1__tool_b", ServerName: "server1", ToolName: "tool_b"},
	}

	result := formatMcpToolsDisplay(tools, DefaultStyles())
	assert.Contains(t, result, "MCP Tools (2)")
	assert.Contains(t, result, "server1 (2 tools)")
	assert.Contains(t, result, "mcp__server1__tool_a")
	assert.Contains(t, result, "mcp__server1__tool_b")
}

func TestFormatMcpToolsDisplay_MultipleServers(t *testing.T) {
	tools := []workflow.McpToolSummary{
		{QualifiedName: "mcp__alpha__read", ServerName: "alpha", ToolName: "read"},
		{QualifiedName: "mcp__beta__write", ServerName: "beta", ToolName: "write"},
		{QualifiedName: "mcp__alpha__list", ServerName: "alpha", ToolName: "list"},
	}

	result := formatMcpToolsDisplay(tools, DefaultStyles())
	assert.Contains(t, result, "MCP Tools (3)")
	assert.Contains(t, result, "alpha (2 tools)")
	assert.Contains(t, result, "beta (1 tools)")

	// Verify alphabetical ordering: alpha before beta.
	alphaIdx := indexOf(result, "alpha")
	betaIdx := indexOf(result, "beta")
	assert.Less(t, alphaIdx, betaIdx, "servers should be sorted alphabetically")
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
