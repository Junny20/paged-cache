// Package allocator implements a paged block allocator over a fixed-size arena.
// Logical byte positions within a sequence map to physical blocks through a
// per-sequence block table, so a sequence grows one block at a time without
// moving prior data and without requiring a contiguous reservation. Blocks may
// be shared between sequences via copy-on-write, tracked by reference counts,
// and reclaimed LRU-first under a memory ceiling.
package allocator

import (
	"errors"
	"sync"

	"github.com/Junny20/paged-cache/internal/cow"
)

var (
	// ErrOutOfMemory is returned when the arena is exhausted and no block can be
	// reclaimed to satisfy a request.
	ErrOutOfMemory = errors.New("allocator: out of memory")
	// ErrNoSequence is returned for operations on an unknown sequence handle.
	ErrNoSequence = errors.New("allocator: no such sequence")
	// ErrRange is returned when a read reaches past the end of a sequence.
	ErrRange = errors.New("allocator: read out of range")
)

// SeqID identifies a sequence (one client's logical byte stream).
type SeqID uint64

// sequence is one client's view of the pool: an ordered block table plus the
// logical length written so far. length lets reads reject out-of-range offsets
// without inspecting block contents.
type sequence struct {
	table  []BlockIndex
	length uint64
}

// Allocator owns the arena and all per-sequence state. Every exported method is
// safe for concurrent use; internal helpers assume the lock is held.
type Allocator struct {
	blockSize uint32

	mu          sync.Mutex
	data        []byte              // arena: totalBlocks * blockSize bytes
	free        *freeList           // unused block indices
	refs        *cow.RefTable       // per-block reference counts
	lru         *cow.LRU            // reclaimable shared blocks, LRU-first
	seqs        map[SeqID]*sequence // live sequences
	nextID      SeqID               // monotonically increasing handle source
	totalBlocks uint32
}

// New builds an allocator with totalBlocks blocks of blockSize bytes each.
func New(blockSize, totalBlocks uint32) *Allocator {
	return &Allocator{
		blockSize:   blockSize,
		data:        make([]byte, uint64(blockSize)*uint64(totalBlocks)),
		free:        newFreeList(totalBlocks),
		refs:        cow.NewRefTable(totalBlocks),
		lru:         cow.NewLRU(),
		seqs:        make(map[SeqID]*sequence),
		nextID:      1,
		totalBlocks: totalBlocks,
	}
}

// BlockSize returns the arena block size in bytes.
func (a *Allocator) BlockSize() uint32 { return a.blockSize }

// Allocate creates an empty sequence and returns its handle.
func (a *Allocator) Allocate() SeqID {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := a.nextID
	a.nextID++
	a.seqs[id] = &sequence{}
	return id
}

// Clone creates a copy-on-write child of parent. The child shares the parent's
// blocks (incrementing their refcounts) until it first writes, at which point
// the touched block is copied so the child diverges without disturbing the
// parent. Shared blocks become reclaimable and are enrolled in the LRU.
func (a *Allocator) Clone(parent SeqID) (SeqID, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p, ok := a.seqs[parent]
	if !ok {
		return 0, ErrNoSequence
	}
	child := &sequence{
		table:  make([]BlockIndex, len(p.table)),
		length: p.length,
	}
	copy(child.table, p.table)
	for _, b := range p.table {
		if b.IsValid() {
			a.refs.Incr(uint32(b))
			a.lru.Touch(uint32(b))
		}
	}
	id := a.nextID
	a.nextID++
	a.seqs[id] = child
	return id, nil
}

// Free releases a sequence, dropping a reference on each block it holds and
// returning any block whose count reaches zero to the arena.
func (a *Allocator) Free(id SeqID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.seqs[id]
	if !ok {
		return ErrNoSequence
	}
	for _, b := range s.table {
		if b.IsValid() {
			a.release(b)
		}
	}
	delete(a.seqs, id)
	return nil
}

// Write copies data into the sequence starting at logical offset, growing the
// block table as needed and honouring copy-on-write for shared blocks. It
// returns the number of bytes written, which equals len(data) unless the arena
// is exhausted.
func (a *Allocator) Write(id SeqID, offset uint64, data []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.seqs[id]
	if !ok {
		return 0, ErrNoSequence
	}

	written := 0
	pos := offset
	for written < len(data) {
		lb := blockOf(pos, a.blockSize)
		if err := a.ensureBlock(s, lb); err != nil {
			return written, err
		}

		phys := s.table[lb]
		// Copy-on-write: a shared block must be privately copied before we
		// mutate it, so other referencing sequences are not disturbed.
		if a.refs.Shared(uint32(phys)) {
			np, err := a.copyBlock(phys)
			if err != nil {
				return written, err
			}
			a.refs.Decr(uint32(phys))
			phys = np
			s.table[lb] = phys
			a.refs.Incr(uint32(phys))
		}
		// A block we are about to write through is no longer a clean shared
		// prefix candidate for eviction.
		a.lru.Remove(uint32(phys))

		start := offsetInBlock(pos, a.blockSize)
		n := copy(a.blockBytes(phys)[start:], data[written:])
		written += n
		pos += uint64(n)
	}

	if pos > s.length {
		s.length = pos
	}
	return written, nil
}

// Read returns length bytes from the sequence starting at logical offset.
func (a *Allocator) Read(id SeqID, offset, length uint64) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.seqs[id]
	if !ok {
		return nil, ErrNoSequence
	}
	if offset+length > s.length {
		return nil, ErrRange
	}

	out := make([]byte, length)
	filled := uint64(0)
	pos := offset
	for filled < length {
		lb := blockOf(pos, a.blockSize)
		phys := s.table[lb]
		start := offsetInBlock(pos, a.blockSize)
		n := copy(out[filled:], a.blockBytes(phys)[start:])
		if remaining := length - filled; uint64(n) > remaining {
			n = int(remaining)
		}
		filled += uint64(n)
		pos += uint64(n)
	}
	return out, nil
}

// Stats reports arena occupancy.
func (a *Allocator) Stats() (blockSize, total, freeBlocks uint32, liveSeqs uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.blockSize, a.totalBlocks, uint32(a.free.len()), uint64(len(a.seqs))
}

// ensureBlock guarantees the sequence's block table has a physical block backing
// logical block lb, extending the table and allocating fresh blocks as needed.
// Assumes the lock is held.
func (a *Allocator) ensureBlock(s *sequence, lb uint64) error {
	for uint64(len(s.table)) <= lb {
		s.table = append(s.table, InvalidBlock)
	}
	if s.table[lb].IsValid() {
		return nil
	}
	b, err := a.acquire()
	if err != nil {
		return err
	}
	a.refs.Incr(uint32(b))
	s.table[lb] = b
	return nil
}

// acquire hands out a free block, evicting the LRU shared block if the free list
// is empty. Assumes the lock is held.
func (a *Allocator) acquire() (BlockIndex, error) {
	if b, ok := a.free.pop(); ok {
		return b, nil
	}
	if victim, ok := a.lru.Evict(); ok {
		a.release(BlockIndex(victim))
		if b, ok := a.free.pop(); ok {
			return b, nil
		}
	}
	return InvalidBlock, ErrOutOfMemory
}

// copyBlock allocates a fresh block and copies src's bytes into it, the physical
// half of the copy-on-write path. Assumes the lock is held.
func (a *Allocator) copyBlock(src BlockIndex) (BlockIndex, error) {
	dst, err := a.acquire()
	if err != nil {
		return InvalidBlock, err
	}
	copy(a.blockBytes(dst), a.blockBytes(src))
	return dst, nil
}

// release drops one reference to b and returns it to the free list when the
// count reaches zero. Assumes the lock is held.
func (a *Allocator) release(b BlockIndex) {
	if a.refs.Decr(uint32(b)) == 0 {
		a.lru.Remove(uint32(b))
		a.free.push(b)
	}
}

// blockBytes returns the arena slice backing physical block b.
func (a *Allocator) blockBytes(b BlockIndex) []byte {
	start := uint64(b) * uint64(a.blockSize)
	return a.data[start : start+uint64(a.blockSize)]
}
