package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	if got := s.Name(); got != "api" {
		t.Errorf("Name() = %q, want \"api\"", got)
	}
}

// TestKind verifies the skill category.
func TestKind(t *testing.T) {
	s := &Skill{}
	if got := s.Kind(); got != skill.KindAPI {
		t.Errorf("Kind() = %q, want %q", got, skill.KindAPI)
	}
}

// TestPriority verifies the configured priority.
func TestPriority(t *testing.T) {
	s := &Skill{}
	if got := s.Priority(); got != skill.PriorityCritical {
		t.Errorf("Priority() = %v, want %v", got, skill.PriorityCritical)
	}
}

// TestConfigureEmpty ensures Configure tolerates an empty raw map and produces no cases.
func TestConfigureEmpty(t *testing.T) {
	s := &Skill{}
	if err := s.Configure(skill.SkillConfig{Raw: map[string]any{}}); err != nil {
		t.Fatalf("Configure empty raw: %v", err)
	}
	if len(s.cases) != 0 {
		t.Errorf("expected 0 cases, got %d", len(s.cases))
	}
}

// TestConfigureNil ensures Configure tolerates a nil raw map.
func TestConfigureNil(t *testing.T) {
	s := &Skill{}
	if err := s.Configure(skill.SkillConfig{}); err != nil {
		t.Fatalf("Configure nil raw: %v", err)
	}
	if len(s.cases) != 0 {
		t.Errorf("expected 0 cases, got %d", len(s.cases))
	}
}

// TestConfigureWithCases verifies all apiCase fields are parsed correctly.
func TestConfigureWithCases(t *testing.T) {
	s := &Skill{}
	raw := map[string]any{
		"timeout": "3s",
		"cases": []any{
			map[string]any{
				"name":            "health",
				"method":          "POST",
				"path":            "/api/v1/health",
				"headers":         map[string]any{"X-Tickraft-Test": "true", "Trace": "abc"},
				"expected_status": 201,
				"expected_code":   7,
				"expected_field":  "data.id",
				"expected_value":  42,
				"max_latency":     "200ms",
			},
		},
	}
	if err := s.Configure(skill.SkillConfig{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if s.timeout != 3*time.Second {
		t.Errorf("timeout = %v, want 3s", s.timeout)
	}
	if len(s.cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(s.cases))
	}
	c := s.cases[0]
	if c.Name != "health" {
		t.Errorf("name = %q, want health", c.Name)
	}
	if c.Method != "POST" {
		t.Errorf("method = %q, want POST", c.Method)
	}
	if c.Path != "/api/v1/health" {
		t.Errorf("path = %q, want /api/v1/health", c.Path)
	}
	if c.ExpectedStatus != 201 {
		t.Errorf("expected_status = %d, want 201", c.ExpectedStatus)
	}
	if c.ExpectedCode != 7 {
		t.Errorf("expected_code = %d, want 7", c.ExpectedCode)
	}
	if c.ExpectedField != "data.id" {
		t.Errorf("expected_field = %q, want data.id", c.ExpectedField)
	}
	if c.ExpectedValue != 42 {
		t.Errorf("expected_value = %v, want 42", c.ExpectedValue)
	}
	if c.MaxLatency != "200ms" {
		t.Errorf("max_latency = %q, want 200ms", c.MaxLatency)
	}
	if got := c.Headers["X-Tickraft-Test"]; got != "true" {
		t.Errorf("headers[X-Tickraft-Test] = %q, want true", got)
	}
	if got := c.Headers["Trace"]; got != "abc" {
		t.Errorf("headers[Trace] = %q, want abc", got)
	}
}

// TestConfigureSkipsInvalidCases verifies cases with empty name or path are skipped.
func TestConfigureSkipsInvalidCases(t *testing.T) {
	s := &Skill{}
	raw := map[string]any{
		"cases": []any{
			map[string]any{"name": "", "path": "/a"},
			map[string]any{"name": "no-path", "path": ""},
			map[string]any{"name": "ok", "path": "/ok"},
			map[string]any{"path": "/no-name"}, // name missing defaults to ""
		},
	}
	if err := s.Configure(skill.SkillConfig{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(s.cases) != 1 {
		t.Fatalf("expected 1 valid case, got %d", len(s.cases))
	}
	if s.cases[0].Name != "ok" || s.cases[0].Path != "/ok" {
		t.Errorf("unexpected case: %+v", s.cases[0])
	}
}

// TestSetup verifies Setup creates an HTTP client without error.
func TestSetup(t *testing.T) {
	s := &Skill{}
	if err := s.Configure(skill.SkillConfig{Raw: map[string]any{}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := s.Setup(nil); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if s.client == nil {
		t.Errorf("client is nil after Setup")
	}
}

// TestTeardown verifies Teardown returns nil.
func TestTeardown(t *testing.T) {
	s := &Skill{}
	if err := s.Teardown(nil); err != nil {
		t.Errorf("Teardown returned non-nil error: %v", err)
	}
}

// TestRunSuccess verifies a fully-passing case records Passed=1.
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
				"name":          "ok",
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
	if res.Summary.Total != 1 || res.Summary.Passed != 1 || res.Summary.Failed != 0 {
		t.Errorf("summary = %+v, want Total=1 Passed=1 Failed=0", res.Summary)
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
				"name":            "boom",
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

// TestRunCodeMismatch verifies an envelope code mismatch fails the case.
func TestRunCodeMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"bad","request_id":"r2"}`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":          "code-mismatch",
				"path":          "/health",
				"expected_code": 0,
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunFieldMismatch verifies a JSON path field mismatch fails the case.
func TestRunFieldMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":41}}`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":           "field-mismatch",
				"path":           "/health",
				"expected_field": "data.id",
				"expected_value": 42,
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunFieldMatch verifies a JSON path field match passes the case.
func TestRunFieldMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":42}}`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":           "field-match",
				"path":           "/health",
				"expected_field": "data.id",
				"expected_value": 42,
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1; summary=%+v", res.Summary.Passed, res.Summary)
	}
}

// TestRunLatencyExceed verifies that exceeding max_latency fails the case.
func TestRunLatencyExceed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":        "slow",
				"path":        "/health",
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

// TestRunLatencyOK verifies that a generous max_latency passes the case.
func TestRunLatencyOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":        "fast",
				"path":        "/health",
				"max_latency": "5s",
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1; summary=%+v", res.Summary.Passed, res.Summary)
	}
}

// TestRunConnectionError verifies that an unreachable BaseURL fails the case.
func TestRunConnectionError(t *testing.T) {
	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"timeout": "200ms",
		"cases": []any{
			map[string]any{
				"name": "unreachable",
				"path": "/health",
			},
		},
	})

	// Port 1 is reserved and should refuse TCP connections.
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
			_, _ = w.Write([]byte(`ok`))
		case "/also-ok":
			_, _ = w.Write([]byte(`ok`))
		case "/fail":
			w.WriteHeader(http.StatusBadRequest)
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

// TestRunEnvelopeDetection verifies that when the response body carries a "code" field,
// the envelope check is performed even when ExpectedCode is the default 0.
func TestRunEnvelopeDetection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Envelope with code != 0; ExpectedCode defaults to 0, so the case should fail.
		_, _ = w.Write([]byte(`{"code":5,"msg":"bad","request_id":"r3"}`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":          "envelope-detected",
				"path":          "/health",
				"expected_code": 0,
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1 (envelope should be detected and code checked); summary=%+v",
			res.Summary.Failed, res.Summary)
	}
}

// TestRunNoEnvelopeNoCodeCheck verifies that when the body has no envelope,
// the case passes despite ExpectedCode being the default 0.
func TestRunNoEnvelopeNoCodeCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(`plain text body`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"cases": []any{
			map[string]any{
				"name":          "no-envelope",
				"path":          "/health",
				"expected_code": 0,
			},
		},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1 (no envelope should mean no code check); summary=%+v",
			res.Summary.Passed, res.Summary)
	}
}
