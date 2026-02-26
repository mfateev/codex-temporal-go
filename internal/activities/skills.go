package activities

import (
	"context"
	"os"
	"path/filepath"

	"github.com/mfateev/temporal-agent-harness/internal/skills"
)

// LoadSkillsInput is the input for the LoadSkills activity.
type LoadSkillsInput struct {
	CodexHome string `json:"codex_home,omitempty"`
	Cwd       string `json:"cwd,omitempty"`
}

// LoadSkillsOutput is the output from the LoadSkills activity.
type LoadSkillsOutput struct {
	Skills []skills.SkillMetadata `json:"skills,omitempty"`
}

// LoadSkills discovers SKILL.md files from the worker's filesystem.
// Non-fatal: returns empty output on failure.
func (a *InstructionActivities) LoadSkills(
	_ context.Context, input LoadSkillsInput,
) (LoadSkillsOutput, error) {
	codexHome := input.CodexHome
	if codexHome == "" {
		codexHome = defaultCodexHome()
	}

	discovered, err := skills.DiscoverSkills(codexHome, input.Cwd)
	if err != nil {
		return LoadSkillsOutput{}, nil // non-fatal
	}

	return LoadSkillsOutput{Skills: discovered}, nil
}

// ReadSkillContentInput is the input for the ReadSkillContent activity.
type ReadSkillContentInput struct {
	Path string `json:"path"`
}

// ReadSkillContentOutput is the output from the ReadSkillContent activity.
type ReadSkillContentOutput struct {
	Content string `json:"content"` // Full markdown content (frontmatter + body)
	Name    string `json:"name"`
}

// ReadSkillContent reads a single SKILL.md file and returns its full content.
// Used when a skill is mentioned ($skill-name) and its content needs to be injected.
func (a *InstructionActivities) ReadSkillContent(
	_ context.Context, input ReadSkillContentInput,
) (ReadSkillContentOutput, error) {
	sc, err := skills.LoadSkillContent(input.Path)
	if err != nil {
		return ReadSkillContentOutput{}, err
	}

	return ReadSkillContentOutput{
		Content: sc.Body,
		Name:    sc.Name,
	}, nil
}

// defaultCodexHome resolves the default codex config directory (~/.codex).
func defaultCodexHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}
