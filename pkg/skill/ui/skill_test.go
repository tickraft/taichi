package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/skill"
)

// newTestContext builds a SkillContext backed by a fresh reporter and assertion engine.
func newTestContext(t *testing.T, baseURL string) *skill.SkillContext {
	t.Helper()
	return &skill.SkillContext{
		Ctx:         context.Background(),
		ProjectName: "test-project",
		BaseURL:     baseURL,
		Asserts:     framework.NewAssertionEngine(),
		Reporter:    framework.NewTestReporter(),
		ReportsDir:  t.TempDir(),
		Logger:      skill.NoOpLogger{},
		Extra:       make(map[string]any),
	}
}

// configureAndSetup wires the skill with the given raw config and invokes Setup.
func configureAndSetup(t *testing.T, s *Skill, raw map[string]any) {
	t.Helper()
	if err := s.Configure(skill.SkillConfig{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := s.Setup(nil); err != nil {
		t.Fatalf("Setup: %v", err)
	}
}

// TestName verifies the skill identifier.
func TestName(t *testing.T) {
	s := &Skill{}
	if got := s.Name(); got != "ui" {
		t.Errorf("Name() = %q, want \"ui\"", got)
	}
}

// TestKind verifies the skill category.
func TestKind(t *testing.T) {
	s := &Skill{}
	if got := s.Kind(); got != skill.KindUI {
		t.Errorf("Kind() = %q, want %q", got, skill.KindUI)
	}
}

// TestPriority verifies the configured priority.
func TestPriority(t *testing.T) {
	s := &Skill{}
	if got := s.Priority(); got != skill.PriorityHigh {
		t.Errorf("Priority() = %v, want %v", got, skill.PriorityHigh)
	}
}

// TestConfigureEmpty ensures Configure tolerates an empty raw map and produces no pages.
func TestConfigureEmpty(t *testing.T) {
	s := &Skill{}
	if err := s.Configure(skill.SkillConfig{Raw: map[string]any{}}); err != nil {
		t.Fatalf("Configure empty raw: %v", err)
	}
	if len(s.pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(s.pages))
	}
}

// TestConfigureWithPages verifies pages are parsed correctly.
func TestConfigureWithPages(t *testing.T) {
	s := &Skill{}
	raw := map[string]any{
		"timeout": "2s",
		"pages": []any{
			map[string]any{
				"path":        "/index",
				"contains":    []any{"<title>Home</title>", "<html"},
				"max_latency": "500ms",
			},
		},
	}
	if err := s.Configure(skill.SkillConfig{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(s.pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(s.pages))
	}
	p := s.pages[0]
	if p.Path != "/index" {
		t.Errorf("path = %q, want /index", p.Path)
	}
	if len(p.Contains) != 2 || p.Contains[0] != "<title>Home</title>" || p.Contains[1] != "<html" {
		t.Errorf("contains = %v, want [<title>Home</title> <html]", p.Contains)
	}
	if p.MaxLatency != "500ms" {
		t.Errorf("max_latency = %q, want 500ms", p.MaxLatency)
	}
}

// TestConfigureSkipsEmptyPath verifies pages with empty path are skipped.
func TestConfigureSkipsEmptyPath(t *testing.T) {
	s := &Skill{}
	raw := map[string]any{
		"pages": []any{
			map[string]any{"path": ""}, // skipped
			map[string]any{"contains": []any{"x"}},
			map[string]any{"path": "/ok"},
		},
	}
	if err := s.Configure(skill.SkillConfig{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(s.pages) != 1 {
		t.Fatalf("expected 1 valid page, got %d", len(s.pages))
	}
	if s.pages[0].Path != "/ok" {
		t.Errorf("page path = %q, want /ok", s.pages[0].Path)
	}
}

// TestRunSuccess verifies a passing page records Passed=1.
func TestRunSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Home</title></head><body>hello</body></html>`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"pages": []any{
			map[string]any{
				"path":     "/",
				"contains": []any{"Home"},
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Error != nil {
		t.Fatalf("Run error: %v", res.Error)
	}
	if res.Summary.Total != 1 || res.Summary.Passed != 1 {
		t.Errorf("summary = %+v, want Total=1 Passed=1", res.Summary)
	}
}

// TestRunStatusMismatch verifies a non-200 status fails the case.
func TestRunStatusMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"pages": []any{
			map[string]any{"path": "/missing"},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunContainsMismatch verifies a missing substring fails the case.
func TestRunContainsMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>nothing here</body></html>`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"pages": []any{
			map[string]any{
				"path":     "/",
				"contains": []any{"WelcomeDashboard"},
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunLatencyExceed verifies that exceeding max_latency fails the case.
func TestRunLatencyExceed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html></html>`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"pages": []any{
			map[string]any{
				"path":        "/",
				"max_latency": "1ns",
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunConnectionError verifies that an unreachable BaseURL fails the case.
func TestRunConnectionError(t *testing.T) {
	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"timeout": "200ms",
		"pages": []any{
			map[string]any{"path": "/"},
		},
	})

	ctx := newTestContext(t, "http://127.0.0.1:1")
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunMultiplePages verifies a mixed page set produces the expected summary.
func TestRunMultiplePages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"pages": []any{
			map[string]any{"path": "/ok"},
			map[string]any{"path": "/broken"},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Total != 2 || res.Summary.Passed != 1 || res.Summary.Failed != 1 {
		t.Errorf("summary = %+v, want Total=2 Passed=1 Failed=1", res.Summary)
	}
}

// TestRunNoContainsCheck verifies that pages without "contains" only check the status code.
func TestRunNoContainsCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return arbitrary content; without "contains" the body is not inspected.
		_, _ = w.Write([]byte(`anything`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"pages": []any{
			map[string]any{"path": "/"},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1; summary=%+v", res.Summary.Passed, res.Summary)
	}
}
