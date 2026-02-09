package cli

import (
	"context"
	"time"

	"go.temporal.io/sdk/client"

	"github.com/mfateev/codex-temporal-go/internal/models"
	"github.com/mfateev/codex-temporal-go/internal/workflow"
)

// PollResult holds the results from a single poll cycle.
type PollResult struct {
	Items  []models.ConversationItem
	Status workflow.TurnStatus
	Err    error
}

// Poller queries the workflow for new items and turn status.
type Poller struct {
	client     client.Client
	workflowID string
	interval   time.Duration
}

// NewPoller creates a poller for the given workflow.
func NewPoller(c client.Client, workflowID string, interval time.Duration) *Poller {
	return &Poller{
		client:     c,
		workflowID: workflowID,
		interval:   interval,
	}
}

// Poll performs a single poll cycle: queries items and turn status.
func (p *Poller) Poll(ctx context.Context) PollResult {
	var result PollResult

	// Query conversation items
	resp, err := p.client.QueryWorkflow(ctx, p.workflowID, "", workflow.QueryGetConversationItems)
	if err != nil {
		result.Err = err
		return result
	}
	if err := resp.Get(&result.Items); err != nil {
		result.Err = err
		return result
	}

	// Query turn status
	statusResp, err := p.client.QueryWorkflow(ctx, p.workflowID, "", workflow.QueryGetTurnStatus)
	if err != nil {
		result.Err = err
		return result
	}
	if err := statusResp.Get(&result.Status); err != nil {
		result.Err = err
		return result
	}

	return result
}

// RunPolling polls in a loop, sending results to the channel.
// Stops when context is cancelled.
func (p *Poller) RunPolling(ctx context.Context, ch chan<- PollResult) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result := p.Poll(ctx)
			select {
			case ch <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}
