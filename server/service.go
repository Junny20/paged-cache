// Package server adapts the cache to the generated gRPC surface. It translates
// protobuf requests into cache calls and maps cache errors to gRPC status codes.
package server

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Junny20/paged-cache/gen/cachepb"
	"github.com/Junny20/paged-cache/internal/allocator"
	"github.com/Junny20/paged-cache/internal/cache"
)

// Service implements cachepb.CacheServer over a Cache.
type Service struct {
	cachepb.UnimplementedCacheServer
	cache *cache.Cache
}

// New wraps a cache as a gRPC service.
func New(c *cache.Cache) *Service {
	return &Service{cache: c}
}

// Allocate creates a new sequence, cloning a parent when parent_seq_id is set.
func (s *Service) Allocate(_ context.Context, req *cachepb.AllocateRequest) (*cachepb.AllocateResponse, error) {
	if req.GetParentSeqId() != 0 {
		id, err := s.cache.Clone(allocator.SeqID(req.GetParentSeqId()))
		if err != nil {
			return nil, toStatus(err)
		}
		return &cachepb.AllocateResponse{SeqId: uint64(id)}, nil
	}
	id := s.cache.Allocate()
	return &cachepb.AllocateResponse{SeqId: uint64(id)}, nil
}

// Write copies request data into the sequence at the given offset.
func (s *Service) Write(_ context.Context, req *cachepb.WriteRequest) (*cachepb.WriteResponse, error) {
	n, err := s.cache.Write(allocator.SeqID(req.GetSeqId()), req.GetOffset(), req.GetData())
	if err != nil {
		return nil, toStatus(err)
	}
	return &cachepb.WriteResponse{BytesWritten: uint64(n)}, nil
}

// Read returns bytes from the sequence at the given offset and length.
func (s *Service) Read(_ context.Context, req *cachepb.ReadRequest) (*cachepb.ReadResponse, error) {
	data, err := s.cache.Read(allocator.SeqID(req.GetSeqId()), req.GetOffset(), req.GetLength())
	if err != nil {
		return nil, toStatus(err)
	}
	return &cachepb.ReadResponse{Data: data}, nil
}

// Free releases the sequence.
func (s *Service) Free(_ context.Context, req *cachepb.FreeRequest) (*cachepb.FreeResponse, error) {
	if err := s.cache.Free(allocator.SeqID(req.GetSeqId())); err != nil {
		return nil, toStatus(err)
	}
	return &cachepb.FreeResponse{}, nil
}

// Stats reports arena occupancy.
func (s *Service) Stats(_ context.Context, _ *cachepb.StatsRequest) (*cachepb.StatsResponse, error) {
	bs, total, free, live := s.cache.Stats()
	return &cachepb.StatsResponse{
		BlockSize:     bs,
		TotalBlocks:   total,
		FreeBlocks:    free,
		LiveSequences: live,
	}, nil
}

// toStatus maps cache errors to gRPC status codes so clients can branch on them.
func toStatus(err error) error {
	switch {
	case errors.Is(err, allocator.ErrNoSequence):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, allocator.ErrRange):
		return status.Error(codes.OutOfRange, err.Error())
	case errors.Is(err, allocator.ErrOutOfMemory):
		return status.Error(codes.ResourceExhausted, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
