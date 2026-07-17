package grpc

import (
	"net"
	"os"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// testServerAddr is the address of the embedded gRPC test server started by TestMain.
// It is set once before all tests and remains valid until the test binary exits.
var testServerAddr string

// TestMain starts a gRPC server with health and reflection services enabled,
// then runs the test suite. The server listens on a random localhost port to
// avoid port conflicts.
//
// Registered services:
//   - grpc.health.v1.Health (status set to SERVING for the empty service name)
//   - grpc.reflection.v1.ServerReflection (auto-registered via reflection.Register)
//
// This server exercises all three gRPC case types (health / dial / reflect)
// without requiring compiled protobuf stubs for a custom service.
func TestMain(m *testing.M) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		// A random port should never fail in practice; surface it clearly if it does.
		panic("testserver: listen: " + err.Error())
	}
	testServerAddr = lis.Addr().String()

	s := grpc.NewServer()

	// Register the standard health service and mark the overall service as SERVING.
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(s, hs)

	// Register server reflection so reflect cases can discover exposed services.
	reflection.Register(s)

	go func() {
		_ = s.Serve(lis)
	}()

	code := m.Run()

	s.GracefulStop()
	os.Exit(code)
}
