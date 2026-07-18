// Command go-buggy-backend is a deliberately broken Go service for taichi
// integration testing. It exposes both an HTTP API and a gRPC service with
// multiple intentional defects for taichi skills to detect:
//
// HTTP bugs:
//   - /api/v1/health returns data.status="unhealthy" (expected "healthy")
//   - /api/v1/users returns HTTP 500 (expected 200)
//   - / returns HTML missing the expected <div id="app"> marker
//   - /favicon.ico is not served (static asset failure)
//
// gRPC bugs (see proto/product.proto):
//   - grpc.health.v1 Health/Check returns NOT_SERVING (expected SERVING)
//   - product.InventoryService is declared in proto but NOT implemented
//     (reflection should report it missing)
//   - product.ProductService.GetProduct returns a wrong price for id=1
//     (expected 99.99, returns 9.99)
//
// Build: go build -o bin/go-buggy-backend .
// Run:   ./bin/go-buggy-backend --addr :18080
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	productpb "github.com/tickraft/taichi/examples/go-buggy-backend/proto/gen/product"
)

const (
	httpAddrFlag = "addr"
	grpcAddr     = "127.0.0.1:50051"
)

func main() {
	addr := flag.String(httpAddrFlag, ":18080", "HTTP listen address")
	flag.Parse()

	go startGRPC()
	startHTTP(*addr)
}

// startGRPC starts a gRPC server on grpcAddr with:
//   - health check set to NOT_SERVING (intentional bug)
//   - ProductService implemented (with a price bug in GetProduct)
//   - InventoryService declared in proto but NOT registered (intentional bug)
//   - server reflection enabled (so the reflect skill can list services)
func startGRPC() {
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Printf("grpc listen error: %v", err)
		return
	}
	s := grpc.NewServer()

	// Register ProductService (implemented, but with a price bug).
	productpb.RegisterProductServiceServer(s, &productServer{})

	// BUG: InventoryService is declared in proto/product.proto but NOT registered.
	// The gRPC reflect skill should detect this when it expects
	// "product.InventoryService" to be present.
	// productpb.RegisterInventoryServiceServer(s, &inventoryServer{})

	// BUG: health status is NOT_SERVING instead of SERVING.
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	healthpb.RegisterHealthServer(s, hs)

	// Enable reflection so the reflect skill can list exposed services.
	reflection.Register(s)

	log.Printf("gRPC server (buggy) listening on %s", grpcAddr)
	if err := s.Serve(lis); err != nil {
		log.Printf("grpc serve error: %v", err)
	}
}

// productServer implements productpb.ProductServiceServer.
// Intentional bug: GetProduct returns a wrong price for id=1.
type productServer struct {
	productpb.UnimplementedProductServiceServer
}

func (s *productServer) GetProduct(_ context.Context, req *productpb.GetProductRequest) (*productpb.Product, error) {
	switch req.GetId() {
	case 1:
		// BUG: price is 9.99 instead of 99.99.
		return &productpb.Product{
			Id:       1,
			Name:     "Widget",
			Price:    9.99, // BUG: should be 99.99
			Category: "tools",
		}, nil
	case 2:
		return &productpb.Product{
			Id:       2,
			Name:     "Gadget",
			Price:    49.99,
			Category: "electronics",
		}, nil
	default:
		return nil, fmt.Errorf("product %d not found", req.GetId())
	}
}

func (s *productServer) ListProducts(_ context.Context, req *productpb.ListProductsRequest) (*productpb.ListProductsResponse, error) {
	items := []*productpb.Product{
		{Id: 1, Name: "Widget", Price: 9.99, Category: "tools"},
		{Id: 2, Name: "Gadget", Price: 49.99, Category: "electronics"},
	}
	limit := int(req.GetLimit())
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return &productpb.ListProductsResponse{
		Items: items,
		Total: int32(len(items)),
	}, nil
}

func startHTTP(addr string) {
	mux := http.NewServeMux()

	// BUG: /api/v1/health returns "unhealthy" instead of "healthy".
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"code":       0,
			"msg":        "ok",
			"request_id": "buggy-health-001",
			"data": map[string]any{
				"status": "unhealthy", // BUG: should be "healthy"
			},
		})
	})

	// BUG: /api/v1/users returns 500 instead of 200.
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":       500,
			"msg":        "database connection failed",
			"request_id": "buggy-users-002",
		})
	})

	// Correct endpoint for contrast: returns a valid product list.
	mux.HandleFunc("/api/v1/products", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"code":       0,
			"msg":        "ok",
			"request_id": "buggy-products-003",
			"data": map[string]any{
				"items": []map[string]any{
					{"id": 1, "name": "Widget", "price": 99.99},
					{"id": 2, "name": "Gadget", "price": 49.99},
				},
			},
		})
	})

	// BUG: homepage HTML is missing the expected <div id="app"> marker.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!DOCTYPE html><html><head><title>Buggy Backend</title></head><body><h1>Hello</h1></body></html>")
	})

	// NOTE: /favicon.ico is intentionally not served (static asset failure).

	log.Printf("HTTP server (buggy) listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("http serve error: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
