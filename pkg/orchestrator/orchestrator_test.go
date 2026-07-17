package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/agent"
	"github.com/tickraft/taichi/pkg/autofix"
	"github.com/tickraft/taichi/pkg/config"
	"github.com/tickraft/taichi/pkg/failure"
	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/report"
	"github.com/tickraft/taichi/pkg/skill"
)

// ---------------------------------------------------------------------------
// Mock types
// ---------------------------------------------------------------------------

// mockSkill is a configurable TestSkill stub used in orchestrator tests.
// Configure/Setup/Teardown return their configured errors; Run delegates to runFn.
type mockSkill struct {
	name          string
	kind          skill.Kind
	priority      skill.Priority
	configErr     error
	setupErr      error
	teardownErr   error
	runFn         func(ctx *skill.Context) skill.Result
	setupCount    int
	runCount      int
	teardownCount int
	configCount   int
	mu            sync.Mutex
	lastCtx       *skill.Context
}

func (m *mockSkill) Name() string             { return m.name }
func (m *mockSkill) Kind() skill.Kind         { return m.kind }
func (m *mockSkill) Priority() skill.Priority { return m.priority }

func (m *mockSkill) Configure(skill.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configCount++
	return m.configErr
}

func (m *mockSkill) Setup(ctx *skill.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setupCount++
	m.lastCtx = ctx
	return m.setupErr
}

func (m *mockSkill) Run(ctx *skill.Context) skill.Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runCount++
	m.lastCtx = ctx
	if m.runFn != nil {
		return m.runFn(ctx)
	}
	return skill.Result{SkillName: m.name}
}

func (m *mockSkill) Teardown(*skill.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.teardownCount++
	return m.teardownErr
}

// mockInvoker is a test double for agent.Invoker that returns pre-configured results.
type mockInvoker struct {
	name       string
	results    []*agent.FixResult
	errs       []error
	sideEffect func()
	calls      int
	mu         sync.Mutex
	captured   []*failure.Context
}

func (m *mockInvoker) AnalyzeAndFix(ctx context.Context, fc *failure.Context) (*agent.FixResult, error) {
	m.mu.Lock()
	idx := m.calls
	m.calls++
	m.captured = append(m.captured, fc)
	m.mu.Unlock()

	if m.sideEffect != nil {
		m.sideEffect()
	}

	var err error
	if idx < len(m.errs) {
		err = m.errs[idx]
	}
	var result *agent.FixResult
	if idx < len(m.results) {
		result = m.results[idx]
	}
	return result, err
}

func (m *mockInvoker) Name() string { return m.name }

func (m *mockInvoker) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// captureLogger records log messages for assertion in tests.
type captureLogger struct {
	mu   sync.Mutex
	msgs []string
}

func (l *captureLogger) Infof(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, "INFO: "+fmt.Sprintf(format, args...))
}

func (l *captureLogger) Warnf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, "WARN: "+fmt.Sprintf(format, args...))
}

func (l *captureLogger) Errorf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, "ERROR: "+fmt.Sprintf(format, args...))
}

func (l *captureLogger) messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.msgs))
	copy(out, l.msgs)
	return out
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// writeYAMLConfig writes a YAML config file to a temp dir and returns its path.
func writeYAMLConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "taichi.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// newMockSkill creates a mockSkill with sensible defaults.
func newMockSkill(name string) *mockSkill {
	return &mockSkill{
		name:     name,
		kind:     skill.KindAPI,
		priority: skill.PriorityNormal,
	}
}

// ---------------------------------------------------------------------------
// Orchestrator construction tests
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	o := New()
	if o == nil {
		t.Fatal("New() returned nil")
	}
	if o.registry == nil {
		t.Fatal("registry is nil")
	}
	if got := o.Registry().Count(); got != 0 {
		t.Fatalf("registry Count() = %d, want 0", got)
	}
}

func TestRegistryReturnsSameInstance(t *testing.T) {
	o := New()
	r1 := o.Registry()
	r2 := o.Registry()
	if r1 != r2 {
		t.Fatal("Registry() returned different instances on consecutive calls")
	}
}

func TestRegisterBuiltinSkills(t *testing.T) {
	o := New()
	skills := []skill.TestSkill{
		newMockSkill("api"),
		newMockSkill("ui"),
	}
	if err := o.RegisterBuiltinSkills(skills); err != nil {
		t.Fatalf("RegisterBuiltinSkills returned error: %v", err)
	}
	if got := o.Registry().Count(); got != 2 {
		t.Fatalf("Count() = %d, want 2", got)
	}
}

func TestRegisterBuiltinSkills_EmptySlice(t *testing.T) {
	o := New()
	if err := o.RegisterBuiltinSkills(nil); err != nil {
		t.Fatalf("RegisterBuiltinSkills(nil) returned error: %v", err)
	}
	if got := o.Registry().Count(); got != 0 {
		t.Fatalf("Count() = %d, want 0", got)
	}
	// Empty (non-nil) slice should also succeed.
	if err := o.RegisterBuiltinSkills([]skill.TestSkill{}); err != nil {
		t.Fatalf("RegisterBuiltinSkills(empty) returned error: %v", err)
	}
}

func TestRegisterBuiltinSkills_DuplicateName(t *testing.T) {
	// RegisterBuiltinSkills registers with overwrite=true, so duplicate names
	// are silently replaced rather than rejected. This test verifies that the
	// latest registration wins and no error is returned.
	o := New()
	first := newMockSkill("api")
	first.priority = skill.PriorityLow
	second := newMockSkill("api")
	second.priority = skill.PriorityHigh
	if err := o.RegisterBuiltinSkills([]skill.TestSkill{first, second}); err != nil {
		t.Fatalf("RegisterBuiltinSkills returned error: %v", err)
	}
	if got := o.Registry().Count(); got != 1 {
		t.Fatalf("Count() = %d, want 1 (duplicate replaced)", got)
	}
	got, err := o.Registry().Get("api")
	if err != nil {
		t.Fatalf("Get(api) error: %v", err)
	}
	if got.Priority() != skill.PriorityHigh {
		t.Fatalf("Priority() = %v, want PriorityHigh (latest registration wins)", got.Priority())
	}
}

// ---------------------------------------------------------------------------
// selectProject tests
// ---------------------------------------------------------------------------

func TestSelectProject(t *testing.T) {
	projects := []config.Project{
		{Name: "alpha"},
		{Name: "beta"},
	}
	cases := []struct {
		name      string
		cfg       *config.Config
		selectArg string
		wantName  string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "empty name returns first project",
			cfg:       &config.Config{Projects: projects},
			selectArg: "",
			wantName:  "alpha",
		},
		{
			name:      "by name returns matching project",
			cfg:       &config.Config{Projects: projects},
			selectArg: "beta",
			wantName:  "beta",
		},
		{
			name:      "unknown name returns error",
			cfg:       &config.Config{Projects: projects},
			selectArg: "gamma",
			wantErr:   true,
			errSubstr: "not found",
		},
		{
			name:      "empty name with no projects returns error",
			cfg:       &config.Config{Projects: nil},
			selectArg: "",
			wantErr:   true,
			errSubstr: "no projects defined",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := selectProject(c.cfg, c.selectArg)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if c.errSubstr != "" && !strings.Contains(err.Error(), c.errSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), c.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.Name != c.wantName {
				t.Fatalf("got project %q, want %q", p.Name, c.wantName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildSkillConfigs tests
// ---------------------------------------------------------------------------

func TestBuildSkillConfigs(t *testing.T) {
	allSkills := []skill.Config{
		{Name: "api", Enabled: true},
		{Name: "ui", Enabled: true},
		{Name: "static", Enabled: true},
		{Name: "disabled-skill", Enabled: false},
	}

	t.Run("no project skills no filter returns all", func(t *testing.T) {
		cfg := &config.Config{Skills: allSkills}
		project := config.Project{Name: "p"}
		got := buildSkillConfigs(cfg, project, nil)
		if len(got) != 4 {
			t.Fatalf("got %d configs, want 4", len(got))
		}
	})

	t.Run("project skills filter restricts", func(t *testing.T) {
		cfg := &config.Config{Skills: allSkills}
		project := config.Project{Name: "p", Skills: []string{"api", "ui"}}
		got := buildSkillConfigs(cfg, project, nil)
		if len(got) != 2 {
			t.Fatalf("got %d configs, want 2", len(got))
		}
		names := []string{got[0].Name, got[1].Name}
		// Order follows the config order (api before ui).
		if names[0] != "api" || names[1] != "ui" {
			t.Fatalf("got %v, want [api ui]", names)
		}
	})

	t.Run("filter overrides project skills", func(t *testing.T) {
		cfg := &config.Config{Skills: allSkills}
		project := config.Project{Name: "p", Skills: []string{"api", "ui"}}
		got := buildSkillConfigs(cfg, project, []string{"ui"})
		if len(got) != 1 {
			t.Fatalf("got %d configs, want 1", len(got))
		}
		if got[0].Name != "ui" {
			t.Fatalf("got %q, want ui", got[0].Name)
		}
	})

	t.Run("filter with unknown name returns empty", func(t *testing.T) {
		cfg := &config.Config{Skills: allSkills}
		project := config.Project{Name: "p"}
		got := buildSkillConfigs(cfg, project, []string{"unknown"})
		if len(got) != 0 {
			t.Fatalf("got %d configs, want 0", len(got))
		}
	})

	t.Run("project skills with unknown name is skipped", func(t *testing.T) {
		cfg := &config.Config{Skills: allSkills}
		project := config.Project{Name: "p", Skills: []string{"api", "unknown"}}
		got := buildSkillConfigs(cfg, project, nil)
		if len(got) != 1 {
			t.Fatalf("got %d configs, want 1", len(got))
		}
		if got[0].Name != "api" {
			t.Fatalf("got %q, want api", got[0].Name)
		}
	})

	t.Run("empty config returns empty", func(t *testing.T) {
		cfg := &config.Config{}
		project := config.Project{Name: "p"}
		got := buildSkillConfigs(cfg, project, nil)
		if len(got) != 0 {
			t.Fatalf("got %d configs, want 0", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// findSkillConfigs tests
// ---------------------------------------------------------------------------

func TestFindSkillConfigs(t *testing.T) {
	cfgs := []skill.Config{
		{Name: "api", Enabled: true},
		{Name: "ui", Enabled: false},
	}

	t.Run("found", func(t *testing.T) {
		c, ok := findSkillConfigs(cfgs, "api")
		if !ok {
			t.Fatal("expected to find api")
		}
		if c.Name != "api" {
			t.Fatalf("got %q, want api", c.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := findSkillConfigs(cfgs, "missing")
		if ok {
			t.Fatal("expected not found")
		}
	})

	t.Run("empty configs returns not found", func(t *testing.T) {
		_, ok := findSkillConfigs(nil, "api")
		if ok {
			t.Fatal("expected not found in nil configs")
		}
	})
}

// ---------------------------------------------------------------------------
// parseFormats tests
// ---------------------------------------------------------------------------

func TestParseFormats(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		if got := parseFormats(nil); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		if got := parseFormats([]string{}); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("converts to Format slice", func(t *testing.T) {
		got := parseFormats([]string{"json", "junit", "summary"})
		if len(got) != 3 {
			t.Fatalf("got %d formats, want 3", len(got))
		}
		if got[0] != report.FormatJSON {
			t.Fatalf("got[0] = %q, want %q", got[0], report.FormatJSON)
		}
		if got[1] != report.FormatJUnit {
			t.Fatalf("got[1] = %q, want %q", got[1], report.FormatJUnit)
		}
		if got[2] != report.FormatSummary {
			t.Fatalf("got[2] = %q, want %q", got[2], report.FormatSummary)
		}
	})
}

// ---------------------------------------------------------------------------
// mapHint tests
// ---------------------------------------------------------------------------

func TestMapHint(t *testing.T) {
	cases := []struct {
		hint skill.ErrorTypeHint
		want autofix.ErrorType
	}{
		{skill.ErrorHintServiceDown, autofix.ErrorTypeServiceDown},
		{skill.ErrorHintRateLimited, autofix.ErrorTypeRateLimited},
		{skill.ErrorHintServerError, autofix.ErrorTypeServerError},
		{skill.ErrorHintUnknown, autofix.ErrorTypeUnknown},
		{skill.ErrorHintNone, autofix.ErrorTypeNone},
		{skill.ErrorTypeHint("invalid"), autofix.ErrorTypeNone},
	}
	for _, c := range cases {
		got := mapHint(c.hint)
		if got != c.want {
			t.Errorf("mapHint(%q) = %v, want %v", c.hint, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// adaptFixEngine and fixEngineAdapter tests
// ---------------------------------------------------------------------------

func TestAdaptFixEngine_Nil(t *testing.T) {
	if got := adaptFixEngine(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
}

func TestAdaptFixEngine_NotNil(t *testing.T) {
	engine := autofix.NewFixEngine(nil, t.TempDir())
	adapter := adaptFixEngine(engine)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
}

func TestFixEngineAdapter_Apply(t *testing.T) {
	engine := autofix.NewFixEngine(nil, t.TempDir())
	adapter := adaptFixEngine(engine)

	t.Run("ServiceDown with nil payload returns no match", func(t *testing.T) {
		// When payload is nil, ctx is nil, engine returns "no matching fix rule".
		outcome := adapter.Apply(skill.ErrorHintServiceDown, nil)
		if outcome.Fixed {
			t.Errorf("Fixed = true, want false")
		}
		if !strings.Contains(outcome.Message, "no matching fix rule") {
			t.Errorf("Message = %q, want it to contain 'no matching fix rule'", outcome.Message)
		}
	})

	t.Run("ServiceDown with real FixContext returns not fixed", func(t *testing.T) {
		// ctx.Lifecycle is nil (engine has nil lifecycle), so ServiceRestartRule
		// returns Fixed=false with a descriptive message.
		ctx := &autofix.FixContext{}
		outcome := adapter.Apply(skill.ErrorHintServiceDown, ctx)
		if outcome.Fixed {
			t.Errorf("Fixed = true, want false")
		}
		if !strings.Contains(outcome.Message, "service restart failed") {
			t.Errorf("Message = %q, want it to contain 'service restart failed'", outcome.Message)
		}
	})

	t.Run("ServerError writes report", func(t *testing.T) {
		ctx := &autofix.FixContext{}
		outcome := adapter.Apply(skill.ErrorHintServerError, ctx)
		if outcome.Fixed {
			t.Errorf("Fixed = true, want false")
		}
	})

	t.Run("Unknown writes report", func(t *testing.T) {
		ctx := &autofix.FixContext{}
		outcome := adapter.Apply(skill.ErrorHintUnknown, ctx)
		if outcome.Fixed {
			t.Errorf("Fixed = true, want false")
		}
	})

	t.Run("None no matching rule", func(t *testing.T) {
		ctx := &autofix.FixContext{}
		outcome := adapter.Apply(skill.ErrorHintNone, ctx)
		if outcome.Fixed {
			t.Errorf("Fixed = true, want false")
		}
		if !strings.Contains(outcome.Message, "no matching fix rule") {
			t.Errorf("Message = %q, want it to contain 'no matching fix rule'", outcome.Message)
		}
	})

	t.Run("payload not a FixContext is treated as nil", func(t *testing.T) {
		// A non-FixContext payload should not panic; ctx becomes nil.
		outcome := adapter.Apply(skill.ErrorHintServiceDown, "not a fix context")
		if outcome.Fixed {
			t.Errorf("Fixed = true, want false")
		}
	})
}

// ---------------------------------------------------------------------------
// Plugin skill registration tests
// ---------------------------------------------------------------------------

func TestRegisterPluginSkills(t *testing.T) {
	o := New()
	cfgs := []skill.Config{
		{Name: "plugin-a", Kind: skill.KindPlugin, Enabled: true, Raw: map[string]any{"command": "echo"}},
		{Name: "plugin-b", Kind: skill.KindAPI, Enabled: true},
		{Name: "plugin-c", Kind: skill.KindPlugin, Enabled: false},
	}
	if err := o.registerPluginSkills(cfgs, skill.NoOpLogger{}); err != nil {
		t.Fatalf("registerPluginSkills: %v", err)
	}

	// plugin-a should be registered (kind=plugin, enabled).
	if _, err := o.Registry().Get("plugin-a"); err != nil {
		t.Errorf("plugin-a not registered: %v", err)
	}
	// plugin-b should NOT be registered (kind=api, not plugin).
	if _, err := o.Registry().Get("plugin-b"); err == nil {
		t.Errorf("plugin-b should not be registered (not kind=plugin)")
	}
	// plugin-c should NOT be registered (enabled=false).
	if _, err := o.Registry().Get("plugin-c"); err == nil {
		t.Errorf("plugin-c should not be registered (disabled)")
	}
}

func TestRegisterPluginSkills_AlreadyRegistered(t *testing.T) {
	o := New()
	// Pre-register a skill with the same name.
	pre := newMockSkill("my-plugin")
	if err := o.Registry().Register(pre, false); err != nil {
		t.Fatalf("pre-register: %v", err)
	}
	cfgs := []skill.Config{
		{Name: "my-plugin", Kind: skill.KindPlugin, Enabled: true, Raw: map[string]any{"command": "echo"}},
	}
	if err := o.registerPluginSkills(cfgs, skill.NoOpLogger{}); err != nil {
		t.Fatalf("registerPluginSkills: %v", err)
	}
	// The pre-registered skill should remain (not overwritten by plugin.New).
	got, err := o.Registry().Get("my-plugin")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != pre {
		t.Fatal("pre-registered skill was overwritten by plugin.New")
	}
}

func TestRegisterPluginSkills_EmptyConfigs(t *testing.T) {
	o := New()
	if err := o.registerPluginSkills(nil, skill.NoOpLogger{}); err != nil {
		t.Fatalf("registerPluginSkills with nil: %v", err)
	}
	if got := o.Registry().Count(); got != 0 {
		t.Fatalf("Count() = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Run() error path tests
// ---------------------------------------------------------------------------

func TestRun_LoadConfigError(t *testing.T) {
	o := New()
	_, err := o.Run(context.Background(), Options{
		ConfigPath: filepath.Join(t.TempDir(), "nonexistent.yaml"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Fatalf("error %q does not contain 'load config'", err.Error())
	}
}

func TestRun_NoProjects(t *testing.T) {
	o := New()
	_, err := o.Run(context.Background(), Options{ConfigPath: ""})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no projects defined") {
		t.Fatalf("error %q does not contain 'no projects defined'", err.Error())
	}
}

func TestRun_ProjectNotFound(t *testing.T) {
	configPath := writeYAMLConfig(t, `
projects:
  - name: alpha
skills:
  - name: api
    enabled: true
`)
	o := New()
	_, err := o.Run(context.Background(), Options{
		ConfigPath:  configPath,
		ProjectName: "beta",
		ReportsDir:  t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error %q does not contain 'not found'", err.Error())
	}
}

func TestRun_ProjectRootNotDir(t *testing.T) {
	// writeYAMLConfig writes to <tmpdir>/taichi.yaml. We create a sibling file
	// "not-a-dir.txt" in the same directory and reference it via a RELATIVE path.
	// ResolveProjectRootWithBase only validates IsDir for relative paths (absolute
	// paths are returned as-is), so we must use a relative path to trigger the error.
	dir := t.TempDir()
	notDir := filepath.Join(dir, "not-a-dir.txt")
	if err := os.WriteFile(notDir, []byte("content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	configPath := filepath.Join(dir, "taichi.yaml")
	cfgContent := `
projects:
  - name: test-project
    root: not-a-dir.txt
skills:
  - name: api
    enabled: true
`
	if err := os.WriteFile(configPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	o := New()
	_, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "resolve project root") {
		t.Fatalf("error %q does not contain 'resolve project root'", err.Error())
	}
}

func TestRun_NoSkillsSelected(t *testing.T) {
	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: missing-skill
    enabled: true
`)
	o := New()
	_, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no skills selected") {
		t.Fatalf("error %q does not contain 'no skills selected'", err.Error())
	}
}

func TestRun_EnvManagerCreationError(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := writeYAMLConfig(t, fmt.Sprintf(`
projects:
  - name: test-project
    root: %s
    env: bad-env
envs:
  bad-env:
    kind: invalid-kind
skills:
  - name: api
    enabled: true
`, projectRoot))

	o := New()
	o.Registry().Register(newMockSkill("api"), false)

	_, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "create env manager") {
		t.Fatalf("error %q does not contain 'create env manager'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Run() success path tests
// ---------------------------------------------------------------------------

func TestRun_Success_NoEnv(t *testing.T) {
	s := &mockSkill{
		name:     "api",
		kind:     skill.KindAPI,
		priority: skill.PriorityNormal,
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{
				Name:   "case-1",
				Passed: true,
			})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: api
    enabled: true
`)

	result, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.ProjectName != "test-project" {
		t.Errorf("ProjectName = %q, want 'test-project'", result.ProjectName)
	}
	if len(result.SkillResults) != 1 {
		t.Fatalf("SkillResults len = %d, want 1", len(result.SkillResults))
	}
	if result.Summary.Total != 1 {
		t.Errorf("Summary.Total = %d, want 1", result.Summary.Total)
	}
	if result.Summary.Passed != 1 {
		t.Errorf("Summary.Passed = %d, want 1", result.Summary.Passed)
	}
	if result.Summary.Failed != 0 {
		t.Errorf("Summary.Failed = %d, want 0", result.Summary.Failed)
	}
	if result.Duration <= 0 {
		t.Errorf("Duration = %v, want positive", result.Duration)
	}
	// Verify lifecycle hooks were invoked.
	if s.setupCount != 1 {
		t.Errorf("setupCount = %d, want 1", s.setupCount)
	}
	if s.runCount != 1 {
		t.Errorf("runCount = %d, want 1", s.runCount)
	}
	if s.teardownCount != 1 {
		t.Errorf("teardownCount = %d, want 1", s.teardownCount)
	}
}

func TestRun_EnvWithBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	var skillBaseURL string
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			skillBaseURL = ctx.BaseURL
			ctx.Reporter.Record(framework.TestResult{
				Name:   "base-url-check",
				Passed: ctx.BaseURL == server.URL,
			})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	projectRoot := t.TempDir()
	configPath := writeYAMLConfig(t, fmt.Sprintf(`
projects:
  - name: test-project
    root: %s
    env: test-env
envs:
  test-env:
    kind: custom
    base_url: %s
skills:
  - name: api
    enabled: true
`, projectRoot, server.URL))

	result, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.BaseURL != server.URL {
		t.Errorf("result.BaseURL = %q, want %q", result.BaseURL, server.URL)
	}
	if skillBaseURL != server.URL {
		t.Errorf("skill BaseURL = %q, want %q", skillBaseURL, server.URL)
	}
	if result.Summary.Passed != 1 {
		t.Errorf("Summary.Passed = %d, want 1", result.Summary.Passed)
	}
}

func TestRun_SkillExecutionOrder(t *testing.T) {
	var order []string
	var mu sync.Mutex

	makeSkill := func(name string, prio skill.Priority) *mockSkill {
		return &mockSkill{
			name:     name,
			priority: prio,
			runFn: func(ctx *skill.Context) skill.Result {
				mu.Lock()
				order = append(order, name)
				mu.Unlock()
				ctx.Reporter.Record(framework.TestResult{Name: name, Passed: true})
				return skill.Result{SkillName: name}
			},
		}
	}

	o := New()
	o.Registry().Register(makeSkill("low", skill.PriorityLow), false)
	o.Registry().Register(makeSkill("critical", skill.PriorityCritical), false)
	o.Registry().Register(makeSkill("normal", skill.PriorityNormal), false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: low
    enabled: true
  - name: critical
    enabled: true
  - name: normal
    enabled: true
`)

	result, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	want := []string{"critical", "normal", "low"}
	if len(order) != len(want) {
		t.Fatalf("execution order %v, want %v", order, want)
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("order[%d] = %q, want %q", i, order[i], w)
		}
	}
	if len(result.SkillResults) != 3 {
		t.Errorf("SkillResults len = %d, want 3", len(result.SkillResults))
	}
}

func TestRun_ReportAggregation(t *testing.T) {
	// Skill 1 records 2 passing cases.
	s1 := &mockSkill{
		name: "skill-a",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{Name: "a-1", Passed: true})
			ctx.Reporter.Record(framework.TestResult{Name: "a-2", Passed: true})
			return skill.Result{SkillName: "skill-a"}
		},
	}
	// Skill 2 records 1 passing, 1 failing case.
	s2 := &mockSkill{
		name: "skill-b",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{Name: "b-1", Passed: true})
			ctx.Reporter.Record(framework.TestResult{Name: "b-2", Passed: false, Message: "fail"})
			return skill.Result{SkillName: "skill-b"}
		},
	}
	o := New()
	o.Registry().Register(s1, false)
	o.Registry().Register(s2, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: skill-a
    enabled: true
  - name: skill-b
    enabled: true
`)

	result, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Summary.Total != 4 {
		t.Errorf("Summary.Total = %d, want 4", result.Summary.Total)
	}
	if result.Summary.Passed != 3 {
		t.Errorf("Summary.Passed = %d, want 3", result.Summary.Passed)
	}
	if result.Summary.Failed != 1 {
		t.Errorf("Summary.Failed = %d, want 1", result.Summary.Failed)
	}
	if len(result.SkillResults) != 2 {
		t.Errorf("SkillResults len = %d, want 2", len(result.SkillResults))
	}
}

func TestRun_SkillConfigureError_SkipsSkill(t *testing.T) {
	bad := &mockSkill{
		name:      "bad-skill",
		configErr: errors.New("invalid config"),
	}
	good := &mockSkill{
		name: "good-skill",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{Name: "ok", Passed: true})
			return skill.Result{SkillName: "good-skill"}
		},
	}
	o := New()
	o.Registry().Register(bad, false)
	o.Registry().Register(good, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: bad-skill
    enabled: true
  - name: good-skill
    enabled: true
`)

	result, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// The bad skill should be skipped (no Setup/Run/Teardown).
	if bad.setupCount != 0 {
		t.Errorf("bad.setupCount = %d, want 0", bad.setupCount)
	}
	if bad.runCount != 0 {
		t.Errorf("bad.runCount = %d, want 0", bad.runCount)
	}
	// The good skill should run.
	if good.runCount != 1 {
		t.Errorf("good.runCount = %d, want 1", good.runCount)
	}
	// Only the good skill's result is in SkillResults.
	if len(result.SkillResults) != 1 {
		t.Errorf("SkillResults len = %d, want 1", len(result.SkillResults))
	}
}

func TestRun_SkillSetupError_SkipsSkill(t *testing.T) {
	bad := &mockSkill{
		name:     "bad-skill",
		setupErr: errors.New("setup failed"),
	}
	good := &mockSkill{
		name: "good-skill",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{Name: "ok", Passed: true})
			return skill.Result{SkillName: "good-skill"}
		},
	}
	o := New()
	o.Registry().Register(bad, false)
	o.Registry().Register(good, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: bad-skill
    enabled: true
  - name: good-skill
    enabled: true
`)

	result, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// The bad skill's Configure succeeded, Setup failed → skip Run.
	if bad.setupCount != 1 {
		t.Errorf("bad.setupCount = %d, want 1", bad.setupCount)
	}
	if bad.runCount != 0 {
		t.Errorf("bad.runCount = %d, want 0 (skipped after Setup error)", bad.runCount)
	}
	// The good skill should run.
	if good.runCount != 1 {
		t.Errorf("good.runCount = %d, want 1", good.runCount)
	}
	if len(result.SkillResults) != 1 {
		t.Errorf("SkillResults len = %d, want 1", len(result.SkillResults))
	}
}

func TestRun_SkillRunError_RecordsResult(t *testing.T) {
	runErr := errors.New("skill execution failed")
	s := &mockSkill{
		name: "error-skill",
		runFn: func(ctx *skill.Context) skill.Result {
			return skill.Result{
				SkillName: "error-skill",
				Error:     runErr,
			}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: error-skill
    enabled: true
`)

	result, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run should not fail even if skill Run errors: %v", err)
	}
	if len(result.SkillResults) != 1 {
		t.Fatalf("SkillResults len = %d, want 1", len(result.SkillResults))
	}
	if result.SkillResults[0].Error == nil {
		t.Errorf("SkillResults[0].Error = nil, want non-nil")
	}
}

func TestRun_SkillTeardownError_Continues(t *testing.T) {
	s := &mockSkill{
		name:        "test-skill",
		teardownErr: errors.New("teardown failed"),
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{Name: "ok", Passed: true})
			return skill.Result{SkillName: "test-skill"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: test-skill
    enabled: true
`)

	_, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run should not fail on Teardown error: %v", err)
	}
	if s.teardownCount != 1 {
		t.Errorf("teardownCount = %d, want 1", s.teardownCount)
	}
}

func TestRun_ReportsDir_Override(t *testing.T) {
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{Name: "ok", Passed: true})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
report:
  output_dir: ignored-dir
skills:
  - name: api
    enabled: true
`)

	overrideDir := t.TempDir()
	result, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: overrideDir,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.ReportsDir != overrideDir {
		t.Errorf("ReportsDir = %q, want %q", result.ReportsDir, overrideDir)
	}
}

func TestRun_ReportsDir_FromConfig(t *testing.T) {
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{Name: "ok", Passed: true})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configReportsDir := t.TempDir()
	configPath := writeYAMLConfig(t, fmt.Sprintf(`
projects:
  - name: test-project
report:
  output_dir: %s
  formats: [json]
skills:
  - name: api
    enabled: true
`, configReportsDir))

	result, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.ReportsDir != configReportsDir {
		t.Errorf("ReportsDir = %q, want %q", result.ReportsDir, configReportsDir)
	}
}

func TestRun_SuiteName_FromConfig(t *testing.T) {
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{Name: "ok", Passed: true})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
report:
  suite_name: my-custom-suite
skills:
  - name: api
    enabled: true
`)

	result, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	_ = result
}

func TestRun_PluginSkillAutoRegistered(t *testing.T) {
	// Pre-condition: no skill named "my-plugin" is registered.
	o := New()
	o.Registry().Register(newMockSkill("api"), false)

	// Use a plugin config with a valid command (echo) that will produce
	// non-JSON output (plugin Run will fail, but registration is what we test).
	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: my-plugin
    kind: plugin
    enabled: true
    raw:
      command: echo
  - name: api
    enabled: true
`)

	// Run should succeed (plugin's Run error is non-fatal).
	_, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// The plugin skill should have been auto-registered.
	got, err := o.Registry().Get("my-plugin")
	if err != nil {
		t.Fatalf("plugin skill not auto-registered: %v", err)
	}
	if got.Name() != "my-plugin" {
		t.Errorf("registered skill name = %q, want my-plugin", got.Name())
	}
	if got.Kind() != skill.KindPlugin {
		t.Errorf("registered skill kind = %q, want %q", got.Kind(), skill.KindPlugin)
	}
}

func TestRun_LoggerOption(t *testing.T) {
	logger := &captureLogger{}
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{Name: "ok", Passed: true})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: api
    enabled: true
`)

	_, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
		Logger:     logger,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	msgs := logger.messages()
	if len(msgs) == 0 {
		t.Errorf("expected log messages, got none")
	}
	foundRunning := false
	for _, m := range msgs {
		if strings.Contains(m, "running") || strings.Contains(m, "skill") {
			foundRunning = true
			break
		}
	}
	if !foundRunning {
		t.Errorf("expected a log message about skill running, got: %v", msgs)
	}
}

// ---------------------------------------------------------------------------
// Copilot: agentFixMessage tests
// ---------------------------------------------------------------------------

func TestAgentFixMessage(t *testing.T) {
	t.Run("nil result", func(t *testing.T) {
		if got := agentFixMessage(nil); got != "nil fix result" {
			t.Errorf("got %q, want 'nil fix result'", got)
		}
	})

	t.Run("empty message", func(t *testing.T) {
		r := &agent.FixResult{}
		if got := agentFixMessage(r); got != "no message from agent" {
			t.Errorf("got %q, want 'no message from agent'", got)
		}
	})

	t.Run("with message", func(t *testing.T) {
		r := &agent.FixResult{Message: "patch applied"}
		if got := agentFixMessage(r); got != "patch applied" {
			t.Errorf("got %q, want 'patch applied'", got)
		}
	})
}

func TestDefaultMaxRounds(t *testing.T) {
	if defaultMaxRounds != 3 {
		t.Errorf("defaultMaxRounds = %d, want 3", defaultMaxRounds)
	}
}

// ---------------------------------------------------------------------------
// RunCopilot tests
// ---------------------------------------------------------------------------

func TestRunCopilot_RunError(t *testing.T) {
	o := New()
	_, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: filepath.Join(t.TempDir(), "nonexistent.yaml"),
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "copilot round 1") {
		t.Fatalf("error %q does not contain 'copilot round 1'", err.Error())
	}
}

func TestRunCopilot_NoFailures(t *testing.T) {
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{Name: "case-1", Passed: true})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: api
    enabled: true
`)

	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
	if len(result.Rounds) != 0 {
		t.Errorf("Rounds len = %d, want 0 (no fix needed)", len(result.Rounds))
	}
}

func TestRunCopilot_NoInvoker_DegradesToNormalRun(t *testing.T) {
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{Name: "case-1", Passed: false, Message: "fail"})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: api
    enabled: true
`)

	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
		// No Invoker set.
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	if result.Fixed {
		t.Errorf("Fixed = true, want false (no invoker)")
	}
	if len(result.Rounds) != 0 {
		t.Errorf("Rounds len = %d, want 0 (degraded)", len(result.Rounds))
	}
	if result.Final.Summary.Failed != 1 {
		t.Errorf("Final.Summary.Failed = %d, want 1", result.Final.Summary.Failed)
	}
}

func TestRunCopilot_AgentFixesFirstRound(t *testing.T) {
	projectRoot := t.TempDir()
	// Create a file that the direct-mode applier will verify.
	fixedFile := filepath.Join(projectRoot, "fixed.go")
	if err := os.WriteFile(fixedFile, []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Mock skill: fails on first run, passes on second.
	runCount := 0
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			runCount++
			if runCount == 1 {
				ctx.Reporter.Record(framework.TestResult{
					Name:    "case-1",
					Passed:  false,
					Message: "initial failure",
				})
			} else {
				ctx.Reporter.Record(framework.TestResult{
					Name:   "case-1",
					Passed: true,
				})
			}
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, fmt.Sprintf(`
projects:
  - name: test-project
    root: %s
skills:
  - name: api
    enabled: true
`, projectRoot))

	invoker := &mockInvoker{
		name: "test-invoker",
		results: []*agent.FixResult{
			{
				Fixed:         true,
				Mode:          agent.FixModeDirect,
				ModifiedFiles: []string{"fixed.go"},
				Message:       "fixed the issue",
			},
		},
	}

	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
		Invoker: invoker,
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("Rounds len = %d, want 1", len(result.Rounds))
	}
	if invoker.callCount() != 1 {
		t.Errorf("invoker calls = %d, want 1", invoker.callCount())
	}
	if result.Rounds[0].Round != 1 {
		t.Errorf("Rounds[0].Round = %d, want 1", result.Rounds[0].Round)
	}
	if result.Rounds[0].FixResult == nil {
		t.Error("Rounds[0].FixResult = nil, want non-nil")
	}
	if result.Rounds[0].ApplyError != nil {
		t.Errorf("Rounds[0].ApplyError = %v, want nil", result.Rounds[0].ApplyError)
	}
}

func TestRunCopilot_AgentCannotFix(t *testing.T) {
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{
				Name:    "case-1",
				Passed:  false,
				Message: "fail",
			})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: api
    enabled: true
`)

	invoker := &mockInvoker{
		name: "cannot-fix-invoker",
		results: []*agent.FixResult{
			{Fixed: false, Message: "unable to fix"},
		},
	}

	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
		Invoker: invoker,
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("Rounds len = %d, want 1", len(result.Rounds))
	}
	if result.Rounds[0].AgentError != nil {
		t.Errorf("Rounds[0].AgentError = %v, want nil", result.Rounds[0].AgentError)
	}
}

func TestRunCopilot_AgentError(t *testing.T) {
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{
				Name:   "case-1",
				Passed: false,
			})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: api
    enabled: true
`)

	invoker := &mockInvoker{
		name: "error-invoker",
		errs: []error{errors.New("agent crashed")},
	}

	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
		Invoker: invoker,
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("Rounds len = %d, want 1", len(result.Rounds))
	}
	if result.Rounds[0].AgentError == nil {
		t.Error("Rounds[0].AgentError = nil, want non-nil")
	}
}

func TestRunCopilot_ApplyError(t *testing.T) {
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{
				Name:   "case-1",
				Passed: false,
			})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: api
    enabled: true
`)

	// Patch mode with empty patch triggers an apply error.
	invoker := &mockInvoker{
		name: "test-invoker",
		results: []*agent.FixResult{
			{
				Fixed:   true,
				Mode:    agent.FixModePatch,
				Patch:   "", // empty patch → ApplyResult returns error
				Message: "patch generated",
			},
		},
	}

	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
		Invoker: invoker,
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("Rounds len = %d, want 1", len(result.Rounds))
	}
	if result.Rounds[0].ApplyError == nil {
		t.Error("Rounds[0].ApplyError = nil, want non-nil")
	}
}

func TestRunCopilot_MaxRoundsExhausted(t *testing.T) {
	projectRoot := t.TempDir()
	fixedFile := filepath.Join(projectRoot, "fixed.go")
	os.WriteFile(fixedFile, []byte("package main"), 0o644)

	// Mock skill always fails.
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{
				Name:   "case-1",
				Passed: false,
			})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, fmt.Sprintf(`
projects:
  - name: test-project
    root: %s
skills:
  - name: api
    enabled: true
`, projectRoot))

	// Invoker always returns Fixed=true, applier succeeds (file exists),
	// but regression always fails.
	invoker := &mockInvoker{
		name: "test-invoker",
		results: []*agent.FixResult{
			{Fixed: true, Mode: agent.FixModeDirect, ModifiedFiles: []string{"fixed.go"}, Message: "fixed"},
			{Fixed: true, Mode: agent.FixModeDirect, ModifiedFiles: []string{"fixed.go"}, Message: "fixed again"},
		},
	}

	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
		Invoker:   invoker,
		MaxRounds: 2,
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if len(result.Rounds) != 2 {
		t.Fatalf("Rounds len = %d, want 2", len(result.Rounds))
	}
	if invoker.callCount() != 2 {
		t.Errorf("invoker calls = %d, want 2", invoker.callCount())
	}
	if result.Final.Summary.Failed != 1 {
		t.Errorf("Final.Summary.Failed = %d, want 1", result.Final.Summary.Failed)
	}
}

func TestRunCopilot_DefaultMaxRounds(t *testing.T) {
	// When MaxRounds is 0, defaultMaxRounds (3) is used.
	projectRoot := t.TempDir()
	fixedFile := filepath.Join(projectRoot, "fixed.go")
	os.WriteFile(fixedFile, []byte("package main"), 0o644)

	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{
				Name:   "case-1",
				Passed: false,
			})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, fmt.Sprintf(`
projects:
  - name: test-project
    root: %s
skills:
  - name: api
    enabled: true
`, projectRoot))

	invoker := &mockInvoker{
		name: "test-invoker",
		// Return Fixed=true for all 3 default rounds.
		results: []*agent.FixResult{
			{Fixed: true, Mode: agent.FixModeDirect, ModifiedFiles: []string{"fixed.go"}, Message: "f1"},
			{Fixed: true, Mode: agent.FixModeDirect, ModifiedFiles: []string{"fixed.go"}, Message: "f2"},
			{Fixed: true, Mode: agent.FixModeDirect, ModifiedFiles: []string{"fixed.go"}, Message: "f3"},
		},
	}

	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
		Invoker:   invoker,
		MaxRounds: 0, // should default to 3
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if len(result.Rounds) != defaultMaxRounds {
		t.Fatalf("Rounds len = %d, want %d (default)", len(result.Rounds), defaultMaxRounds)
	}
}

func TestRunCopilot_RegressionRunError(t *testing.T) {
	projectRoot := t.TempDir()
	fixedFile := filepath.Join(projectRoot, "fixed.go")
	os.WriteFile(fixedFile, []byte("package main"), 0o644)

	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{
				Name:   "case-1",
				Passed: false,
			})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "taichi.yaml")
	configContent := fmt.Sprintf(`
projects:
  - name: test-project
    root: %s
skills:
  - name: api
    enabled: true
`, projectRoot)
	os.WriteFile(configPath, []byte(configContent), 0o644)

	// Invoker deletes the config file after the first call to cause
	// the regression Run to fail with a "load config" error.
	invoker := &mockInvoker{
		name: "test-invoker",
		results: []*agent.FixResult{
			{Fixed: true, Mode: agent.FixModeDirect, ModifiedFiles: []string{"fixed.go"}, Message: "fixed"},
		},
		sideEffect: func() {
			os.Remove(configPath)
		},
	}

	_, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
		Invoker: invoker,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "regression") {
		t.Fatalf("error %q does not contain 'regression'", err.Error())
	}
}

func TestRunCopilot_DirectMode_VerifiesModifiedFiles(t *testing.T) {
	projectRoot := t.TempDir()
	// Create the file that the applier will verify in direct mode.
	fixedFile := filepath.Join(projectRoot, "main.go")
	os.WriteFile(fixedFile, []byte("package main"), 0o644)

	runCount := 0
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			runCount++
			if runCount == 1 {
				ctx.Reporter.Record(framework.TestResult{
					Name:   "case-1",
					Passed: false,
				})
			} else {
				ctx.Reporter.Record(framework.TestResult{
					Name:   "case-1",
					Passed: true,
				})
			}
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, fmt.Sprintf(`
projects:
  - name: test-project
    root: %s
skills:
  - name: api
    enabled: true
`, projectRoot))

	invoker := &mockInvoker{
		name: "direct-invoker",
		results: []*agent.FixResult{
			{
				Fixed:         true,
				Mode:          agent.FixModeDirect,
				ModifiedFiles: []string{"main.go"},
				Message:       "directly modified",
			},
		},
	}

	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
		Invoker: invoker,
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
	if result.Rounds[0].FixResult.Mode != agent.FixModeDirect {
		t.Errorf("FixResult.Mode = %q, want %q",
			result.Rounds[0].FixResult.Mode, agent.FixModeDirect)
	}
}

func TestRunCopilot_PatchModeSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git-based patch test in short mode")
	}

	projectRoot := t.TempDir()

	// Initialize a git repo (needed for git apply).
	if err := exec.Command("git", "-C", projectRoot, "init").Run(); err != nil {
		t.Skipf("git not available: %v", err)
	}
	// Configure git user (required for some operations).
	exec.Command("git", "-C", projectRoot, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", projectRoot, "config", "user.name", "Test").Run()

	// Create an initial file.
	initialContent := "package main\n\nfunc main() {}\n"
	mainGo := filepath.Join(projectRoot, "main.go")
	if err := os.WriteFile(mainGo, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// A valid unified diff patch that adds a comment line.
	patch := `--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
 
 func main() {}
+// fixed
`

	runCount := 0
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			runCount++
			if runCount == 1 {
				ctx.Reporter.Record(framework.TestResult{
					Name:   "case-1",
					Passed: false,
				})
			} else {
				ctx.Reporter.Record(framework.TestResult{
					Name:   "case-1",
					Passed: true,
				})
			}
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, fmt.Sprintf(`
projects:
  - name: test-project
    root: %s
skills:
  - name: api
    enabled: true
`, projectRoot))

	invoker := &mockInvoker{
		name: "patch-invoker",
		results: []*agent.FixResult{
			{
				Fixed:   true,
				Mode:    agent.FixModePatch,
				Patch:   patch,
				Message: "patch applied",
			},
		},
	}

	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
		Invoker: invoker,
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("Rounds len = %d, want 1", len(result.Rounds))
	}
	if result.Rounds[0].ApplyError != nil {
		t.Errorf("Rounds[0].ApplyError = %v, want nil", result.Rounds[0].ApplyError)
	}

	// Verify the patch was actually applied.
	content, err := os.ReadFile(mainGo)
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !strings.Contains(string(content), "// fixed") {
		t.Errorf("patch was not applied: %s", string(content))
	}
}

func TestRunCopilot_FailureContextWritten(t *testing.T) {
	projectRoot := t.TempDir()
	fixedFile := filepath.Join(projectRoot, "fixed.go")
	os.WriteFile(fixedFile, []byte("package main"), 0o644)

	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{
				Name:    "case-1",
				Passed:  false,
				Message: "fail",
			})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, fmt.Sprintf(`
projects:
  - name: test-project
    root: %s
skills:
  - name: api
    enabled: true
`, projectRoot))

	invoker := &mockInvoker{
		name: "test-invoker",
		results: []*agent.FixResult{
			{Fixed: false, Message: "cannot fix"},
		},
	}

	reportsDir := t.TempDir()
	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: reportsDir,
		},
		Invoker: invoker,
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	// The invoker should have received a FailureContext.
	if len(invoker.captured) != 1 {
		t.Fatalf("invoker captured %d contexts, want 1", len(invoker.captured))
	}
	fc := invoker.captured[0]
	if fc.ProjectName != "test-project" {
		t.Errorf("FailureContext.ProjectName = %q, want 'test-project'", fc.ProjectName)
	}
	if fc.ProjectRoot != projectRoot {
		t.Errorf("FailureContext.ProjectRoot = %q, want %q", fc.ProjectRoot, projectRoot)
	}
	if len(fc.FailedCases) != 1 {
		t.Errorf("FailedCases len = %d, want 1", len(fc.FailedCases))
	}
	if fc.FailedCases[0].Name != "case-1" {
		t.Errorf("FailedCases[0].Name = %q, want 'case-1'", fc.FailedCases[0].Name)
	}

	// The failure context JSON file should have been written to ReportsDir.
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		t.Fatalf("read reports dir: %v", err)
	}
	foundFC := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "failures-round-") && strings.HasSuffix(e.Name(), ".json") {
			foundFC = true
			break
		}
	}
	if !foundFC {
		t.Errorf("failure context JSON file not found in %s", reportsDir)
	}
}

func TestRunCopilot_CustomPatchApplier(t *testing.T) {
	// When PatchApplier is provided in CopilotOptions, it should be used
	// instead of creating a default one.
	s := &mockSkill{
		name: "api",
		runFn: func(ctx *skill.Context) skill.Result {
			ctx.Reporter.Record(framework.TestResult{
				Name:   "case-1",
				Passed: false,
			})
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	configPath := writeYAMLConfig(t, `
projects:
  - name: test-project
skills:
  - name: api
    enabled: true
`)

	invoker := &mockInvoker{
		name: "test-invoker",
		results: []*agent.FixResult{
			{
				Fixed:   true,
				Mode:    agent.FixModePatch,
				Patch:   "", // empty patch → apply error
				Message: "patch generated",
			},
		},
	}

	// Use a custom PatchApplier with empty ProjectRoot.
	customApplier := &agent.PatchApplier{ProjectRoot: t.TempDir()}

	result, err := o.RunCopilot(context.Background(), CopilotOptions{
		Options: Options{
			ConfigPath: configPath,
			ReportsDir: t.TempDir(),
		},
		Invoker:      invoker,
		PatchApplier: customApplier,
	})
	if err != nil {
		t.Fatalf("RunCopilot error: %v", err)
	}
	// Patch mode with empty patch should fail.
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("Rounds len = %d, want 1", len(result.Rounds))
	}
	if result.Rounds[0].ApplyError == nil {
		t.Error("Rounds[0].ApplyError = nil, want non-nil (empty patch)")
	}
}

// ---------------------------------------------------------------------------
// Integration test: full Run with httptest server and env BaseURL
// ---------------------------------------------------------------------------

func TestRun_Integration_FullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Start a real HTTP server that responds to /health.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		case "/api/data":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"code":0,"msg":"success","request_id":"abc"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// A more realistic mock skill that makes an HTTP request to the service.
	s := &mockSkill{
		name:     "api",
		priority: skill.PriorityNormal,
		runFn: func(ctx *skill.Context) skill.Result {
			start := time.Now()
			resp, body, err := skill.HTTPRequest(
				&http.Client{Timeout: 5 * time.Second},
				"GET",
				ctx.BaseURL+"/api/data",
				nil,
			)
			if err != nil {
				skill.RecordResult(ctx.Reporter, "api-data", start, false, err.Error(), err)
				return skill.Result{SkillName: "api", Error: err}
			}
			// Assert status code.
			assertResult := ctx.Asserts.AssertStatusCode(resp, http.StatusOK)
			if !assertResult.Passed {
				skill.RecordResult(ctx.Reporter, "api-data", start, false, assertResult.Message, nil)
				return skill.Result{SkillName: "api"}
			}
			// Assert response envelope.
			fieldsResult, codeResult := skill.AssertCommonEnvelope(ctx.Asserts, body, 0)
			if !fieldsResult.Passed {
				skill.RecordResult(ctx.Reporter, "api-data", start, false, fieldsResult.Message, nil)
				return skill.Result{SkillName: "api"}
			}
			if !codeResult.Passed {
				skill.RecordResult(ctx.Reporter, "api-data", start, false, codeResult.Message, nil)
				return skill.Result{SkillName: "api"}
			}
			skill.RecordResult(ctx.Reporter, "api-data", start, true, "OK", nil)
			return skill.Result{SkillName: "api"}
		},
	}
	o := New()
	o.Registry().Register(s, false)

	projectRoot := t.TempDir()
	configPath := writeYAMLConfig(t, fmt.Sprintf(`
projects:
  - name: test-project
    root: %s
    env: test-env
envs:
  test-env:
    kind: custom
    base_url: %s
skills:
  - name: api
    enabled: true
    kind: api
    priority: 20
report:
  suite_name: integration-test
  formats: [json, junit, summary]
`, projectRoot, server.URL))

	result, err := o.Run(context.Background(), Options{
		ConfigPath: configPath,
		ReportsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.ProjectName != "test-project" {
		t.Errorf("ProjectName = %q, want 'test-project'", result.ProjectName)
	}
	if result.BaseURL != server.URL {
		t.Errorf("BaseURL = %q, want %q", result.BaseURL, server.URL)
	}
	if result.Summary.Total != 1 {
		t.Errorf("Summary.Total = %d, want 1", result.Summary.Total)
	}
	if result.Summary.Passed != 1 {
		t.Errorf("Summary.Passed = %d, want 1", result.Summary.Passed)
	}
	if result.Summary.Failed != 0 {
		t.Errorf("Summary.Failed = %d, want 0", result.Summary.Failed)
	}
	if len(result.Results) != 1 {
		t.Errorf("Results len = %d, want 1", len(result.Results))
	}
}
