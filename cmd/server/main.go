// Command server runs the paged cache as a gRPC service.
package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"github.com/Junny20/paged-cache/gen/cachepb"
	"github.com/Junny20/paged-cache/internal/cache"
	"github.com/Junny20/paged-cache/server"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	blockSize := flag.Uint("block-size", 4096, "arena block size in bytes")
	totalBlocks := flag.Uint("blocks", 16384, "number of blocks in the arena")
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen %s: %v", *addr, err)
	}

	c := cache.New(uint32(*blockSize), uint32(*totalBlocks))
	grpcServer := grpc.NewServer()
	cachepb.RegisterCacheServer(grpcServer, server.New(c))

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("shutting down")
		grpcServer.GracefulStop()
	}()

	log.Printf("serving on %s (block=%d bytes, blocks=%d)", *addr, *blockSize, *totalBlocks)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
