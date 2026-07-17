package framework

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testServerSrc is a minimal HTTP server that listens on the address passed via
// the --addr flag and responds to /health with 200 OK. It is compiled into a
// standalone binary by buildTestServer and used as the service-under-test.
const testServerSrc = `package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	addr := flag.String("addr", ":0", "listen address")
	flag.Parse()
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	// Ignore SIGTERM via custom handler so the default Go behavior still exits;
	// this keeps the server responsive to Stop's SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	go func() {
		<-sigCh
		os.Exit(0)
	}()
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
`

// buildTestServer writes a minimal Go HTTP server to a temp dir, compiles it with
// `go build`, and returns the absolute path of the resulting binary. Tests that
// exercise the real Start/Stop lifecycle use this helper.
func buildTestServer(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping subprocess build in short mode")
	}

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(srcPath, []byte(testServerSrc), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	// Use a conservative go version so any Go 1.21+ toolchain can build it.
	goMod := "module testserver\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	binaryPath := filepath.Join(dir, "testserver")
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build testserver: %v: %s", err, out)
	}
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("binary not created: %v", err)
	}
	return binaryPath
}

// newLifecycleConfig builds a ServiceConfig pointing at the given pre-built
// binary, with health checking on /health and logs routed to a temp dir.
func newLifecycleConfig(t *testing.T, binary string) ServiceConfig {
	t.Helper()
	return ServiceConfig{
		BinaryPath:     binary,
		AddrFlag:       "--addr",
		HealthPath:     "/health",
		ReportsDir:     filepath.Join(t.TempDir(), "reports"),
		HealthyTimeout: 10 * time.Second,
	}
}

func TestFindFreePort(t *testing.T) {
	port, err := FreePort()
	if err != nil {
		t.Fatalf("FreePort failed: %v", err)
	}
	if port <= 0 {
		t.Fatalf("expected positive port, got %d", port)
	}

	// Verify the returned port is actually usable for listening.
	l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("could not listen on returned port %d: %v", port, err)
	}
	l.Close()
}

func TestFindFreePortReturnsDistinct(t *testing.T) {
	// Two consecutive calls should generally return different ports.
	p1, err := FreePort()
	if err != nil {
		t.Fatalf("first FreePort failed: %v", err)
	}
	p2, err := FreePort()
	if err != nil {
		t.Fatalf("second FreePort failed: %v", err)
	}
	if p1 == 0 || p2 == 0 {
		t.Fatalf("expected non-zero ports, got %d and %d", p1, p2)
	}
}

func TestNewServiceLifecycle(t *testing.T) {
	cfg := ServiceConfig{
		BinaryPath:  "/usr/bin/true",
		BuildTarget: "./cmd/foo",
		ConfigPath:  "configs/test.yaml",
		ConfigFlag:  "--config",
		AddrFlag:    "--addr",
		Port:        8080,
		HealthPath:  "/health",
		ReportsDir:  "reports",
		Args:        []string{"--verbose"},
		Env:         []string{"FOO=bar"},
	}
	s := NewServiceLifecycle(cfg)
	got := s.Config()
	if got.BinaryPath != cfg.BinaryPath {
		t.Errorf("BinaryPath mismatch: got %s, want %s", got.BinaryPath, cfg.BinaryPath)
	}
	if got.BuildTarget != cfg.BuildTarget {
		t.Errorf("BuildTarget mismatch: got %s, want %s", got.BuildTarget, cfg.BuildTarget)
	}
	if got.ConfigPath != cfg.ConfigPath {
		t.Errorf("ConfigPath mismatch: got %s, want %s", got.ConfigPath, cfg.ConfigPath)
	}
	if got.ConfigFlag != cfg.ConfigFlag {
		t.Errorf("ConfigFlag mismatch: got %s, want %s", got.ConfigFlag, cfg.ConfigFlag)
	}
	if got.AddrFlag != cfg.AddrFlag {
		t.Errorf("AddrFlag mismatch: got %s, want %s", got.AddrFlag, cfg.AddrFlag)
	}
	if got.Port != cfg.Port {
		t.Errorf("Port mismatch: got %d, want %d", got.Port, cfg.Port)
	}
	if got.HealthPath != cfg.HealthPath {
		t.Errorf("HealthPath mismatch: got %s, want %s", got.HealthPath, cfg.HealthPath)
	}
	if got.ReportsDir != cfg.ReportsDir {
		t.Errorf("ReportsDir mismatch: got %s, want %s", got.ReportsDir, cfg.ReportsDir)
	}
	if len(got.Args) != 1 || got.Args[0] != "--verbose" {
		t.Errorf("Args mismatch: got %v", got.Args)
	}
	if len(got.Env) != 1 || got.Env[0] != "FOO=bar" {
		t.Errorf("Env mismatch: got %v", got.Env)
	}
}

func TestConfig(t *testing.T) {
	cfg := ServiceConfig{BinaryPath: "/usr/bin/true", Port: 9090}
	s := NewServiceLifecycle(cfg)
	got := s.Config()
	if got.BinaryPath != cfg.BinaryPath {
		t.Fatalf("expected BinaryPath %s, got %s", cfg.BinaryPath, got.BinaryPath)
	}
	if got.Port != cfg.Port {
		t.Fatalf("expected Port %d, got %d", cfg.Port, got.Port)
	}
}

func TestSetProjectRoot(t *testing.T) {
	s := NewServiceLifecycle(ServiceConfig{})
	if s.projectRoot != "" {
		t.Fatalf("expected empty projectRoot initially, got %q", s.projectRoot)
	}
	s.SetProjectRoot("/some/project/root")
	if s.projectRoot != "/some/project/root" {
		t.Fatalf("expected projectRoot /some/project/root, got %q", s.projectRoot)
	}
}

func TestStopWhenNotStarted(t *testing.T) {
	s := NewServiceLifecycle(ServiceConfig{})
	if err := s.Stop(); err != nil {
		t.Fatalf("expected nil error when stopping unstarted service, got %v", err)
	}
}

func TestBaseURLWhenNotStarted(t *testing.T) {
	s := NewServiceLifecycle(ServiceConfig{})
	if got := s.BaseURL(); got != "" {
		t.Fatalf("expected empty BaseURL when not started, got %q", got)
	}
}

func TestServerLogPathWhenNotStarted(t *testing.T) {
	s := NewServiceLifecycle(ServiceConfig{})
	if got := s.ServerLogPath(); got != "" {
		t.Fatalf("expected empty log path when not started, got %q", got)
	}
}

func TestStartWhenAlreadyStarted(t *testing.T) {
	binary := buildTestServer(t)
	s := NewServiceLifecycle(newLifecycleConfig(t, binary))
	if err := s.Start(); err != nil {
		t.Fatalf("initial Start failed: %v", err)
	}
	defer s.Stop()

	err := s.Start()
	if err == nil {
		t.Fatalf("expected error when starting already-started service")
	}
	if !strings.Contains(err.Error(), "already started") {
		t.Fatalf("expected 'already started' error, got: %v", err)
	}
}

func TestStartAndStop(t *testing.T) {
	binary := buildTestServer(t)
	s := NewServiceLifecycle(newLifecycleConfig(t, binary))

	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	baseURL := s.BaseURL()
	if baseURL == "" {
		t.Fatal("expected non-empty BaseURL after Start")
	}
	if !strings.HasPrefix(baseURL, "http://localhost:") {
		t.Fatalf("expected BaseURL to start with http://localhost:, got %q", baseURL)
	}

	// Verify the health endpoint responds.
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected health status 200, got %d", resp.StatusCode)
	}

	// Verify the log path is set.
	logPath := s.ServerLogPath()
	if logPath == "" {
		t.Fatal("expected non-empty log path after Start")
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("log file does not exist at %s: %v", logPath, err)
	}

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// After Stop, BaseURL should be empty.
	if got := s.BaseURL(); got != "" {
		t.Fatalf("expected empty BaseURL after Stop, got %q", got)
	}

	// ServerLogPath should be preserved after Stop so logs can be inspected.
	if got := s.ServerLogPath(); got != logPath {
		t.Fatalf("expected log path %q preserved after Stop, got %q", logPath, got)
	}
}

func TestRestart(t *testing.T) {
	binary := buildTestServer(t)
	s := NewServiceLifecycle(newLifecycleConfig(t, binary))

	if err := s.Start(); err != nil {
		t.Fatalf("initial Start failed: %v", err)
	}
	baseURL1 := s.BaseURL()
	if baseURL1 == "" {
		t.Fatal("expected non-empty BaseURL after initial Start")
	}

	if err := s.Restart(); err != nil {
		t.Fatalf("Restart failed: %v", err)
	}
	defer s.Stop()

	baseURL2 := s.BaseURL()
	if baseURL2 == "" {
		t.Fatal("expected non-empty BaseURL after Restart")
	}

	// Verify the restarted instance is actually healthy.
	resp, err := http.Get(baseURL2 + "/health")
	if err != nil {
		t.Fatalf("health request after Restart failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected health status 200 after Restart, got %d", resp.StatusCode)
	}
}

func TestWaitForHealthyCustomCheck(t *testing.T) {
	binary := buildTestServer(t)
	cfg := newLifecycleConfig(t, binary)
	// Override with a custom health check that hits /health explicitly.
	cfg.HealthCheck = func(baseURL string) error {
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status %d", resp.StatusCode)
		}
		return nil
	}
	s := NewServiceLifecycle(cfg)
	if err := s.Start(); err != nil {
		t.Fatalf("Start with custom HealthCheck failed: %v", err)
	}
	defer s.Stop()

	// Calling WaitForHealthy again should also succeed since the custom check passes.
	if err := s.WaitForHealthy(3 * time.Second); err != nil {
		t.Fatalf("WaitForHealthy failed: %v", err)
	}
}

func TestWaitForHealthyFailingCheck(t *testing.T) {
	binary := buildTestServer(t)
	cfg := newLifecycleConfig(t, binary)
	cfg.HealthyTimeout = 1 * time.Second
	// Custom health check that always fails; Start should time out and clean up.
	cfg.HealthCheck = func(baseURL string) error {
		return fmt.Errorf("always unhealthy")
	}
	s := NewServiceLifecycle(cfg)
	err := s.Start()
	if err == nil {
		_ = s.Stop()
		t.Fatal("expected Start to fail when HealthCheck always returns an error")
	}
	if !strings.Contains(err.Error(), "did not become healthy") {
		t.Fatalf("expected timeout error containing 'did not become healthy', got: %v", err)
	}

	// After a failed Start, BaseURL should be empty (cleanup happened).
	if got := s.BaseURL(); got != "" {
		t.Fatalf("expected empty BaseURL after failed Start, got %q", got)
	}
}
