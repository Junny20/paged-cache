package allocator

import (
	"bytes"
	"errors"
	"testing"
)

func TestFreeListRoundTrip(t *testing.T) {
	f := newFreeList(3)
	if f.len() != 3 {
		t.Fatalf("len = %d, want 3", f.len())
	}
	seen := map[BlockIndex]bool{}
	for i := 0; i < 3; i++ {
		b, ok := f.pop()
		if !ok {
			t.Fatalf("pop %d: empty", i)
		}
		seen[b] = true
	}
	if _, ok := f.pop(); ok {
		t.Fatal("pop on empty list returned ok")
	}
	for _, want := range []BlockIndex{0, 1, 2} {
		if !seen[want] {
			t.Fatalf("block %d never handed out", want)
		}
	}
	f.push(1)
	if f.len() != 1 {
		t.Fatalf("len after push = %d, want 1", f.len())
	}
}

func TestWriteReadWithinBlock(t *testing.T) {
	a := New(16, 4)
	id := a.Allocate()
	msg := []byte("hello")
	n, err := a.Write(id, 0, msg)
	if err != nil || n != len(msg) {
		t.Fatalf("write: n=%d err=%v", n, err)
	}
	got, err := a.Read(id, 0, uint64(len(msg)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("read %q, want %q", got, msg)
	}
}

func TestWriteSpansBlocks(t *testing.T) {
	a := New(4, 8) // 4-byte blocks force multi-block writes
	id := a.Allocate()
	msg := []byte("abcdefghij") // 10 bytes -> 3 blocks
	if _, err := a.Write(id, 0, msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := a.Read(id, 0, uint64(len(msg)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("read %q, want %q", got, msg)
	}
	// Partial read across a block boundary.
	got, err = a.Read(id, 2, 5)
	if err != nil {
		t.Fatalf("partial read: %v", err)
	}
	if !bytes.Equal(got, msg[2:7]) {
		t.Fatalf("partial read %q, want %q", got, msg[2:7])
	}
}

func TestReadOutOfRange(t *testing.T) {
	a := New(16, 4)
	id := a.Allocate()
	if _, err := a.Write(id, 0, []byte("hi")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := a.Read(id, 0, 3); !errors.Is(err, ErrRange) {
		t.Fatalf("read past end: err=%v, want ErrRange", err)
	}
}

func TestUnknownSequence(t *testing.T) {
	a := New(16, 4)
	if _, err := a.Write(SeqID(999), 0, []byte("x")); !errors.Is(err, ErrNoSequence) {
		t.Fatalf("write unknown: err=%v, want ErrNoSequence", err)
	}
	if err := a.Free(SeqID(999)); !errors.Is(err, ErrNoSequence) {
		t.Fatalf("free unknown: err=%v, want ErrNoSequence", err)
	}
}

func TestFreeReturnsBlocks(t *testing.T) {
	a := New(4, 4)
	id := a.Allocate()
	if _, err := a.Write(id, 0, []byte("abcdefgh")); err != nil { // 2 blocks
		t.Fatalf("write: %v", err)
	}
	_, _, free, _ := a.Stats()
	if free != 2 {
		t.Fatalf("free after write = %d, want 2", free)
	}
	if err := a.Free(id); err != nil {
		t.Fatalf("free: %v", err)
	}
	_, _, free, live := a.Stats()
	if free != 4 {
		t.Fatalf("free after free = %d, want 4", free)
	}
	if live != 0 {
		t.Fatalf("live sequences = %d, want 0", live)
	}
}

func TestOutOfMemory(t *testing.T) {
	a := New(4, 2) // only 2 blocks, 8 bytes total
	id := a.Allocate()
	n, err := a.Write(id, 0, []byte("abcdefghij")) // needs 3 blocks
	if !errors.Is(err, ErrOutOfMemory) {
		t.Fatalf("err = %v, want ErrOutOfMemory", err)
	}
	if n != 8 {
		t.Fatalf("bytes written before OOM = %d, want 8", n)
	}
}

func TestEvictionReclaimsSharedBlocks(t *testing.T) {
	// 2 blocks total. Parent fills both; a clone shares them, making them
	// reclaimable. A fresh sequence should then force eviction to get a block.
	a := New(4, 2)
	parent := a.Allocate()
	if _, err := a.Write(parent, 0, []byte("abcdefgh")); err != nil { // 2 blocks
		t.Fatalf("parent write: %v", err)
	}
	if _, err := a.Clone(parent); err != nil {
		t.Fatalf("clone: %v", err)
	}
	// Freeing the parent drops one ref on each shared block; the clone still
	// holds them but they remain LRU-tracked and reclaimable.
	if err := a.Free(parent); err != nil {
		t.Fatalf("free parent: %v", err)
	}
	fresh := a.Allocate()
	if _, err := a.Write(fresh, 0, []byte("XY")); err != nil {
		t.Fatalf("fresh write should evict, got: %v", err)
	}
	got, err := a.Read(fresh, 0, 2)
	if err != nil || !bytes.Equal(got, []byte("XY")) {
		t.Fatalf("fresh read = %q err=%v", got, err)
	}
}
