// Package execsession manages interactive command sessions (PTY or pipes)
// that persist across multiple tool invocations.
//
// Maps to: codex-rs/core/src/unified_exec/
package execsession

import "sync"

// DefaultMaxBytes is the default output buffer cap (1 MiB).
const DefaultMaxBytes = 1 << 20

// HeadTailBuffer is a capped output buffer that preserves a stable prefix
// ("head") and suffix ("tail"), dropping the middle once it exceeds the
// configured maximum. The buffer is symmetric: 50% of capacity is allocated
// to the head and 50% to the tail.
//
// Maps to: codex-rs/core/src/unified_exec/head_tail_buffer.rs
type HeadTailBuffer struct {
	mu         sync.Mutex
	maxBytes   int
	headBudget int
	tailBudget int
	head       [][]byte
	tail       [][]byte
	headBytes  int
	tailBytes  int
	omitted    int
	// totalEver tracks total bytes ever pushed (for DrainSince marks).
	totalEver int
}

// NewHeadTailBuffer creates a buffer that retains at most maxBytes of output.
func NewHeadTailBuffer(maxBytes int) *HeadTailBuffer {
	headBudget := maxBytes / 2
	tailBudget := maxBytes - headBudget
	return &HeadTailBuffer{
		maxBytes:   maxBytes,
		headBudget: headBudget,
		tailBudget: tailBudget,
	}
}

// Push appends a chunk of bytes to the buffer. Bytes fill the head budget
// first; overflow goes to the tail, with older tail bytes dropped to stay
// within the tail budget.
func (b *HeadTailBuffer) Push(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.totalEver += len(chunk)
	b.pushUnlocked(chunk)
}

func (b *HeadTailBuffer) pushUnlocked(chunk []byte) {
	if b.maxBytes == 0 {
		b.omitted += len(chunk)
		return
	}

	// Fill head budget first.
	if b.headBytes < b.headBudget {
		remaining := b.headBudget - b.headBytes
		if len(chunk) <= remaining {
			b.headBytes += len(chunk)
			b.head = append(b.head, copyBytes(chunk))
			return
		}
		// Split: part to head, remainder to tail.
		headPart := chunk[:remaining]
		tailPart := chunk[remaining:]
		if len(headPart) > 0 {
			b.headBytes += len(headPart)
			b.head = append(b.head, copyBytes(headPart))
		}
		b.pushToTail(copyBytes(tailPart))
		return
	}

	b.pushToTail(copyBytes(chunk))
}

func (b *HeadTailBuffer) pushToTail(chunk []byte) {
	if b.tailBudget == 0 {
		b.omitted += len(chunk)
		return
	}

	if len(chunk) >= b.tailBudget {
		// Chunk alone exceeds tail budget. Keep only last tailBudget bytes.
		start := len(chunk) - b.tailBudget
		kept := chunk[start:]
		dropped := len(chunk) - len(kept)
		b.omitted += b.tailBytes + dropped
		b.tail = [][]byte{kept}
		b.tailBytes = len(kept)
		return
	}

	b.tailBytes += len(chunk)
	b.tail = append(b.tail, chunk)
	b.trimTailToBudget()
}

func (b *HeadTailBuffer) trimTailToBudget() {
	excess := b.tailBytes - b.tailBudget
	for excess > 0 && len(b.tail) > 0 {
		front := b.tail[0]
		if excess >= len(front) {
			excess -= len(front)
			b.tailBytes -= len(front)
			b.omitted += len(front)
			b.tail = b.tail[1:]
		} else {
			b.tail[0] = front[excess:]
			b.tailBytes -= excess
			b.omitted += excess
			break
		}
	}
}

// Snapshot returns all retained output as a single byte slice (head + tail).
func (b *HeadTailBuffer) Snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.toBytesUnlocked()
}

func (b *HeadTailBuffer) toBytesUnlocked() []byte {
	size := b.headBytes + b.tailBytes
	if size == 0 {
		return nil
	}
	out := make([]byte, 0, size)
	for _, c := range b.head {
		out = append(out, c...)
	}
	for _, c := range b.tail {
		out = append(out, c...)
	}
	return out
}

// RetainedBytes returns the number of bytes currently in the buffer.
func (b *HeadTailBuffer) RetainedBytes() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.headBytes + b.tailBytes
}

// OmittedBytes returns the number of bytes dropped from the middle.
func (b *HeadTailBuffer) OmittedBytes() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.omitted
}

// TotalWritten returns total bytes ever pushed (for use as drain marks).
func (b *HeadTailBuffer) TotalWritten() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.totalEver
}

// DrainChunks removes and returns all retained chunks, resetting the buffer.
func (b *HeadTailBuffer) DrainChunks() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([][]byte, 0, len(b.head)+len(b.tail))
	out = append(out, b.head...)
	out = append(out, b.tail...)
	b.head = nil
	b.tail = nil
	b.headBytes = 0
	b.tailBytes = 0
	b.omitted = 0
	return out
}

func copyBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
