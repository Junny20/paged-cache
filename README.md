# paged-cache

A paged memory allocator and KV-cache service in Go, exposed over gRPC.

## Core mechanisms

- **Paged block allocator.** The pool is a flat arena of fixed-size blocks
  indexed by a compact `u32` handle. A free list tracks unused indices; alloc
  pops, free pushes. Fixed-size blocks give O(1) alloc/free and no external
  fragmentation; the cost is internal fragmentation in each client's last
  partially filled block, which the block size trades against.
- **Logical-to-physical mapping.** Each sequence owns a block table — an ordered
  slice of physical block indices. Logical position `P` resolves to
  `table[P / block_size]` at offset `P % block_size`. This page-table analogy is
  what lets a sequence grow without moving prior data.
- **Copy-on-write prefix sharing.** Cloned sequences share the parent's physical
  blocks, tracked by a per-block reference count. A write to a shared block
  copies it first, so the writer diverges without disturbing other readers; a
  refcount reaching zero returns the block to the free list.
- **LRU eviction.** Under memory pressure the allocator reclaims the
  least-recently-touched reclaimable (shared) block first, keeping hot prefixes
  resident.
- **gRPC service.** `Allocate`, `Write`, `Read`, `Free`, and `Stats` RPCs over
  Protocol Buffers, serving many in-flight clients concurrently. A separate load
  generator drives it to measure throughput and tail latency.

## Layout

```
cmd/server/       flag parsing, wiring, serve
internal/
  allocator/      paged block allocator: arena, free list, block tables, COW read/write
  cow/            reference counting and LRU eviction bookkeeping
  cache/          sequence-oriented facade over the allocator
api/proto/        service and message definitions
gen/cachepb/      generated protobuf/gRPC code (checked in)
server/           gRPC service implementation
bench/            concurrent load generator (throughput + p99)
```

## Build and run

Requires Go 1.22+. On first build with network access, run `go mod tidy` to
populate `go.sum`.

```sh
make build                       # compile server and loadgen into ./bin
./bin/server -addr :8080 -block-size 4096 -blocks 65536
./bin/loadgen -addr localhost:8080 -concurrency 32 -duration 10s -payload 8192
```

Regenerate the gRPC code after editing the proto:

```sh
make tools     # install protoc-gen-go and protoc-gen-go-grpc
make proto
```

## Testing

```sh
make test      # unit tests
make race      # unit tests under the race detector
```

Coverage includes the arena and free list, the multi-block read/write path,
out-of-range and unknown-sequence errors, out-of-memory behaviour, LRU order,
copy-on-write divergence (a child's write leaves the parent intact), and block
reclamation on free.
