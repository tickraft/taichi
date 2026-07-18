package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/skill"
)

// TestLoadEmptyPath verifies that Load returns an empty Config with no error
// when the path is empty.
func TestLoadEmptyPath(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load(\"\") returned nil config")
	}
	if len(cfg.Projects) != 0 {
		t.Errorf("expected empty projects, got %d", len(cfg.Projects))
	}
	if len(cfg.Envs) != 0 {
		t.Errorf("expected empty envs, got %d", len(cfg.Envs))
	}
	if len(cfg.Skills) != 0 {
		t.Errorf("expected empty skills, got %d", len(cfg.Skills))
	}
}

// TestLoadYAML writes a temp YAML config and verifies all fields are parsed correctly.
func TestLoadYAML(t *testing.T) {
	const yamlContent = `
projects:
  - name: tickraft
    root: ../tickraft
    env: tickraft-backend
    skills: [api, static, regression]
  - name: frontend
    root: ../frontend
    env: tickraft-frontend
envs:
  tickraft-backend:
    kind: backend.go
    binary: bin/tickraft
    build: go build -o bin/tickraft ./cmd/tickraft
    config_path: configs/config.yaml
    config_flag: --config
    addr_flag: --addr
    health_path: /api/v1/health
    healthy_timeout: 30s
    port: 8080
    base_url: http://localhost:8080
    args: ["--verbose"]
    env: ["FOO=bar"]
  tickraft-frontend:
    kind: frontend.vite
    command: pnpm dev
    port: 5173
    base_url: http://localhost:5173
    cwd: /tmp/app
    ready_url: http://localhost:5173
    ready_text: ready
skills:
  - name: api
    enabled: true
    priority: 0
    kind: api
    raw:
      timeout: 5s
  - name: static
    enabled: true
    priority: 20
    kind: static
report:
  suite_name: taichi-tests
  output_dir: reports
  formats: [json, junit, summary]
autofix:
  enabled: true
  reports_dir: autofix-reports
locale: zh-CN
`

	dir := t.TempDir()
	path := filepath.Join(dir, "taichi.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Verify projects.
	if len(cfg.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(cfg.Projects))
	}
	p0 := cfg.Projects[0]
	if p0.Name != "tickraft" {
		t.Errorf("project[0].name = %q, want %q", p0.Name, "tickraft")
	}
	if p0.Root != "../tickraft" {
		t.Errorf("project[0].root = %q, want %q", p0.Root, "../tickraft")
	}
	if p0.Env != "tickraft-backend" {
		t.Errorf("project[0].env = %q, want %q", p0.Env, "tickraft-backend")
	}
	if len(p0.Skills) != 3 || p0.Skills[0] != "api" || p0.Skills[1] != "static" || p0.Skills[2] != "regression" {
		t.Errorf("project[0].skills = %v, want [api static regression]", p0.Skills)
	}

	// Verify envs.
	if len(cfg.Envs) != 2 {
		t.Fatalf("expected 2 envs, got %d", len(cfg.Envs))
	}
	backend, ok := cfg.Envs["tickraft-backend"]
	if !ok {
		t.Fatal("env tickraft-backend not found")
	}
	if backend.Kind != EnvKindBackendGo {
		t.Errorf("backend kind = %q, want %q", backend.Kind, EnvKindBackendGo)
	}
	if backend.BinaryPath != "bin/tickraft" {
		t.Errorf("backend binary = %q, want %q", backend.BinaryPath, "bin/tickraft")
	}
	if backend.Build != "go build -o bin/tickraft ./cmd/tickraft" {
		t.Errorf("backend build = %q, want %q", backend.Build, "go build -o bin/tickraft ./cmd/tickraft")
	}
	if backend.ConfigPath != "configs/config.yaml" {
		t.Errorf("backend config_path = %q, want %q", backend.ConfigPath, "configs/config.yaml")
	}
	if backend.ConfigFlag != "--config" {
		t.Errorf("backend config_flag = %q, want %q", backend.ConfigFlag, "--config")
	}
	if backend.AddrFlag != "--addr" {
		t.Errorf("backend addr_flag = %q, want %q", backend.AddrFlag, "--addr")
	}
	if backend.HealthPath != "/api/v1/health" {
		t.Errorf("backend health_path = %q, want %q", backend.HealthPath, "/api/v1/health")
	}
	if backend.HealthyTimeout != "30s" {
		t.Errorf("backend healthy_timeout = %q, want %q", backend.HealthyTimeout, "30s")
	}
	if backend.Port != 8080 {
		t.Errorf("backend port = %d, want 8080", backend.Port)
	}
	if backend.BaseURL != "http://localhost:8080" {
		t.Errorf("backend base_url = %q, want %q", backend.BaseURL, "http://localhost:8080")
	}
	if len(backend.Args) != 1 || backend.Args[0] != "--verbose" {
		t.Errorf("backend args = %v, want [--verbose]", backend.Args)
	}
	if len(backend.Env) != 1 || backend.Env[0] != "FOO=bar" {
		t.Errorf("backend env = %v, want [FOO=bar]", backend.Env)
	}

	frontend, ok := cfg.Envs["tickraft-frontend"]
	if !ok {
		t.Fatal("env tickraft-frontend not found")
	}
	if frontend.Kind != EnvKindFrontendVite {
		t.Errorf("frontend kind = %q, want %q", frontend.Kind, EnvKindFrontendVite)
	}
	if frontend.Command != "pnpm dev" {
		t.Errorf("frontend command = %q, want %q", frontend.Command, "pnpm dev")
	}
	if frontend.Port != 5173 {
		t.Errorf("frontend port = %d, want 5173", frontend.Port)
	}
	if frontend.BaseURL != "http://localhost:5173" {
		t.Errorf("frontend base_url = %q, want %q", frontend.BaseURL, "http://localhost:5173")
	}
	if frontend.Cwd != "/tmp/app" {
		t.Errorf("frontend cwd = %q, want %q", frontend.Cwd, "/tmp/app")
	}
	if frontend.ReadyURL != "http://localhost:5173" {
		t.Errorf("frontend ready_url = %q, want %q", frontend.ReadyURL, "http://localhost:5173")
	}
	if frontend.ReadyText != "ready" {
		t.Errorf("frontend ready_text = %q, want %q", frontend.ReadyText, "ready")
	}

	// Verify skills.
	if len(cfg.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(cfg.Skills))
	}
	s0 := cfg.Skills[0]
	if s0.Name != "api" {
		t.Errorf("skill[0].name = %q, want %q", s0.Name, "api")
	}
	if !s0.Enabled {
		t.Error("skill[0].enabled = false, want true")
	}
	if s0.Priority != skill.PriorityCritical {
		t.Errorf("skill[0].priority = %d, want %d", s0.Priority, skill.PriorityCritical)
	}
	if s0.Kind != skill.KindAPI {
		t.Errorf("skill[0].kind = %q, want %q", s0.Kind, skill.KindAPI)
	}
	if s0.Raw == nil {
		t.Error("skill[0].raw is nil")
	} else if _, ok := s0.Raw["timeout"]; !ok {
		t.Errorf("skill[0].raw missing 'timeout' key, got %v", s0.Raw)
	}

	s1 := cfg.Skills[1]
	if s1.Name != "static" {
		t.Errorf("skill[1].name = %q, want %q", s1.Name, "static")
	}
	if s1.Priority != skill.PriorityNormal {
		t.Errorf("skill[1].priority = %d, want %d", s1.Priority, skill.PriorityNormal)
	}
	if s1.Kind != skill.KindStatic {
		t.Errorf("skill[1].kind = %q, want %q", s1.Kind, skill.KindStatic)
	}

	// Verify report.
	if cfg.Report.SuiteName != "taichi-tests" {
		t.Errorf("report.suite_name = %q, want %q", cfg.Report.SuiteName, "taichi-tests")
	}
	if cfg.Report.OutputDir != "reports" {
		t.Errorf("report.output_dir = %q, want %q", cfg.Report.OutputDir, "reports")
	}
	if len(cfg.Report.Formats) != 3 {
		t.Fatalf("expected 3 report formats, got %d", len(cfg.Report.Formats))
	}
	for i, want := range []string{"json", "junit", "summary"} {
		if cfg.Report.Formats[i] != want {
			t.Errorf("report.formats[%d] = %q, want %q", i, cfg.Report.Formats[i], want)
		}
	}

	// Verify autofix.
	if !cfg.Autofix.Enabled {
		t.Error("autofix.enabled = false, want true")
	}
	if cfg.Autofix.ReportsDir != "autofix-reports" {
		t.Errorf("autofix.reports_dir = %q, want %q", cfg.Autofix.ReportsDir, "autofix-reports")
	}

	// Verify locale.
	if cfg.Locale != "zh-CN" {
		t.Errorf("locale = %q, want %q", cfg.Locale, "zh-CN")
	}
}

// TestLoadJSON writes a temp JSON config and verifies all fields are parsed correctly.
func TestLoadJSON(t *testing.T) {
	const jsonContent = `{
  "projects": [
    {
      "name": "tickraft",
      "root": "../tickraft",
      "env": "tickraft-backend",
      "skills": ["api", "static"]
    }
  ],
  "envs": {
    "tickraft-backend": {
      "kind": "backend.go",
      "binary": "bin/tickraft",
      "build": "./cmd/tickraft",
      "config_path": "configs/config.yaml",
      "config_flag": "--config",
      "addr_flag": "--addr",
      "health_path": "/api/v1/health",
      "healthy_timeout": "45s",
      "port": 9090,
      "base_url": "http://localhost:9090"
    }
  },
  "skills": [
    {
      "name": "api",
      "enabled": true,
      "priority": 0,
      "kind": "api",
      "raw": {
        "timeout": "5s"
      }
    }
  ],
  "report": {
    "suite_name": "json-tests",
    "output_dir": "json-reports",
    "formats": ["json"]
  },
  "autofix": {
    "enabled": false,
    "reports_dir": "json-errors"
  },
  "locale": "en-US"
}`

	dir := t.TempDir()
	path := filepath.Join(dir, "taichi.json")
	if err := os.WriteFile(path, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("write temp json: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Verify projects.
	if len(cfg.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(cfg.Projects))
	}
	p := cfg.Projects[0]
	if p.Name != "tickraft" {
		t.Errorf("project.name = %q, want %q", p.Name, "tickraft")
	}
	if p.Root != "../tickraft" {
		t.Errorf("project.root = %q, want %q", p.Root, "../tickraft")
	}
	if p.Env != "tickraft-backend" {
		t.Errorf("project.env = %q, want %q", p.Env, "tickraft-backend")
	}
	if len(p.Skills) != 2 || p.Skills[0] != "api" || p.Skills[1] != "static" {
		t.Errorf("project.skills = %v, want [api static]", p.Skills)
	}

	// Verify envs.
	if len(cfg.Envs) != 1 {
		t.Fatalf("expected 1 env, got %d", len(cfg.Envs))
	}
	env, ok := cfg.Envs["tickraft-backend"]
	if !ok {
		t.Fatal("env tickraft-backend not found")
	}
	if env.Kind != EnvKindBackendGo {
		t.Errorf("env kind = %q, want %q", env.Kind, EnvKindBackendGo)
	}
	if env.BinaryPath != "bin/tickraft" {
		t.Errorf("env binary = %q, want %q", env.BinaryPath, "bin/tickraft")
	}
	if env.Build != "./cmd/tickraft" {
		t.Errorf("env build = %q, want %q", env.Build, "./cmd/tickraft")
	}
	if env.HealthyTimeout != "45s" {
		t.Errorf("env healthy_timeout = %q, want %q", env.HealthyTimeout, "45s")
	}
	if env.Port != 9090 {
		t.Errorf("env port = %d, want 9090", env.Port)
	}
	if env.BaseURL != "http://localhost:9090" {
		t.Errorf("env base_url = %q, want %q", env.BaseURL, "http://localhost:9090")
	}

	// Verify skills.
	if len(cfg.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(cfg.Skills))
	}
	s := cfg.Skills[0]
	if s.Name != "api" {
		t.Errorf("skill.name = %q, want %q", s.Name, "api")
	}
	if !s.Enabled {
		t.Error("skill.enabled = false, want true")
	}
	if s.Kind != skill.KindAPI {
		t.Errorf("skill.kind = %q, want %q", s.Kind, skill.KindAPI)
	}
	if s.Raw == nil {
		t.Error("skill.raw is nil")
	} else if _, ok := s.Raw["timeout"]; !ok {
		t.Errorf("skill.raw missing 'timeout' key, got %v", s.Raw)
	}

	// Verify report.
	if cfg.Report.SuiteName != "json-tests" {
		t.Errorf("report.suite_name = %q, want %q", cfg.Report.SuiteName, "json-tests")
	}
	if cfg.Report.OutputDir != "json-reports" {
		t.Errorf("report.output_dir = %q, want %q", cfg.Report.OutputDir, "json-reports")
	}
	if len(cfg.Report.Formats) != 1 || cfg.Report.Formats[0] != "json" {
		t.Errorf("report.formats = %v, want [json]", cfg.Report.Formats)
	}

	// Verify autofix.
	if cfg.Autofix.Enabled {
		t.Error("autofix.enabled = true, want false")
	}
	if cfg.Autofix.ReportsDir != "json-errors" {
		t.Errorf("autofix.reports_dir = %q, want %q", cfg.Autofix.ReportsDir, "json-errors")
	}

	// Verify locale.
	if cfg.Locale != "en-US" {
		t.Errorf("locale = %q, want %q", cfg.Locale, "en-US")
	}
}

// TestLoadInvalidPath verifies that Load returns an error for a non-existent file.
func TestLoadInvalidPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.yaml")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

// TestLoadInvalidYAML verifies that Load returns an error for malformed YAML.
func TestLoadInvalidYAML(t *testing.T) {
	const malformedYAML = `
projects:
  - name: tickraft
    env: backend
    broken: [unclosed bracket
`

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(malformedYAML), 0644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

// TestValidate covers various validation scenarios for the Config.validate method.
func TestValidate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &Config{
			Projects: []Project{
				{Name: "tickraft", Env: "backend"},
			},
			Envs: map[string]Env{
				"backend": {Kind: EnvKindBackendGo},
			},
			Skills: []skill.Config{
				{Name: "api", Kind: skill.KindAPI},
				{Name: "static", Kind: skill.KindStatic},
			},
		}
		if err := cfg.validate(); err != nil {
			t.Errorf("validate returned error for valid config: %v", err)
		}
	})

	t.Run("project with empty name", func(t *testing.T) {
		cfg := &Config{
			Projects: []Project{
				{Name: ""},
			},
		}
		err := cfg.validate()
		if err == nil {
			t.Fatal("expected error for empty project name, got nil")
		}
	})

	t.Run("project with env not in envs map", func(t *testing.T) {
		cfg := &Config{
			Projects: []Project{
				{Name: "tickraft", Env: "missing-env"},
			},
			Envs: map[string]Env{
				"backend": {Kind: EnvKindBackendGo},
			},
		}
		err := cfg.validate()
		if err == nil {
			t.Fatal("expected error for undefined env reference, got nil")
		}
	})

	t.Run("skill with empty name", func(t *testing.T) {
		cfg := &Config{
			Skills: []skill.Config{
				{Name: ""},
			},
		}
		err := cfg.validate()
		if err == nil {
			t.Fatal("expected error for empty skill name, got nil")
		}
	})

	t.Run("duplicate skill names", func(t *testing.T) {
		cfg := &Config{
			Skills: []skill.Config{
				{Name: "api", Kind: skill.KindAPI},
				{Name: "api", Kind: skill.KindAPI},
			},
		}
		err := cfg.validate()
		if err == nil {
			t.Fatal("expected error for duplicate skill names, got nil")
		}
	})

	t.Run("empty config is valid", func(t *testing.T) {
		cfg := &Config{}
		if err := cfg.validate(); err != nil {
			t.Errorf("validate returned error for empty config: %v", err)
		}
	})

	t.Run("project with empty env is valid", func(t *testing.T) {
		cfg := &Config{
			Projects: []Project{
				{Name: "tickraft", Env: ""},
			},
		}
		if err := cfg.validate(); err != nil {
			t.Errorf("validate returned error for project with empty env: %v", err)
		}
	})
}

// TestProjectByName verifies lookup of a project by name.
func TestProjectByName(t *testing.T) {
	cfg := &Config{
		Projects: []Project{
			{Name: "tickraft", Env: "backend"},
			{Name: "frontend", Env: "frontend-env"},
		},
	}

	t.Run("existing project", func(t *testing.T) {
		p, err := cfg.ProjectByName("tickraft")
		if err != nil {
			t.Fatalf("ProjectByName returned error: %v", err)
		}
		if p.Name != "tickraft" {
			t.Errorf("got name %q, want %q", p.Name, "tickraft")
		}
		if p.Env != "backend" {
			t.Errorf("got env %q, want %q", p.Env, "backend")
		}
	})

	t.Run("non-existing project", func(t *testing.T) {
		_, err := cfg.ProjectByName("nonexistent")
		if err == nil {
			t.Fatal("expected error for non-existing project, got nil")
		}
	})
}

// TestSkillConfigsByName verifies the name-to-config map returned by SkillConfigsByName.
func TestSkillConfigsByName(t *testing.T) {
	cfg := &Config{
		Skills: []skill.Config{
			{Name: "api", Kind: skill.KindAPI, Enabled: true, Priority: skill.PriorityCritical},
			{Name: "static", Kind: skill.KindStatic, Enabled: false, Priority: skill.PriorityNormal},
		},
	}

	m := cfg.SkillConfigsByName()
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}

	api, ok := m["api"]
	if !ok {
		t.Fatal("key 'api' not found in map")
	}
	if api.Kind != skill.KindAPI {
		t.Errorf("api kind = %q, want %q", api.Kind, skill.KindAPI)
	}
	if !api.Enabled {
		t.Error("api enabled = false, want true")
	}
	if api.Priority != skill.PriorityCritical {
		t.Errorf("api priority = %d, want %d", api.Priority, skill.PriorityCritical)
	}

	static, ok := m["static"]
	if !ok {
		t.Fatal("key 'static' not found in map")
	}
	if static.Kind != skill.KindStatic {
		t.Errorf("static kind = %q, want %q", static.Kind, skill.KindStatic)
	}
	if static.Enabled {
		t.Error("static enabled = true, want false")
	}
}

// TestResolveProjectRootWithBase covers root resolution scenarios.
func TestResolveProjectRootWithBase(t *testing.T) {
	t.Run("empty root returns empty string", func(t *testing.T) {
		p := Project{Name: "test", Root: ""}
		got, err := ResolveProjectRootWithBase(p, "/tmp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("absolute root returned as-is", func(t *testing.T) {
		abs := "/usr/local/project"
		p := Project{Name: "test", Root: abs}
		got, err := ResolveProjectRootWithBase(p, "/tmp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != abs {
			t.Errorf("got %q, want %q", got, abs)
		}
	})

	t.Run("relative root resolves against baseDir", func(t *testing.T) {
		baseDir := t.TempDir()
		// Create a subdirectory so the resolved path exists.
		subDir := filepath.Join(baseDir, "myproject")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		p := Project{Name: "test", Root: "myproject"}
		got, err := ResolveProjectRootWithBase(p, baseDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := subDir
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("non-existent path returns error", func(t *testing.T) {
		baseDir := t.TempDir()
		p := Project{Name: "test", Root: "does-not-exist"}
		_, err := ResolveProjectRootWithBase(p, baseDir)
		if err == nil {
			t.Fatal("expected error for non-existent path, got nil")
		}
	})

	t.Run("existing file returns error", func(t *testing.T) {
		baseDir := t.TempDir()
		// Create a file (not a directory) at the resolved path.
		filePath := filepath.Join(baseDir, "afile")
		if err := os.WriteFile(filePath, []byte("hello"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		p := Project{Name: "test", Root: "afile"}
		_, err := ResolveProjectRootWithBase(p, baseDir)
		if err == nil {
			t.Fatal("expected error for file (not dir), got nil")
		}
	})
}

// TestHealthyTimeoutDuration covers the HealthyTimeoutDuration method on Env.
func TestHealthyTimeoutDuration(t *testing.T) {
	t.Run("empty string returns 0", func(t *testing.T) {
		e := Env{HealthyTimeout: ""}
		if got := e.HealthyTimeoutDuration(); got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})

	t.Run("valid duration string", func(t *testing.T) {
		e := Env{HealthyTimeout: "30s"}
		got := e.HealthyTimeoutDuration()
		want := 30 * time.Second
		if got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("invalid string returns 0", func(t *testing.T) {
		e := Env{HealthyTimeout: "not-a-duration"}
		if got := e.HealthyTimeoutDuration(); got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})

	t.Run("complex duration string", func(t *testing.T) {
		e := Env{HealthyTimeout: "1m30s"}
		got := e.HealthyTimeoutDuration()
		want := 90 * time.Second
		if got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

// TestEnvKindConstants verifies all EnvKind constants have the expected string values.
func TestEnvKindConstants(t *testing.T) {
	cases := []struct {
		name string
		got  EnvKind
		want string
	}{
		{"EnvKindBackendGo", EnvKindBackendGo, "backend.go"},
		{"EnvKindBackendNode", EnvKindBackendNode, "backend.node"},
		{"EnvKindFrontendVite", EnvKindFrontendVite, "frontend.vite"},
		{"EnvKindFrontendNuxt", EnvKindFrontendNuxt, "frontend.nuxt"},
		{"EnvKindCustom", EnvKindCustom, "custom"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if string(c.got) != c.want {
				t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
			}
		})
	}
}
