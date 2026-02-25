package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mfateev/temporal-agent-harness/internal/workflow"
)

// formatMcpToolsDisplay formats MCP tool summaries grouped by server for display.
func formatMcpToolsDisplay(tools []workflow.McpToolSummary, styles Styles) string {
	if len(tools) == 0 {
		return "No MCP tools registered.\n"
	}

	// Group by server name.
	byServer := make(map[string][]workflow.McpToolSummary)
	for _, t := range tools {
		byServer[t.ServerName] = append(byServer[t.ServerName], t)
	}

	// Sort server names.
	servers := make([]string, 0, len(byServer))
	for s := range byServer {
		servers = append(servers, s)
	}
	sort.Strings(servers)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("MCP Tools (%d)\n", len(tools)))
	b.WriteString("─────────────\n")

	for _, server := range servers {
		serverTools := byServer[server]
		// Sort tools within server.
		sort.Slice(serverTools, func(i, j int) bool {
			return serverTools[i].ToolName < serverTools[j].ToolName
		})

		b.WriteString(fmt.Sprintf("  %s (%d tools)\n", server, len(serverTools)))
		for _, t := range serverTools {
			b.WriteString(fmt.Sprintf("    %s\n", t.QualifiedName))
		}
	}

	return b.String()
}
