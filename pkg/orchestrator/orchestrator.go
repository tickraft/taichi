// Package orchestrator is the core of taichi test orchestration.
//
// It coordinates config loading, skill registration, env start/stop, skill
// execution, and report generation. Callers only need to provide the config file
// path and optional extra skills to complete a full test run.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tickraft/taichi/pkg/autofix"
	"github.com/tickraft/taichi/pkg/config"
	"github.com/tickraft/taichi/pkg/env"
	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/i18n"
	"github.com/tickraft/taichi/pkg/registry"
	"github.com/tickraft/taichi/pkg/report"
	"github.com/tickraft/taichi/pkg/skill"
	"github.com/tickraft/taichi/pkg/skill/plugin"
)

// Options holds the run parameters of the orchestrator.
type Options struct {
	// ConfigPath is the config file path.
	ConfigPath string
	// ProjectName specifies the project name for this run. Empty means run the first project in config.
	ProjectName string
	// SkillFilter restricts this run to the specified skill names. Empty means run all skills configured for the project.
	SkillFilter []string
	// ReportsDir overrides the report output directory in config. Empty means use the config value.
	ReportsDir string
	// Logger is used to record orchestration logs. nil uses NoOpLogger.
	Logger skill.Logger
}

// Result is the output of a single orchestration run.
type Result struct {
	// ProjectName is the name of the project under test.
	ProjectName string
	// BaseURL is the base URL of the service under test.
	BaseURL string
	// Duration is the total execution time.
	Duration time.Duration
	// Summary is the aggregated test statistics.
	Summary framework.TestSummary
	// SkillResults is the execution result of each skill.
	SkillResults []skill.SkillResult
	// EnvLogPath is the env log path (if any).
	EnvLogPath string
	// ProjectRoot is the absolute path of the project under test's root directory.
	ProjectRoot string
	// Results is a snapshot of all test results (used for failure context extraction and report generation).
	Results []framework.TestResult
	// ReportsDir is the report output directory.
	ReportsDir string
}

// Orchestrator coordinates a complete test run.
type Orchestrator struct {
	registry *registry.Registry
}

// New creates an Orchestrator. The registry is empty; callers must register via
// RegisterBuiltinSkills or register themselves.
func New() *Orchestrator {
	return &Orchestrator{registry: registry.NewRegistry()}
}

// Registry returns the skill registry, allowing callers to register custom skills.
func (o *Orchestrator) Registry() *registry.Registry {
	return o.registry
}

// RegisterBuiltinSkills registers taichi builtin skill instances.
// Callers pass in the already-constructed TestSkill slice to avoid this package
// circularly depending on the skills/* subpackages.
func (o *Orchestrator) RegisterBuiltinSkills(skills []skill.TestSkill) error {
	for _, s := range skills {
		if err := o.registry.Register(s, true); err != nil {
			return fmt.Errorf("register builtin skill %s: %w", s.Name(), err)
		}
	}
	return nil
}

// Run executes one test orchestration according to opts.
func (o *Orchestrator) Run(ctx context.Context, opts Options) (Result, error) {
	start := time.Now()
	logger := opts.Logger
	if logger == nil {
		logger = skill.NoOpLogger{}
	}
	result := Result{}

	// 1. Load config.
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return result, fmt.Errorf("load config: %w", err)
	}

	// 2. Select project.
	project, err := selectProject(cfg, opts.ProjectName)
	if err != nil {
		return result, err
	}
	result.ProjectName = project.Name

	// 3. Resolve project root directory.
	configDir := filepath.Dir(opts.ConfigPath)
	projectRoot, err := config.ResolveProjectRootWithBase(project, configDir)
	if err != nil {
		return result, fmt.Errorf("resolve project root: %w", err)
	}
	result.ProjectRoot = projectRoot

	// 4. Start env (if configured).
	var baseURL string
	var envLogPath string
	if project.Env != "" {
		envCfg, ok := cfg.Envs[project.Env]
		if !ok {
			return result, fmt.Errorf("env %q not defined", project.Env)
		}
		spec := env.NewSpec(project.Env, envCfg)
		mgr, err := env.NewManager(spec, projectRoot)
		if err != nil {
			return result, fmt.Errorf("create env manager: %w", err)
		}
		logger.Infof(i18n.T("orchestrator.run.env_start"), project.Env, project.Name)
		baseURL, err = mgr.Start(ctx)
		if err != nil {
			return result, fmt.Errorf("start env: %w", err)
		}
		envLogPath = mgr.LogPath()
		result.BaseURL = baseURL
		result.EnvLogPath = envLogPath
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := mgr.Stop(stopCtx); err != nil {
				logger.Warnf(i18n.T("orchestrator.run.env_stop_error"), err)
			}
		}()
	}

	// 5. Prepare skill configs.
	skillCfgs := buildSkillConfigs(cfg, project, opts.SkillFilter)

	// 5.1 Auto-register third-party plugin skills (kind: plugin) from config.
	if err := o.registerPluginSkills(skillCfgs, logger); err != nil {
		return result, fmt.Errorf("register plugin skills: %w", err)
	}

	selected, missing := o.registry.Select(skillCfgs)
	if len(missing) > 0 {
		logger.Warnf(i18n.T("orchestrator.run.skills_missing"), strings.Join(missing, ", "))
	}
	if len(selected) == 0 {
		return result, fmt.Errorf("no skills selected for project %q", project.Name)
	}

	// 6. Create shared context resources.
	reporter := framework.NewTestReporter()
	if cfg.Report.SuiteName != "" {
		reporter.SuiteName = cfg.Report.SuiteName
	} else {
		reporter.SuiteName = "taichi-" + project.Name
	}
	asserts := framework.NewAssertionEngine()
	reportsDir := opts.ReportsDir
	if reportsDir == "" {
		reportsDir = cfg.Report.OutputDir
	}
	if reportsDir == "" {
		reportsDir = "reports"
	}
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return result, fmt.Errorf("create reports dir: %w", err)
	}

	// 7. Auto-fix engine (optional).
	var fixEngine *autofix.FixEngine
	if cfg.Autofix.Enabled {
		fixEngine = autofix.NewFixEngine(nil, cfg.Autofix.ReportsDir)
	}

	// 8. Execute skills in order: Configure → Setup → Run → Teardown.
	for _, s := range selected {
		sc, _ := findSkillConfigs(skillCfgs, s.Name())
		if err := s.Configure(sc); err != nil {
			logger.Warnf(i18n.T("orchestrator.run.skill_configure_failed"), s.Name(), err)
			continue
		}
		skillCtx := &skill.SkillContext{
			Ctx:         ctx,
			ProjectName: project.Name,
			BaseURL:     baseURL,
			Asserts:     asserts,
			Reporter:    reporter,
			ReportsDir:  reportsDir,
			Logger:      logger,
			FixEngine:   adaptFixEngine(fixEngine),
			Extra:       make(map[string]any),
		}
		if err := s.Setup(skillCtx); err != nil {
			logger.Warnf(i18n.T("orchestrator.run.skill_setup_failed"), s.Name(), err)
			continue
		}
		logger.Infof(i18n.T("orchestrator.run.skill_running"), s.Name())
		sr := s.Run(skillCtx)
		result.SkillResults = append(result.SkillResults, sr)
		if sr.Error != nil {
			logger.Errorf(i18n.T("orchestrator.run.skill_run_error"), s.Name(), sr.Error)
		}
		if err := s.Teardown(skillCtx); err != nil {
			logger.Warnf(i18n.T("orchestrator.run.skill_teardown_failed"), s.Name(), err)
		}
	}

	// 9. Generate reports.
	formats := parseFormats(cfg.Report.Formats)
	if err := report.Generate(reporter, nil, formats, func(f report.Format) string {
		ext := string(f)
		switch f {
		case report.FormatJSON:
			ext = "json"
		case report.FormatJUnit:
			ext = "xml"
		case report.FormatSummary:
			ext = "txt"
		}
		return filepath.Join(reportsDir, fmt.Sprintf("%s-%s.%s", project.Name, time.Now().Format("20060102-150405"), ext))
	}); err != nil {
		logger.Warnf(i18n.T("orchestrator.run.reports_error"), err)
	}

	result.Summary = reporter.Summary()
	result.Results = reporter.Snapshot()
	result.ReportsDir = reportsDir
	result.Duration = time.Since(start)
	return result, nil
}

// selectProject selects a project by opts.ProjectName; if empty, returns the first project in config.
func selectProject(cfg *config.Config, name string) (config.Project, error) {
	if name == "" {
		if len(cfg.Projects) == 0 {
			return config.Project{}, fmt.Errorf("no projects defined in config")
		}
		return cfg.Projects[0], nil
	}
	return cfg.ProjectByName(name)
}

// buildSkillConfigs builds the skill config list for this run.
// If filter is specified, only skills in filter are kept; otherwise the project skills field (empty means all) is used.
func buildSkillConfigs(cfg *config.Config, project config.Project, filter []string) []skill.SkillConfig {
	all := cfg.Skills
	if len(project.Skills) > 0 {
		filtered := make([]skill.SkillConfig, 0, len(project.Skills))
		want := make(map[string]struct{}, len(project.Skills))
		for _, name := range project.Skills {
			want[name] = struct{}{}
		}
		for _, s := range all {
			if _, ok := want[s.Name]; ok {
				filtered = append(filtered, s)
			}
		}
		all = filtered
	}
	if len(filter) > 0 {
		want := make(map[string]struct{}, len(filter))
		for _, name := range filter {
			want[name] = struct{}{}
		}
		filtered := make([]skill.SkillConfig, 0, len(filter))
		for _, s := range all {
			if _, ok := want[s.Name]; ok {
				filtered = append(filtered, s)
			}
		}
		all = filtered
	}
	return all
}

// findSkillConfig looks up the config for the given skill name in the config list.
func findSkillConfigs(cfgs []skill.SkillConfig, name string) (skill.SkillConfig, bool) {
	for _, c := range cfgs {
		if c.Name == name {
			return c, true
		}
	}
	return skill.SkillConfig{}, false
}

// parseFormats converts a string list to a report.Format list. Returns nil when empty to trigger defaults.
func parseFormats(formats []string) []report.Format {
	if len(formats) == 0 {
		return nil
	}
	out := make([]report.Format, 0, len(formats))
	for _, f := range formats {
		out = append(out, report.Format(f))
	}
	return out
}

// adaptFixEngine adapts *autofix.FixEngine to skill.FixEngineAccessor.
// Returns nil when e is nil.
func adaptFixEngine(e *autofix.FixEngine) skill.FixEngineAccessor {
	if e == nil {
		return nil
	}
	return fixEngineAdapter{e: e}
}

// fixEngineAdapter implements skill.FixEngineAccessor.
type fixEngineAdapter struct {
	e *autofix.FixEngine
}

// Apply implements skill.FixEngineAccessor.
func (a fixEngineAdapter) Apply(hint skill.ErrorTypeHint, payload any) skill.FixOutcome {
	detected := mapHint(hint)
	ctx, _ := payload.(*autofix.FixContext)
	r := a.e.Apply(detected, ctx)
	return skill.FixOutcome{
		Fixed:   r.Fixed,
		Retry:   r.Retry,
		Message: r.Message,
	}
}

// mapHint converts skill.ErrorTypeHint to autofix.ErrorType.
func mapHint(h skill.ErrorTypeHint) autofix.ErrorType {
	switch h {
	case skill.ErrorHintServiceDown:
		return autofix.ErrorTypeServiceDown
	case skill.ErrorHintRateLimited:
		return autofix.ErrorTypeRateLimited
	case skill.ErrorHintServerError:
		return autofix.ErrorTypeServerError
	case skill.ErrorHintUnknown:
		return autofix.ErrorTypeUnknown
	default:
		return autofix.ErrorTypeNone
	}
}

// registerPluginSkills scans skill configs and auto-creates and registers plugin skills for kind=plugin entries.
//
// Already-registered skills with the same name are not overwritten (allowing callers to pre-register overrides for plugin configs).
// Plugin skill Configure is invoked later by the orchestrator; here only the instance is created.
func (o *Orchestrator) registerPluginSkills(cfgs []skill.SkillConfig, logger skill.Logger) error {
	for _, cfg := range cfgs {
		if !cfg.Enabled {
			continue
		}
		if cfg.Kind != skill.KindPlugin {
			continue
		}
		// Skip if already registered (allows callers to pre-register overrides).
		if _, err := o.registry.Get(cfg.Name); err == nil {
			continue
		}
		p := plugin.New(cfg.Name)
		if err := o.registry.Register(p, false); err != nil {
			return fmt.Errorf("register plugin skill %q: %w", cfg.Name, err)
		}
		logger.Infof(i18n.T("orchestrator.run.plugin_registered"), cfg.Name)
	}
	return nil
}
