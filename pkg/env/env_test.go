package env

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/config"
)

// =====================================================================
// NewSpec
// =====================================================================

// TestNewSpec verifies that NewSpec maps every config.Env field onto the
// corresponding EnvSpec field, including the HealthyTimeout string→Duration
// conversion performed via HealthyTimeoutDuration.
func TestNewSpec(t *testing.T) {
	t.Run("full field mapping", func(t *testing.T) {
		ce := config.Env{
			Kind:           config.EnvKindBackendGo,
			Port:           8080,
			BaseURL:        "http://localhost:8080",
			BinaryPath:     "bin/tickraft",
			BuildTarget:    "./cmd/tickraft",
			ConfigPath:     "configs/config.yaml",
			ConfigFlag:     "--config",
			AddrFlag:       "--addr",
			HealthPath:     "/api/v1/health",
			HealthyTimeout: "30s",
			Args:           []string{"--verbose"},
			Env:            []string{"FOO=bar"},
			Command:        "pnpm dev",
			Cwd:            "/tmp/app",
			ReadyURL:       "http://localhost:5173",
			ReadyText:      "ready",
		}
		got := NewSpec("tickraft-backend", ce)

		if got.Name != "tickraft-backend" {
			t.Errorf("Name = %q, want %q", got.Name, "tickraft-backend")
		}
		if got.Kind != config.EnvKindBackendGo {
			t.Errorf("Kind = %q, want %q", got.Kind, config.EnvKindBackendGo)
		}
		if got.Port != 8080 {
			t.Errorf("Port = %d, want 8080", got.Port)
		}
		if got.BaseURL != "http://localhost:8080" {
			t.Errorf("BaseURL = %q, want %q", got.BaseURL, "http://localhost:8080")
		}
		if got.BinaryPath != "bin/tickraft" {
			t.Errorf("BinaryPath = %q, want %q", got.BinaryPath, "bin/tickraft")
		}
		if got.BuildTarget != "./cmd/tickraft" {
			t.Errorf("BuildTarget = %q, want %q", got.BuildTarget, "./cmd/tickraft")
		}
		if got.ConfigPath != "configs/config.yaml" {
			t.Errorf("ConfigPath = %q, want %q", got.ConfigPath, "configs/config.yaml")
		}
		if got.ConfigFlag != "--config" {
			t.Errorf("ConfigFlag = %q, want %q", got.ConfigFlag, "--config")
		}
		if got.AddrFlag != "--addr" {
			t.Errorf("AddrFlag = %q, want %q", got.AddrFlag, "--addr")
		}
		if got.HealthPath != "/api/v1/health" {
			t.Errorf("HealthPath = %q, want %q", got.HealthPath, "/api/v1/health")
		}
		if got.HealthyTimeout != 30*time.Second {
			t.Errorf("HealthyTimeout = %v, want %v", got.HealthyTimeout, 30*time.Second)
		}
		if len(got.Args) != 1 || got.Args[0] != "--verbose" {
			t.Errorf("Args = %v, want [--verbose]", got.Args)
		}
		if len(got.Env) != 1 || got.Env[0] != "FOO=bar" {
			t.Errorf("Env = %v, want [FOO=bar]", got.Env)
		}
		if got.Command != "pnpm dev" {
			t.Errorf("Command = %q, want %q", got.Command, "pnpm dev")
		}
		if got.Cwd != "/tmp/app" {
			t.Errorf("Cwd = %q, want %q", got.Cwd, "/tmp/app")
		}
		if got.ReadyURL != "http://localhost:5173" {
			t.Errorf("ReadyURL = %q, want %q", got.ReadyURL, "http://localhost:5173")
		}
		if got.ReadyText != "ready" {
			t.Errorf("ReadyText = %q, want %q", got.ReadyText, "ready")
		}
	})

	// HealthyTimeout conversion edge cases are driven by config.Env's own
	// HealthyTimeoutDuration, so exercise the inputs that flow through NewSpec.
	cases := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"empty yields zero", "", 0},
		{"valid duration", "1m30s", 90 * time.Second},
		{"invalid yields zero", "not-a-duration", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := NewSpec("e", config.Env{HealthyTimeout: c.in})
			if spec.HealthyTimeout != c.want {
				t.Errorf("HealthyTimeout = %v, want %v", spec.HealthyTimeout, c.want)
			}
		})
	}

	t.Run("empty env yields zero-value spec with name", func(t *testing.T) {
		spec := NewSpec("empty", config.Env{})
		if spec.Name != "empty" {
			t.Errorf("Name = %q, want %q", spec.Name, "empty")
		}
		if spec.Kind != "" {
			t.Errorf("Kind = %q, want empty", spec.Kind)
		}
		if spec.Port != 0 {
			t.Errorf("Port = %d, want 0", spec.Port)
		}
		if spec.HealthyTimeout != 0 {
			t.Errorf("HealthyTimeout = %v, want 0", spec.HealthyTimeout)
		}
		if spec.Args != nil || spec.Env != nil {
			t.Errorf("Args/Env = %v/%v, want nil", spec.Args, spec.Env)
		}
	})
}

// =====================================================================
// New factory
// =====================================================================

// TestNew_BackendKinds verifies that backend.* kinds yield a *backend.
func TestNew_BackendKinds(t *testing.T) {
	for _, kind := range []config.EnvKind{config.EnvKindBackendGo, config.EnvKindBackendNode} {
		t.Run(string(kind), func(t *testing.T) {
			e, err := New(EnvSpec{Kind: kind, BinaryPath: "bin/x"}, "/tmp")
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			if e == nil {
				t.Fatal("New returned nil Environment")
			}
			if _, ok := e.(*backend); !ok {
				t.Fatalf("New returned %T, want *backend", e)
			}
		})
	}
}

// TestNew_ProcessKinds verifies that frontend.* and custom kinds yield a *processEnv.
func TestNew_ProcessKinds(t *testing.T) {
	for _, kind := range []config.EnvKind{
		config.EnvKindFrontendVite,
		config.EnvKindFrontendNuxt,
		config.EnvKindCustom,
	} {
		t.Run(string(kind), func(t *testing.T) {
			e, err := New(EnvSpec{Kind: kind, Command: "sleep 30"}, "/tmp")
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			if e == nil {
				t.Fatal("New returned nil Environment")
			}
			if _, ok := e.(*processEnv); !ok {
				t.Fatalf("New returned %T, want *processEnv", e)
			}
		})
	}
}

// TestNew_UnknownKind verifies that an unrecognized kind produces an error.
func TestNew_UnknownKind(t *testing.T) {
	_, err := New(EnvSpec{Kind: config.EnvKind("bogus.kind")}, "/tmp")
	if err == nil {
		t.Fatal("expected error for unknown kind, got nil")
	}
	if !strings.Contains(err.Error(), "bogus.kind") {
		t.Errorf("error %q does not mention the unknown kind", err)
	}
}

// =====================================================================
// backend
// =====================================================================

// TestBackend_WithBaseURL verifies that a preset BaseURL short-circuits Start
// and makes Stop a no-op, without ever invoking the underlying lifecycle.
func TestBackend_WithBaseURL(t *testing.T) {
	const want = "http://example.test:1234"
	b := newBackend(EnvSpec{Kind: config.EnvKindBackendGo, BaseURL: want}, "/tmp")

	// Before Start, BaseURL and LogPath are empty (lifecycle never started).
	if got := b.BaseURL(); got != "" {
		t.Errorf("BaseURL before Start = %q, want empty", got)
	}
	if got := b.LogPath(); got != "" {
		t.Errorf("LogPath before Start = %q, want empty", got)
	}

	got, err := b.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if got != want {
		t.Errorf("Start returned %q, want %q", got, want)
	}
	if b.BaseURL() != want {
		t.Errorf("BaseURL after Start = %q, want %q", b.BaseURL(), want)
	}
	// Externally managed env must not open a log file.
	if b.LogPath() != "" {
		t.Errorf("LogPath after Start = %q, want empty", b.LogPath())
	}

	if err := b.Stop(context.Background()); err != nil {
		t.Errorf("Stop returned error for externally managed env: %v", err)
	}
	// BaseURL is preserved after Stop.
	if b.BaseURL() != want {
		t.Errorf("BaseURL after Stop = %q, want %q", b.BaseURL(), want)
	}
}

// TestBackend_BaseURLAndLogPathBeforeStart confirms empty getters prior to Start.
func TestBackend_BaseURLAndLogPathBeforeStart(t *testing.T) {
	b := newBackend(EnvSpec{Kind: config.EnvKindBackendGo, BinaryPath: "bin/x"}, "/tmp")
	if b.BaseURL() != "" {
		t.Errorf("BaseURL = %q, want empty", b.BaseURL())
	}
	if b.LogPath() != "" {
		t.Errorf("LogPath = %q, want empty", b.LogPath())
	}
}

// =====================================================================
// processEnv: Start with BaseURL
// =====================================================================

// TestProcessEnv_StartWithBaseURL verifies that a preset BaseURL skips process
// launch entirely.
func TestProcessEnv_StartWithBaseURL(t *testing.T) {
	const want = "http://example.test:5173"
	f := newProcessEnv(EnvSpec{Kind: config.EnvKindFrontendVite, BaseURL: want}, "/tmp")

	if f.BaseURL() != "" {
		t.Errorf("BaseURL before Start = %q, want empty", f.BaseURL())
	}
	if f.LogPath() != "" {
		t.Errorf("LogPath before Start = %q, want empty", f.LogPath())
	}
	if f.cmd != nil {
		t.Errorf("cmd set before Start = %v, want nil", f.cmd)
	}

	got, err := f.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if got != want {
		t.Errorf("Start returned %q, want %q", got, want)
	}
	if f.BaseURL() != want {
		t.Errorf("BaseURL after Start = %q, want %q", f.BaseURL(), want)
	}
	// No log file is opened on the BaseURL short-circuit path.
	if f.LogPath() != "" {
		t.Errorf("LogPath after Start = %q, want empty", f.LogPath())
	}
	if f.cmd != nil {
		t.Errorf("cmd set after Start = %v, want nil", f.cmd)
	}

	if err := f.Stop(context.Background()); err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
}

// =====================================================================
// processEnv: Start error paths
// =====================================================================

// TestProcessEnv_StartEmptyCommand verifies that Start fails when Command is
// empty and no BaseURL is preset.
func TestProcessEnv_StartEmptyCommand(t *testing.T) {
	f := newProcessEnv(EnvSpec{Kind: config.EnvKindCustom, Name: "no-cmd"}, t.TempDir())
	_, err := f.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
	if !strings.Contains(err.Error(), "no-cmd") {
		t.Errorf("error %q does not mention env name", err)
	}
	if f.cmd != nil {
		t.Errorf("cmd should be nil after failed Start, got %v", f.cmd)
	}
}

// TestProcessEnv_StartCommandNotFound verifies that a missing binary surfaces a
// wrapped start error and leaves no running process behind.
func TestProcessEnv_StartCommandNotFound(t *testing.T) {
	f := newProcessEnv(EnvSpec{
		Kind:    config.EnvKindCustom,
		Name:    "missing",
		Command: "definitely-not-a-real-binary-xyzzy",
	}, t.TempDir())
	_, err := f.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error %q does not mention env name", err)
	}
	if f.cmd != nil {
		t.Errorf("cmd should be nil after failed Start, got %v", f.cmd)
	}
}

// TestProcessEnv_StartReadyURLEmpty verifies that a launched process with no
// ReadyURL fails fast and the process is cleaned up.
func TestProcessEnv_StartReadyURLEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	f := newProcessEnv(EnvSpec{
		Kind:    config.EnvKindCustom,
		Name:    "no-ready",
		Command: "sleep 30",
	}, t.TempDir())
	_, err := f.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for empty ready_url, got nil")
	}
	if !strings.Contains(err.Error(), "no-ready") {
		t.Errorf("error %q does not mention env name", err)
	}
	// stopLocked must have been invoked, clearing the command.
	if f.cmd != nil {
		t.Errorf("cmd should be nil after failed Start, got %v", f.cmd)
	}
}

// =====================================================================
// processEnv: Start success (end-to-end with httptest)
// =====================================================================

// startReadyServer returns an httptest server that responds with status 200 and
// a body containing the given text.
func startReadyServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
}

// TestProcessEnv_StartSuccess exercises the full Start→Stop flow: a real
// subprocess (sleep) is launched while readiness is detected by polling an
// httptest server.
func TestProcessEnv_StartSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	srv := startReadyServer(t, "Vite ready")
	defer srv.Close()

	root := t.TempDir()
	f := newProcessEnv(EnvSpec{
		Kind:      config.EnvKindFrontendVite,
		Name:      "e2e",
		Command:   "sleep 30",
		ReadyURL:  srv.URL,
		ReadyText: "ready",
	}, root)

	got, err := f.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if got != srv.URL {
		t.Errorf("Start returned %q, want %q", got, srv.URL)
	}
	if f.BaseURL() != srv.URL {
		t.Errorf("BaseURL = %q, want %q", f.BaseURL(), srv.URL)
	}

	// A log file must have been created under <root>/reports.
	logPath := f.LogPath()
	if logPath == "" {
		t.Fatal("LogPath is empty after Start")
	}
	if !strings.HasPrefix(logPath, filepath.Join(root, "reports")) {
		t.Errorf("LogPath = %q, want it under %q", logPath, filepath.Join(root, "reports"))
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("log file does not exist: %v", err)
	}

	if err := f.Stop(context.Background()); err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
	// LogPath is preserved after Stop; BaseURL is kept.
	if f.LogPath() != logPath {
		t.Errorf("LogPath after Stop = %q, want %q", f.LogPath(), logPath)
	}
	if f.BaseURL() != srv.URL {
		t.Errorf("BaseURL after Stop = %q, want %q", f.BaseURL(), srv.URL)
	}
	if f.cmd != nil {
		t.Errorf("cmd should be nil after Stop, got %v", f.cmd)
	}
}

// TestProcessEnv_StartSuccessNoReadyText verifies the ReadyText-empty branch:
// any 2xx/4xx response is treated as ready.
func TestProcessEnv_StartSuccessNoReadyText(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	f := newProcessEnv(EnvSpec{
		Kind:     config.EnvKindCustom,
		Name:     "no-text",
		Command:  "sleep 30",
		ReadyURL: srv.URL,
	}, t.TempDir())

	got, err := f.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if got != srv.URL {
		t.Errorf("Start returned %q, want %q", got, srv.URL)
	}
	if err := f.Stop(context.Background()); err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
}

// =====================================================================
// processEnv: cwd resolution
// =====================================================================

// TestProcessEnv_CwdResolution covers the three Cwd resolution branches in
// Start: absolute path, relative path (joined with projectRoot), and a
// non-existent directory (must fail).
func TestProcessEnv_CwdResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	srv := startReadyServer(t, "ready")
	defer srv.Close()

	t.Run("absolute existing cwd", func(t *testing.T) {
		abs := t.TempDir()
		f := newProcessEnv(EnvSpec{
			Kind: config.EnvKindCustom, Name: "abs",
			Command: "sleep 30", Cwd: abs, ReadyURL: srv.URL, ReadyText: "ready",
		}, t.TempDir())
		got, err := f.Start(context.Background())
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
		if got != srv.URL {
			t.Errorf("Start returned %q, want %q", got, srv.URL)
		}
		_ = f.Stop(context.Background())
	})

	t.Run("relative cwd resolves against projectRoot", func(t *testing.T) {
		root := t.TempDir()
		sub := filepath.Join(root, "mydir")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		f := newProcessEnv(EnvSpec{
			Kind: config.EnvKindCustom, Name: "rel",
			Command: "sleep 30", Cwd: "mydir", ReadyURL: srv.URL, ReadyText: "ready",
		}, root)
		got, err := f.Start(context.Background())
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
		if got != srv.URL {
			t.Errorf("Start returned %q, want %q", got, srv.URL)
		}
		_ = f.Stop(context.Background())
	})

	t.Run("non-existent cwd fails to start", func(t *testing.T) {
		f := newProcessEnv(EnvSpec{
			Kind: config.EnvKindCustom, Name: "badcwd",
			Command: "sleep 30",
			Cwd:     filepath.Join(t.TempDir(), "does-not-exist"),
		}, t.TempDir())
		_, err := f.Start(context.Background())
		if err == nil {
			t.Fatal("expected error for non-existent cwd, got nil")
		}
		if f.cmd != nil {
			t.Errorf("cmd should be nil after failed Start, got %v", f.cmd)
		}
	})
}

// =====================================================================
// processEnv: Stop / getters without Start
// =====================================================================

// TestProcessEnv_StopWithoutStart verifies Stop is a no-op when nothing ran.
func TestProcessEnv_StopWithoutStart(t *testing.T) {
	f := newProcessEnv(EnvSpec{Kind: config.EnvKindCustom, Name: "x"}, t.TempDir())
	if err := f.Stop(context.Background()); err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
}

// TestProcessEnv_BaseURLAndLogPathBeforeStart confirms empty getters prior to Start.
func TestProcessEnv_BaseURLAndLogPathBeforeStart(t *testing.T) {
	f := newProcessEnv(EnvSpec{Kind: config.EnvKindCustom, Name: "x"}, t.TempDir())
	if f.BaseURL() != "" {
		t.Errorf("BaseURL = %q, want empty", f.BaseURL())
	}
	if f.LogPath() != "" {
		t.Errorf("LogPath = %q, want empty", f.LogPath())
	}
}

// =====================================================================
// processEnv: openLog
// =====================================================================

// TestProcessEnv_OpenLog verifies openLog creates the reports directory and a
// log file under the project root.
func TestProcessEnv_OpenLog(t *testing.T) {
	root := t.TempDir()
	f := newProcessEnv(EnvSpec{Kind: config.EnvKindCustom, Name: "logger"}, root)

	path, file, err := f.openLog()
	if err != nil {
		t.Fatalf("openLog returned error: %v", err)
	}
	if file == nil {
		t.Fatal("openLog returned nil file")
	}
	defer file.Close()

	wantDir := filepath.Join(root, "reports")
	if !strings.HasPrefix(path, wantDir) {
		t.Errorf("path = %q, want under %q", path, wantDir)
	}
	if info, err := os.Stat(wantDir); err != nil {
		t.Errorf("reports dir not created: %v", err)
	} else if !info.IsDir() {
		t.Errorf("reports path is not a directory: %v", info)
	}
	if info, err := os.Stat(path); err != nil {
		t.Errorf("log file not created: %v", err)
	} else if info.Size() != 0 {
		// A freshly created log file should be empty until the process writes.
		t.Errorf("new log file size = %d, want 0", info.Size())
	}
	// The filename must encode the env name.
	if !strings.Contains(filepath.Base(path), "logger") {
		t.Errorf("log filename %q does not contain env name", filepath.Base(path))
	}
}

// TestProcessEnv_OpenLogNoProjectRoot verifies the empty-projectRoot branch
// writes to a relative "reports" directory. t.Chdir isolates the working
// directory so the test does not pollute the package directory.
func TestProcessEnv_OpenLogNoProjectRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	f := newProcessEnv(EnvSpec{Kind: config.EnvKindCustom, Name: "norel"}, "")

	path, file, err := f.openLog()
	if err != nil {
		t.Fatalf("openLog returned error: %v", err)
	}
	defer file.Close()

	wantDir := filepath.Join(".", "reports")
	if !strings.HasPrefix(path, wantDir) {
		t.Errorf("path = %q, want under %q", path, wantDir)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("log file not created: %v", err)
	}
}

// =====================================================================
// processEnv: waitForReady (direct, no subprocess)
// =====================================================================

// TestProcessEnv_WaitForReady_ReadyURLEmpty verifies the immediate error path.
func TestProcessEnv_WaitForReady_ReadyURLEmpty(t *testing.T) {
	f := newProcessEnv(EnvSpec{Kind: config.EnvKindCustom, Name: "x"}, "")
	_, err := f.waitForReady(context.Background())
	if err == nil {
		t.Fatal("expected error for empty ready_url, got nil")
	}
	if !strings.Contains(err.Error(), "x") {
		t.Errorf("error %q does not mention env name", err)
	}
}

// TestProcessEnv_WaitForReady_NoReadyText verifies that a 2xx response with no
// ReadyText configured returns the URL immediately.
func TestProcessEnv_WaitForReady_NoReadyText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	f := newProcessEnv(EnvSpec{Kind: config.EnvKindCustom, ReadyURL: srv.URL}, "")
	got, err := f.waitForReady(context.Background())
	if err != nil {
		t.Fatalf("waitForReady returned error: %v", err)
	}
	if got != srv.URL {
		t.Errorf("waitForReady returned %q, want %q", got, srv.URL)
	}
}

// TestProcessEnv_WaitForReady_ReadyTextMatch verifies that the URL is returned
// when the response body contains the configured ReadyText.
func TestProcessEnv_WaitForReady_ReadyTextMatch(t *testing.T) {
	srv := startReadyServer(t, "the server is ready now")
	defer srv.Close()
	f := newProcessEnv(EnvSpec{
		Kind: config.EnvKindCustom, ReadyURL: srv.URL, ReadyText: "ready",
	}, "")
	got, err := f.waitForReady(context.Background())
	if err != nil {
		t.Fatalf("waitForReady returned error: %v", err)
	}
	if got != srv.URL {
		t.Errorf("waitForReady returned %q, want %q", got, srv.URL)
	}
}

// TestProcessEnv_WaitForReady_NotReadyStatus verifies that a 500 response keeps
// polling until the context expires.
func TestProcessEnv_WaitForReady_NotReadyStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	f := newProcessEnv(EnvSpec{Kind: config.EnvKindCustom, ReadyURL: srv.URL}, "")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := f.waitForReady(ctx)
	if err == nil {
		t.Fatal("expected error for unhealthy server, got nil")
	}
	// The context deadline must fire before the 60s internal timeout.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
}

// TestProcessEnv_WaitForReady_ReadyTextMismatch verifies that a 2xx response
// whose body lacks ReadyText keeps polling until the context expires.
func TestProcessEnv_WaitForReady_ReadyTextMismatch(t *testing.T) {
	srv := startReadyServer(t, "still compiling")
	defer srv.Close()
	f := newProcessEnv(EnvSpec{
		Kind: config.EnvKindCustom, ReadyURL: srv.URL, ReadyText: "ready",
	}, "")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := f.waitForReady(ctx)
	if err == nil {
		t.Fatal("expected error for ready_text mismatch, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
}

// =====================================================================
// Manager
// =====================================================================

// TestManager_UnknownKind verifies that NewManager propagates the New error.
func TestManager_UnknownKind(t *testing.T) {
	_, err := NewManager(EnvSpec{Kind: config.EnvKind("bogus")}, "/tmp")
	if err == nil {
		t.Fatal("expected error for unknown kind, got nil")
	}
}

// TestManager_Delegation verifies that Manager delegates Start/Stop/BaseURL/
// LogPath to the underlying Environment and exposes the original spec.
func TestManager_Delegation(t *testing.T) {
	const want = "http://example.test:9999"
	spec := EnvSpec{Kind: config.EnvKindFrontendVite, Name: "m", BaseURL: want}
	m, err := NewManager(spec, "/tmp")
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	// Spec() must round-trip the original spec.
	gotSpec := m.Spec()
	if gotSpec.Kind != spec.Kind || gotSpec.Name != spec.Name || gotSpec.BaseURL != spec.BaseURL {
		t.Errorf("Spec = %+v, want %+v", gotSpec, spec)
	}

	// Before Start, BaseURL/LogPath are empty.
	if m.BaseURL() != "" {
		t.Errorf("BaseURL before Start = %q, want empty", m.BaseURL())
	}
	if m.LogPath() != "" {
		t.Errorf("LogPath before Start = %q, want empty", m.LogPath())
	}

	got, err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if got != want {
		t.Errorf("Start returned %q, want %q", got, want)
	}
	if m.BaseURL() != want {
		t.Errorf("BaseURL after Start = %q, want %q", m.BaseURL(), want)
	}
	if m.LogPath() != "" {
		t.Errorf("LogPath after Start = %q, want empty (externally managed)", m.LogPath())
	}

	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
	if m.BaseURL() != want {
		t.Errorf("BaseURL after Stop = %q, want %q", m.BaseURL(), want)
	}
}

// TestManager_BackendDelegation verifies the same delegation for a backend env
// using a BaseURL preset.
func TestManager_BackendDelegation(t *testing.T) {
	const want = "http://example.test:8080"
	spec := EnvSpec{Kind: config.EnvKindBackendGo, Name: "be", BaseURL: want}
	m, err := NewManager(spec, "/tmp")
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	got, err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if got != want {
		t.Errorf("Start returned %q, want %q", got, want)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
}

// =====================================================================
// FreePort
// =====================================================================

// TestFreePort verifies that FreePort returns a non-zero port that is
// immediately bindable.
func TestFreePort(t *testing.T) {
	port, err := FreePort()
	if err != nil {
		t.Fatalf("FreePort returned error: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Fatalf("port = %d, want a valid TCP port (1-65535)", port)
	}

	// The returned port must be bindable immediately (TOCTOU notwithstanding,
	// a single immediate Listen must succeed in practice).
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("could not listen on returned port %d: %v", port, err)
	}
	defer l.Close()

	// A second call must also succeed and return a valid port.
	port2, err := FreePort()
	if err != nil {
		t.Fatalf("second FreePort returned error: %v", err)
	}
	if port2 <= 0 || port2 > 65535 {
		t.Fatalf("second port = %d, want a valid TCP port", port2)
	}
}

// TestFreePort_Concurrent verifies that concurrent calls to FreePort are safe
// (run with -race) and every returned port is a valid TCP port number.
// Uniqueness is NOT asserted because the underlying listener is closed before
// the port is returned, so sequential calls may legitimately reuse a port.
func TestFreePort_Concurrent(t *testing.T) {
	const n = 100
	ports := make([]int, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			ports[idx], errs[idx] = FreePort()
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d error: %v", i, errs[i])
			continue
		}
		if ports[i] <= 0 || ports[i] > 65535 {
			t.Errorf("goroutine %d port = %d, want a valid TCP port", i, ports[i])
		}
	}
}

// =====================================================================
// splitCommand
// =====================================================================

// TestSplitCommand verifies command-string parsing including double-quoted
// segments containing whitespace and backslash-escaped characters.
func TestSplitCommand(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"simple", "npm run dev", []string{"npm", "run", "dev"}},
		{"quoted segment with space", `npm "run dev" --port 5173`, []string{"npm", "run dev", "--port", "5173"}},
		{"sh -c quoted", `sh -c "echo hi"`, []string{"sh", "-c", "echo hi"}},
		{"empty", "", nil},
		{"single token", "single", []string{"single"}},
		{"whitespace only", "  ", nil},
		{"escaped quote inside", `prog "a\"b"`, []string{"prog", `a"b`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitCommand(c.in)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if len(got) != len(c.want) {
				t.Errorf("splitCommand(%q) = %v, want %v", c.in, got, c.want)
				return
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("splitCommand(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

// =====================================================================
// processEnv: waitForReady configurable timeout
// =====================================================================

// TestProcessEnv_ReadyTimeoutConfigurable verifies that waitForReady honors
// spec.HealthyTimeout instead of the hardcoded 60s default. An unreachable
// ReadyURL (connection refused on port 1) keeps polling; with HealthyTimeout
// set to 200ms the function must return well within 1 second.
func TestProcessEnv_ReadyTimeoutConfigurable(t *testing.T) {
	f := newProcessEnv(EnvSpec{
		Kind:           config.EnvKindCustom,
		Name:           "timeout",
		ReadyURL:       "http://127.0.0.1:1/ready",
		HealthyTimeout: 200 * time.Millisecond,
	}, "")

	start := time.Now()
	_, err := f.waitForReady(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error for unreachable ready URL, got nil")
	}
	if elapsed >= time.Second {
		t.Errorf("waitForReady took %s, want < 1s (HealthyTimeout=200ms should be respected, not the 60s default)", elapsed)
	}
}
