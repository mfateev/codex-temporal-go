package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chzyer/readline"
	"github.com/google/uuid"
	"go.temporal.io/sdk/client"

	"github.com/mfateev/codex-temporal-go/internal/models"
	"github.com/mfateev/codex-temporal-go/internal/workflow"
)

const (
	TaskQueue    = "codex-temporal"
	PollInterval = 200 * time.Millisecond
)

// State represents the CLI state machine state.
type State int

const (
	StateStartup State = iota
	StateInput
	StateWatching
	StateInterrupted
	StateShutdown
)

// Config holds CLI configuration.
type Config struct {
	TemporalHost string
	WorkflowID   string // Resume existing workflow
	Message      string // Initial message for new workflow
	Model        string
	NoMarkdown   bool
	NoColor      bool
	EnableShell  bool
	EnableRead   bool
	Cwd          string
}

// App is the interactive CLI application.
type App struct {
	config   Config
	client   client.Client
	renderer *Renderer
	spinner  *Spinner
	poller   *Poller

	workflowID      string
	state           State
	lastRenderedSeq int

	// Channels
	pollCh  chan PollResult
	inputCh chan string
	sigCh   chan os.Signal

	// Ctrl+C tracking
	lastInterruptTime time.Time
	interruptMu       sync.Mutex

	// Readline instance
	rl *readline.Instance
}

// NewApp creates a new CLI app.
func NewApp(config Config) *App {
	return &App{
		config:          config,
		lastRenderedSeq: -1,
		pollCh:          make(chan PollResult, 1),
		inputCh:         make(chan string, 1),
		sigCh:           make(chan os.Signal, 1),
	}
}

// Run is the main entry point.
func (a *App) Run() error {
	// Connect to Temporal
	c, err := client.Dial(client.Options{
		HostPort: a.config.TemporalHost,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Temporal: %w", err)
	}
	defer c.Close()
	a.client = c

	// Set up renderer and spinner
	a.renderer = NewRenderer(os.Stdout, a.config.NoColor, a.config.NoMarkdown)
	a.spinner = NewSpinner(os.Stderr)

	// Set up readline
	a.rl, err = readline.NewEx(&readline.Config{
		Prompt:          "> ",
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return fmt.Errorf("failed to init readline: %w", err)
	}
	defer a.rl.Close()

	// Set up signal handling
	signal.Notify(a.sigCh, syscall.SIGINT)
	defer signal.Stop(a.sigCh)

	// Startup: either resume or start new workflow
	if a.config.WorkflowID != "" {
		if err := a.resumeWorkflow(); err != nil {
			return err
		}
	} else {
		// If no initial message, prompt for one
		if a.config.Message == "" {
			fmt.Fprintf(os.Stderr, "codex-temporal (type /exit to quit)\n")
			line, err := a.rl.Readline()
			if err != nil {
				return nil // User cancelled
			}
			line = strings.TrimSpace(line)
			if line == "" || line == "/exit" || line == "/quit" {
				return nil
			}
			a.config.Message = line
		}

		if err := a.startWorkflow(); err != nil {
			return err
		}
	}

	// Main loop
	return a.mainLoop()
}

func (a *App) startWorkflow() error {
	a.workflowID = fmt.Sprintf("codex-%s", uuid.New().String()[:8])

	cwd := a.config.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	input := workflow.WorkflowInput{
		ConversationID: a.workflowID,
		UserMessage:    a.config.Message,
		Config: models.SessionConfiguration{
			Model: models.ModelConfig{
				Model:         a.config.Model,
				Temperature:   0.7,
				MaxTokens:     4096,
				ContextWindow: 128000,
			},
			Tools: models.ToolsConfig{
				EnableShell:    a.config.EnableShell,
				EnableReadFile: a.config.EnableRead,
			},
			Cwd:           cwd,
			SessionSource: "interactive-cli",
		},
	}

	ctx := context.Background()
	_, err := a.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        a.workflowID,
		TaskQueue: TaskQueue,
	}, "AgenticWorkflow", input)
	if err != nil {
		return fmt.Errorf("failed to start workflow: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Session: %s\n", a.workflowID)

	if a.config.Message != "" {
		// We sent the initial message, go to watching state
		a.state = StateWatching
	} else {
		a.state = StateInput
	}

	return nil
}

func (a *App) resumeWorkflow() error {
	a.workflowID = a.config.WorkflowID

	fmt.Fprintf(os.Stderr, "Resuming session: %s\n", a.workflowID)

	// Fetch and render existing history
	ctx := context.Background()
	poller := NewPoller(a.client, a.workflowID, PollInterval)
	result := poller.Poll(ctx)
	if result.Err != nil {
		return fmt.Errorf("failed to query workflow: %w", result.Err)
	}

	// Render history items
	if len(result.Items) > 0 {
		fmt.Fprintf(os.Stderr, "... %d previous items ...\n", len(result.Items))
		// Show last few items for context
		start := 0
		if len(result.Items) > 20 {
			start = len(result.Items) - 20
			fmt.Fprintf(os.Stderr, "... showing last %d items ...\n", len(result.Items)-start)
		}
		for _, item := range result.Items[start:] {
			a.renderer.RenderItemForResume(item)
		}
		a.lastRenderedSeq = result.Items[len(result.Items)-1].Seq
	}

	// Determine initial state based on turn status
	if result.Status.Phase == workflow.PhaseWaitingForInput {
		a.state = StateInput
	} else {
		a.state = StateWatching
	}

	return nil
}

func (a *App) mainLoop() error {
	// Set up poller
	a.poller = NewPoller(a.client, a.workflowID, PollInterval)

	var pollCancel context.CancelFunc
	var inputDone chan struct{}

	startPolling := func() {
		if pollCancel != nil {
			pollCancel()
		}
		var pollCtx context.Context
		pollCtx, pollCancel = context.WithCancel(context.Background())
		go a.poller.RunPolling(pollCtx, a.pollCh)
	}

	stopPolling := func() {
		if pollCancel != nil {
			pollCancel()
			pollCancel = nil
		}
	}

	startInput := func() {
		inputDone = make(chan struct{})
		go func() {
			defer close(inputDone)
			a.readInput()
		}()
	}

	// Start in the appropriate mode
	switch a.state {
	case StateWatching:
		startPolling()
		a.spinner.Start("Thinking...")
	case StateInput:
		startInput()
	}

	defer stopPolling()

	for {
		select {
		case line := <-a.inputCh:
			line = strings.TrimSpace(line)
			if line == "" {
				startInput()
				continue
			}

			// Handle special commands
			if line == "/exit" || line == "/quit" {
				a.state = StateShutdown
				a.spinner.Start("Shutting down...")
				if err := a.sendShutdown(); err != nil {
					fmt.Fprintf(os.Stderr, "Error sending shutdown: %v\n", err)
				}
				return a.waitForCompletion()
			}

			// Send user input to workflow
			if err := a.sendUserInput(line); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				startInput()
				continue
			}

			// Transition to watching
			a.state = StateWatching
			a.spinner.Start("Thinking...")
			startPolling()

		case result := <-a.pollCh:
			if result.Err != nil {
				// Check if workflow completed
				if isWorkflowCompleted(result.Err) {
					a.spinner.Stop()
					fmt.Fprintf(os.Stderr, "Session ended.\n")
					return nil
				}
				// Transient error (e.g., during ContinueAsNew) — ignore
				continue
			}

			// Render new items
			a.renderNewItems(result.Items)

			// Update spinner message based on phase
			a.spinner.SetMessage(PhaseMessage(result.Status.Phase, result.Status.ToolsInFlight))

			// Check if turn is complete
			if a.isTurnComplete(result.Items) && result.Status.Phase == workflow.PhaseWaitingForInput {
				a.spinner.Stop()

				// Render status line
				a.renderer.RenderStatusLine(a.config.Model, result.Status.TotalTokens, result.Status.TurnCount)

				// Transition to input
				stopPolling()
				a.state = StateInput
				startInput()
			}

		case <-a.sigCh:
			a.handleInterrupt(startPolling, stopPolling, startInput)
			if a.state == StateShutdown {
				return a.waitForCompletion()
			}
		}
	}
}

func (a *App) readInput() {
	line, err := a.rl.Readline()
	if err != nil {
		if err == readline.ErrInterrupt {
			// Ctrl+C during input — send to sigCh
			a.sigCh <- syscall.SIGINT
			return
		}
		if err == io.EOF {
			// Ctrl+D — exit
			a.inputCh <- "/exit"
			return
		}
		return
	}
	a.inputCh <- line
}

func (a *App) sendUserInput(content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	updateHandle, err := a.client.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   a.workflowID,
		UpdateName:   workflow.UpdateUserInput,
		Args:         []interface{}{workflow.UserInput{Content: content}},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		return err
	}

	var accepted workflow.UserInputAccepted
	return updateHandle.Get(ctx, &accepted)
}

func (a *App) sendInterrupt() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	updateHandle, err := a.client.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   a.workflowID,
		UpdateName:   workflow.UpdateInterrupt,
		Args:         []interface{}{workflow.InterruptRequest{}},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		return err
	}

	var resp workflow.InterruptResponse
	return updateHandle.Get(ctx, &resp)
}

func (a *App) sendShutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	updateHandle, err := a.client.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   a.workflowID,
		UpdateName:   workflow.UpdateShutdown,
		Args:         []interface{}{workflow.ShutdownRequest{}},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		return err
	}

	var resp workflow.ShutdownResponse
	return updateHandle.Get(ctx, &resp)
}

func (a *App) handleInterrupt(startPolling, stopPolling, startInput func()) {
	a.interruptMu.Lock()
	defer a.interruptMu.Unlock()

	now := time.Now()

	switch a.state {
	case StateWatching:
		if now.Sub(a.lastInterruptTime) < 2*time.Second {
			// Second Ctrl+C within 2s — shutdown
			a.spinner.Stop()
			fmt.Fprintf(os.Stderr, "\nShutting down...\n")
			a.state = StateShutdown
			_ = a.sendShutdown()
			return
		}

		// First Ctrl+C — interrupt current turn
		a.lastInterruptTime = now
		a.spinner.Stop()
		fmt.Fprintf(os.Stderr, "\nInterrupting... (press Ctrl+C again to exit)\n")
		_ = a.sendInterrupt()

		// Stay in watching mode, wait for turn_complete(interrupted)
		a.spinner.Start("Interrupting...")

	case StateInput:
		// Ctrl+C during input — shutdown
		fmt.Fprintf(os.Stderr, "\nShutting down...\n")
		a.state = StateShutdown
		_ = a.sendShutdown()

	case StateInterrupted:
		// Already interrupted — force shutdown
		fmt.Fprintf(os.Stderr, "\nForce shutting down...\n")
		a.state = StateShutdown
		_ = a.sendShutdown()
	}
}

func (a *App) renderNewItems(items []models.ConversationItem) {
	rendered := false
	for _, item := range items {
		if item.Seq <= a.lastRenderedSeq {
			continue
		}
		if !rendered {
			// Stop spinner once before rendering batch
			a.spinner.Stop()
			rendered = true
		}
		a.renderer.RenderItem(item)
		a.lastRenderedSeq = item.Seq
	}
}

func (a *App) isTurnComplete(items []models.ConversationItem) bool {
	for _, item := range items {
		if item.Seq <= a.lastRenderedSeq-1 {
			continue
		}
		if item.Type == models.ItemTypeTurnComplete {
			return true
		}
	}
	return false
}

func (a *App) waitForCompletion() error {
	// Wait briefly for workflow to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	run := a.client.GetWorkflow(ctx, a.workflowID, "")
	var result workflow.WorkflowResult
	if err := run.Get(ctx, &result); err != nil {
		// Workflow might take time to complete, that's OK
		fmt.Fprintf(os.Stderr, "Session closed.\n")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Session ended. Tokens: %d, Tools: %d\n",
		result.TotalTokens, len(result.ToolCallsExecuted))
	return nil
}

func isWorkflowCompleted(err error) bool {
	// Check for common "workflow not found" or "completed" errors
	errStr := err.Error()
	return strings.Contains(errStr, "workflow execution already completed") ||
		strings.Contains(errStr, "not found")
}
