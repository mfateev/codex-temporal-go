package activities

import (
	"context"
	"time"

	"github.com/mfateev/temporal-agent-harness/internal/execsession"
)

// ExecSessionActivities contains exec-session management activities.
type ExecSessionActivities struct {
	store *execsession.Store
}

// NewExecSessionActivities creates a new ExecSessionActivities instance.
func NewExecSessionActivities(store *execsession.Store) *ExecSessionActivities {
	return &ExecSessionActivities{store: store}
}

// ExecSessionSummary is a serialization-compatible view of an exec session.
// Matches workflow.ExecSessionSummary for Temporal serialization.
type ExecSessionSummary struct {
	ProcessID string    `json:"process_id"`
	Command   string    `json:"command"`
	Cwd       string    `json:"cwd"`
	StartedAt time.Time `json:"started_at"`
	Exited    bool      `json:"exited"`
	ExitCode  int       `json:"exit_code"`
}

// ListExecSessionsRequest is the payload for the ListExecSessions activity.
type ListExecSessionsRequest struct{}

// ListExecSessionsResponse is the output of the ListExecSessions activity.
type ListExecSessionsResponse struct {
	Sessions []ExecSessionSummary `json:"sessions"`
}

// CleanExecSessionsRequest is the payload for the CleanExecSessions activity.
type CleanExecSessionsRequest struct{}

// CleanExecSessionsResponse is the output of the CleanExecSessions activity.
type CleanExecSessionsResponse struct {
	Closed int `json:"closed"`
}

// ListExecSessions returns a summary of all exec sessions.
func (a *ExecSessionActivities) ListExecSessions(_ context.Context, _ ListExecSessionsRequest) (ListExecSessionsResponse, error) {
	storeSummaries := a.store.ListAll()
	summaries := make([]ExecSessionSummary, len(storeSummaries))
	for i, s := range storeSummaries {
		summaries[i] = ExecSessionSummary{
			ProcessID: s.ProcessID,
			Command:   s.Command,
			Cwd:       s.Cwd,
			StartedAt: s.StartedAt,
			Exited:    s.Exited,
			ExitCode:  s.ExitCode,
		}
	}
	return ListExecSessionsResponse{Sessions: summaries}, nil
}

// CleanExecSessions closes all exec sessions and returns the count.
func (a *ExecSessionActivities) CleanExecSessions(_ context.Context, _ CleanExecSessionsRequest) (CleanExecSessionsResponse, error) {
	closed := a.store.CloseAll()
	return CleanExecSessionsResponse{Closed: closed}, nil
}
