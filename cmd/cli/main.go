// Interactive CLI for codex-temporal-go workflows.
//
// A REPL-style interface that connects to a Temporal workflow,
// shows conversation items as they appear, and lets you type
// follow-up messages.
//
// Usage:
//
//	cli -m "hello"                    Start new session with initial message
//	cli                               Start new session, enter input immediately
//	cli --session <id>               Resume existing session
//	cli -m "hello" --model gpt-4o    Use a specific model
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"go.temporal.io/sdk/client"

	"github.com/mfateev/codex-temporal-go/internal/cli"
	"github.com/mfateev/codex-temporal-go/internal/instructions"
	"github.com/mfateev/codex-temporal-go/internal/models"
)

func main() {
	message := flag.String("m", "", "Initial message (starts new workflow)")
	message2 := flag.String("message", "", "Initial message (alias for -m)")
	session := flag.String("session", "", "Resume existing session")
	workflowID := flag.String("workflow-id", "", "Resume existing session (alias for --session)")
	model := flag.String("model", "gpt-4o-mini", "LLM model to use")
	temporalHost := flag.String("temporal-host", client.DefaultHostPort, "Temporal server address")
	noMarkdown := flag.Bool("no-markdown", false, "Disable markdown rendering")
	noColor := flag.Bool("no-color", false, "Disable colored output")
	enableShell := flag.Bool("enable-shell", true, "Enable shell tool")
	enableRead := flag.Bool("enable-read-file", true, "Enable read_file tool")
	fullAuto := flag.Bool("full-auto", false, "Auto-approve all tool calls without prompting")
	codexHome := flag.String("codex-home", "", "Path to codex config directory (default: ~/.codex)")
	flag.Parse()

	// Support both -m and --message
	msg := *message
	if msg == "" {
		msg = *message2
	}

	// Support both --session and --workflow-id (backward compat)
	sess := *session
	if sess == "" {
		sess = *workflowID
	}

	approvalMode := models.ApprovalUnlessTrusted
	if *fullAuto {
		approvalMode = models.ApprovalNever
	}

	// Load CLI-side project docs (AGENTS.md from current project)
	cwd, _ := os.Getwd()
	var cliProjectDocs string
	if gitRoot, err := instructions.FindGitRoot(cwd); err == nil && gitRoot != "" {
		cliProjectDocs, _ = instructions.LoadProjectDocs(gitRoot, cwd)
	}

	// Load user personal instructions (~/.codex/instructions.md)
	var userPersonalInstructions string
	configDir := *codexHome
	if configDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configDir = filepath.Join(home, ".codex")
		}
	}
	if configDir != "" {
		if data, err := os.ReadFile(filepath.Join(configDir, "instructions.md")); err == nil {
			userPersonalInstructions = string(data)
		}
	}

	config := cli.Config{
		TemporalHost:             *temporalHost,
		Session:                  sess,
		Message:                  msg,
		Model:                    *model,
		NoMarkdown:               *noMarkdown,
		NoColor:                  *noColor,
		EnableShell:              *enableShell,
		EnableRead:               *enableRead,
		ApprovalMode:             approvalMode,
		CLIProjectDocs:           cliProjectDocs,
		UserPersonalInstructions: userPersonalInstructions,
	}

	app := cli.NewApp(config)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
