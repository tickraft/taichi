# Framework Extension Guide

> 🌐 Languages: [English](extending.md) | [中文](extending.zh.md)

## 1. Extension Points Overview

Taichi provides four extension points, ordered by usage frequency:

| Extension Point | Interface | Package | Typical Scenario |
|-----------------|-----------|---------|------------------|
| Custom skill | `skill.TestSkill` | `pkg/skill` | Add new test types (e.g. gRPC, Playwright E2E) |
| Third-party plugin skill | Any language + JSON-over-stdio | `pkg/skill/plugin` + `sdks/` | Write test skills in Python/Node/Shell etc., no need to compile into taichi |
| Custom environment | `env.Environment` | `pkg/env` | Add environment types (e.g. Docker, K8s port-forward); `custom` type already supports any startup command |
| Custom report format | `report.Writer` | `pkg/report` | HTML reports, Slack notifications, Lark cards |
| Fix rule | `autofix.FixRule` | `pkg/autofix` | Add auto-fix strategies |

> **Built-in skills**: taichi ships five built-in skills — `api` / `grpc` / `ui` / `static` / `regression`. The `grpc` skill (`pkg/skill/grpc`, `kind: grpc`) covers config-driven smoke checks only (health / connectivity / reflection); it intentionally avoids dynamic protobuf message construction. For full unary/streaming RPC testing with compiled protobuf stubs, implement a third-party plugin skill (`kind: plugin`) with a small Go helper that imports the generated client package.

> **Plugin process environment**: plugin processes inherit all of the taichi parent process's environment variables (`os.Environ()`) plus any `env` fields configured for the plugin. Be mindful of sensitive variable isolation.

## 2. Custom Skill

### 2.1 Implement the TestSkill Interface

```go
package grpc

import (
    "time"
    "github.com/tickraft/taichi/pkg/framework"
    "github.com/tickraft/taichi/pkg/skill"
)

// Skill is the gRPC test skill.
type Skill struct {
    cfg    skill.SkillConfig
    cases  []grpcCase
    timeout time.Duration
}

type grpcCase struct {
    Service string
    Method  string
    // ...
}

// Name returns the unique skill identifier.
func (s *Skill) Name() string { return "grpc" }

// Kind returns the skill category.
func (s *Skill) Kind() skill.Kind { return skill.KindCustom }

// Priority returns the execution priority.
func (s *Skill) Priority() skill.Priority { return skill.PriorityNormal }

// Configure parses the raw config.
func (s *Skill) Configure(cfg skill.SkillConfig) error {
    s.cfg = cfg
    s.timeout = skill.GetDuration(cfg.Raw, "timeout", skill.DefaultHTTPTimeout)
    // parse cases...
    return nil
}

// Setup prepares resources (e.g. establish gRPC connection).
func (s *Skill) Setup(ctx *skill.SkillContext) error {
    return nil
}

// Run executes the test; results are reported via ctx.Reporter.Record.
func (s *Skill) Run(ctx *skill.SkillContext) skill.SkillResult {
    start := time.Now()
    for _, c := range s.cases {
        caseStart := time.Now()
        // execute gRPC call...
        passed := true
        msg := "ok"
        skill.RecordResult(ctx.Reporter, "grpc:"+c.Service, caseStart, passed, msg, nil)
    }
    return skill.SkillResult{
        SkillName: s.Name(),
        Duration:  time.Since(start),
        Summary:   ctx.Reporter.Summary(),
    }
}

// Teardown releases resources.
func (s *Skill) Teardown(ctx *skill.SkillContext) error {
    return nil
}
```

### 2.2 Register the Skill

#### Option 1: Compile into the taichi binary

Append to `builtinSkills()` in `cmd/taichi/run.go`:

```go
func builtinSkills() []skill.TestSkill {
    return []skill.TestSkill{
        &api.Skill{},
        &ui.Skill{},
        &static.Skill{},
        &regression.Skill{},
        &grpc.Skill{},  // new
    }
}
```

#### Option 2: Dynamic registration at runtime

```go
o := orchestrator.New()
o.RegisterBuiltinSkills(builtinSkills())
// dynamically register a custom skill
o.Registry().Register(&grpc.Skill{}, true)
```

#### Option 3: Unregister via the registry

```go
o.Registry().Unregister("static")  // unregister the static resource skill
```

### 2.3 Config File Integration

Add config under `skills` in `taichi.yaml`:

```yaml
skills:
  - name: grpc
    kind: custom
    enabled: true
    priority: 20
    raw:
      timeout: 10s
      cases:
        - service: helloworld.Greeter
          method: SayHello
          expected_code: 0
```

The skill name (`name`) must match the return value of `Skill.Name()`.

### 2.4 Inter-skill Data Passing

Pass loosely-typed data via the `ctx.Extra` map (prefix keys with the skill name to avoid conflicts):

```go
// producer skill
ctx.Extra["screenshot.last_dir"] = "/tmp/shots"

// consumer skill
if dir, ok := ctx.Extra["screenshot.last_dir"].(string); ok {
    // use dir
}
```

## 3. Custom Environment

### 3.1 Implement the Environment Interface

```go
type Environment interface {
    Start(ctx context.Context) (string, error)  // start and return BaseURL
    Stop(ctx context.Context) error              // stop
    BaseURL() string                             // current BaseURL
    LogPath() string                             // log path (if any)
}
```

### 3.2 Register a Custom Environment

`env.New()` dispatches by `spec.Kind`. To support a new Kind:

1. Add a constant in `pkg/config/config.go`: `EnvKindDocker EnvKind = "docker"`
2. Add a case in `pkg/env/env.go` `New()`:

```go
func New(spec EnvSpec, projectRoot string) (Environment, error) {
    switch spec.Kind {
    case config.EnvKindBackendGo, config.EnvKindBackendNode:
        return newBackend(spec, projectRoot), nil
    case config.EnvKindFrontendVite, config.EnvKindFrontendNuxt:
        return newFrontend(spec, projectRoot), nil
    case config.EnvKindDocker:
        return newDocker(spec, projectRoot), nil
    // ...
    }
}
```

3. Implement the environment type:

```go
type dockerEnv struct {
    spec        EnvSpec
    projectRoot string
    containerID string
    baseURL     string
}

func newDocker(spec EnvSpec, projectRoot string) *dockerEnv {
    return &dockerEnv{spec: spec, projectRoot: projectRoot}
}

func (d *dockerEnv) Start(ctx context.Context) (string, error) {
    // docker run -d -p <port>:<port> <image>
    // return BaseURL
}

func (d *dockerEnv) Stop(ctx context.Context) error {
    // docker stop <containerID>
}
```

## 4. Custom Report Format

### 4.1 Implement the Writer Interface

```go
type Writer interface {
    Format() report.Format
    Write(w io.Writer, summary framework.TestSummary, results []framework.TestResult) error
}
```

### 4.2 Register and Use

```go
// register in cmd/taichi/run.go
reg := report.NewRegistry()
reg.Register(&htmlWriter{})

// modify the orchestrator call, pass in the registry (requires extending Options)
```

### 4.3 HTML Report Example

```go
type htmlWriter struct{}

func (htmlWriter) Format() report.Format { return "html" }

func (htmlWriter) Write(w io.Writer, summary framework.TestSummary, results []framework.TestResult) error {
    fmt.Fprintf(w, "<html><body><h1>Test Report</h1>")
    fmt.Fprintf(w, "<p>Total: %d, Passed: %d, Failed: %d</p>",
        summary.Total, summary.Passed, summary.Failed)
    // render result table...
    fmt.Fprintf(w, "</body></html>")
    return nil
}
```

Enable in config:

```yaml
report:
  formats: [json, junit, html]
```

## 5. Custom Fix Rule

### 5.1 Implement the FixRule Interface

```go
type FixRule interface {
    Match(detected ErrorType) bool
    Apply(ctx *FixContext) FixResult
    Name() string
}
```

### 5.2 Register the Rule

```go
engine := autofix.NewFixEngine(lifecycle, reportsDir)
engine.Register(&myFixRule{})
```

### 5.3 Example: Database Reconnect Rule

```go
type DBReconnectRule struct{}

func (DBReconnectRule) Match(detected autofix.ErrorType) bool {
    return detected == autofix.ErrorTypeServerError
}

func (DBReconnectRule) Name() string { return "DBReconnectRule" }

func (DBReconnectRule) Apply(ctx *autofix.FixContext) autofix.FixResult {
    // attempt database reconnect
    if reconnectDB() {
        return autofix.FixResult{
            RuleName: "DBReconnectRule",
            Fixed:    true,
            Retry:    true,
            Message:  "database reconnected",
        }
    }
    return autofix.FixResult{RuleName: "DBReconnectRule", Fixed: false}
}
```

Rules match in registration order; the first rule whose `Match` returns true wins.

## 6. Hook Mechanism

### 6.1 Skill-level Hooks

Implement the `skill.Hook` interface to receive global lifecycle notifications:

```go
func (s *MySkill) BeforeAll(ctx *skill.SkillContext) error {
    // before all skills execute: initialize shared resources
    return nil
}

func (s *MySkill) AfterAll(ctx *skill.SkillContext) error {
    // after all skills execute: clean up shared resources
    return nil
}
```

### 6.2 Orchestrator-level Extension Points

The 9-step flow of the orchestrator's `Run` method provides natural extension points:

| Step | Extension Method |
|------|------------------|
| Config loading | Custom `config.Load` or config file preprocessing |
| Project selection | Via `--project` flag |
| Environment start/stop | Custom `Environment` implementation |
| Skill selection | Via `--skill` flag or project `skills` field |
| Skill execution | Register custom skills |
| Report generation | Register custom `report.Writer` |

## 7. Cross-Project Reuse

Taichi is a standalone Go module that can be referenced by any project:

### 7.1 Reference as a Library

```go
// in another project's go.mod
require github.com/tickraft/taichi v0.1.0
replace github.com/tickraft/taichi => ../taichi  // local development
```

```go
import (
    "github.com/tickraft/taichi/pkg/framework"
    "github.com/tickraft/taichi/pkg/skill"
)
```

### 7.2 Use as a Binary

No code integration needed; just write a config file:

```bash
taichi run -c my-project-taichi.yaml
```

### 7.3 tickraft Integration Example

The tickraft project integrates taichi as follows:
- tickraft's test runner imports `github.com/tickraft/taichi/pkg/{framework,autofix}`
- `tickraft/go.mod` references the local taichi via a `replace` directive

See [tickraft/tests/runner/main_test.go](../../tickraft/tests/runner/main_test.go).

## 8. Extension Development Checklist

- [ ] Skill `Name()` return value matches the config `name` field
- [ ] Skill `Kind()` return value matches the config `kind` field
- [ ] `Configure` correctly parses the `Raw` field, with sensible defaults for missing fields
- [ ] `Run` reports each result via `ctx.Reporter.Record`
- [ ] `Teardown` releases all resources (connections, files, subprocesses)
- [ ] Result names are prefixed with the skill name to avoid conflicts
- [ ] No `fmt.Println`; logging goes through `ctx.Logger`
- [ ] No `panic`; errors are returned via `error`
- [ ] Concurrency-safe (when using shared resources)
