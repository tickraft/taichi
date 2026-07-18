// Package env implements the lifecycle management of test environments.
//
// An environment is "the runtime state of the service under test": taichi starts
// the environment before running skills and stops it after.
// This package provides two environment implementations:
//   - Backend: based on Go/Node binaries, reuses framework.ServiceLifecycle
//   - Process: based on an arbitrary launch command (npm/pnpm dev server, uvicorn,
//     cargo run, java -jar, etc.), started as a subprocess and waited until the
//     ready URL becomes reachable. Serves both frontend.* and custom Kind.
//
// Environments are configured via Spec (isomorphic to config.Env) and
// orchestrated by the Manager.
package env

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tickraft/taichi/pkg/config"
	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/i18n"
)

const (
	// defaultFrontendReadyTimeout is the total timeout to wait for a
	// frontend/custom env to become ready when HealthyTimeout is not set.
	defaultFrontendReadyTimeout = 60 * time.Second
)

// Spec is the runtime config of an environment, isomorphic to config.Env but
// supplemented with fields needed at runtime.
//
// Converted from config.Env via NewSpec.
type Spec struct {
	// Name is the unique identifier of the env (matching the config.Env map key).
	Name string
	// Kind is the major env category.
	Kind config.EnvKind
	// Port is the fixed listening port. 0 means auto.
	Port int
	// BaseURL is the base URL of an already-ready env. If non-empty, the Manager skips startup and uses it directly.
	BaseURL string

	// Backend fields (used when Kind=backend.*).
	BinaryPath     string
	ConfigPath     string
	ConfigFlag     string
	AddrFlag       string
	HealthPath     string
	HealthyTimeout time.Duration
	Args           []string
	Env            []string

	// Frontend fields (used when Kind=frontend.*).
	Command   string
	Cwd       string
	ReadyURL  string
	ReadyText string

	// Build is the pre-start build step — a shell command executed via `sh -c`.
	// For backend.go it runs in the project root; for custom/frontend.* it runs
	// in Cwd before Command launches. Empty means no build step.
	Build string
}

// NewSpec builds an Spec from a config.Env.
func NewSpec(name string, e config.Env) Spec {
	return Spec{
		Name:           name,
		Kind:           e.Kind,
		Port:           e.Port,
		BaseURL:        e.BaseURL,
		BinaryPath:     e.BinaryPath,
		Build:          e.Build,
		ConfigPath:     e.ConfigPath,
		ConfigFlag:     e.ConfigFlag,
		AddrFlag:       e.AddrFlag,
		HealthPath:     e.HealthPath,
		HealthyTimeout: e.HealthyTimeoutDuration(),
		Args:           e.Args,
		Env:            e.Env,
		Command:        e.Command,
		Cwd:            e.Cwd,
		ReadyURL:       e.ReadyURL,
		ReadyText:      e.ReadyText,
	}
}

// Environment is the abstraction for the env lifecycle.
// Both Backend and Frontend implement this interface.
type Environment interface {
	// Start starts the env and waits until it is ready. Returns the BaseURL.
	Start(ctx context.Context) (string, error)
	// Stop stops the env.
	Stop(ctx context.Context) error
	// BaseURL returns the current BaseURL. Empty string when not started.
	BaseURL() string
	// LogPath returns the log file path (if any). Empty when there is no log.
	LogPath() string
}

// New builds an Environment instance from spec and projectRoot.
//
// frontend.* and custom Kinds share the process-based implementation: as long as
// command (launch command) and ready_url (health check URL) are provided, a service
// in any language can be started (Python/Rust/Java/Ruby/Node backend, etc.).
// Pure Go/Node binary services are recommended to use backend.* to get build and
// port management capabilities.
func New(spec Spec, projectRoot string) (Environment, error) {
	switch spec.Kind {
	case config.EnvKindBackendGo, config.EnvKindBackendNode:
		return newBackend(spec, projectRoot), nil
	case config.EnvKindFrontendVite, config.EnvKindFrontendNuxt, config.EnvKindCustom:
		return newProcessEnv(spec, projectRoot), nil
	default:
		return nil, errors.New(i18n.T("env.unknown_kind", spec.Kind))
	}
}

// backend wraps framework.ServiceLifecycle to adapt it to the Environment interface.
type backend struct {
	lifecycle *framework.ServiceLifecycle
	spec      Spec
	baseURL   string
}

func newBackend(spec Spec, projectRoot string) *backend {
	cfg := framework.ServiceConfig{
		BinaryPath:     spec.BinaryPath,
		Build:          spec.Build,
		ConfigPath:     spec.ConfigPath,
		ConfigFlag:     spec.ConfigFlag,
		AddrFlag:       spec.AddrFlag,
		Port:           spec.Port,
		HealthPath:     spec.HealthPath,
		HealthyTimeout: spec.HealthyTimeout,
		Args:           spec.Args,
		Env:            spec.Env,
	}
	lc := framework.NewServiceLifecycle(cfg)
	if projectRoot != "" {
		lc.SetProjectRoot(projectRoot)
	}
	return &backend{lifecycle: lc, spec: spec}
}

func (b *backend) Start(ctx context.Context) (string, error) {
	if b.spec.BaseURL != "" {
		b.baseURL = b.spec.BaseURL
		return b.baseURL, nil
	}
	if err := b.lifecycle.Start(); err != nil {
		return "", err
	}
	b.baseURL = b.lifecycle.BaseURL()
	return b.baseURL, nil
}

func (b *backend) Stop(ctx context.Context) error {
	if b.spec.BaseURL != "" {
		// Externally managed env; do not stop.
		return nil
	}
	return b.lifecycle.Stop()
}

func (b *backend) BaseURL() string {
	return b.baseURL
}

func (b *backend) LogPath() string {
	return b.lifecycle.ServerLogPath()
}

// processEnv manages a subprocess running an arbitrary launch command, treating
// it as ready once the ready URL becomes reachable.
//
// Serves frontend.* and custom Kinds: the former targets JS toolchain dev servers
// (npm/pnpm, etc.), while the latter targets service processes in any language
// (uvicorn / cargo run / java -jar / rails server, etc.).
// On stop, it first sends Interrupt; if the process does not exit within 10
// seconds, it sends Kill.
type processEnv struct {
	spec        Spec
	projectRoot string
	cmd         *exec.Cmd
	logFile     *os.File
	logPath     string
	baseURL     string
	mu          sync.Mutex
}

func newProcessEnv(spec Spec, projectRoot string) *processEnv {
	return &processEnv{spec: spec, projectRoot: projectRoot}
}

func (f *processEnv) Start(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.spec.BaseURL != "" {
		f.baseURL = f.spec.BaseURL
		return f.baseURL, nil
	}
	if f.spec.Command == "" {
		return "", errors.New(i18n.T("env.frontend.cmd_empty", f.spec.Name))
	}

	// Run the optional build step before launching the service.
	// Build is used for projects whose source must be compiled before
	// running (e.g. Maven, cargo, npm run build). It runs in the resolved Cwd.
	// Build failures are propagated: a failing build means the service cannot
	// be in a known-good state, so we abort the start.
	if f.spec.Build != "" {
		if err := f.runBuildCommand(ctx); err != nil {
			return "", err
		}
	}

	logPath, logFile, err := f.openLog()
	if err != nil {
		return "", err
	}
	f.logPath = logPath
	f.logFile = logFile

	parts := splitCommand(f.spec.Command)
	if len(parts) == 0 {
		return "", errors.New(i18n.T("env.frontend.cmd_empty", f.spec.Name))
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// cwd resolution: absolute path used directly; relative path resolved against projectRoot; both empty inherits the parent process's cwd.
	switch {
	case f.spec.Cwd == "":
		// Inherit the parent process's working directory.
	case filepath.IsAbs(f.spec.Cwd):
		cmd.Dir = f.spec.Cwd
	case f.projectRoot != "":
		cmd.Dir = filepath.Join(f.projectRoot, f.spec.Cwd)
	default:
		cmd.Dir = f.spec.Cwd
	}
	if len(f.spec.Env) > 0 {
		cmd.Env = append(os.Environ(), f.spec.Env...)
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close() // best-effort: release the log file on start failure; the start error is the primary result.
		return "", fmt.Errorf("%s: %w", i18n.T("env.frontend.start_failed", f.spec.Name), err)
	}
	f.cmd = cmd

	baseURL, err := f.waitForReady(ctx)
	if err != nil {
		_ = f.stopLocked() // best-effort: tear down the launched process on readiness failure; the readiness error is propagated.
		return "", err
	}
	f.baseURL = baseURL
	return baseURL, nil
}

// runBuildCommand executes the pre-start build command in the resolved Cwd.
// The command runs via `sh -c` so shell features (pipes, &&, env vars) are
// available. Build output (stdout+stderr) is inherited by the parent process
// so the user can see the build progress. Returns an error wrapping the build
// output if the command exits non-zero.
func (f *processEnv) runBuildCommand(ctx context.Context) error {
	if strings.TrimSpace(f.spec.Build) == "" {
		return errors.New(i18n.T("env.frontend.build_empty", f.spec.Name))
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", f.spec.Build)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Resolve cwd using the same rules as the service command.
	switch {
	case f.spec.Cwd == "":
	case filepath.IsAbs(f.spec.Cwd):
		cmd.Dir = f.spec.Cwd
	case f.projectRoot != "":
		cmd.Dir = filepath.Join(f.projectRoot, f.spec.Cwd)
	default:
		cmd.Dir = f.spec.Cwd
	}
	if len(f.spec.Env) > 0 {
		cmd.Env = append(os.Environ(), f.spec.Env...)
	}
	cmd.WaitDelay = 30 * time.Second
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("env.frontend.build_failed", f.spec.Name), err)
	}
	return nil
}

func (f *processEnv) waitForReady(ctx context.Context) (string, error) {
	if f.spec.ReadyURL == "" {
		return "", errors.New(i18n.T("env.frontend.ready_url_empty", f.spec.Name))
	}
	baseURL := f.spec.ReadyURL
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := ctx.Done()
	readyTimeout := f.spec.HealthyTimeout
	if readyTimeout <= 0 {
		readyTimeout = defaultFrontendReadyTimeout
	}
	timeout := time.After(readyTimeout)
	for {
		select {
		case <-deadline:
			return "", ctx.Err()
		case <-timeout:
			return "", errors.New(i18n.T("env.frontend.not_ready", f.spec.Name))
		default:
		}
		resp, err := client.Get(baseURL)
		if err == nil {
			body := make([]byte, 0)
			if f.spec.ReadyText != "" {
				buf := make([]byte, 4096)
				n, _ := resp.Body.Read(buf) // best-effort: a short read just means the body is smaller than the buffer; the partial content is still usable for ReadyText matching.
				body = buf[:n]
				_ = resp.Body.Close() // best-effort: close the response body after reading the readiness snippet; read errors do not affect the readiness verdict.
			} else {
				_ = resp.Body.Close() // best-effort: close the response body when ReadyText is not configured; nothing more to read.
			}
			if resp.StatusCode >= 200 && resp.StatusCode < 500 {
				if f.spec.ReadyText == "" || strings.Contains(string(body), f.spec.ReadyText) {
					return baseURL, nil
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (f *processEnv) Stop(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopLocked()
}

func (f *processEnv) stopLocked() error {
	if f.cmd == nil || f.cmd.Process == nil {
		return nil
	}
	_ = f.cmd.Process.Signal(os.Interrupt) // best-effort: ask the process to exit gracefully; escalation to Kill follows if it ignores Interrupt.
	done := make(chan error, 1)
	go func() {
		done <- f.cmd.Wait()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = f.cmd.Process.Kill() // best-effort: force-kill the process after the 10s grace period; the result is observed via cmd.Wait below.
		<-done
	}
	if f.logFile != nil {
		_ = f.logFile.Close() // best-effort: release the log file handle during shutdown; nothing more can be written after this point.
		f.logFile = nil
	}
	f.cmd = nil
	return nil
}

func (f *processEnv) BaseURL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.baseURL
}

func (f *processEnv) LogPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logPath
}

func (f *processEnv) openLog() (string, *os.File, error) {
	dir := filepath.Join(f.projectRoot, "reports")
	if f.projectRoot == "" {
		dir = "reports"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, fmt.Errorf("create reports dir: %w", err)
	}
	name := fmt.Sprintf("frontend-%s-%s.log", f.spec.Name, time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		return "", nil, fmt.Errorf("create log file: %w", err)
	}
	return path, file, nil
}

// splitCommand splits a command string into program and arguments.
// It supports double-quoted segments containing whitespace and
// backslash-escaped characters (\", \\). Single quotes are treated
// literally (not as quoting characters) to keep the grammar minimal.
// Returns nil if s is empty or contains no tokens.
func splitCommand(s string) []string {
	var args []string
	var b strings.Builder
	inQuote := false
	escaped := false
	hasToken := false
	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
			hasToken = true
		case r == '"':
			inQuote = !inQuote
			hasToken = true
		case (r == ' ' || r == '\t') && !inQuote:
			if hasToken {
				args = append(args, b.String())
				b.Reset()
				hasToken = false
			}
		default:
			b.WriteRune(r)
			hasToken = true
		}
	}
	if hasToken {
		args = append(args, b.String())
	}
	return args
}

// Manager manages the start/stop of the env for a single project.
//
// One Manager instance corresponds to a single test run. To reuse, stop the
// current env first.
type Manager struct {
	env  Environment
	spec Spec
}

// NewManager creates a Manager from spec and projectRoot.
func NewManager(spec Spec, projectRoot string) (*Manager, error) {
	e, err := New(spec, projectRoot)
	if err != nil {
		return nil, err
	}
	return &Manager{env: e, spec: spec}, nil
}

// Start starts the env and returns the BaseURL.
func (m *Manager) Start(ctx context.Context) (string, error) {
	return m.env.Start(ctx)
}

// Stop stops the env.
func (m *Manager) Stop(ctx context.Context) error {
	return m.env.Stop(ctx)
}

// BaseURL returns the current BaseURL.
func (m *Manager) BaseURL() string {
	return m.env.BaseURL()
}

// LogPath returns the env log path (if any).
func (m *Manager) LogPath() string {
	return m.env.LogPath()
}

// Spec returns the env config.
func (m *Manager) Spec() Spec {
	return m.spec
}

// FreePort returns a free TCP port (exposed for use by config generators).
// It delegates to framework.FreePort so the port-allocation logic is
// defined in a single place rather than duplicated across packages.
func FreePort() (int, error) {
	return framework.FreePort()
}
