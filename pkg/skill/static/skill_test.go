package static

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
	if got := s.Name(); got != "static" {
		t.Errorf("Name() = %q, want \"static\"", got)
	}
}

// TestKind verifies the skill category.
func TestKind(t *testing.T) {
	s := &Skill{}
	if got := s.Kind(); got != skill.KindStatic {
		t.Errorf("Kind() = %q, want %q", got, skill.KindStatic)
	}
}

// TestPriority verifies the configured priority.
func TestPriority(t *testing.T) {
	s := &Skill{}
	if got := s.Priority(); got != skill.PriorityNormal {
		t.Errorf("Priority() = %v, want %v", got, skill.PriorityNormal)
	}
}

// TestConfigureEmpty ensures Configure tolerates an empty raw map.
func TestConfigureEmpty(t *testing.T) {
	s := &Skill{}
	if err := s.Configure(skill.Config{Raw: map[string]any{}}); err != nil {
		t.Fatalf("Configure empty raw: %v", err)
	}
	if len(s.pages) != 0 || len(s.assets) != 0 {
		t.Errorf("expected empty pages and assets, got pages=%d assets=%d", len(s.pages), len(s.assets))
	}
}

// TestConfigureWithPagesAndAssets verifies pages and assets are parsed correctly.
func TestConfigureWithPagesAndAssets(t *testing.T) {
	s := &Skill{}
	raw := map[string]any{
		"timeout": "4s",
		"pages":   []any{"/index.html", "/about.html"},
		"assets":  []any{"/static/app.js", "/static/style.css"},
	}
	if err := s.Configure(skill.Config{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(s.pages) != 2 {
		t.Errorf("pages = %v, want 2 entries", s.pages)
	}
	if len(s.assets) != 2 {
		t.Errorf("assets = %v, want 2 entries", s.assets)
	}
	if s.pages[0] != "/index.html" || s.pages[1] != "/about.html" {
		t.Errorf("pages = %v", s.pages)
	}
	if s.assets[0] != "/static/app.js" || s.assets[1] != "/static/style.css" {
		t.Errorf("assets = %v", s.assets)
	}
}

// TestRunPageSuccess verifies a page returning 200 with an <html marker passes.
func TestRunPageSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>home</body></html>`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"pages": []any{"/index.html"},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Error != nil {
		t.Fatalf("Run error: %v", res.Error)
	}
	if res.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1; summary=%+v", res.Summary.Passed, res.Summary)
	}
}

// TestRunPage404Skipped verifies that a 404 page is recorded as Skipped.
func TestRunPage404Skipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"pages": []any{"/missing.html"},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1; summary=%+v", res.Summary.Skipped, res.Summary)
	}
	if res.Summary.Failed != 0 {
		t.Errorf("Failed = %d, want 0 (404 should not be a failure)", res.Summary.Failed)
	}
}

// TestRunPage500 verifies a 500 response fails the page case.
func TestRunPage500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"pages": []any{"/broken"},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunPageNoHtmlMarker verifies a 200 body without "<html" fails the page case.
func TestRunPageNoHtmlMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`plain text without html marker`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"pages": []any{"/index"},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunAsset200 verifies a 200 asset passes.
func TestRunAsset200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`console.log("hi")`))
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"assets": []any{"/app.js"},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1; summary=%+v", res.Summary.Passed, res.Summary)
	}
}

// TestRunAsset404 verifies a 404 asset still counts as Passed (assets not built is acceptable).
func TestRunAsset404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"assets": []any{"/missing.js"},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1 (404 should be a pass for assets); summary=%+v",
			res.Summary.Passed, res.Summary)
	}
}

// TestRunAsset500 verifies a 500 asset fails.
func TestRunAsset500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"assets": []any{"/broken.js"},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunConnectionError verifies an unreachable BaseURL fails the case.
func TestRunConnectionError(t *testing.T) {
	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"timeout": "200ms",
		"pages":   []any{"/index"},
	})

	ctx := newTestContext(t, "http://127.0.0.1:1")
	res := s.Run(ctx)
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
}

// TestRunMixed verifies a combined page + asset run produces the expected summary.
func TestRunMixed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index":
			_, _ = w.Write([]byte(`<html><body>home</body></html>`))
		case "/good.js":
			_, _ = w.Write([]byte(`ok`))
		case "/bad.js":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	s := &Skill{}
	configureAndSetup(t, s, map[string]any{
		"pages":  []any{"/index"},
		"assets": []any{"/good.js", "/bad.js"},
	})

	ctx := newTestContext(t, srv.URL)
	res := s.Run(ctx)
	if res.Summary.Total != 3 {
		t.Errorf("Total = %d, want 3; summary=%+v", res.Summary.Total, res.Summary)
	}
	if res.Summary.Passed != 2 {
		t.Errorf("Passed = %d, want 2 (page + good asset); summary=%+v", res.Summary.Passed, res.Summary)
	}
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1 (bad asset); summary=%+v", res.Summary.Failed, res.Summary)
	}
}
