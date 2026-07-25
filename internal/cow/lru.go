package cow

import "container/list"

// LRU tracks reclaimable blocks in least-recently-used order. Under memory
// pressure the allocator evicts the front (oldest) block so that hot shared
// prefixes stay resident. Only blocks that are safe to drop should be enrolled;
// the allocator decides that policy and calls Touch/Remove accordingly.
//
// LRU is not safe for concurrent use.
type LRU struct {
	order *list.List               // front = least recently used
	nodes map[uint32]*list.Element // block index -> element in order
}

// NewLRU returns an empty tracker.
func NewLRU() *LRU {
	return &LRU{
		order: list.New(),
		nodes: make(map[uint32]*list.Element),
	}
}

// Touch records access to block b, inserting it if new or moving it to the
// most-recently-used end if already tracked.
func (l *LRU) Touch(b uint32) {
	if e, ok := l.nodes[b]; ok {
		l.order.MoveToBack(e)
		return
	}
	l.nodes[b] = l.order.PushBack(b)
}

// Remove drops b from tracking, e.g. when it is written through or freed outside
// the eviction path. It is a no-op if b is not tracked.
func (l *LRU) Remove(b uint32) {
	if e, ok := l.nodes[b]; ok {
		l.order.Remove(e)
		delete(l.nodes, b)
	}
}

// Evict removes and returns the least-recently-used block. ok is false when
// nothing is tracked.
func (l *LRU) Evict() (b uint32, ok bool) {
	e := l.order.Front()
	if e == nil {
		return 0, false
	}
	b = e.Value.(uint32)
	l.order.Remove(e)
	delete(l.nodes, b)
	return b, true
}

// Len reports how many blocks are currently reclaimable.
func (l *LRU) Len() int { return l.order.Len() }
