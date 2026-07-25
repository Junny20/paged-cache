// Package cow provides the reference-counting and eviction bookkeeping that
// backs copy-on-write prefix sharing in the paged allocator. It tracks how many
// sequences point at each physical block and, through the companion LRU, which
// shared block to reclaim first under memory pressure.
package cow

// RefTable maps a physical block index to the number of sequences that
// reference it. It is deliberately unaware of the arena: it only counts. The
// allocator consults it to decide whether a write may proceed in place (refcount
// == 1) or must copy first (refcount > 1), and whether a free returns the block
// to the arena (refcount reaches 0).
//
// RefTable is not safe for concurrent use; callers serialize access with the
// allocator lock.
type RefTable struct {
	counts []uint32
}

// NewRefTable allocates a table sized for n blocks, all starting at zero
// references.
func NewRefTable(n uint32) *RefTable {
	return &RefTable{counts: make([]uint32, n)}
}

// Get returns the current reference count for block b.
func (r *RefTable) Get(b uint32) uint32 { return r.counts[b] }

// Incr adds a reference and returns the new count. It is used both when a block
// is first handed to a sequence and when a clone shares an existing block.
func (r *RefTable) Incr(b uint32) uint32 {
	r.counts[b]++
	return r.counts[b]
}

// Decr removes a reference and returns the new count. A result of zero tells the
// caller the block is now unreferenced and may return to the free list.
func (r *RefTable) Decr(b uint32) uint32 {
	if r.counts[b] == 0 {
		panic("cow: refcount underflow")
	}
	r.counts[b]--
	return r.counts[b]
}

// Shared reports whether block b is referenced by more than one sequence, i.e.
// whether a write to it must copy first.
func (r *RefTable) Shared(b uint32) bool { return r.counts[b] > 1 }
