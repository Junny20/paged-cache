package allocator

// freeList tracks unused block indices as a LIFO stack. Push and pop are O(1);
// LIFO reuse keeps recently freed (and likely cache-warm) blocks at hand. The
// list is not safe for concurrent use and is always accessed under the
// allocator's lock.
type freeList struct {
	indices []BlockIndex
}

// newFreeList seeds the list with every block index in [0, n) so a fresh arena
// starts fully free.
func newFreeList(n uint32) *freeList {
	indices := make([]BlockIndex, n)
	for i := range indices {
		// Reverse order so pop hands out block 0 first, which makes tests and
		// dumps read naturally.
		indices[i] = BlockIndex(n - 1 - uint32(i))
	}
	return &freeList{indices: indices}
}

// pop removes and returns a free block index. ok is false when the list is
// empty, signalling out-of-memory to the caller.
func (f *freeList) pop() (BlockIndex, bool) {
	n := len(f.indices)
	if n == 0 {
		return InvalidBlock, false
	}
	b := f.indices[n-1]
	f.indices = f.indices[:n-1]
	return b, true
}

// push returns a block to the free list.
func (f *freeList) push(b BlockIndex) {
	f.indices = append(f.indices, b)
}

// len reports how many blocks are currently free.
func (f *freeList) len() int { return len(f.indices) }
