package regression

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/skill"
)

// newTestContext builds a SkillContext backed by a fresh reporter and assertion engine.
func newTestContext(t *testing.T, baseURL string) *skill.Context {
	t.Helper()
	return &skill.Context{
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
	if err := s.Configure(skill.Config{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := s.Setup(nil); err != nil {
		t.Fatalf("Setup: %v", err)
	}
}

// TestName verifies the skill identifier.
func TestName(t *testing.T) {
	s := &Skill{}
	if got := s.Name(); got != "regression" {
		t.Errorf("Name() = %q, want \"regression\"", got)
	}
}

// TestKind verifies the skill category.
func TestKind(t *testing.T) {
	s := &Skill{}
	if got := s.Kind(); got != skill.KindRegression {
		t.Errorf("Kind() = %q, want %q", got, skill.KindRegression)
	}
}

// TestPriority verifies the configured priority.
func TestPriority(t *testing.T) {
	s := &Skill{}
	if got := s.Priority(); got != skill.PriorityLow {
		t.Errorf("Priority() = %v, want %v", got, skill.PriorityLow)
	}
}

// TestConfigureEmpty ensures Configure tolerates an empty raw map.
func TestConfigureEmpty(t *testing.T) {
	s := &Skill{}
	if err := s.Configure(skill.Config{Raw: map[string]any{}}); err != nil {
		t.Fatalf("Configure empty raw: %v", err)
	}
	if len(s.cases) != 0 {
		t.Errorf("expected 0 cases, got %d", len(s.cases))
	}
}

// TestConfigureWithCases verifies cases are parsed correctly.
func TestConfigureWithCases(t *testing.T) {
	s := &Skill{}
	raw := map[string]any{
		"timeout": "4s",
		"cases": []any{
			map[string]any{
				"name":            "health",
				"path":            "/health",
				"expected_status": 200,
				"expected_code":   1,
				"skip_on_404":     true,
			},
		},
	}
	if err := s.Configure(skill.Config{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(s.cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(s.cases))
	}
	c := s.cases[0]
	if c.Name != "health" {
		t.Errorf("name = %q, want health", c.Name)
	}
	if c.Path != "/health" {
		t.Errorf("path = %q, want /health", c.Path)
	}
	if c.ExpectedStatus != 200 {
		t.Errorf("expected_status = %d, want 200", c.ExpectedStatus)
	}
	if c.ExpectedCode != 1 {
		t.Errorf("expected_code = %d, want 1", c.ExpectedCode)
	}
	if !c.SkipOn404 {
		t.Errorf("skip_on_404 = false, want true")
	}
}

// TestConfigureSkipsInvalid verifies cases with empty name or path are skipped.
func TestConfigureSkipsInvalid(t *testing.T) {
	s := &Skill{}
	raw := map[string]any{
		"cases": []any{
			map[string]any{"name": "", "path": "/a"},
			map[string]any{"name": "no-path", "path": ""},
			map[string]any{"name": "ok", "path": "/ok"},
		},
	}
	if err := s.Configure(skill.Config{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(s.cases) != 1 {
		t.Fatalf("expected 1 valid case, got %d", len(s.cases))
	}
	if s.cases[0].Name != "ok" || s.cases[0].Path != "/ok" {
		t.Errorf("unexpected case: %+v", s.cases[0])
	}
}

// TestRunSuccess verifies a passing case records Passed=1.
func TestRunSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","request_id":"r1"}`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":          "health",
				"path":          "/health",
				"expected_code": 0,
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

// TestRunStatusMismatch verifies a status code mismatch fails the case.
func TestRunStatusMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":            "health",
				"path":            "/health",
				"expected_status": 200,
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunCodeMismatch verifies a code mismatch fails the case when ExpectedCode != 0.
// Note: the regression skill only enforces the envelope code check when ExpectedCode is non-zero;
// we therefore use expected_code=2 against a server returning code=1 to exercise the mismatch path.
func TestRunCodeMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"ok","request_id":"r2"}`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":          "health",
				"path":          "/health",
				"expected_code": 2,
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunCodeMatch verifies a code match passes the case.
func TestRunCodeMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"ok","request_id":"r3"}`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":          "health",
				"path":          "/health",
				"expected_code": 1,
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1; summary=%+v", res.Summary.Passed, res.Summary)
	}
}

// TestRunSkipOn404 verifies that SkipOn404 records the case as passed with a "skipped" message.
func TestRunSkipOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":        "health",
				"path":        "/health",
				"skip_on_404": true,
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1; summary=%+v", res.Summary.Passed, res.Summary)
	}
	results := ctx.Reporter.Snapshot()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Message == "" {
		t.Errorf("expected non-empty message")
	}
	// The skill's skip message must contain "skipped".
	if !contains(results[0].Message, "skipped") {
		t.Errorf("message = %q, want substring \"skipped\"", results[0].Message)
	}
}

// contains is a small helper to avoid importing strings just for substring checks.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestRunConnectionError verifies an unreachable BaseURL fails the case.
func TestRunConnectionError(t *testing.T) {
	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"timeout": "200ms",
		"cases": []any{
			map[string]any{"name": "health", "path": "/health"},
		},
	})

	ctx := newTestContext(t, "http://127.0.0.1:1")
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunMultipleCases verifies a mixed case set produces the expected summary.
func TestRunMultipleCases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","request_id":"r"}`))
		case "/also-ok":
			w.WriteHeader(http.StatusOK)
		case "/fail":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{"name": "ok", "path": "/ok"},
			map[string]any{"name": "also-ok", "path": "/also-ok"},
			map[string]any{"name": "fail", "path": "/fail"},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Total != 3 || res.Summary.Passed != 2 || res.Summary.Failed != 1 {
		t.Errorf("summary = %+v, want Total=3 Passed=2 Failed=1", res.Summary)
	}
}
