package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mfateev/temporal-agent-harness/internal/tools"
)

func newWriteInvocation(args map[string]interface{}) *tools.ToolInvocation {
	return &tools.ToolInvocation{
		CallID:    "test-call",
		ToolName:  "write_file",
		Arguments: args,
	}
}

func TestWriteFile_MissingPath(t *testing.T) {
	tool := NewWriteFileTool()
	inv := newWriteInvocation(map[string]interface{}{
		"content": "hello",
	})

	_, err := tool.Handle(context.Background(), inv)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
	assert.Contains(t, err.Error(), "missing required argument: path")
}

func TestWriteFile_PathWrongType(t *testing.T) {
	tool := NewWriteFileTool()
	inv := newWriteInvocation(map[string]interface{}{
		"path":    123,
		"content": "hello",
	})

	_, err := tool.Handle(context.Background(), inv)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
	assert.Contains(t, err.Error(), "path must be a string")
}

func TestWriteFile_EmptyPath(t *testing.T) {
	tool := NewWriteFileTool()
	inv := newWriteInvocation(map[string]interface{}{
		"path":    "",
		"content": "hello",
	})

	_, err := tool.Handle(context.Background(), inv)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
	assert.Contains(t, err.Error(), "path cannot be empty")
}

func TestWriteFile_MissingContent(t *testing.T) {
	tool := NewWriteFileTool()
	inv := newWriteInvocation(map[string]interface{}{
		"path": "/tmp/test.txt",
	})

	_, err := tool.Handle(context.Background(), inv)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
	assert.Contains(t, err.Error(), "missing required argument: content")
}

func TestWriteFile_ContentWrongType(t *testing.T) {
	tool := NewWriteFileTool()
	inv := newWriteInvocation(map[string]interface{}{
		"path":    "/tmp/test.txt",
		"content": 42,
	})

	_, err := tool.Handle(context.Background(), inv)
	require.Error(t, err)
	assert.True(t, tools.IsValidationError(err))
	assert.Contains(t, err.Error(), "content must be a string")
}

func TestWriteFile_SuccessfulWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	tool := NewWriteFileTool()
	inv := newWriteInvocation(map[string]interface{}{
		"path":    path,
		"content": "hello world\n",
	})

	output, err := tool.Handle(context.Background(), inv)
	require.NoError(t, err)
	require.NotNil(t, output.Success)
	assert.True(t, *output.Success)
	assert.Contains(t, output.Content, "12 bytes")
	assert.Contains(t, output.Content, path)

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", string(contents))
}

func TestWriteFile_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "deep.txt")

	tool := NewWriteFileTool()
	inv := newWriteInvocation(map[string]interface{}{
		"path":    path,
		"content": "deep content",
	})

	output, err := tool.Handle(context.Background(), inv)
	require.NoError(t, err)
	require.NotNil(t, output.Success)
	assert.True(t, *output.Success)

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "deep content", string(contents))
}

func TestWriteFile_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	require.NoError(t, os.WriteFile(path, []byte("old content"), 0o644))

	tool := NewWriteFileTool()
	inv := newWriteInvocation(map[string]interface{}{
		"path":    path,
		"content": "new content",
	})

	output, err := tool.Handle(context.Background(), inv)
	require.NoError(t, err)
	require.NotNil(t, output.Success)
	assert.True(t, *output.Success)

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(contents))
}

func TestWriteFile_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	tool := NewWriteFileTool()
	inv := newWriteInvocation(map[string]interface{}{
		"path":    path,
		"content": "",
	})

	output, err := tool.Handle(context.Background(), inv)
	require.NoError(t, err)
	require.NotNil(t, output.Success)
	assert.True(t, *output.Success)
	assert.Contains(t, output.Content, "0 bytes")

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "", string(contents))
}

func TestWriteFile_ReadonlyDirectoryError(t *testing.T) {
	// Verify that the OS actually enforces readonly permissions.
	probe := filepath.Join(t.TempDir(), "probe")
	require.NoError(t, os.Mkdir(probe, 0o555))
	if err := os.WriteFile(filepath.Join(probe, "x"), []byte("x"), 0o644); err == nil {
		t.Skip("Skipping readonly test: environment does not enforce directory permissions")
	}

	dir := t.TempDir()
	readonlyDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.Mkdir(readonlyDir, 0o555))
	t.Cleanup(func() { os.Chmod(readonlyDir, 0o755) })

	path := filepath.Join(readonlyDir, "file.txt")

	tool := NewWriteFileTool()
	inv := newWriteInvocation(map[string]interface{}{
		"path":    path,
		"content": "should fail",
	})

	output, err := tool.Handle(context.Background(), inv)
	require.NoError(t, err) // filesystem errors are tool output, not Go errors
	require.NotNil(t, output.Success)
	assert.False(t, *output.Success)
	assert.Contains(t, output.Content, "Failed to write file")
}

func TestWriteFile_ToolMetadata(t *testing.T) {
	tool := NewWriteFileTool()
	assert.Equal(t, "write_file", tool.Name())
	assert.Equal(t, tools.ToolKindFunction, tool.Kind())
	assert.True(t, tool.IsMutating(nil))
}
