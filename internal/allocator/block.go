package allocator

// BlockIndex is a compact handle into the arena. u32 keeps block tables small
// and cache-friendly; a 32-bit index addresses up to ~4B blocks, far beyond any
// single-host pool.
type BlockIndex uint32

// InvalidBlock marks an unset block-table slot. It is the max u32 rather than 0
// because 0 is a legitimate block index.
const InvalidBlock BlockIndex = ^BlockIndex(0)

// IsValid reports whether b refers to a real block.
func (b BlockIndex) IsValid() bool { return b != InvalidBlock }

// blockOf returns the logical block number that contains byte offset, given a
// block size in bytes.
func blockOf(offset uint64, blockSize uint32) uint64 {
	return offset / uint64(blockSize)
}

// offsetInBlock returns the byte offset within a block for a logical byte
// offset.
func offsetInBlock(offset uint64, blockSize uint32) uint32 {
	return uint32(offset % uint64(blockSize))
}
