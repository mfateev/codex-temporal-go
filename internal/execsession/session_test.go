package execsession

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartSession_PipeMode_ShortLived(t *testing.T) {
	s, err := StartSession(SessionOpts{
		ProcessID: "1001",
		Command:   []string{"echo", "hello world"},
		TTY:       false,
	})
	require.NoError(t, err)
	defer s.Close()

	deadline := time.Now().Add(5 * time.Second)
	output := s.CollectOutput(deadline, nil)

	assert.Contains(t, string(output), "hello world")
	assert.True(t, s.HasExited())
	code := s.ExitCode()
	require.NotNil(t, code)
	assert.Equal(t, 0, *code)
}

func TestStartSession_PipeMode_NonZeroExit(t *testing.T) {
	s, err := StartSession(SessionOpts{
		ProcessID: "1002",
		Command:   []string{"sh", "-c", "echo fail >&2; exit 42"},
		TTY:       false,
	})
	require.NoError(t, err)
	defer s.Close()

	deadline := time.Now().Add(5 * time.Second)
	output := s.CollectOutput(deadline, nil)

	assert.Contains(t, string(output), "fail")
	assert.True(t, s.HasExited())
	code := s.ExitCode()
	require.NotNil(t, code)
	assert.Equal(t, 42, *code)
}

func TestStartSession_PipeMode_LongRunning(t *testing.T) {
	// Start a process that runs longer than our yield time.
	s, err := StartSession(SessionOpts{
		ProcessID: "1003",
		Command:   []string{"sh", "-c", "echo start; sleep 10; echo done"},
		TTY:       false,
	})
	require.NoError(t, err)
	defer s.Close()

	// Collect with a short deadline — should get "start" but not "done".
	deadline := time.Now().Add(500 * time.Millisecond)
	output := s.CollectOutput(deadline, nil)

	assert.Contains(t, string(output), "start")
	assert.NotContains(t, string(output), "done")
	assert.False(t, s.HasExited(), "process should still be running")
	assert.Nil(t, s.ExitCode())
}

func TestStartSession_PTYMode_ShortLived(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("PTY tests require Linux or macOS")
	}

	s, err := StartSession(SessionOpts{
		ProcessID: "1004",
		Command:   []string{"echo", "pty hello"},
		TTY:       true,
	})
	require.NoError(t, err)
	defer s.Close()

	deadline := time.Now().Add(5 * time.Second)
	output := s.CollectOutput(deadline, nil)

	assert.Contains(t, string(output), "pty hello")
	// PTY process should exit quickly.
	assert.True(t, s.HasExited())
}

func TestWriteStdin_PipeMode_Rejected(t *testing.T) {
	s, err := StartSession(SessionOpts{
		ProcessID: "1005",
		Command:   []string{"sleep", "1"},
		TTY:       false,
	})
	require.NoError(t, err)
	defer s.Close()

	err = s.WriteStdin([]byte("input\n"))
	assert.ErrorIs(t, err, ErrStdinClosed)
}

func TestWriteStdin_PTYMode_Interactive(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("PTY tests require Linux or macOS")
	}

	// Start cat in PTY mode — it echoes stdin to stdout.
	s, err := StartSession(SessionOpts{
		ProcessID: "1006",
		Command:   []string{"cat"},
		TTY:       true,
	})
	require.NoError(t, err)
	defer s.Close()

	// Write something.
	err = s.WriteStdin([]byte("test input\n"))
	require.NoError(t, err)

	// Collect output — should contain the echoed input.
	deadline := time.Now().Add(3 * time.Second)
	output := s.CollectOutput(deadline, nil)

	assert.Contains(t, string(output), "test input")
	assert.False(t, s.HasExited(), "cat should still be running")
}

func TestCollectOutput_HeartbeatCalled(t *testing.T) {
	s, err := StartSession(SessionOpts{
		ProcessID: "1007",
		Command:   []string{"sleep", "30"},
		TTY:       false,
	})
	require.NoError(t, err)
	defer s.Close()

	heartbeatCount := 0
	heartbeat := func(details ...interface{}) {
		heartbeatCount++
	}

	// Wait 6 seconds — should trigger at least 1 heartbeat (every 5s).
	deadline := time.Now().Add(6 * time.Second)
	_ = s.CollectOutput(deadline, heartbeat)

	assert.GreaterOrEqual(t, heartbeatCount, 1, "heartbeat should have been called at least once")
}

func TestStartSession_EmptyCommand(t *testing.T) {
	_, err := StartSession(SessionOpts{
		ProcessID: "1008",
		Command:   nil,
	})
	assert.Error(t, err)
}

func TestStartSession_InvalidCommand(t *testing.T) {
	_, err := StartSession(SessionOpts{
		ProcessID: "1009",
		Command:   []string{"/nonexistent/binary"},
		TTY:       false,
	})
	assert.Error(t, err)
}

func TestExitCode_NilWhileRunning(t *testing.T) {
	s, err := StartSession(SessionOpts{
		ProcessID: "1010",
		Command:   []string{"sleep", "10"},
		TTY:       false,
	})
	require.NoError(t, err)
	defer s.Close()

	assert.Nil(t, s.ExitCode())
	assert.False(t, s.HasExited())
}
