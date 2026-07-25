package cow

import "testing"

func TestRefTableIncrDecr(t *testing.T) {
	r := NewRefTable(2)
	if r.Get(0) != 0 {
		t.Fatalf("initial count = %d, want 0", r.Get(0))
	}
	if got := r.Incr(0); got != 1 {
		t.Fatalf("incr = %d, want 1", got)
	}
	if r.Shared(0) {
		t.Fatal("single ref reported as shared")
	}
	r.Incr(0)
	if !r.Shared(0) {
		t.Fatal("two refs not reported as shared")
	}
	if got := r.Decr(0); got != 1 {
		t.Fatalf("decr = %d, want 1", got)
	}
	if got := r.Decr(0); got != 0 {
		t.Fatalf("decr to zero = %d, want 0", got)
	}
}

func TestRefTableUnderflowPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("decr below zero did not panic")
		}
	}()
	NewRefTable(1).Decr(0)
}

func TestLRUEvictsOldest(t *testing.T) {
	l := NewLRU()
	l.Touch(1)
	l.Touch(2)
	l.Touch(3)
	// Re-touching 1 makes it most recent, so 2 should evict first.
	l.Touch(1)
	if b, ok := l.Evict(); !ok || b != 2 {
		t.Fatalf("evict = %d ok=%v, want 2", b, ok)
	}
	if b, ok := l.Evict(); !ok || b != 3 {
		t.Fatalf("evict = %d ok=%v, want 3", b, ok)
	}
	if b, ok := l.Evict(); !ok || b != 1 {
		t.Fatalf("evict = %d ok=%v, want 1", b, ok)
	}
	if _, ok := l.Evict(); ok {
		t.Fatal("evict on empty returned ok")
	}
}

func TestLRURemove(t *testing.T) {
	l := NewLRU()
	l.Touch(1)
	l.Touch(2)
	l.Remove(1)
	if l.Len() != 1 {
		t.Fatalf("len after remove = %d, want 1", l.Len())
	}
	if b, ok := l.Evict(); !ok || b != 2 {
		t.Fatalf("evict = %d, want 2", b)
	}
	l.Remove(999) // no-op on untracked
}
