// Package skill defines the interface contract and context types for test skills (TestSkill).
//
// A skill is the extension unit of the taichi test orchestration framework: each skill
// is responsible for one category of tests (API / UI / Static / Regression / custom),
// implements the TestSkill interface, and registers into pkg/registry. Both the taichi
// binary and external projects can extend capabilities by implementing this interface.
//
// Design principles:
//   - Low coupling between skills: skills should not reference each other directly; all shared state flows through SkillContext.
//   - Config-driven: skills receive configuration via SkillConfig, avoiding hardcoding.
//   - Hook mechanism: Setup/Teardown provide lifecycle hooks; Run is the execution core.
package skill

import (
	"context"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
)

// Kind enumerates the major skill categories, used for config filtering and documentation grouping.
type Kind string

const (
	// KindAPI indicates an API test skill (REST / GraphQL over HTTP).
	KindAPI Kind = "api"
	// KindGRPC indicates a gRPC test skill (health / connectivity / reflection smoke checks).
	KindGRPC Kind = "grpc"
	// KindUI indicates a UI / page test skill.
	KindUI Kind = "ui"
	// KindStatic indicates a static-asset test skill.
	KindStatic Kind = "static"
	// KindRegression indicates a regression test skill.
	KindRegression Kind = "regression"
	// KindPlugin indicates a third-party plugin skill, loaded via an external process protocol.
	KindPlugin Kind = "plugin"
	// KindCustom indicates a user-defined skill.
	KindCustom Kind = "custom"
)

// Priority enumerates skill execution priorities, determining the run order of skills within a project.
type Priority int

const (
	// PriorityCritical runs first (P0 critical path).
	PriorityCritical Priority = 0
	// PriorityHigh is high priority (P1).
	PriorityHigh Priority = 10
	// PriorityNormal is normal priority (P2).
	PriorityNormal Priority = 20
	// PriorityLow is low priority (P3, may be deferred or tolerate failures).
	PriorityLow Priority = 30
)

// SkillConfig is the configuration view of a skill, injected after the taichi config file is loaded.
// Skill implementations parse the Raw field into their own defined structures as needed.
type SkillConfig struct {
	// Name is the unique identifier of the skill, e.g. "api", "ui".
	Name string
	// Kind is the major skill category.
	Kind Kind
	// Enabled controls whether the skill participates in execution.
	Enabled bool
	// Priority determines execution order; lower values run first.
	Priority Priority
	// Raw is the original map of skill-specific config, parsed by the skill implementation itself.
	Raw map[string]any
}

// SkillContext carries runtime state during skill execution: the base URL of the service
// under test, the assertion engine, the reporter, the auto-fix engine, logger, and context cancellation.
//
// Skills should access shared resources through SkillContext rather than global variables,
// ensuring low coupling and testability.
type SkillContext struct {
	// Ctx controls the skill execution lifecycle and supports graceful cancellation.
	Ctx context.Context
	// ProjectName is the name of the project under test (e.g. "tickraft").
	ProjectName string
	// BaseURL is the base URL of the service under test (e.g. "http://localhost:6153"). May be empty for frontend skills.
	BaseURL string
	// Asserts is the assertion engine; skills use it to validate responses.
	Asserts *framework.AssertionEngine
	// Reporter collects test results; each skill should Record every result onto it.
	Reporter *framework.TestReporter
	// ReportsDir is the skill output directory (for screenshots, har files, etc.).
	ReportsDir string
	// Logger is the structured logger (zap.SugaredLogger-style interface).
	Logger Logger
	// FixEngine is optionally invoked by the skill to attempt an auto-fix when a failure occurs.
	FixEngine FixEngineAccessor
	// Extra allows skills to pass weakly-typed data between each other via a map. Keys should be prefixed with the skill name to avoid collisions (e.g. "ui.screenshot_dir").
	Extra map[string]any
}

// Logger is the structured logging interface used by skills, compatible with a subset of zap.SugaredLogger.
// Skill implementations should log through this interface; direct fmt.Println is prohibited.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// FixEngineAccessor is the interface through which skills invoke auto-fix.
// It decouples from *autofix.FixEngine so skills can run in environments without autofix.
type FixEngineAccessor interface {
	// Apply attempts to fix the given error type and returns the fix outcome.
	Apply(detected ErrorTypeHint, payload any) FixOutcome
}

// ErrorTypeHint aligns with autofix.ErrorType but is exposed as a string contract,
// preventing the skill package from directly depending on autofix (preserving low coupling).
type ErrorTypeHint string

const (
	// ErrorHintNone indicates no service-level error.
	ErrorHintNone ErrorTypeHint = "none"
	// ErrorHintServiceDown indicates the service is unresponsive.
	ErrorHintServiceDown ErrorTypeHint = "service_down"
	// ErrorHintRateLimited indicates the service returned 429.
	ErrorHintRateLimited ErrorTypeHint = "rate_limited"
	// ErrorHintServerError indicates the service returned 5xx.
	ErrorHintServerError ErrorTypeHint = "server_error"
	// ErrorHintUnknown indicates an unclassified error.
	ErrorHintUnknown ErrorTypeHint = "unknown"
)

// FixOutcome describes the outcome of an auto-fix attempt, aligned with autofix.FixResult.
type FixOutcome struct {
	// Fixed is true when the error has been fixed.
	Fixed bool
	// Retry is true when the original operation should be retried.
	Retry bool
	// Message is a human-readable description.
	Message string
}

// SkillResult is the aggregated output of a skill execution.
type SkillResult struct {
	// SkillName is the skill name.
	SkillName string
	// Duration is the total execution time of the skill.
	Duration time.Duration
	// Summary is the aggregated statistics of results produced by this skill.
	Summary framework.TestSummary
	// Error holds a fatal error that occurred during skill execution (if any). A non-nil value means the skill did not run to completion.
	Error error
}

// TestSkill is the interface that all test skills must implement.
//
// Lifecycle: Setup → Run → Teardown. Setup and Teardown are invoked even when Run fails.
type TestSkill interface {
	// Name returns the unique identifier of the skill; it should match SkillConfig.Name.
	Name() string
	// Kind returns the major skill category.
	Kind() Kind
	// Configure receives the skill config; the skill should parse the Raw field and initialize internal state here.
	// Returning an error indicates invalid config and the skill will be disabled.
	Configure(cfg SkillConfig) error
	// Priority returns the execution priority. When explicitly set in config, the config value takes precedence.
	Priority() Priority
	// Setup runs before Run and is used to prepare resources (e.g. start a browser, load a case set).
	Setup(ctx *SkillContext) error
	// Run is the execution core of the skill; test results are reported via ctx.Reporter.Record.
	// Returning an error indicates a skill-level fatal error; per-case failures should be expressed via TestResult.Passed=false.
	Run(ctx *SkillContext) SkillResult
	// Teardown runs after Run (whether it succeeded or failed) and is used to release resources.
	// Any returned error is only logged and does not affect the final result.
	Teardown(ctx *SkillContext) error
}

// Hook is an optional hook interface. Skills implementing it receive finer-grained lifecycle notifications.
type Hook interface {
	// BeforeAll is invoked once before all skills execute.
	BeforeAll(ctx *SkillContext) error
	// AfterAll is invoked once after all skills execute.
	AfterAll(ctx *SkillContext) error
}

// NoOpLogger is a no-op implementation of the Logger interface, convenient for tests or no-log scenarios.
type NoOpLogger struct{}

// Infof implements the Logger interface.
func (NoOpLogger) Infof(string, ...any) {}

// Warnf implements the Logger interface.
func (NoOpLogger) Warnf(string, ...any) {}

// Errorf implements the Logger interface.
func (NoOpLogger) Errorf(string, ...any) {}
