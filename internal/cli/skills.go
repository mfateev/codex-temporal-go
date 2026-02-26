package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mfateev/temporal-agent-harness/internal/skills"
)

// formatSkillsListDisplay formats the skills list for the viewport.
func formatSkillsListDisplay(allSkills []skills.SkillMetadata, disabledPaths []string) string {
	if len(allSkills) == 0 {
		return "No skills found.\n"
	}

	disabled := make(map[string]bool, len(disabledPaths))
	for _, p := range disabledPaths {
		disabled[p] = true
	}

	var sb strings.Builder
	sb.WriteString("Skills:\n")
	for i, s := range allSkills {
		status := "[x]"
		if disabled[s.Path] {
			status = "[ ]"
		}

		desc := s.ShortDescription
		if desc == "" {
			desc = s.Description
		}
		// Truncate long descriptions
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}

		name := s.Name
		scope := string(s.Scope)
		sb.WriteString(fmt.Sprintf("  %d. %s %-20s (%s) %s\n", i+1, status, name, scope, desc))
	}
	sb.WriteString("\nUse /skills toggle to enable/disable skills.\n")
	return sb.String()
}

// buildSkillsToggleSelector creates a selector for toggling skills.
func buildSkillsToggleSelector(allSkills []skills.SkillMetadata, disabledPaths []string, styles Styles) *SelectorModel {
	disabled := make(map[string]bool, len(disabledPaths))
	for _, p := range disabledPaths {
		disabled[p] = true
	}

	var opts []SelectorOption
	for _, s := range allSkills {
		status := "[x]"
		action := "Disable"
		if disabled[s.Path] {
			status = "[ ]"
			action = "Enable"
		}

		short := s.Name
		desc := s.ShortDescription
		if desc == "" {
			desc = s.Description
		}
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}

		label := fmt.Sprintf("%s %s — %s (%s)", status, short, desc, action)
		opts = append(opts, SelectorOption{Label: label})
	}

	sel := NewSelectorModel(opts, styles)
	return sel
}

// skillDisplayName returns a short display name for a skill path.
func skillDisplayName(path string) string {
	dir := filepath.Dir(path)
	return filepath.Base(dir)
}
