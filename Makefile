.PHONY: all proto build test race vet fmt server loadgen clean tools

MODULE      := github.com/Junny20/paged-cache
PROTO       := api/proto/cache.proto
GEN_DIR     := gen
BIN_DIR     := bin

all: build

## proto: regenerate Go code from the .proto definitions
proto:
	protoc \
		--go_out=. --go_opt=module=$(MODULE) \
		--go-grpc_out=. --go-grpc_opt=module=$(MODULE) \
		$(PROTO)

## build: compile the server and load generator
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/server  ./cmd/server
	go build -o $(BIN_DIR)/loadgen ./bench

## test: run unit tests
test:
	go test ./... -count=1

## race: run unit tests under the race detector
race:
	go test ./... -count=1 -race

## vet: run go vet
vet:
	go vet ./...

## fmt: format all Go source
fmt:
	gofmt -w cmd internal server bench

## server: build and run the server with defaults
server: build
	./$(BIN_DIR)/server

## loadgen: build and run the load generator against a local server
loadgen: build
	./$(BIN_DIR)/loadgen

## tools: install the protoc Go plugins
tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR)
