package execsession

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
)

// Session store constants matching Codex.
// Maps to: codex-rs/core/src/unified_exec/process_manager.rs
const (
	MaxSessions    = 64 // Hard cap on concurrent sessions.
	ProtectedCount = 8  // Most-recent sessions protected from pruning.
	MinProcessID   = 1000
	MaxProcessID   = 100000
)

// ErrUnknownProcessID is returned when a session ID is not found.
var ErrUnknownProcessID = errors.New("unknown process ID")

// Store is a thread-safe session map with ID allocation and LRU pruning.
//
// Maps to: codex-rs/core/src/unified_exec/process_manager.rs ProcessStore
type Store struct {
	mu       sync.Mutex
	sessions map[string]*ExecSession
	reserved map[string]bool
}

// NewStore creates a new empty session store.
func NewStore() *Store {
	return &Store{
		sessions: make(map[string]*ExecSession),
		reserved: make(map[string]bool),
	}
}

// AllocateID generates a unique random process ID in [1000, 100000).
func (s *Store) AllocateID() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		id := fmt.Sprintf("%d", MinProcessID+rand.Intn(MaxProcessID-MinProcessID))
		if !s.reserved[id] {
			s.reserved[id] = true
			return id
		}
	}
}

// Store adds a session to the store, pruning if at capacity.
func (s *Store) Store(session *ExecSession) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.ProcessID] = session
	s.reserved[session.ProcessID] = true

	if len(s.sessions) > MaxSessions {
		s.pruneOneLocked()
	}
}

// Get retrieves a session by process ID, updating LastUsed.
func (s *Store) Get(processID string) (*ExecSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[processID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProcessID, processID)
	}
	return sess, nil
}

// Remove removes a session from the store and releases its ID.
func (s *Store) Remove(processID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, processID)
	delete(s.reserved, processID)
}

// ReleaseID removes a process ID from the reserved set (for short-lived
// commands that exit before being stored).
func (s *Store) ReleaseID(processID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.reserved, processID)
}

// Count returns the number of active sessions.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.sessions)
}

// pruneOneLocked evicts the least recently used session, preferring exited
// sessions over running ones. The most recent ProtectedCount sessions are
// never evicted regardless of state.
//
// Maps to: codex-rs/core/src/unified_exec/process_manager.rs process_id_to_prune_from_meta
func (s *Store) pruneOneLocked() {
	type candidate struct {
		id       string
		lastUsed int64 // UnixNano for sorting
		exited   bool
	}

	candidates := make([]candidate, 0, len(s.sessions))
	for id, sess := range s.sessions {
		sess.mu.Lock()
		lu := sess.LastUsed
		sess.mu.Unlock()
		candidates = append(candidates, candidate{
			id:       id,
			lastUsed: lu.UnixNano(),
			exited:   sess.HasExited(),
		})
	}

	// Sort by most recent first.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].lastUsed > candidates[j].lastUsed
	})

	// Protect the most recent ProtectedCount sessions.
	unprotected := candidates
	if len(unprotected) > ProtectedCount {
		unprotected = candidates[ProtectedCount:]
	} else {
		// Everything is protected; still need to evict one.
		// Pick the oldest from the full list.
		unprotected = candidates
	}

	// Prefer exited sessions (LRU among exited).
	var victim string
	for i := len(unprotected) - 1; i >= 0; i-- {
		if unprotected[i].exited {
			victim = unprotected[i].id
			break
		}
	}

	// Fallback: LRU among all unprotected.
	if victim == "" && len(unprotected) > 0 {
		victim = unprotected[len(unprotected)-1].id
	}

	if victim != "" {
		if sess, ok := s.sessions[victim]; ok {
			sess.Close()
		}
		delete(s.sessions, victim)
		delete(s.reserved, victim)
	}
}
