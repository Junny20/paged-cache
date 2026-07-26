package measure

import (
	"testing"

	"github.com/Junny20/paged-cache/internal/allocator"
)

// BenchmarkResidency measures memory residency for a workload where many
// sequences share a common prefix (e.g. a shared system prompt), comparing
// copy-on-write cloning against giving each sequence its own independent copy.
// It reports blocks resident in each case and the residency saved by sharing.
//
// The workload: one parent holds a shared prefix; each of numClients sequences
// derives from it and then writes a small private suffix that diverges only its
// own last block. Under COW the shared prefix blocks are held once; under
// independent copies they are duplicated per client.
func TestResidency(t *testing.T) {
	const (
		numClients   = 500
		prefixBytes  = 8192 // shared prefix (e.g. system prompt)
		suffixBytes  = 128  // per-client divergent tail
		blocksNeeded = 1 << 20
	)

	prefix := make([]byte, prefixBytes)
	suffix := make([]byte, suffixBytes)

	// Copy-on-write: clone from a shared parent, then write a private tail.
	cow := allocator.New(blockSize, blocksNeeded)
	parent := cow.Allocate()
	if _, err := cow.Write(parent, 0, prefix); err != nil {
		t.Fatalf("cow parent write: %v", err)
	}
	for i := 0; i < numClients; i++ {
		child, err := cow.Clone(parent)
		if err != nil {
			t.Fatalf("cow clone %d: %v", i, err)
		}
		// Append a private suffix past the shared prefix; only the touched
		// block diverges via copy-on-write.
		if _, err := cow.Write(child, uint64(prefixBytes), suffix); err != nil {
			t.Fatalf("cow child write %d: %v", i, err)
		}
	}
	_, cowTotal, cowFree, _ := cow.Stats()
	cowBlocks := int(cowTotal - cowFree)

	//Independent copies: each client writes the full prefix itself.
	indep := allocator.New(blockSize, blocksNeeded)
	full := make([]byte, prefixBytes+suffixBytes)
	for i := 0; i < numClients; i++ {
		id := indep.Allocate()
		if _, err := indep.Write(id, 0, full); err != nil {
			t.Fatalf("indep write %d: %v", i, err)
		}
	}
	_, indepTotal, indepFree, _ := indep.Stats()
	indepBlocks := int(indepTotal - indepFree)

	saved := float64(indepBlocks-cowBlocks) / float64(indepBlocks) * 100

	t.Logf("clients:                    %d", numClients)
	t.Logf("shared prefix:              %d bytes", prefixBytes)
	t.Logf("private suffix per client:  %d bytes", suffixBytes)
	t.Logf("block size:                 %d bytes", blockSize)
	t.Logf("residency, independent:     %d blocks (%d bytes)", indepBlocks, indepBlocks*blockSize)
	t.Logf("residency, copy-on-write:   %d blocks (%d bytes)", cowBlocks, cowBlocks*blockSize)
	t.Logf("residency saved by sharing: %.1f%%", saved)
}
