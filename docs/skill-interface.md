# Skill Interface Specification

> 🌐 Languages: [English](skill-interface.md) | [中文](skill-interface.zh.md)

## 1. TestSkill Interface

All test skills must implement the `pkg/skill.TestSkill` interface:

```go
type TestSkill interface {
    // Name returns the unique skill identifier, should match SkillConfig.Name.
    Name() string
    // Kind returns the skill category.
    Kind() Kind
    // Configure receives the skill config; the skill should parse the Raw field
    // and initialize internal state at this point.
    // Returning an error indicates invalid config; the skill will be disabled.
    Configure(cfg SkillConfig) error
    // Priority returns the execution priority. When explicitly set in config,
    // the config value takes precedence.
    Priority() Priority
    // Setup runs before Run, used to prepare resources (e.g. start browser, load case set).
    Setup(ctx *SkillContext) error
    // Run is the skill execution core; reports test results via ctx.Reporter.Record.
    // Returning an error indicates a skill-level fatal error; case-level failures
    // should be expressed via TestResult.Passed=false.
    Run(ctx *SkillContext) SkillResult
    // Teardown runs after Run (whether success or failure), used to release resources.
    // The returned error is only logged and does not affect the final result.
    Teardown(ctx *SkillContext) error
}
```

Source: [pkg/skill/skill.go](../pkg/skill/skill.go)

## 2. Lifecycle

```
Configure(cfg)     ← config injection (once only)
    │
    ▼
Setup(ctx)         ← resource preparation (per run)
    │
    ▼
Run(ctx) → result  ← execution core (per run)
    │
    ▼
Teardown(ctx)      ← resource release (per run, even if Run fails)
```

- `Configure` is called once after skill registration, before execution, to parse config
- `Setup` / `Run` / `Teardown` are called on each orchestration run
- `Teardown` is called even if `Run` returns an error

## 3. Skill Categories (Kind)

| Constant | Value | Purpose |
|----------|-------|---------|
| `KindAPI` | `api` | RESTful API testing |
| `KindUI` | `ui` | UI / page testing |
| `KindStatic` | `static` | Static resource testing |
| `KindRegression` | `regression` | Regression testing |
| `KindCustom` | `custom` | User custom |

## 4. Priority

Lower numbers run first. Within a project, skills execute in ascending priority order.

| Constant | Value | Recommended Scenario |
|----------|-------|----------------------|
| `PriorityCritical` | 0 | Critical path (API smoke) |
| `PriorityHigh` | 10 | High priority (UI pages) |
| `PriorityNormal` | 20 | Normal (static resources) |
| `PriorityLow` | 30 | Deferrable (regression, long-running) |

When the `priority` field in config is non-zero, it overrides the skill's default priority.

## 5. SkillConfig View

Injected into the skill after being loaded from the taichi config file:

```go
type SkillConfig struct {
    Name     string         // unique identifier
    Kind     Kind           // category
    Enabled  bool           // whether to participate in execution
    Priority Priority       // execution order
    Raw      map[string]any // skill-specific config (parsed by the skill itself)
}
```

The `Raw` field is a config map freely defined by the skill; the skill reads it in `Configure` using helper functions like `skill.GetString` / `GetInt` / `GetDuration` / `GetBool`.

## 6. SkillContext Runtime Context

Passed during `Setup` / `Run` / `Teardown`:

```go
type SkillContext struct {
    Ctx         context.Context      // lifecycle control, supports graceful cancellation
    ProjectName string               // project under test name
    BaseURL     string               // service under test base URL
    Asserts     *framework.AssertionEngine  // assertion engine
    Reporter    *framework.TestReporter     // result collector
    ReportsDir  string               // output directory (screenshots, har, etc.)
    Logger      Logger               // structured logging
    FixEngine   FixEngineAccessor    // auto-fix (optional)
    Extra       map[string]any       // inter-skill loosely-typed data passing
}
```

**Extra usage convention**: keys should be prefixed with the skill name to avoid conflicts, e.g. `"ui.screenshot_dir"`.

## 7. SkillResult Output

```go
type SkillResult struct {
    SkillName string
    Duration  time.Duration
    Summary   framework.TestSummary  // aggregate statistics of the skill's results
    Error     error                  // non-nil indicates the skill did not fully execute
}
```

- Case-level failure: reported via `ctx.Reporter.Record(TestResult{Passed: false, ...})`
- Skill-level fatal error: expressed via `SkillResult.Error`

## 8. Registration Mechanism

### 8.1 Register via the Orchestrator

```go
o := orchestrator.New()
skills := []skill.TestSkill{
    &mySkill.Skill{},
}
o.RegisterBuiltinSkills(skills)
```

### 8.2 Operate Directly via the Registry

```go
reg := registry.NewRegistry()
reg.Register(&mySkill.Skill{}, true)   // overwrite=true
reg.Unregister("my-skill")
s, err := reg.Get("my-skill")
list := reg.List()                       // sorted by name
selected, missing := reg.Select(configs) // filter by config + sort by priority
```

The registry is concurrency-safe (`sync.RWMutex`).

## 9. Helper Functions

`pkg/skill` provides common helper functions that skills can call directly:

| Function | Purpose |
|----------|---------|
| `HTTPRequest(client, method, url, headers)` | Execute an HTTP request, returning response, body, error |
| `RecordResult(reporter, name, start, passed, msg, err)` | Record a result and auto-calculate duration |
| `AssertCommonEnvelope(asserts, body, expectedCode)` | Validate the unified response contract (code/msg/request_id) |
| `GetString(raw, key, fallback)` | Read a string from the config map |
| `GetInt(raw, key, fallback)` | Read an int from the config map |
| `GetDuration(raw, key, fallback)` | Read a duration from the config map |
| `GetBool(raw, key, fallback)` | Read a bool from the config map |

## 10. Optional Hook Interface

Implement the `Hook` interface for finer-grained lifecycle notifications:

```go
type Hook interface {
    BeforeAll(ctx *SkillContext) error  // before all skills execute
    AfterAll(ctx *SkillContext) error   // after all skills execute
}
```

## 11. Logger Interface

Skills should log via `ctx.Logger`; `fmt.Println` is prohibited:

```go
type Logger interface {
    Infof(format string, args ...any)
    Warnf(format string, args ...any)
    Errorf(format string, args ...any)
}
```

Compatible with a subset of `zap.SugaredLogger`. Use `skill.NoOpLogger{}` in test scenarios.
