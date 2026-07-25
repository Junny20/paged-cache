// Command loadgen drives the cache service with concurrent clients and reports
// throughput and tail latency. Each worker repeatedly allocates a sequence,
// writes a payload, reads it back, and frees it, timing the full round trip.
package main

import (
	"context"
	"flag"
	"log"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Junny20/paged-cache/gen/cachepb"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "server address")
	concurrency := flag.Int("concurrency", 32, "number of concurrent workers")
	duration := flag.Duration("duration", 10*time.Second, "test duration")
	payload := flag.Int("payload", 8192, "bytes written per operation")
	flag.Parse()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer conn.Close()
	client := cachepb.NewCacheClient(conn)

	var (
		wg        sync.WaitGroup
		ops       atomic.Int64
		errs      atomic.Int64
		latMu     sync.Mutex
		latencies []time.Duration
	)

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	start := time.Now()
	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed))
			data := make([]byte, *payload)
			local := make([]time.Duration, 0, 1024)
			for ctx.Err() == nil {
				rng.Read(data)
				t0 := time.Now()
				if err := roundTrip(ctx, client, data); err != nil {
					if ctx.Err() == nil {
						errs.Add(1)
					}
					continue
				}
				local = append(local, time.Since(t0))
				ops.Add(1)
			}
			latMu.Lock()
			latencies = append(latencies, local...)
			latMu.Unlock()
		}(int64(w) + 1)
	}
	wg.Wait()
	elapsed := time.Since(start)

	report(elapsed, ops.Load(), errs.Load(), latencies)
}

// roundTrip performs one allocate/write/read/free cycle.
func roundTrip(ctx context.Context, client cachepb.CacheClient, data []byte) error {
	alloc, err := client.Allocate(ctx, &cachepb.AllocateRequest{})
	if err != nil {
		return err
	}
	id := alloc.GetSeqId()
	if _, err := client.Write(ctx, &cachepb.WriteRequest{SeqId: id, Offset: 0, Data: data}); err != nil {
		return err
	}
	if _, err := client.Read(ctx, &cachepb.ReadRequest{SeqId: id, Offset: 0, Length: uint64(len(data))}); err != nil {
		return err
	}
	_, err = client.Free(ctx, &cachepb.FreeRequest{SeqId: id})
	return err
}

// report prints throughput and latency percentiles.
func report(elapsed time.Duration, ops, errs int64, latencies []time.Duration) {
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	throughput := float64(ops) / elapsed.Seconds()

	log.Printf("duration:   %s", elapsed.Round(time.Millisecond))
	log.Printf("operations: %d (%d errors)", ops, errs)
	log.Printf("throughput: %.0f ops/s", throughput)
	log.Printf("latency p50: %s", percentile(latencies, 0.50))
	log.Printf("latency p99: %s", percentile(latencies, 0.99))
	log.Printf("latency max: %s", percentile(latencies, 1.0))
}

// percentile returns the p-quantile latency; latencies must be sorted ascending.
func percentile(latencies []time.Duration, p float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	idx := int(p * float64(len(latencies)-1))
	return latencies[idx].Round(time.Microsecond)
}
