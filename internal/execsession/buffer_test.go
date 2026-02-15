package execsession

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ported from: codex-rs/core/src/unified_exec/head_tail_buffer.rs tests

func TestHeadTailBuffer_KeepsPrefixAndSuffixWhenOverBudget(t *testing.T) {
	buf := NewHeadTailBuffer(10)

	buf.Push([]byte("0123456789"))
	assert.Equal(t, 0, buf.OmittedBytes())

	// Exceeds max by 2; should keep head+tail and omit the middle.
	buf.Push([]byte("ab"))
	assert.Greater(t, buf.OmittedBytes(), 0)

	out := string(buf.Snapshot())
	assert.True(t, len(out) <= 10, "output should not exceed max bytes")
	assert.Equal(t, "01234", out[:5], "should start with head")
	assert.Equal(t, "89ab", out[len(out)-4:], "should end with tail")
}

func TestHeadTailBuffer_MaxBytesZeroDropsEverything(t *testing.T) {
	buf := NewHeadTailBuffer(0)
	buf.Push([]byte("abc"))

	assert.Equal(t, 0, buf.RetainedBytes())
	assert.Equal(t, 3, buf.OmittedBytes())
	assert.Nil(t, buf.Snapshot())
}

func TestHeadTailBuffer_HeadBudgetZeroKeepsOnlyLastByteInTail(t *testing.T) {
	buf := NewHeadTailBuffer(1)
	buf.Push([]byte("abc"))

	assert.Equal(t, 1, buf.RetainedBytes())
	assert.Equal(t, 2, buf.OmittedBytes())
	assert.Equal(t, []byte("c"), buf.Snapshot())
}

func TestHeadTailBuffer_DrainingResetsState(t *testing.T) {
	buf := NewHeadTailBuffer(10)
	buf.Push([]byte("0123456789"))
	buf.Push([]byte("ab"))

	drained := buf.DrainChunks()
	require.NotEmpty(t, drained)

	assert.Equal(t, 0, buf.RetainedBytes())
	assert.Equal(t, 0, buf.OmittedBytes())
	assert.Nil(t, buf.Snapshot())
}

func TestHeadTailBuffer_ChunkLargerThanTailBudgetKeepsOnlyTailEnd(t *testing.T) {
	buf := NewHeadTailBuffer(10)
	buf.Push([]byte("0123456789"))

	// Tail budget is 5 bytes. This chunk replaces the tail keeping only last 5.
	buf.Push([]byte("ABCDEFGHIJK"))

	out := string(buf.Snapshot())
	assert.Equal(t, "01234", out[:5], "head preserved")
	assert.Equal(t, "GHIJK", out[5:], "only last 5 bytes of chunk in tail")
	assert.Greater(t, buf.OmittedBytes(), 0)
}

func TestHeadTailBuffer_FillsHeadThenTailAcrossMultipleChunks(t *testing.T) {
	buf := NewHeadTailBuffer(10)

	// Fill the 5-byte head budget across multiple chunks.
	buf.Push([]byte("01"))
	buf.Push([]byte("234"))
	assert.Equal(t, []byte("01234"), buf.Snapshot())

	// Fill the 5-byte tail budget.
	buf.Push([]byte("567"))
	buf.Push([]byte("89"))
	assert.Equal(t, []byte("0123456789"), buf.Snapshot())
	assert.Equal(t, 0, buf.OmittedBytes())

	// One more byte causes the tail to drop its oldest byte.
	buf.Push([]byte("a"))
	assert.Equal(t, []byte("012346789a"), buf.Snapshot())
	assert.Equal(t, 1, buf.OmittedBytes())
}

func TestHeadTailBuffer_TotalWrittenTracksAllPushes(t *testing.T) {
	buf := NewHeadTailBuffer(10)
	buf.Push([]byte("hello"))
	buf.Push([]byte("world"))
	buf.Push([]byte("!!!"))

	assert.Equal(t, 13, buf.TotalWritten())
}

func TestHeadTailBuffer_EmptyPushIgnored(t *testing.T) {
	buf := NewHeadTailBuffer(10)
	buf.Push(nil)
	buf.Push([]byte{})

	assert.Equal(t, 0, buf.RetainedBytes())
	assert.Equal(t, 0, buf.TotalWritten())
}

func TestHeadTailBuffer_ConcurrentAccess(t *testing.T) {
	buf := NewHeadTailBuffer(1024)
	done := make(chan struct{})

	// Concurrent writers.
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				buf.Push([]byte("data"))
			}
			done <- struct{}{}
		}()
	}

	// Concurrent readers.
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = buf.Snapshot()
				_ = buf.RetainedBytes()
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 15; i++ {
		<-done
	}

	assert.Equal(t, 4000, buf.TotalWritten())
}
