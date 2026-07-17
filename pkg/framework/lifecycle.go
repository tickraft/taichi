package framework

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Default values. Used when the corresponding ServiceConfig field is zero.
const (
	// defaultHealthPollInterval is the polling interval for health checks.
	defaultHealthPollInterval = 500 * time.Millisecond
	// defaultHealthRequestTimeout is the timeout for a single health-check request.
	defaultHealthRequestTimeout = 2 * time.Second
	// defaultHealthyTimeout is the total timeout to wait for the service to become ready after Start.
	defaultHealthyTimeout = 30 * time.Second
	// defaultStopGracePeriod is the grace period after SIGTERM to wait for process exit before sending SIGKILL.
	defaultStopGracePeriod = 10 * time.Second
	// defaultReportsDir is the default service log output directory.
	defaultReportsDir = "reports"
)

// ServiceConfig describes how the service under test is started, its health-check path, and log output location.
//
// All project-specific parameters are extracted into fields so ServiceLifecycle can serve any Go binary.
type ServiceConfig struct {
	// BinaryPath is the build artifact path, relative to the project root. For example "bin/tickraft".
	// If it does not exist, Start will invoke go build on BuildTarget.
	BinaryPath string
	// BuildTarget is the go build target, for example "./cmd/tickraft".
	// Used only when BinaryPath does not exist.
	BuildTarget string
	// ConfigPath is the config file path (relative to the project root). An empty string means no config argument is passed.
	ConfigPath string
	// ConfigFlag is the config argument name, for example "--config". An empty string means no config argument is sent.
	ConfigFlag string
	// AddrFlag is the listen-address argument name, for example "--addr" or "--listen".
	// An empty string means the address is not passed via the command line (the service may read it from the config file).
	AddrFlag string
	// Port is the fixed listen port. 0 means automatically find a free port.
	Port int
	// HealthPath is the HTTP health-check path, for example "/api/v1/health" or "/healthz".
	// An empty string means no HTTP health check is performed (the caller must customize via HealthCheck).
	HealthPath string
	// HealthExpectedStatus is the expected HTTP status code for the health check. 0 means default 200.
	HealthExpectedStatus int
	// HealthyTimeout is the total timeout to wait for the service to become ready. 0 means use the default.
	HealthyTimeout time.Duration
	// ReportsDir is the service log output directory (relative to the project root). An empty string means use "reports".
	ReportsDir string
	// Args are extra command-line arguments, appended after BinaryPath.
	Args []string
	// Env are extra environment variables, in "KEY=VALUE" format.
	Env []string
	// HealthCheck allows the caller to customize the health-check logic. When non-nil it overrides the default HTTP check based on HealthPath.
	// The baseURL argument looks like "http://localhost:12345".
	HealthCheck func(baseURL string) error
}

// ServiceLifecycle manages the lifecycle of the service-under-test process: building the binary on demand,
// starting it on a free port, waiting for health, and stopping/restarting on demand.
//
// All methods are concurrency-safe, but the lifecycle should be driven by a single coordinating goroutine.
// Tests are expected to run from the project root so that relative paths resolve correctly.
type ServiceLifecycle struct {
	cfg         ServiceConfig
	addr        string
	baseURL     string
	logPath     string
	cmd         *exec.Cmd
	logFile     *os.File
	mu          sync.Mutex
	projectRoot string
}

// NewServiceLifecycle creates a ServiceLifecycle from cfg.
// When projectRoot is empty, the caller should set it via SetProjectRoot before Start,
// or ensure the process working directory is the project root.
func NewServiceLifecycle(cfg ServiceConfig) *ServiceLifecycle {
	return &ServiceLifecycle{cfg: cfg}
}

// Config returns a copy of the ServiceConfig passed at creation.
func (s *ServiceLifecycle) Config() ServiceConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// SetProjectRoot explicitly sets the project root directory, used to resolve relative BinaryPath / ConfigPath / ReportsDir.
// After calling this method, ServiceLifecycle no longer depends on the process working directory.
func (s *ServiceLifecycle) SetProjectRoot(root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projectRoot = root
}

// Start builds the binary if it does not exist, finds a free port, starts the service process, and waits for it to become healthy.
// Returns an error if the service is already started.
func (s *ServiceLifecycle) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil {
		return fmt.Errorf("service already started")
	}

	if err := s.ensureBinary(); err != nil {
		return fmt.Errorf("ensure binary: %w", err)
	}

	port := s.cfg.Port
	if port == 0 {
		p, err := FreePort()
		if err != nil {
			return fmt.Errorf("find free port: %w", err)
		}
		port = p
	}
	s.addr = ":" + strconv.Itoa(port)
	s.baseURL = "http://localhost:" + strconv.Itoa(port)

	if err := s.openLogFile(); err != nil {
		s.addr = ""
		s.baseURL = ""
		return fmt.Errorf("open log file: %w", err)
	}

	args := s.buildArgs(port)
	cmd := exec.Command(s.absBinaryPath(), args...)
	cmd.Stdout = s.logFile
	cmd.Stderr = s.logFile
	if len(s.cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), s.cfg.Env...)
	}
	if s.projectRoot != "" {
		cmd.Dir = s.projectRoot
	}
	if err := cmd.Start(); err != nil {
		// best-effort: release log file on start-failure cleanup; the start error is the primary result.
		_ = s.logFile.Close()
		s.logFile = nil
		s.logPath = ""
		s.addr = ""
		s.baseURL = ""
		return fmt.Errorf("start process: %w", err)
	}
	s.cmd = cmd

	timeout := s.cfg.HealthyTimeout
	if timeout <= 0 {
		timeout = defaultHealthyTimeout
	}
	if err := s.waitForHealthyLocked(timeout); err != nil {
		// best-effort: tear down the started process on health-check failure; the health error is propagated.
		_ = s.stopLocked()
		return fmt.Errorf("wait for healthy: %w", err)
	}
	return nil
}

// buildArgs assembles the startup command-line arguments.
func (s *ServiceLifecycle) buildArgs(port int) []string {
	var args []string
	if s.cfg.ConfigFlag != "" && s.cfg.ConfigPath != "" {
		args = append(args, s.cfg.ConfigFlag, s.cfg.ConfigPath)
	}
	if s.cfg.AddrFlag != "" {
		args = append(args, s.cfg.AddrFlag, s.addr)
	}
	args = append(args, s.cfg.Args...)
	return args
}

// WaitForHealthy polls the health endpoint periodically until it returns the expected status or times out.
func (s *ServiceLifecycle) WaitForHealthy(timeout time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.waitForHealthyLocked(timeout)
}

func (s *ServiceLifecycle) waitForHealthyLocked(timeout time.Duration) error {
	if s.baseURL == "" {
		return fmt.Errorf("base URL is empty; service not started")
	}
	// Custom health check takes precedence.
	if s.cfg.HealthCheck != nil {
		deadline := time.Now().Add(timeout)
		for {
			err := s.cfg.HealthCheck(s.baseURL)
			if err == nil {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("service did not become healthy within %s: %w", timeout, err)
			}
			time.Sleep(defaultHealthPollInterval)
		}
	}
	// Default to HTTP health check.
	if s.cfg.HealthPath == "" {
		// No health check configured: the process has started, treat it as ready.
		return nil
	}
	expected := s.cfg.HealthExpectedStatus
	if expected == 0 {
		expected = http.StatusOK
	}
	client := &http.Client{Timeout: defaultHealthRequestTimeout}
	url := s.baseURL + s.cfg.HealthPath
	deadline := time.Now().Add(timeout)
	for {
		resp, err := client.Get(url)
		if err == nil {
			// best-effort: drain and close the response body between poll iterations; read errors do not affect the poll loop.
			_ = resp.Body.Close()
			if resp.StatusCode == expected {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service did not become healthy within %s", timeout)
		}
		time.Sleep(defaultHealthPollInterval)
	}
}

// Stop terminates the running service process. It first sends SIGTERM, and if the process is still alive after the grace period, sends SIGKILL.
// It closes the log file and clears runtime state. Returns nil when not running.
func (s *ServiceLifecycle) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLocked()
}

func (s *ServiceLifecycle) stopLocked() error {
	if s.cmd == nil {
		return nil
	}
	cmd := s.cmd
	process := cmd.Process

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	if process != nil {
		// best-effort: signal the process to shut down gracefully; escalation to SIGKILL follows if it ignores SIGTERM.
		_ = process.Signal(syscall.SIGTERM)
	}

	select {
	case <-done:
		// Process exited cleanly after SIGTERM.
	case <-time.After(defaultStopGracePeriod):
		if process != nil {
			// best-effort: force-kill the process after the grace period expires; the result is observed via cmd.Wait below.
			_ = process.Kill()
		}
		<-done
	}

	if s.logFile != nil {
		// best-effort: release the log file handle during shutdown; nothing more can be written after this point.
		_ = s.logFile.Close()
		s.logFile = nil
	}
	s.cmd = nil
	s.addr = ""
	s.baseURL = ""
	// Preserve logPath so the log can be inspected after Stop.
	return nil
}

// Restart stops (if running) and starts again.
func (s *ServiceLifecycle) Restart() error {
	if err := s.Stop(); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	return nil
}

// BaseURL returns the base URL of the running service (e.g. "http://localhost:12345"). Returns an empty string when not running.
func (s *ServiceLifecycle) BaseURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baseURL
}

// ServerLogPath returns the path of the current (or most recent) service log file. Returns an empty string if Start has never been called.
func (s *ServiceLifecycle) ServerLogPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logPath
}

// absBinaryPath returns the absolute binary path after resolving the project root (if set).
func (s *ServiceLifecycle) absBinaryPath() string {
	if s.projectRoot == "" {
		return s.cfg.BinaryPath
	}
	if filepath.IsAbs(s.cfg.BinaryPath) {
		return s.cfg.BinaryPath
	}
	return filepath.Join(s.projectRoot, s.cfg.BinaryPath)
}

// ensureBinary builds the binary when it does not exist.
func (s *ServiceLifecycle) ensureBinary() error {
	bp := s.absBinaryPath()
	if bp == "" {
		return fmt.Errorf("binary path is empty")
	}
	if _, err := os.Stat(bp); err == nil {
		return nil
	}
	if s.cfg.BuildTarget == "" {
		return fmt.Errorf("binary %s not found and BuildTarget is empty", bp)
	}
	cmd := exec.Command("go", "build", "-o", bp, s.cfg.BuildTarget)
	if s.projectRoot != "" {
		cmd.Dir = s.projectRoot
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build: %w: %s", err, output)
	}
	return nil
}

// openLogFile creates the log directory and opens the log file.
func (s *ServiceLifecycle) openLogFile() error {
	dir := s.cfg.ReportsDir
	if dir == "" {
		dir = defaultReportsDir
	}
	if s.projectRoot != "" && !filepath.IsAbs(dir) {
		dir = filepath.Join(s.projectRoot, dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create reports dir: %w", err)
	}
	name := fmt.Sprintf("server-%s.log", time.Now().Format("20060102-150405"))
	s.logPath = filepath.Join(dir, name)
	f, err := os.Create(s.logPath)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	s.logFile = f
	return nil
}

// FreePort obtains a free TCP port by listening on ":0" and returning the
// assigned port number. The listener is closed before returning, so there is
// an inherent TOCTOU race: another process may grab the port before the caller
// binds to it. This is acceptable for test orchestration where collisions are
// rare and the caller will fail loudly if the port is no longer free.
//
// Exported so that config generators and other packages (e.g. pkg/env) can
// reuse the same port-allocation strategy without duplicating the logic.
func FreePort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("listen: %w", err)
	}
	defer func() { _ = l.Close() }()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected address type: %T", l.Addr())
	}
	return addr.Port, nil
}
