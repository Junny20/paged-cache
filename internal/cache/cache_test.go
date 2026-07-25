package cache

import (
	"bytes"
	"testing"
)

func TestCacheWriteRead(t *testing.T) {
	c := New(16, 8)
	id := c.Allocate()
	msg := []byte("paged cache")
	if _, err := c.Write(id, 0, msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := c.Read(id, 0, uint64(len(msg)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("read %q, want %q", got, msg)
	}
}

func TestClonePreservesPrefix(t *testing.T) {
	c := New(8, 16)
	parent := c.Allocate()
	prefix := []byte("shared-prompt")
	if _, err := c.Write(parent, 0, prefix); err != nil {
		t.Fatalf("parent write: %v", err)
	}
	child, err := c.Clone(parent)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	got, err := c.Read(child, 0, uint64(len(prefix)))
	if err != nil {
		t.Fatalf("child read: %v", err)
	}
	if !bytes.Equal(got, prefix) {
		t.Fatalf("child sees %q, want %q", got, prefix)
	}
}

func TestCopyOnWriteDivergence(t *testing.T) {
	c := New(8, 16)
	parent := c.Allocate()
	base := []byte("AAAAAAAAAAAAAAAA") // 16 bytes -> 2 shared blocks
	if _, err := c.Write(parent, 0, base); err != nil {
		t.Fatalf("parent write: %v", err)
	}
	child, err := c.Clone(parent)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	// Child overwrites the first block; the parent must be unaffected.
	if _, err := c.Write(child, 0, []byte("BBBBBBBB")); err != nil {
		t.Fatalf("child write: %v", err)
	}

	pGot, err := c.Read(parent, 0, 16)
	if err != nil {
		t.Fatalf("parent read: %v", err)
	}
	if !bytes.Equal(pGot, base) {
		t.Fatalf("parent mutated by child: %q", pGot)
	}

	cGot, err := c.Read(child, 0, 16)
	if err != nil {
		t.Fatalf("child read: %v", err)
	}
	want := []byte("BBBBBBBBAAAAAAAA")
	if !bytes.Equal(cGot, want) {
		t.Fatalf("child = %q, want %q", cGot, want)
	}
}

func TestFreeParentKeepsChildData(t *testing.T) {
	c := New(8, 16)
	parent := c.Allocate()
	msg := []byte("survivor")
	if _, err := c.Write(parent, 0, msg); err != nil {
		t.Fatalf("parent write: %v", err)
	}
	child, err := c.Clone(parent)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if err := c.Free(parent); err != nil {
		t.Fatalf("free parent: %v", err)
	}
	got, err := c.Read(child, 0, uint64(len(msg)))
	if err != nil {
		t.Fatalf("child read after parent free: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("child data lost: %q, want %q", got, msg)
	}
}
