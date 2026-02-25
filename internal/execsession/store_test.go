package execsession

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_AllocateID_Unique(t *testing.T) {
	store := NewStore()
	seen := make(map[string]bool)

	for i := 0; i < 100; i++ {
		id := store.AllocateID()
		assert.False(t, seen[id], "duplicate ID: %s", id)
		seen[id] = true

		// Verify ID is in valid range.
		n, err := strconv.Atoi(id)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, n, MinProcessID)
		assert.Less(t, n, MaxProcessID)
	}
}

func TestStore_StoreAndGet(t *testing.T) {
	store := NewStore()
	sess := &ExecSession{
		ProcessID: "1001",
		Command:   []string{"echo", "test"},
		StartedAt: time.Now(),
		LastUsed:  time.Now(),
		exitCh:    make(chan struct{}),
		outputBuf: NewHeadTailBuffer(1024),
	}

	store.Store(sess)
	assert.Equal(t, 1, store.Count())

	got, err := store.Get("1001")
	require.NoError(t, err)
	assert.Equal(t, sess, got)
}

func TestStore_GetUnknown(t *testing.T) {
	store := NewStore()

	_, err := store.Get("9999")
	assert.ErrorIs(t, err, ErrUnknownProcessID)
}

func TestStore_Remove(t *testing.T) {
	store := NewStore()
	sess := &ExecSession{
		ProcessID: "1001",
		StartedAt: time.Now(),
		LastUsed:  time.Now(),
		exitCh:    make(chan struct{}),
		outputBuf: NewHeadTailBuffer(1024),
	}

	store.Store(sess)
	assert.Equal(t, 1, store.Count())

	store.Remove("1001")
	assert.Equal(t, 0, store.Count())

	_, err := store.Get("1001")
	assert.ErrorIs(t, err, ErrUnknownProcessID)
}

func TestStore_ReleaseID(t *testing.T) {
	store := NewStore()
	id := store.AllocateID()

	// After release, the same ID can be allocated again (eventually).
	store.ReleaseID(id)

	// Verify the ID was removed from reserved.
	store.mu.Lock()
	assert.False(t, store.reserved[id])
	store.mu.Unlock()
}

func TestStore_PruningEvictsLRUExitedFirst(t *testing.T) {
	store := NewStore()
	now := time.Now()

	// Create MaxSessions+1 sessions. Mark some as exited.
	for i := 0; i < MaxSessions+1; i++ {
		id := strconv.Itoa(2000 + i)
		sess := &ExecSession{
			ProcessID: id,
			StartedAt: now,
			LastUsed:  now.Add(time.Duration(i) * time.Millisecond), // Increasing recency
			exitCh:    make(chan struct{}),
			outputBuf: NewHeadTailBuffer(1024),
		}
		// Mark older sessions as exited.
		if i < 10 {
			sess.exited.Store(true)
			sess.exitCode.Store(0)
		}
		store.Store(sess)
	}

	// Should have pruned down to MaxSessions.
	assert.Equal(t, MaxSessions, store.Count())

	// The evicted session should be the oldest exited one (ID "2000").
	_, err := store.Get("2000")
	assert.ErrorIs(t, err, ErrUnknownProcessID, "oldest exited session should have been pruned")

	// Recent sessions should still exist.
	_, err = store.Get(strconv.Itoa(2000 + MaxSessions))
	assert.NoError(t, err, "most recent session should still exist")
}

func TestStore_PruningProtectsRecentSessions(t *testing.T) {
	store := NewStore()
	now := time.Now()

	// Fill store with exited sessions.
	for i := 0; i < MaxSessions+1; i++ {
		id := strconv.Itoa(3000 + i)
		sess := &ExecSession{
			ProcessID: id,
			StartedAt: now,
			LastUsed:  now.Add(time.Duration(i) * time.Millisecond),
			exitCh:    make(chan struct{}),
			outputBuf: NewHeadTailBuffer(1024),
		}
		sess.exited.Store(true)
		sess.exitCode.Store(0)
		store.Store(sess)
	}

	// The 8 most recent sessions (ProtectedCount) should all survive.
	for i := MaxSessions + 1 - ProtectedCount; i <= MaxSessions; i++ {
		id := strconv.Itoa(3000 + i)
		_, err := store.Get(id)
		assert.NoError(t, err, "recent session %s should be protected from pruning", id)
	}
}

func TestStore_PruningPrefersExitedOverRunning(t *testing.T) {
	store := NewStore()
	now := time.Now()

	// Add one running session (oldest).
	runningSess := &ExecSession{
		ProcessID: "running-1",
		StartedAt: now,
		LastUsed:  now,
		exitCh:    make(chan struct{}),
		outputBuf: NewHeadTailBuffer(1024),
	}
	store.Store(runningSess)

	// Add one exited session (slightly newer).
	exitedSess := &ExecSession{
		ProcessID: "exited-1",
		StartedAt: now,
		LastUsed:  now.Add(time.Millisecond),
		exitCh:    make(chan struct{}),
		outputBuf: NewHeadTailBuffer(1024),
	}
	exitedSess.exited.Store(true)
	store.Store(exitedSess)

	// Fill the rest with recent sessions to trigger pruning.
	for i := 0; i < MaxSessions; i++ {
		id := strconv.Itoa(4000 + i)
		sess := &ExecSession{
			ProcessID: id,
			StartedAt: now,
			LastUsed:  now.Add(time.Duration(100+i) * time.Millisecond),
			exitCh:    make(chan struct{}),
			outputBuf: NewHeadTailBuffer(1024),
		}
		store.Store(sess)
	}

	// The exited session should have been pruned before the running one.
	_, err := store.Get("exited-1")
	assert.ErrorIs(t, err, ErrUnknownProcessID, "exited session should have been pruned first")

	// The running session might or might not survive depending on count, but
	// the key invariant is that exited sessions are pruned preferentially.
}

func TestStore_ListAll(t *testing.T) {
	store := NewStore()

	// Empty store.
	assert.Empty(t, store.ListAll())

	// Add sessions.
	sess1 := &ExecSession{
		ProcessID: "1001",
		Command:   []string{"echo", "hello"},
		Cwd:       "/tmp",
		StartedAt: time.Now(),
		LastUsed:  time.Now(),
		exitCh:    make(chan struct{}),
		outputBuf: NewHeadTailBuffer(1024),
	}
	sess2 := &ExecSession{
		ProcessID: "1002",
		Command:   []string{"sleep", "10"},
		Cwd:       "/home",
		StartedAt: time.Now(),
		LastUsed:  time.Now(),
		exitCh:    make(chan struct{}),
		outputBuf: NewHeadTailBuffer(1024),
	}
	sess2.exited.Store(true)
	sess2.exitCode.Store(0)

	store.Store(sess1)
	store.Store(sess2)

	summaries := store.ListAll()
	assert.Len(t, summaries, 2)

	// Find by PID.
	var found1001, found1002 bool
	for _, s := range summaries {
		switch s.ProcessID {
		case "1001":
			found1001 = true
			assert.Equal(t, "echo hello", s.Command)
			assert.Equal(t, "/tmp", s.Cwd)
			assert.False(t, s.Exited)
		case "1002":
			found1002 = true
			assert.Equal(t, "sleep 10", s.Command)
			assert.True(t, s.Exited)
			assert.Equal(t, 0, s.ExitCode)
		}
	}
	assert.True(t, found1001, "session 1001 should be in list")
	assert.True(t, found1002, "session 1002 should be in list")
}

func TestStore_CloseAll(t *testing.T) {
	store := NewStore()

	// Empty store.
	assert.Equal(t, 0, store.CloseAll())

	// Add sessions.
	for i := 0; i < 3; i++ {
		id := strconv.Itoa(5000 + i)
		sess := &ExecSession{
			ProcessID: id,
			StartedAt: time.Now(),
			LastUsed:  time.Now(),
			exitCh:    make(chan struct{}),
			outputBuf: NewHeadTailBuffer(1024),
		}
		store.Store(sess)
	}
	assert.Equal(t, 3, store.Count())

	closed := store.CloseAll()
	assert.Equal(t, 3, closed)
	assert.Equal(t, 0, store.Count())
}
