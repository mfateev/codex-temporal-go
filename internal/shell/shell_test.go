package shell

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// DetectShellType
// ---------------------------------------------------------------------------

func TestDetectShellType_Bash(t *testing.T) {
	st, ok := DetectShellType("bash")
	require.True(t, ok)
	assert.Equal(t, ShellTypeBash, st)
}

func TestDetectShellType_Zsh(t *testing.T) {
	st, ok := DetectShellType("zsh")
	require.True(t, ok)
	assert.Equal(t, ShellTypeZsh, st)
}

func TestDetectShellType_Sh(t *testing.T) {
	st, ok := DetectShellType("sh")
	require.True(t, ok)
	assert.Equal(t, ShellTypeSh, st)
}

func TestDetectShellType_FullPath_Bash(t *testing.T) {
	st, ok := DetectShellType("/usr/bin/bash")
	require.True(t, ok)
	assert.Equal(t, ShellTypeBash, st)
}

func TestDetectShellType_FullPath_Zsh(t *testing.T) {
	st, ok := DetectShellType("/bin/zsh")
	require.True(t, ok)
	assert.Equal(t, ShellTypeZsh, st)
}

func TestDetectShellType_Unknown(t *testing.T) {
	_, ok := DetectShellType("fish")
	assert.False(t, ok)
}

func TestDetectShellType_UnknownFullPath(t *testing.T) {
	_, ok := DetectShellType("/usr/local/bin/fish")
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// DeriveExecArgs
// ---------------------------------------------------------------------------

func TestDeriveExecArgs_BashLogin(t *testing.T) {
	s := &Shell{Type: ShellTypeBash, Path: "/usr/bin/bash"}
	args := s.DeriveExecArgs("ls -la", true)
	assert.Equal(t, []string{"/usr/bin/bash", "-lc", "ls -la"}, args)
}

func TestDeriveExecArgs_BashNoLogin(t *testing.T) {
	s := &Shell{Type: ShellTypeBash, Path: "/usr/bin/bash"}
	args := s.DeriveExecArgs("ls -la", false)
	assert.Equal(t, []string{"/usr/bin/bash", "-c", "ls -la"}, args)
}

func TestDeriveExecArgs_ZshLogin(t *testing.T) {
	s := &Shell{Type: ShellTypeZsh, Path: "/bin/zsh"}
	args := s.DeriveExecArgs("echo hello", true)
	assert.Equal(t, []string{"/bin/zsh", "-lc", "echo hello"}, args)
}

func TestDeriveExecArgs_ZshNoLogin(t *testing.T) {
	s := &Shell{Type: ShellTypeZsh, Path: "/bin/zsh"}
	args := s.DeriveExecArgs("echo hello", false)
	assert.Equal(t, []string{"/bin/zsh", "-c", "echo hello"}, args)
}

func TestDeriveExecArgs_Sh(t *testing.T) {
	s := &Shell{Type: ShellTypeSh, Path: "/bin/sh"}
	args := s.DeriveExecArgs("pwd", true)
	assert.Equal(t, []string{"/bin/sh", "-lc", "pwd"}, args)
}

// ---------------------------------------------------------------------------
// Shell.Name
// ---------------------------------------------------------------------------

func TestShellName(t *testing.T) {
	assert.Equal(t, "bash", (&Shell{Type: ShellTypeBash}).Name())
	assert.Equal(t, "zsh", (&Shell{Type: ShellTypeZsh}).Name())
	assert.Equal(t, "sh", (&Shell{Type: ShellTypeSh}).Name())
}

// ---------------------------------------------------------------------------
// DetectUserShell
// ---------------------------------------------------------------------------

func TestDetectUserShell_FromEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")
	s := DetectUserShell()
	require.NotNil(t, s)
	assert.Equal(t, ShellTypeZsh, s.Type)
	assert.Equal(t, "/usr/bin/zsh", s.Path)
}

func TestDetectUserShell_FallbackWhenEmpty(t *testing.T) {
	t.Setenv("SHELL", "")

	// Override lookPath so we control the fallback without needing
	// real binaries on the test host.
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	lookPath = func(name string) (string, error) {
		if name == "bash" {
			return "/usr/bin/bash", nil
		}
		return "", os.ErrNotExist
	}

	s := DetectUserShell()
	require.NotNil(t, s)
	assert.Equal(t, ShellTypeBash, s.Type)
	assert.Equal(t, "/usr/bin/bash", s.Path)
}

func TestDetectUserShell_FallbackToSh(t *testing.T) {
	t.Setenv("SHELL", "")

	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	lookPath = func(name string) (string, error) {
		if name == "sh" {
			return "/bin/sh", nil
		}
		return "", os.ErrNotExist
	}

	s := DetectUserShell()
	require.NotNil(t, s)
	assert.Equal(t, ShellTypeSh, s.Type)
	assert.Equal(t, "/bin/sh", s.Path)
}

func TestDetectUserShell_UnknownShellFallback(t *testing.T) {
	t.Setenv("SHELL", "/usr/local/bin/fish")

	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	lookPath = func(name string) (string, error) {
		if name == "bash" {
			return "/usr/bin/bash", nil
		}
		return "", os.ErrNotExist
	}

	s := DetectUserShell()
	require.NotNil(t, s)
	assert.Equal(t, ShellTypeBash, s.Type)
	assert.Equal(t, "/usr/bin/bash", s.Path)
}
