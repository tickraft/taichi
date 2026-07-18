# go-buggy-backend Makefile

PROTO_DIR := proto
GEN_DIR := proto/gen/product
BIN_DIR := bin

.PHONY: all proto build clean

all: proto build

# Regenerate gRPC stub code from proto/product.proto.
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc
proto:
	@mkdir -p $(GEN_DIR)
	protoc \
		--go_out=$(GEN_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(GEN_DIR) --go-grpc_opt=paths=source_relative \
		--proto_path=$(PROTO_DIR) \
		$(PROTO_DIR)/product.proto
	@# protoc generates files in GEN_DIR root; move if needed
	@echo "proto stubs generated in $(GEN_DIR)"

# Build both the server and the gRPC plugin checker.
build: proto
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/go-buggy-backend .
	go build -o $(BIN_DIR)/grpc-check ./cmd/grpc-check
	@echo "binaries built in $(BIN_DIR)"

clean:
	rm -rf $(BIN_DIR)
