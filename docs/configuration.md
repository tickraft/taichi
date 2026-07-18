# Configuration Guide

> 🌐 Languages: [English](configuration.md) | [中文](configuration.zh.md)

## 1. Configuration File Overview

Taichi uses a single YAML configuration file to describe the projects under test, environments, skills, and output. The config schema is defined in [pkg/config/config.go](../pkg/config/config.go).

Five top-level structures:

```yaml
projects: []     # list of projects under test
envs: {}         # environment definition map
skills: []       # skill config list
report: {}       # report output
autofix: {}      # auto-fix
```

## 2. Project

```yaml
projects:
  - name: tickraft              # required, unique project identifier
    root: ../../tickraft        # project root directory (relative to config file location, or absolute path)
    env: tickraft-backend       # references a key in envs
    skills: [api, regression]   # list of enabled skill names; empty means all enabled skills
```

- When `root` is empty, the service under test is externally hosted (accessed only via `env.base_url`)
- When `skills` is empty or omitted, all skills with `enabled: true` in the config are enabled

## 3. Environment (Env)

### 3.1 Environment Types (Kind)

| Kind | Value | Description |
|------|-------|-------------|
| `EnvKindBackendGo` | `backend.go` | Go backend: auto go build + start binary |
| `EnvKindBackendNode` | `backend.node` | Node backend: start node binary |
| `EnvKindFrontendVite` | `frontend.vite` | Vite frontend: start dev server |
| `EnvKindFrontendNuxt` | `frontend.nuxt` | Nuxt frontend: start dev server |
| `EnvKindCustom` | `custom` | Custom: reuse the process implementation; start any-language service via `command` and poll `ready_url` |

> **Important constraint**: env names (the `envs` map keys) **must not contain dots (`.`)**, otherwise viper treats them as nested key separators. Use hyphens (`-`) as separators, e.g. `tickraft-backend`.

### 3.2 Backend Environment Fields

```yaml
envs:
  tickraft-backend:
    kind: backend.go
    binary: bin/tickraft           # binary path (relative to project root); rebuilt on every Start when build is set
    build: go build -o bin/tickraft ./cmd/tickraft   # shell command run via `sh -c` in project root before Start; supports any build command (go build, make, etc.). Always runs to pick up source changes (critical for copilot regression). When empty, binary must already exist
    config_path: configs/config.yaml  # config file path (relative to project root)
    config_flag: --config         # config parameter name
    addr_flag: --addr             # listen address parameter name
    health_path: /api/v1/health   # health check path
    healthy_timeout: 30s          # wait-for-ready timeout
    port: 0                       # 0=auto free port; specific value fixes the port
    args: [--debug]               # additional command-line arguments
    env: [LOG_LEVEL=debug]        # additional environment variables (KEY=VALUE)
    base_url: ""                  # when non-empty, skip startup and use this URL directly (externally hosted)
```

Startup command assembly: `<binary> <config_flag> <config_path> <addr_flag> :<port> <args...>`

### 3.3 Frontend Environment Fields

```yaml
envs:
  vite-local:
    kind: frontend.vite
    command: pnpm dev             # startup command (split into program + args; double-quoted segments with spaces are preserved, e.g. `npm "run dev" --port 5173`)
    cwd: .                        # working directory (relative to project root)
    build: ""             # optional pre-start build step (shell command run via `sh -c` in cwd before command); supports any build command, e.g. `cargo build`, `./gradlew build`, `go build -o bin/app ./cmd/app`, `make build`. Empty (default) skips the build step and runs command directly.
    ready_url: http://localhost:5173  # poll this URL until 2xx/4xx
    ready_text: ""                # when non-empty, response body must contain this substring
    healthy_timeout: 60s          # wait-for-ready timeout (default 60s); also applies to custom envs
    port: 5173
    base_url: ""                  # when non-empty, skip startup
```

The frontend environment starts a subprocess, captures stdout/stderr to a log file, and polls `ready_url` until it returns a 2xx/4xx status code (and the response body contains `ready_text`, if configured). Maximum wait is governed by `healthy_timeout` (default 60s).

When `build` is non-empty, it runs via `sh -c` in the resolved `cwd` (with the same `env` applied) before `command` is launched. A non-zero exit aborts Start. This is useful for compiled languages driven via `kind: custom` (e.g. Rust, Java) so that each test run — including copilot regression rounds — exercises the latest source. When `build` is empty, Start runs `command` directly, preserving the original behavior. The `build` field has the same shell-command semantics across all env kinds — for `backend.go` it runs in the project root, for `custom`/`frontend.*` it runs in `cwd`.

### 3.4 Externally Hosted Environment

When `base_url` is non-empty, Taichi skips startup/shutdown and uses this URL to access the service under test:

```yaml
envs:
  external:
    kind: backend.go    # kind is just a placeholder
    base_url: https://api.example.com
```

## 4. Skill Configuration (skill.Config)

```yaml
skills:
  - name: api            # required, must match the skill's Name()
    kind: api            # skill category
    enabled: true        # whether enabled
    priority: 0          # execution priority (lower numbers run first)
    raw:                 # skill-specific config (parsed by the skill itself)
      timeout: 5s
      cases: [...]
```

The contents of the `raw` field are defined by each skill; see [Test Cases Specification](./test-cases.md).

### 4.1 gRPC Skill Configuration

The built-in `grpc` skill (`kind: grpc`, package `github.com/tickraft/taichi/pkg/skill/grpc`) performs config-driven gRPC smoke checks against a target service. It focuses on readiness/smoke checks; for full unary/streaming RPC testing with compiled protobuf stubs, use a third-party plugin skill (`kind: plugin`) with a small Go helper.

Fields under `raw`:

| Field | Description | Default |
|-------|-------------|---------|
| `target` | Default target address `host:port`; can be overridden per case via `case.target` | — |
| `insecure` | Use plaintext h2c (no TLS). When `false`, TLS transport credentials are used | `true` |
| `timeout` | Connection / call timeout | `5s` |
| `cases` | List of gRPC cases (see below) | — |

Each case supports the following fields:

| Field | Description |
|-------|-------------|
| `name` | Case name (required; used as the result name) |
| `type` | Case type: `health` / `dial` / `reflect` (defaults to `health`) |
| `target` | Per-case `host:port`; falls back to the skill-level `target` |
| `insecure` | Per-case TLS override (boolean); falls back to the skill-level `insecure` |
| `expected_status` | `health` only: expected serving status (`SERVING` / `NOT_SERVING` / `UNKNOWN` / `SERVICE_UNKNOWN`); defaults to `SERVING` |
| `expected_services` | `reflect` only: list of fully-qualified service names that must be exposed |
| `max_latency` | Optional latency ceiling for the case (e.g. `2s`); empty skips the latency assertion |

Case types:

- **`health`** — calls `grpc.health.v1.Health/Check` and asserts the serving status.
- **`dial`** — establishes a gRPC connection and verifies the service is reachable.
- **`reflect`** — queries `grpc.reflection.v1.ServerReflection` and asserts the exposed service list contains all `expected_services`.

Example:

```yaml
skills:
  - name: grpc
    kind: grpc
    enabled: true
    raw:
      target: 127.0.0.1:9090
      insecure: true
      timeout: 5s
      cases:
        - name: HealthServing
          type: health
          expected_status: SERVING
          max_latency: 2s
        - name: ServerReachable
          type: dial
        - name: ExposesExpectedServices
          type: reflect
          expected_services:
            - myapp.UserService
            - myapp.OrderService
```

## 5. Report Configuration (Report)

```yaml
report:
  suite_name: taichi-tickraft    # JUnit testsuite name and testcase classname
  output_dir: reports           # report output directory
  formats: [json, junit, summary]  # enabled formats; empty/omitted enables all three by default
```

| Format | Value | Output File |
|--------|-------|-------------|
| JSON | `json` | `<project>-<timestamp>.json` |
| JUnit XML | `junit` | `<project>-<timestamp>.xml` |
| Summary | `summary` | `<project>-<timestamp>.txt` (outputs to stdout when path is empty) |

## 6. Auto-Fix Configuration (Autofix)

```yaml
autofix:
  enabled: false          # disabled by default
  reports_dir: reports/errors  # error report JSON output directory
```

When enabled, on skill failure it will attempt:
1. **Service unresponsive** → restart service (up to 2 times)
2. **Rate-limited (429)** → wait backoff then notify retry
3. **5xx / unknown error** → write error report JSON for manual analysis

## 7. Environment Configuration Templates

See [`configs/taichi.example.yaml`](../configs/taichi.example.yaml) for a complete template covering all environment types and skill options.

### 7.1 Quickly Create a Project Config

1. Copy `configs/taichi.example.yaml` to your `taichi.yaml`
2. Modify `projects[0].name`, `root`, `env`
3. Define the corresponding environment under `envs`
4. Trim `skills` as needed
5. Run `taichi run -c taichi.yaml`

### 7.2 Multi-Project Configuration

A single config file can contain multiple projects, selected via `--project`:

```bash
taichi run -c taichi.yaml -p my-backend
taichi run -c taichi.yaml -p my-frontend
```

## 8. Configuration Validation Rules

Taichi automatically validates on load:

- `projects[*].name` must not be empty
- `projects[*].env` referenced key must exist in `envs`
- `skills[*].name` must not be empty and must be unique

On validation failure, a clear error message is returned and no tests are executed.

## 9. CLI Configuration Overrides

The `run` command supports config overrides:

| Flag | Purpose |
|------|---------|
| `-c, --config` | Config file path (default `configs/taichi.yaml`) |
| `-p, --project` | Project name for this run |
| `-s, --skill` | Run only the specified skill (repeatable) |
| `--reports-dir` | Override report output directory |
| `--timeout` | Total timeout for this run |
| `--log-level` | Log level (debug/info/warn/error) |
