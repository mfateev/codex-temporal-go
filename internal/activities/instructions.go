package activities

import (
	"context"

	"github.com/mfateev/codex-temporal-go/internal/instructions"
)

// LoadWorkerInstructionsInput is the input for the LoadWorkerInstructions activity.
type LoadWorkerInstructionsInput struct {
	Cwd string `json:"cwd"`
}

// LoadWorkerInstructionsOutput is the output from the LoadWorkerInstructions activity.
type LoadWorkerInstructionsOutput struct {
	ProjectDocs string `json:"project_docs,omitempty"`
	GitRoot     string `json:"git_root,omitempty"`
}

// InstructionActivities contains instruction-loading activities.
type InstructionActivities struct{}

// NewInstructionActivities creates a new InstructionActivities instance.
func NewInstructionActivities() *InstructionActivities {
	return &InstructionActivities{}
}

// LoadWorkerInstructions discovers and loads AGENTS.md files from the
// worker's file system. Runs on the session task queue so it executes
// on the same machine where tools run.
func (a *InstructionActivities) LoadWorkerInstructions(
	ctx context.Context, input LoadWorkerInstructionsInput,
) (LoadWorkerInstructionsOutput, error) {
	if input.Cwd == "" {
		return LoadWorkerInstructionsOutput{}, nil
	}

	gitRoot, err := instructions.FindGitRoot(input.Cwd)
	if err != nil {
		return LoadWorkerInstructionsOutput{}, nil // non-fatal
	}

	if gitRoot == "" {
		// Not in a git repo — no project docs to load
		return LoadWorkerInstructionsOutput{}, nil
	}

	projectDocs, err := instructions.LoadProjectDocs(gitRoot, input.Cwd)
	if err != nil {
		return LoadWorkerInstructionsOutput{}, nil // non-fatal
	}

	return LoadWorkerInstructionsOutput{
		ProjectDocs: projectDocs,
		GitRoot:     gitRoot,
	}, nil
}
