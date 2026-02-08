package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mfateev/codex-temporal-go/internal/tools"
)

// WriteFileTool creates or overwrites a file with given content.
//
// This is a new addition (not ported from Codex Rust, which routes all
// file writes through apply_patch).
type WriteFileTool struct{}

// NewWriteFileTool creates a new write file tool handler.
func NewWriteFileTool() *WriteFileTool {
	return &WriteFileTool{}
}

// Name returns the tool's name.
func (t *WriteFileTool) Name() string {
	return "write_file"
}

// Kind returns ToolKindFunction.
func (t *WriteFileTool) Kind() tools.ToolKind {
	return tools.ToolKindFunction
}

// IsMutating returns true - writing files modifies the environment.
func (t *WriteFileTool) IsMutating(invocation *tools.ToolInvocation) bool {
	return true
}

// Handle writes content to a file, creating parent directories as needed.
func (t *WriteFileTool) Handle(_ context.Context, invocation *tools.ToolInvocation) (*tools.ToolOutput, error) {
	pathArg, ok := invocation.Arguments["path"]
	if !ok {
		return nil, tools.NewValidationError("missing required argument: path")
	}

	path, ok := pathArg.(string)
	if !ok {
		return nil, tools.NewValidationError("path must be a string")
	}

	if path == "" {
		return nil, tools.NewValidationError("path cannot be empty")
	}

	contentArg, ok := invocation.Arguments["content"]
	if !ok {
		return nil, tools.NewValidationError("missing required argument: content")
	}

	content, ok := contentArg.(string)
	if !ok {
		return nil, tools.NewValidationError("content must be a string")
	}

	// Create parent directories if they don't exist.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		success := false
		return &tools.ToolOutput{
			Content: fmt.Sprintf("Failed to create directory %s: %v", dir, err),
			Success: &success,
		}, nil
	}

	// Write the file.
	data := []byte(content)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		success := false
		return &tools.ToolOutput{
			Content: fmt.Sprintf("Failed to write file: %v", err),
			Success: &success,
		}, nil
	}

	success := true
	return &tools.ToolOutput{
		Content: fmt.Sprintf("Successfully wrote %d bytes to %s", len(data), path),
		Success: &success,
	}, nil
}
