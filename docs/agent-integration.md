# Taichi & AI Agent Integration Guide

> 🌐 Languages: [English](agent-integration.md) | [中文](agent-integration.zh.md)

> This document describes the bidirectional integration architecture, calling interfaces, and fix loop between the Taichi test orchestration framework and AI Agents (e.g. Trae IDE Agent, custom agents).

## 1. Overview

The integration between Taichi and AI Agents is **bidirectional**, with the two directions complementing each other to form a fully automated closed loop of "test → analyze → fix → regression". The failure context (`Context`) serves as the information exchange contract between both sides.

### Direction 1: taichi as MCP Server called by AI Agent (Agent-driven)

The AI Agent takes the lead, calling taichi-exposed MCP tools on demand to run tests, list configs, read failures, and run regressions. The Agent orchestrates the analysis, fix, and verification flow itself.

- Applicable: Agent workflows that need to flexibly trigger tests, where the Agent has full analyze-fix capability
- Protocol: MCP (Model Context Protocol) or CLI subprocess invocation
- Information flow: Agent → taichi (call tools) → Taichi returns results → Agent decides

### Direction 2: Taichi calls AI Agent for fixes during orchestration (taichi-driven copilot mode)

Taichi takes the lead, proactively calling the AI Agent via the `agent.Invoker` interface for analysis and fixes on test failure, then auto-running regression, looping until passing or exhausting rounds.

- Applicable: CI / command-line scenarios where Taichi should self-drive the "test-fix-regression" loop
- Protocol: CLI (stdin/stdout JSON) or HTTP API
- Information flow: taichi → Agent (pass `Context`) → Agent returns `FixResult` → Taichi applies and regresses

### Failure Context: Information Exchange Contract

Regardless of direction, Taichi and the Agent use `Context` JSON as the standard carrier for failure information, and `FixResult` JSON as the standard carrier for fix output. Both are defined in:

- `pkg/failure/failure.go`: `Context` / `FailedCase`
- `pkg/agent/agent.go`: `FixResult` / `FixMode` / `Invoker` interface

## 2. Architecture Diagram

```
┌─────────────┐     MCP/CLI      ┌─────────────┐
│  AI Agent   │ ◄──────────────► │   taichi    │
│  (Trae/     │                  │  (MCP Server │
│   Custom)   │                  │   + Runner)  │
└──────┬──────┘                  └──────┬───────┘
       │                               │
       │  AnalyzeAndFix                │  Run Tests
       ▼                               ▼
┌─────────────┐               ┌─────────────┐
│  Failure    │ ◄── JSON ───  │  Test       │
│  Context    │               │  Results    │
└─────────────┘               └─────────────┘
       │                               ▲
       │  FixResult (patch/direct)     │
       └───────────────────────────────┘
```

**Closed-loop flow**:

```
taichi run (test)
   │
   ├─ all pass ──► end (success)
   │
   └─ has failures ──► generate failure context ──► Agent.AnalyzeAndFix
                                                    │
                                                    ▼
                                           apply FixResult
                                           (git apply / verify)
                                                    │
                                                    ▼
                                           taichi run (regression)
                                                    │
                                                    ├─ pass ──► end (fix succeeded)
                                                    │
                                                    └─ still failing ──► next round (≤ max-rounds)
```

## 3. MCP Server Usage

Taichi can run as an MCP Server, exposing test orchestration capabilities to AI Agents.

### 3.1 Start the MCP Server

```bash
taichi mcp -c configs/taichi.yaml
```

`-c` specifies the config file; the MCP Server listens for MCP protocol messages on stdio for Agent clients to connect.

### 3.2 Exposed Tools

| Tool Name | Purpose | Key Parameters | Return Format |
|-----------|---------|----------------|---------------|
| `taichi_run` | Execute a test orchestration | `config_path`, `project`, `skills`, `timeout` | Test result summary (see 3.3) |
| `taichi_list` | List projects, environments, and registered skills in config | `config_path` | Project/env/skill listing |
| `taichi_failures` | Read the failure context of the most recent run | `config_path`, `reports_dir` | `Context` JSON |
| `taichi_regression` | Run regression tests (only the `regression` skill) | `config_path`, `project`, `timeout` | Test result summary |

### 3.3 Tool Parameters and Return Formats

#### taichi_run

Request parameters:

```json
{
  "config_path": "configs/taichi.yaml",
  "project": "tickraft",
  "skills": ["api", "ui"],
  "timeout": "30m"
}
```

Return (test result summary):

```json
{
  "project": "tickraft",
  "base_url": "http://127.0.0.1:8080",
  "duration": "12.34s",
  "summary": {
    "total": 24,
    "passed": 22,
    "failed": 2,
    "skipped": 0
  },
  "env_log": "/tmp/taichi-tickraft-env.log",
  "skill_results": [
    {
      "skill": "api",
      "duration": "3.21s",
      "summary": { "total": 10, "passed": 10, "failed": 0, "skipped": 0 },
      "error": null
    }
  ]
}
```

#### taichi_list

Request parameters:

```json
{ "config_path": "configs/taichi.yaml" }
```

Returns a listing of projects, environments, and registered skills (name, type, enabled status, priority).

#### taichi_failures

Request parameters:

```json
{ "config_path": "configs/taichi.yaml", "reports_dir": "reports" }
```

Returns the `Context` JSON of the most recent run (structure in [Section 7](#7-failure-context-format)). Returns an empty object if no failures.

#### taichi_regression

Request parameters:

```json
{
  "config_path": "configs/taichi.yaml",
  "project": "tickraft",
  "timeout": "15m"
}
```

Returns a test result summary in the same format as `taichi_run`; `skill_results` contains only the `regression` entry.

## 4. Copilot Usage

The taichi-driven copilot mode is triggered via the `taichi copilot` command, automatically completing "test → failure → Agent fix → regression test → loop".

### 4.1 CLI Invocation

```bash
taichi copilot -c configs/taichi.yaml \
  --agent-cli trae \
  --agent-args "agent fix" \
  --max-rounds 3
```

### 4.2 Flow

1. **Test**: Execute a complete test orchestration (equivalent to `taichi run`)
2. **Failure determination**: If all pass, return success directly; if there are failures, enter the fix loop
3. **Build failure context**: Encapsulate failed cases into `Context`, written to `reports/failures-round-<N>-<timestamp>.json`
4. **Call Agent**: Pass `Context` to the AI Agent via `agent.Invoker` for analysis and fix
5. **Apply fix**:
   - `patch` mode: `git apply` the unified diff generated by the Agent
   - `direct` mode: verify that the files modified by the Agent exist and are readable
6. **Regression test**: Re-run tests to verify the fix effect
7. **Loop or end**:
   - Regression passes → mark `Fixed=true`, end
   - Regression still has failures → round +1, go back to step 3 (not exceeding `max-rounds`)
   - Exhaust rounds or Agent returns `fixed=false` → return final failure result

### 4.3 Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `-c, --config` | Config file path | `configs/taichi.yaml` |
| `--agent-cli` | AI Agent CLI executable (e.g. `trae`, `python3`) | required |
| `--agent-args` | Arguments passed to the Agent command | empty |
| `--max-rounds` | Maximum fix rounds | `3` |
| `--project` | Project under test | first project in config |
| `--timeout` | Single round test timeout | unlimited |
| `--log-level` | Log level | `info` |

> Implementation: `RunCopilot` in [`pkg/orchestrator/copilot.go`](../pkg/orchestrator/copilot.go). Default max rounds constant `defaultMaxRounds = 3`.

## 5. Agent Invoker Implementation

Taichi abstracts AI Agent invocation via the `agent.Invoker` interface, with two built-in implementations and custom support.

### 5.1 Invoker Interface

```go
// defined in pkg/agent/agent.go
type Invoker interface {
    // AnalyzeAndFix analyzes the failure context and returns a fix result.
    AnalyzeAndFix(ctx context.Context, fc *failure.Context) (*FixResult, error)
    // Name returns a human-readable name for the invoker.
    Name() string
}
```

### 5.2 CLIInvoker: Command-line Invocation

Invokes the AI Agent via subprocess. The Agent script must **read `Context` JSON from stdin and output `FixResult` JSON to stdout**.

```go
invoker := &agent.CLIInvoker{
    Command: "trae",            // Agent executable
    Args:    []string{"agent", "fix"},  // Agent arguments
    Timeout: 5 * time.Minute,   // single call timeout, 0 means default 5 minutes
    WorkDir: "/path/to/project", // command working directory, empty means current directory
}
```

Contract:
- stdin: `Context` JSON (compact format)
- stdout: `FixResult` JSON (compact format)
- stderr: Agent free output (taichi only logs on error)
- exit code: non-zero is treated as call failure

### 5.3 HTTPInvoker: HTTP API Invocation

Invokes the AI Agent service via HTTP POST.

```go
invoker := &agent.HTTPInvoker{
    Endpoint: "https://agent.example.com/api/fix",  // Agent HTTP endpoint
    Token:    "bearer-token",        // Bearer auth token (optional)
    Timeout:  5 * time.Minute,       // request timeout, 0 means default 5 minutes
    Client:   customHTTPClient,      // custom HTTP client (optional)
}
```

Contract:
- Request: `POST <Endpoint>`, `Content-Type: application/json`, body is `Context` JSON
- Auth: if `Token` is non-empty, append `Authorization: Bearer <Token>` header
- Response: HTTP 200, body is `FixResult` JSON; non-200 is treated as failure

### 5.4 Custom Invoker

Implement the `agent.Invoker` interface to connect any Agent backend (e.g. gRPC, message queue, built-in LLM call):

```go
type MyInvoker struct{}

func (m *MyInvoker) Name() string { return "my-agent" }

func (m *MyInvoker) AnalyzeAndFix(ctx context.Context, fc *failure.Context) (*agent.FixResult, error) {
    // 1. serialize fc and send to self-hosted Agent service
    // 2. wait for Agent analysis and fix
    // 3. parse response and construct FixResult
    return &agent.FixResult{
        Fixed:         true,
        Mode:          agent.FixModePatch,
        Patch:         patchContent,
        ModifiedFiles: []string{"internal/handler/home.go"},
        Message:       "fixed nil dereference",
        Analysis:      "home.go:42 added nil check",
    }, nil
}
```

Inject into copilot orchestration:

```go
result, err := o.RunCopilot(ctx, orchestrator.CopilotOptions{
    Options:    opts,
    MaxRounds:  3,
    Invoker:    &MyInvoker{},
})
```

## 6. Fix Modes

Taichi applies the Agent's fix output via `agent.PatchApplier`, supporting two modes.

### 6.1 patch mode

The Agent generates a unified diff patch; Taichi applies it to the project source.

- **Application method**: prefer `git apply --whitespace=fix`, fall back to `patch -p1 --no-backup-if-mismatch` on failure
- **Path resolution**: paths in the patch have `a/` `b/` prefixes (git diff style), resolved relative to `project_root`
- **Rollback**: can roll back the working tree via `git checkout`

`FixResult` example:

```json
{
  "fixed": true,
  "mode": "patch",
  "patch": "--- a/internal/handler/home.go\n+++ b/internal/handler/home.go\n@@ -39,6 +39,9 @@\n func HomeHandler(c *Context) {\n+    if c.Config == nil {\n+        c.Config = DefaultConfig()\n+    }\n     theme := c.Config.Theme\n }\n",
  "modified_files": ["internal/handler/home.go"],
  "message": "fixed null pointer dereference"
}
```

### 6.2 direct mode

The Agent directly modifies source files via file editing tools; Taichi only verifies the modification is valid (file exists and is readable, not a directory).

- **Applicable**: Agents with file editing capability (e.g. in-IDE Agent)
- **Verification**: Taichi calls `PatchApplier.VerifyDirectFix` to check the `modified_files` list
- **No write intervention**: Taichi does not modify any files, only validates

`FixResult` example:

```json
{
  "fixed": true,
  "mode": "direct",
  "modified_files": ["internal/handler/home.go", "internal/config/default.go"],
  "message": "added default theme fallback when config is missing"
}
```

> Implementation: `PatchApplier.ApplyResult` in [`pkg/agent/patch.go`](../pkg/agent/patch.go) dispatches by `Mode`.

## 7. Failure Context Format

`Context` is the information exchange contract between Taichi ↔ AI Agent, defined in [`pkg/failure/failure.go`](../pkg/failure/failure.go).

### 7.1 JSON Structure

```json
{
  "project_name": "tickraft",
  "base_url": "http://127.0.0.1:8080",
  "timestamp": "2026-07-17T10:30:00Z",
  "project_root": "/Users/derekray/workspace/auzekalabs/tickraft/tickraft",
  "env_log_path": "/tmp/taichi-tickraft-env.log",
  "reports_dir": "/Users/derekray/workspace/auzekalabs/tickraft/taichi/reports",
  "total_cases": 24,
  "passed_cases": 22,
  "failed_cases": [
    {
      "skill_name": "ui",
      "name": "ui:home_page_render",
      "message": "expected status 200, got 500",
      "error": "GET http://127.0.0.1:8080/ returned 500",
      "duration": "1.23s"
    }
  ]
}
```

### 7.2 Field Descriptions

| Field | Type | Description |
|-------|------|-------------|
| `project_name` | string | Project under test name |
| `base_url` | string | Service under test base URL (empty when environment not started) |
| `timestamp` | string | Generation time (RFC3339, UTC) |
| `project_root` | string | Absolute path to project root; Agent uses this to locate source |
| `env_log_path` | string | Environment (service) log path; Agent uses this to inspect server errors |
| `reports_dir` | string | Report output directory, contains JSON / JUnit XML / summary |
| `total_cases` | int | Total case count |
| `passed_cases` | int | Pass count |
| `failed_cases[]` | array | Failed case details |
| `failed_cases[].skill_name` | string | Skill name (`api` / `ui` / `static` / `regression`) |
| `failed_cases[].name` | string | Case identifier |
| `failed_cases[].message` | string | Human-readable failure description |
| `failed_cases[].error` | string | Underlying error string (if any) |
| `failed_cases[].duration` | string | Case execution duration |

### 7.3 Generation and Consumption

- **Generation**: When tests have failures, Taichi builds from the test result snapshot via `failure.FromResults`, written to `reports/failures-round-<N>-<timestamp>.json` via `WriteToFile`
- **Consumption**: Agent reads via file path, or receives via `CLIInvoker` stdin / `HTTPInvoker` body
- **Serializable**: `Context` implements JSON serialization/deserialization; `ReadFromFile` can restore from a file

## 8. Skill File Index

`taichi/skills/` contains SKILL.md files for AI Agents, describing the capability contract of the Agent at each stage of the integration loop. Via `scripts/link-skills.sh`, they can be symlinked to `.trae/skills/` for Trae IDE Agent discovery.

| Skill Directory | Name | Purpose | Loop Stage |
|-----------------|------|---------|------------|
| [`skills/config-generator/`](../skills/config-generator/SKILL.md) | `taichi-config-generator` | Analyzes project source to auto-generate taichi.yaml config | Config initialization |
| [`skills/test-runner/`](../skills/test-runner/SKILL.md) | `taichi-test-runner` | Executes automated tests via CLI or MCP, produces test result summary | Test execution |
| [`skills/failure-analyzer/`](../skills/failure-analyzer/SKILL.md) | `taichi-failure-analyzer` | Reads failure context, analyzes failure root cause with logs and source | Root cause analysis |
| [`skills/code-fixer/`](../skills/code-fixer/SKILL.md) | `taichi-code-fixer` | Produces fixes from failure context (patch or direct mode) | Code fix |
| [`skills/regression-runner/`](../skills/regression-runner/SKILL.md) | `taichi-regression-runner` | Runs regression tests after a fix, verifying no new issues introduced | Regression verification |

### Loop Orchestration

```
taichi-config-generator ──► taichi-test-runner ──fail──► taichi-failure-analyzer ──► taichi-code-fixer ──► taichi-regression-runner
         ▲                          ▲                                                                                    │
         │ first onboarding         └──────────────────────── regression still fails, back to start ────────────────┘
         └──────── rebuild config after major refactor ──────────────────────────────────────────────────────────────┘
```

### Symlink to Trae IDE

```bash
# symlink taichi/skills/ Skills to .trae/skills/ so Trae Agent can discover them
bash taichi/scripts/link-skills.sh
```

After symlinking, Skills are named `taichi-<skill-name>` under `.trae/skills/` (e.g. `taichi-test-runner`), and the Trae IDE AI Agent can invoke them on demand.
