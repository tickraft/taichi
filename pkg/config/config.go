// Package config defines the taichi configuration schema and loading logic.
//
// The config file uses YAML format with the following structure:
//
//	projects:
//	  - name: tickraft
//	    root: ../tickraft
//	    env: backend.go
//	    skills: [api, static, regression]
//	envs:
//	  backend.go:
//	    kind: backend.go
//	    binary: bin/tickraft
//	    build_target: ./cmd/tickraft
//	    config_path: configs/config.yaml
//	    config_flag: --config
//	    addr_flag: --addr
//	    health_path: /api/v1/health
//	  frontend.vite:
//	    kind: frontend
//	    command: pnpm dev
//	    port: 5173
//	    base_url: http://localhost:5173
//	skills:
//	  - name: api
//	    enabled: true
//	    priority: 0
//	    raw:
//	      timeout: 5s
//	report:
//	  suite_name: taichi-tests
//	  output_dir: reports
//	  formats: [json, junit, summary]
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"

	"github.com/tickraft/taichi/pkg/skill"
)

// Config is the top-level taichi config.
type Config struct {
	// Projects is the list of projects under test.
	Projects []Project `mapstructure:"projects"`
	// Envs is the map of environment definitions, keyed by env name.
	Envs map[string]Env `mapstructure:"envs"`
	// Skills is the list of skill configs.
	Skills []skill.Config `mapstructure:"skills"`
	// Report is the report output config.
	Report Report `mapstructure:"report"`
	// Autofix is the auto-fix config.
	Autofix Autofix `mapstructure:"autofix"`
	// Locale is the UI language setting. Allowed values: auto / zh-CN / en-US.
	// An empty string is equivalent to auto, which auto-detects from system environment variables.
	Locale string `mapstructure:"locale"`
}

// Project describes a project under test.
type Project struct {
	// Name is the unique identifier of the project (e.g. "tickraft").
	Name string `mapstructure:"name"`
	// Root is the project root directory (relative to the taichi working dir, or an absolute path).
	Root string `mapstructure:"root"`
	// Env is the env name used by this project, referencing a key in Envs.
	Env string `mapstructure:"env"`
	// Skills is the list of skill names enabled for this project. Empty means all configured skills.
	Skills []string `mapstructure:"skills"`
}

// EnvKind enumerates the major env categories.
type EnvKind string

const (
	// EnvKindBackendGo represents a Go backend service env.
	EnvKindBackendGo EnvKind = "backend.go"
	// EnvKindBackendNode represents a Node backend service env.
	EnvKindBackendNode EnvKind = "backend.node"
	// EnvKindFrontendVite represents a Vite frontend env.
	EnvKindFrontendVite EnvKind = "frontend.vite"
	// EnvKindFrontendNuxt represents a Nuxt frontend env.
	EnvKindFrontendNuxt EnvKind = "frontend.nuxt"
	// EnvKindCustom represents a custom env.
	EnvKindCustom EnvKind = "custom"
)

// Env describes a test environment. Backend and Frontend fields are used selectively based on Kind.
type Env struct {
	// Kind is the major env category.
	Kind EnvKind `mapstructure:"kind"`
	// Port is the fixed listening port. 0 means auto.
	Port int `mapstructure:"port"`
	// BaseURL is the base URL of an already-ready env (common for frontends, to skip startup).
	BaseURL string `mapstructure:"base_url"`

	// Backend fields (used when Kind=backend.*).
	BinaryPath     string   `mapstructure:"binary"`
	BuildTarget    string   `mapstructure:"build_target"`
	ConfigPath     string   `mapstructure:"config_path"`
	ConfigFlag     string   `mapstructure:"config_flag"`
	AddrFlag       string   `mapstructure:"addr_flag"`
	HealthPath     string   `mapstructure:"health_path"`
	HealthyTimeout string   `mapstructure:"healthy_timeout"`
	Args           []string `mapstructure:"args"`
	Env            []string `mapstructure:"env"`

	// Frontend fields (used when Kind=frontend.*).
	Command   string `mapstructure:"command"`
	Cwd       string `mapstructure:"cwd"`
	ReadyURL  string `mapstructure:"ready_url"`
	ReadyText string `mapstructure:"ready_text"`
}

// HealthyTimeoutDuration parses the healthy_timeout field into a time.Duration. Returns 0 when empty.
func (e Env) HealthyTimeoutDuration() time.Duration {
	if e.HealthyTimeout == "" {
		return 0
	}
	d, err := time.ParseDuration(e.HealthyTimeout)
	if err != nil {
		return 0
	}
	return d
}

// Report is the report output config.
type Report struct {
	// SuiteName is the JUnit XML testsuite name and testcase classname.
	SuiteName string `mapstructure:"suite_name"`
	// OutputDir is the report output directory.
	OutputDir string `mapstructure:"output_dir"`
	// Formats is the list of enabled report formats: json, junit, summary.
	Formats []string `mapstructure:"formats"`
}

// Autofix is the auto-fix config.
type Autofix struct {
	// Enabled controls whether auto-fix is enabled.
	Enabled bool `mapstructure:"enabled"`
	// ReportsDir is the error report output directory.
	ReportsDir string `mapstructure:"reports_dir"`
}

// Load loads the config from path. Supports .yaml/.yml/.json extensions.
// Returns an empty default config when path is empty.
func Load(path string) (*Config, error) {
	if path == "" {
		return &Config{}, nil
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

// validate checks the integrity of the config.
func (c *Config) validate() error {
	for i, p := range c.Projects {
		if p.Name == "" {
			return fmt.Errorf("projects[%d].name is empty", i)
		}
		if p.Env != "" {
			if _, ok := c.Envs[p.Env]; !ok {
				return fmt.Errorf("projects[%d].env %q not defined in envs", i, p.Env)
			}
		}
	}
	seenSkill := make(map[string]struct{}, len(c.Skills))
	for i, s := range c.Skills {
		if s.Name == "" {
			return fmt.Errorf("skills[%d].name is empty", i)
		}
		if _, exists := seenSkill[s.Name]; exists {
			return fmt.Errorf("skills[%d].name %q duplicated", i, s.Name)
		}
		seenSkill[s.Name] = struct{}{}
	}
	return nil
}

// ProjectByName looks up a project by name. Returns an error if not found.
func (c *Config) ProjectByName(name string) (Project, error) {
	for _, p := range c.Projects {
		if p.Name == name {
			return p, nil
		}
	}
	return Project{}, fmt.Errorf("project %q not found", name)
}

// SkillConfigsByName returns a map from skill name to skill config.
func (c *Config) SkillConfigsByName() map[string]skill.Config {
	out := make(map[string]skill.Config, len(c.Skills))
	for _, s := range c.Skills {
		out[s.Name] = s
	}
	return out
}

// ResolveProjectRootWithBase resolves the project root to an absolute path (relative to baseDir).
func ResolveProjectRootWithBase(p Project, baseDir string) (string, error) {
	if p.Root == "" {
		return "", nil
	}
	if filepath.IsAbs(p.Root) {
		return p.Root, nil
	}
	abs := filepath.Join(baseDir, p.Root)
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("project %s root %s: %w", p.Name, abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project %s root %s is not a directory", p.Name, abs)
	}
	return abs, nil
}
