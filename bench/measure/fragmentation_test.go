// Package measure holds standalone measurements for the two memory-efficiency
// claims the design targets: internal fragmentation versus per-client
// preallocation, and memory residency with versus without copy-on-write prefix
// sharing. They import the allocator directly so the figures reflect the data
// structure alone, with no gRPC or network cost mixed in.
//
// These are footprint measurements (fixed quantities), not timing benchmarks, so
// they are written as tests. Run them with:
//
//	go test -v ./bench/measure/
//
// Each test prints its result table via t.Log; -v surfaces those lines.
package measure

import (
	"math"
	"math/rand"
	"testing"

	"github.com/Junny20/paged-cache/internal/allocator"
)

// blockSize is the arena block size used across the memory benchmarks. 256 bytes
// is small enough that the last-block waste per sequence is visible against
// realistic KV-cache-like sequence lengths.
const blockSize = 256

// lognormalLengths draws n sequence lengths from a lognormal distribution, a
// reasonable stand-in for real request-length data: many short sequences with a
// long tail of large ones. mu and sigma are in log space; results are clamped to
// at least one byte.
func lognormalLengths(rng *rand.Rand, n int, mu, sigma float64) []int {
	lengths := make([]int, n)
	for i := range lengths {
		v := math.Exp(rng.NormFloat64()*sigma + mu)
		if v < 1 {
			v = 1
		}
		lengths[i] = int(v)
	}
	return lengths
}

// ceilDiv returns ceil(a / b) for non-negative integers.
func ceilDiv(a, b int) int { return (a + b - 1) / b }

// BenchmarkFragmentation measures how much memory paging actually consumes for a
// realistic length distribution versus (a) the theoretical minimum for a paged
// layout and (b) naive per-client preallocation sized to the longest sequence.
// The reported fragmentation figure is paging's overhead relative to naive
// preallocation, which is the number the design claims to cut.
func TestFragmentation(t *testing.T) {
	const (
		numSeqs = 2000
		mu      = 6.5 // exp(6.5) ~= 665 bytes median
		sigma   = 1.0
	)
	rng := rand.New(rand.NewSource(1))
	lengths := lognormalLengths(rng, numSeqs, mu, sigma)

	maxLen := 0
	sumLen := 0
	for _, l := range lengths {
		if l > maxLen {
			maxLen = l
		}
		sumLen += l
	}

	// Size the arena generously so allocation never evicts; we are measuring
	// footprint, not reclamation.
	totalBlocks := 0
	for _, l := range lengths {
		totalBlocks += ceilDiv(l, blockSize)
	}
	arena := allocator.New(blockSize, uint32(totalBlocks*2))

	payload := make([]byte, maxLen)
	for _, l := range lengths {
		id := arena.Allocate()
		if _, err := arena.Write(id, 0, payload[:l]); err != nil {
			t.Fatalf("write len=%d: %v", l, err)
		}
	}

	_, total, free, _ := arena.Stats()
	pagedBlocks := int(total - free)

	pagedBytes := pagedBlocks * blockSize         // what paging really used
	livePayload := sumLen                         // bytes clients actually wrote
	naiveBytes := numSeqs * maxLen                // per-client worst-case reservation
	pagedInternalFrag := pagedBytes - livePayload // last-block waste under paging
	savingsVsNaive := float64(naiveBytes-pagedBytes) / float64(naiveBytes) * 100

	t.Logf("sequences:                     %d", numSeqs)
	t.Logf("block size:                    %d bytes", blockSize)
	t.Logf("live payload (client bytes):   %d bytes", livePayload)
	t.Logf("paged footprint:               %d bytes (%d blocks)", pagedBytes, pagedBlocks)
	t.Logf("  internal fragmentation:      %d bytes (%.1f%% of footprint)",
		pagedInternalFrag, float64(pagedInternalFrag)/float64(pagedBytes)*100)
	t.Logf("naive preallocation footprint: %d bytes (%d x max len %d)", naiveBytes, numSeqs, maxLen)
	t.Logf("paging saves vs preallocation: %.1f%%", savingsVsNaive)
}
