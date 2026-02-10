package instructions

import "fmt"

// ComposeDeveloperInstructions generates developer-role instructions
// based on the session's approval mode and working directory.
func ComposeDeveloperInstructions(approvalMode, cwd string) string {
	var parts []string

	if cwd != "" {
		parts = append(parts, fmt.Sprintf("Working directory: %s", cwd))
		parts = append(parts, "All file paths in tool calls are relative to this directory unless absolute.")
	}

	switch approvalMode {
	case "never":
		parts = append(parts, "Approval mode: full-auto. All tool calls execute without user confirmation.")
	case "unless-trusted":
		parts = append(parts, "Approval mode: unless-trusted. Read-only tools (read_file, list_dir, grep_files) and safe shell commands execute automatically. Mutating operations require user approval.")
	default:
		// No approval mode info if unset (backward compat)
	}

	if len(parts) == 0 {
		return ""
	}

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n"
		}
		result += p
	}
	return result
}
