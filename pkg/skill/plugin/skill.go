// Package plugin implements support for third-party test skill plugins.
//
// Plugins communicate with taichi via an external process protocol: taichi writes the
// skill context as JSON to the plugin process's stdin, the plugin executes tests and
// writes the result JSON to stdout, and logs to stderr. taichi parses the stdout JSON
// and records the results onto the reporter.
//
// Protocol contract:
//
//	stdin  → Input  (JSON)
//	stdout ← Output (JSON)
//	stderr ← free-form logs (taichi forwards them to its own logger)
//	exit 0 = plugin executed normally (pass/fail is expressed via stdout JSON)
//	exit ≠ 0 = plugin-level fatal error
//
// Configuration example:
//
//	skills:
//	  - name: my-check
//	    kind: plugin
//	    enabled: true
//	    priority: 20
//	    raw:
//	      command: ./my-plugin
//	      args: ["--verbose"]
//	      env: ["API_KEY=xxx"]
//	      workdir: .
//	      timeout: 30s
//	      # Any custom fields below are passed through to the plugin's input.config
//	      endpoints:
//	        - /api/v1/custom
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/skill"
)

// Input is the JSON structure taichi writes to the plugin process's stdin.
type Input struct {
	// SkillName is the skill name (from config skills[].name).
	SkillName string `json:"skill_name"`
	// ProjectName is the name of the project under test.
	ProjectName string `json:"project_name"`
	// BaseURL is the base URL of the service under test.
	BaseURL string `json:"base_url,omitempty"`
	// ReportsDir is the report output directory.
	ReportsDir string `json:"reports_dir,omitempty"`
	// Config is the skill's raw config (the remaining fields after removing command/args/env/workdir/timeout).
	Config map[string]any `json:"config,omitempty"`
}

// Output is the JSON structure the plugin process writes to stdout.
type Output struct {
	// Cases holds the test case results produced by the plugin.
	Cases []Case `json:"cases"`
	// Error holds a plugin-level fatal error message (non-empty means the skill did not run to completion).
	Error string `json:"error,omitempty"`
}

// Case is a single plugin test case result.
type Case struct {
	// Name is the case name.
	Name string `json:"name"`
	// Passed indicates whether the case passed.
	Passed bool `json:"passed"`
	// Skipped indicates whether the case was skipped.
	Skipped bool `json:"skipped,omitempty"`
	// Message is a human-readable description of the case result.
	Message string `json:"message,omitempty"`
	// DurationMs is the case execution duration in milliseconds.
	DurationMs int64 `json:"duration_ms,omitempty"`
	// Error is the case error detail (populated on failure).
	Error string `json:"error,omitempty"`
}

// Skill is the third-party plugin skill implementation.
//
// Each Skill instance corresponds to one plugin config. It parses command/args/env/workdir/timeout
// during Configure, and launches the plugin process and exchanges JSON during Run.
type Skill struct {
	cfg     skill.Config
	name    string
	command string
	args    []string
	env     []string
	workdir string
	timeout time.Duration
	// config is the raw config with plugin control fields removed; passed through to the plugin.
	config map[string]any
}

// New creates a plugin skill instance from the skill name and config.
// name must match skills[].name in the config.
func New(name string) *Skill {
	return &Skill{name: name}
}

// Name implements skill.TestSkill.
func (s *Skill) Name() string { return s.name }

// Kind implements skill.TestSkill.
func (s *Skill) Kind() skill.Kind { return skill.KindPlugin }

// Configure implements skill.TestSkill.
// It extracts the control fields command/args/env/workdir/timeout from raw;
// the remaining fields are kept as the plugin's business config in s.config.
func (s *Skill) Configure(cfg skill.Config) error {
	s.cfg = cfg
	raw := cfg.Raw
	if raw == nil {
		return fmt.Errorf("plugin skill %q: raw.command is required", s.name)
	}

	cmd, ok := raw["command"]
	if !ok {
		return fmt.Errorf("plugin skill %q: raw.command is required", s.name)
	}
	cmdStr, ok := cmd.(string)
	if !ok || cmdStr == "" {
		return fmt.Errorf("plugin skill %q: raw.command must be a non-empty string", s.name)
	}
	s.command = cmdStr

	if v, ok := raw["args"]; ok {
		if arr, ok := v.([]any); ok {
			s.args = make([]string, 0, len(arr))
			for _, a := range arr {
				if str, ok := a.(string); ok {
					s.args = append(s.args, str)
				}
			}
		}
	}

	if v, ok := raw["env"]; ok {
		if arr, ok := v.([]any); ok {
			s.env = make([]string, 0, len(arr))
			for _, e := range arr {
				if str, ok := e.(string); ok {
					s.env = append(s.env, str)
				}
			}
		}
	}

	if v, ok := raw["workdir"]; ok {
		if str, ok := v.(string); ok {
			s.workdir = str
		}
	}

	s.timeout = skill.GetDuration(raw, "timeout", 5*time.Minute)

	// Extract remaining fields as the plugin's business config.
	s.config = make(map[string]any, len(raw))
	for k, v := range raw {
		switch k {
		case "command", "args", "env", "workdir", "timeout":
			continue
		default:
			s.config[k] = v
		}
	}
	return nil
}

// Priority implements skill.TestSkill.
func (s *Skill) Priority() skill.Priority { return skill.PriorityNormal }

// Setup implements skill.TestSkill. Plugin skills have no extra resources to prepare; return directly.
func (s *Skill) Setup(ctx *skill.Context) error { return nil }

// Run implements skill.TestSkill. It launches the plugin process, exchanges JSON, and records results onto the reporter.
func (s *Skill) Run(ctx *skill.Context) skill.Result {
	start := time.Now()
	logger := ctx.Logger
	if logger == nil {
		logger = skill.NoOpLogger{}
	}

	input := Input{
		SkillName:   s.name,
		ProjectName: ctx.ProjectName,
		BaseURL:     ctx.BaseURL,
		ReportsDir:  ctx.ReportsDir,
		Config:      s.config,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return skill.Result{
			SkillName: s.name,
			Duration:  time.Since(start),
			Error:     fmt.Errorf("marshal plugin input: %w", err),
		}
	}

	// Build the plugin process.
	cmdCtx := ctx.Ctx
	if s.timeout > 0 {
		var cancel context.CancelFunc
		cmdCtx, cancel = context.WithTimeout(ctx.Ctx, s.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(cmdCtx, s.command, s.args...)
	cmd.Stdin = bytesReader(inputJSON)
	cmd.Stderr = newLogWriter(logger, s.name)

	// Resolve workdir.
	cwd := s.workdir
	if cwd != "" && !filepath.IsAbs(cwd) && ctx.ReportsDir != "" {
		// Relative paths are resolved against the parent of reportsDir (i.e. the project root).
		cwd = filepath.Join(filepath.Dir(ctx.ReportsDir), cwd)
	}
	if cwd != "" {
		cmd.Dir = cwd
	}
	if len(s.env) > 0 {
		cmd.Env = append(os.Environ(), s.env...)
	}

	// Run the plugin and capture stdout.
	stdout, err := cmd.Output()
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return skill.Result{
				SkillName: s.name,
				Duration:  time.Since(start),
				Error:     fmt.Errorf("plugin %q timeout after %s", s.name, s.timeout),
			}
		}
		return skill.Result{
			SkillName: s.name,
			Duration:  time.Since(start),
			Error:     fmt.Errorf("plugin %q execution failed: %w", s.name, err),
		}
	}

	// Parse the plugin output.
	var output Output
	if err := json.Unmarshal(stdout, &output); err != nil {
		return skill.Result{
			SkillName: s.name,
			Duration:  time.Since(start),
			Error:     fmt.Errorf("plugin %q stdout is not valid JSON: %w", s.name, err),
		}
	}

	if output.Error != "" {
		return skill.Result{
			SkillName: s.name,
			Duration:  time.Since(start),
			Error:     fmt.Errorf("plugin %q: %s", s.name, output.Error),
		}
	}

	// Record plugin case results onto the reporter.
	for _, c := range output.Cases {
		name := c.Name
		if name == "" {
			name = s.name
		}
		var caseErr error
		if c.Error != "" {
			caseErr = fmt.Errorf("%s", c.Error)
		}
		ctx.Reporter.Record(framework.TestResult{
			Name:     name,
			Passed:   c.Passed,
			Skipped:  c.Skipped,
			Message:  c.Message,
			Duration: time.Duration(c.DurationMs) * time.Millisecond,
			Error:    caseErr,
		})
	}

	summary := ctx.Reporter.Summary()
	return skill.Result{
		SkillName: s.name,
		Duration:  time.Since(start),
		Summary:   summary,
	}
}

// Teardown implements skill.TestSkill. Plugin skills have no extra resources to clean up.
func (s *Skill) Teardown(ctx *skill.Context) error { return nil }
