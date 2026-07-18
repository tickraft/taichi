# Architecture Design

> 🌐 Languages: [English](architecture.md) | [中文](architecture.zh.md)

## 1. Design Goals

Taichi is a **general-purpose automated test orchestration framework**. Core design goals:

1. **Cross-project reuse**: Not bound to any specific project; describes any service under test via configuration
2. **Skill extension**: Uses Skill as the extension unit, supporting API / gRPC / UI / Static / Regression / custom test types; third-party plugins connect via the JSON-over-stdio protocol, language-agnostic
3. **Multi-environment support**: Natively supports Go/Node binary backends, Vite/Nuxt frontends; the `custom` type supports any startup command + health check URL, covering Python/Rust/Java/Ruby and any tech stack or externally hosted service
4. **Low coupling**: Skills do not reference each other directly; shared state is passed via context
5. **Observability**: Multi-format reports (JSON / JUnit XML / summary) + auto-fix + structured logging

## 2. Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      cmd/taichi (CLI)                        │
│   run / list / validate / version / mcp / copilot           │
│   subcommands + flag parsing                                 │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                   pkg/orchestrator                           │
│                                                             │
│  Config load → Project select → Env start/stop → Skill exec │
│  → Report generation                                        │
└──────┬────────────┬─────────────┬────────────┬──────────────┘
       │            │             │            │
       ▼            ▼             ▼            ▼
┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐
│ pkg/env  │ │pkg/config│ │pkg/registry│ │pkg/report│
│ Env mgmt │ │ Config   │ │ Skill reg │ │ Report ext  │
└────┬─────┘ └──────────┘ └──────┬───┘ └──────┬───────┘
     │                          │             │
     ▼                          ▼             ▼
┌──────────┐            ┌──────────────┐  ┌──────────┐
│ backend  │            │ pkg/skill    │  │ framework│
│ frontend │            │ Interface+Ctx│  │ Assert+Rpt│
└────┬─────┘            └──────┬───────┘  └──────────┘
     │                         │
     │                         ▼
     │            ┌──────────────────────────┐
     │            │ pkg/skill/builtin        │
     │            │ builtin.Skills()         │
     │            │ api / grpc / ui /        │
     │            │ static / regression      │
     │            └──────────────────────────┘
     ▼
┌──────────────────┐
│ pkg/autofix      │
│ Error detect+fix │
└──────────────────┘
```

## 3. Module Description

### 3.1 pkg/framework — Test Core

Provides the base capabilities for test orchestration, with no dependency on business logic:

| File | Responsibility |
|------|----------------|
| `types.go` | TestCase / TestResult / TestSuite / AssertResult type definitions |
| `assert.go` | AssertionEngine: status code, JSON field, JSONPath, HTML contains, response latency assertions |
| `reporter.go` | TestReporter: result collection, JSON / JUnit XML / summary output, Snapshot |
| `lifecycle.go` | ServiceLifecycle: on-demand binary build, free-port startup, health check, stop/restart. Exports `FreePort()` as the single free-port implementation (reused by `pkg/env`) |

**Key design**: `ServiceLifecycle` is fully configured via `ServiceConfig`, with no hardcoded project paths, and can serve any Go binary.

### 3.2 pkg/autofix — Auto-Fix

| File | Responsibility |
|------|----------------|
| `detector.go` | ErrorDetector: classifies errors from HTTP responses (service unresponsive / rate-limited / 5xx / unknown) |
| `engine.go` | FixRule interface, FixEngine, 3 built-in rules (service restart / rate-limit backoff / unknown error report) |

**Key design**: `FixContext.Lifecycle` is typed as the `ServiceRestarter` interface rather than a concrete type, keeping it decoupled from framework.

### 3.3 pkg/skill — Skill Interface Contract

Defines the `TestSkill` interface that all skills must implement, and the runtime context `Context`:

- Lifecycle: `Configure → Setup → Run → Teardown`
- Priority: Critical(0) / High(10) / Normal(20) / Low(30)
- Context passing: BaseURL / Asserts / Reporter / Logger / FixEngine / Extra
- Helper functions: HTTPRequest / RecordResult / AssertCommonEnvelope / Get* config readers

### 3.4 pkg/registry — Skill Registry

Concurrency-safe skill registration, query, unregistration, config-based filtering and priority sorting:

- `Register(s, overwrite)` / `Unregister(name)` / `Get(name)` / `List()`
- `Select(configs)`: filters enabled skills by `skill.Config`, sorts ascending by Priority

### 3.5 pkg/env — Environment Management

| Implementation | Applicable Scenario |
|----------------|---------------------|
| `backend` | Go / Node backend binaries, reuses framework.ServiceLifecycle |
| `frontend` | npm/pnpm dev server, subprocess startup + wait for ready URL |
| `external` (base_url non-empty) | Externally hosted service, skips startup/shutdown |

`Manager` orchestrates the start/stop of the environment corresponding to a single project.

### 3.6 pkg/config — Config Loading

YAML config schema, loaded by viper, with five top-level structures:

```
projects: []Project      # list of projects under test
envs: map[string]Env     # environment definitions
skills: []skill.Config   # skill configs
report: Report            # report output
autofix: Autofix          # auto-fix
```

### 3.7 pkg/report — Report Extension Point

- Built-in formats (json / junit / summary): natively implemented by `framework.TestReporter`
- Custom formats: implement the `Writer` interface, register to `Registry`
- `Generate(reporter, registry, formats, pathFor)` for unified dispatch

### 3.8 pkg/orchestrator — Orchestration Core

Coordinates a 9-step flow for a complete test run:

1. Load config (`config.Load`)
2. Select project (by `--project` or the first in config)
3. Resolve project root directory (relative to config file directory)
4. Start environment (if env is configured)
5. Prepare skill config (filter by project skills + filter)
6. Create shared context resources (Reporter / Asserts / ReportsDir)
7. Create auto-fix engine (if `autofix.enabled`)
8. Execute skills sequentially (Configure → Setup → Run → Teardown, by priority)
9. Generate report

### 3.9 pkg/skill/* — Built-in Skills

Built-in skill implementations live under `pkg/skill/{api,grpc,ui,static,regression}` and are aggregated by `pkg/skill/builtin.Skills()`. (The top-level `skills/` directory holds AI Agent `SKILL.md` capability files, not Go implementations.)

| Skill | Kind | Priority | Verified Content |
|-------|------|----------|------------------|
| api | api | Critical(0) | Status code + unified response contract (code/msg/request_id) + specified field values + response latency |
| grpc | grpc | Critical(0) | Config-driven gRPC smoke checks (health / connectivity / reflection) |
| ui | ui | High(10) | Page accessibility + HTML markup + keyword contains + first-byte latency |
| static | static | Normal(20) | Pages (200+html, 404 skip) + assets (200 or 404 both pass) |
| regression | regression | Low(30) | Re-probe key path endpoints, verify fix introduced no regression |

### 3.10 cmd/taichi — CLI

| Subcommand | Purpose |
|------------|---------|
| `run` | Load config → register built-in skills → orchestrate execution → print summary → generate report |
| `list` | Show project, environment, skill config, report and auto-fix config |
| `validate` | Validate config file integrity (projects / envs / skills / uniqueness) without executing tests |
| `version` | Print version, Go runtime, target platform |
| `mcp` | Start the MCP Server (JSON-RPC over stdio), exposing taichi tools to AI Agents |
| `copilot` | Run the test → fix → regression closed loop driven by an external AI Agent via the `agent.Invoker` interface |

## 4. Dependency Relationships

```
cmd/taichi → orchestrator, skill, pkg/skill/*, config, registry
orchestrator → config, env, framework, registry, report, skill, autofix
pkg/skill/* → skill, framework
env → config, framework
report → framework
autofix → (no external deps, ServiceRestarter is an interface)
```

**Circular dependency avoidance**: `pkg/orchestrator` does not directly import `pkg/skill/*`; instead, `RegisterBuiltinSkills([]skill.TestSkill)` is called by `cmd/taichi` to pass in constructed skill instances. The canonical list of built-in skill instances is `builtin.Skills()` in `pkg/skill/builtin` — the single source of truth shared by `cmd/taichi` (run/list/copilot) and `pkg/mcp` to avoid divergence.
