// Package cache presents a sequence-oriented key-value view over the paged
// allocator. A sequence is one client's logical byte stream; the cache maps
// logical ranges onto physical blocks via the allocator and exposes the
// operations the gRPC service needs, including copy-on-write cloning for shared
// prefixes.
package cache

import "github.com/Junny20/paged-cache/internal/allocator"

// Cache is a concurrency-safe facade over the allocator. It adds no locking of
// its own; the allocator serializes all mutating operations.
type Cache struct {
	alloc *allocator.Allocator
}

// New builds a cache backed by an arena of totalBlocks blocks, blockSize bytes
// each.
func New(blockSize, totalBlocks uint32) *Cache {
	return &Cache{alloc: allocator.New(blockSize, totalBlocks)}
}

// Allocate creates a fresh empty sequence.
func (c *Cache) Allocate() allocator.SeqID {
	return c.alloc.Allocate()
}

// Clone creates a copy-on-write child sharing parent's blocks until it diverges.
func (c *Cache) Clone(parent allocator.SeqID) (allocator.SeqID, error) {
	return c.alloc.Clone(parent)
}

// Write copies data into the sequence at the given logical offset.
func (c *Cache) Write(id allocator.SeqID, offset uint64, data []byte) (int, error) {
	return c.alloc.Write(id, offset, data)
}

// Read returns length bytes from the sequence at the given logical offset.
func (c *Cache) Read(id allocator.SeqID, offset, length uint64) ([]byte, error) {
	return c.alloc.Read(id, offset, length)
}

// Free releases a sequence.
func (c *Cache) Free(id allocator.SeqID) error {
	return c.alloc.Free(id)
}

// Stats reports arena occupancy for the load generator and observability.
func (c *Cache) Stats() (blockSize, total, freeBlocks uint32, liveSeqs uint64) {
	return c.alloc.Stats()
}
