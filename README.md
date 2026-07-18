# <img src="assets/logo.svg" width="44" align="absmiddle" alt="Taichi logo"> Taichi · Plugin-Driven Test Orchestration Framework

[![Test](https://github.com/tickraft/taichi/actions/workflows/test.yaml/badge.svg)](https://github.com/tickraft/taichi/actions/workflows/test.yaml)
[![Release](https://github.com/tickraft/taichi/actions/workflows/release.yaml/badge.svg)](https://github.com/tickraft/taichi/actions/workflows/release.yaml)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![GitHub release](https://img.shields.io/github/v/release/tickraft/taichi?include_prereleases)](https://github.com/tickraft/taichi/releases)
[![GitHub Downloads](https://img.shields.io/github/downloads/tickraft/taichi/total?logo=github)](https://github.com/tickraft/taichi/releases)
[![GitHub Issues](https://img.shields.io/github/issues/tickraft/taichi?logo=github)](https://github.com/tickraft/taichi/issues)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen?logo=github)](CONTRIBUTING.md)

> 🌐 Languages: [English](README.md) | [中文](README.zh.md)

A language-agnostic, plugin-driven test orchestration framework — run API, gRPC, UI, static, and regression tests with a single CLI, extend via third-party skill plugins in any language through JSON-over-stdio, integrate AI agents for auto-fix loops, and ship JUnit/JSON/HTML reports.

## Why Taichi

- **One binary, many projects**: configure multiple projects (Go services, Python/Rust/Java apps, Vite frontends...) in a single YAML and run their full test suites with one command.
- **Skills as the extension unit**: five built-in skill types cover common paths; implement the `TestSkill` interface for custom Go skills, or drop in a third-party plugin (any language) via a simple JSON-over-stdio protocol — no recompilation required.
- **AI-native loop**: the `copilot` command chains test → failure analysis → agent patch → regression re-run for an autonomous fix-and-verify workflow.
- **Multi-format reports**: JSON, JUnit XML, and human-readable summary out of the box, plus a `report.Writer` extension point for custom writers (HTML, Slack, Feishu...).
- **Lifecycle-aware environment management**: start/stop backend binaries and dev servers, wait for health checks, then tear down automatically.

## Built-in Skills

| Skill | Kind | Description |
|-------|------|-------------|
| API | `api` | HTTP status, envelope (`code`/`msg`/`request_id`), field, latency assertions |
| gRPC | `grpc` | Health check, connectivity dial, reflection service discovery |
| UI | `ui` | Page reachability, HTML markers, TTFB |
| Static | `static` | Page + asset availability |
| Regression | `regression` | Re-probe critical paths after fixes |
| Plugin | `plugin` | Any external process via JSON-over-stdio (Python/Node/Shell SDKs provided) |

## Installation

### From source (requires Go 1.26+)

```bash
git clone https://github.com/tickraft/taichi.git
cd taichi
make build
# Binary: ./bin/taichi
```

### Install to GOBIN

```bash
go install github.com/tickraft/taichi/cmd/taichi@latest
```

### Download prebuilt binary

See [Releases](https://github.com/tickraft/taichi/releases) for cross-compiled binaries (Linux / macOS / Windows, amd64 / arm64) with SHA256 checksums.

## Quick Start

```bash
# 1. Build the binary
make build

# 2. Inspect the configured projects, environments, and skills
./bin/taichi list --config configs/taichi.yaml

# 3. Run all tests for the first project in the config
./bin/taichi run --config configs/taichi.yaml

# 4. Run only the api skill
./bin/taichi run --config configs/taichi.yaml --skill api

# 5. Run a specific project by name (-p)
./bin/taichi run --config configs/taichi.yaml -p tickraft

# 6. Override the report output directory
./bin/taichi run --config configs/taichi.yaml --reports-dir /tmp/taichi-reports

# 7. Validate a config file without running tests
./bin/taichi validate -c configs/taichi.yaml
```

## Minimal Configuration

A minimal config with one project, one environment, and one API skill:

```yaml
projects:
  - name: my-service
    root: ./my-service
    env: my-service-env
    skills: [api]

envs:
  my-service-env:
    kind: backend.go
    binary: bin/my-service
    build: go build -o bin/my-service ./cmd/my-service
    health_path: /api/v1/health
    healthy_timeout: 30s

skills:
  - name: api
    kind: api
    enabled: true
    raw:
      timeout: 5s
      cases:
        - name: Health
          method: GET
          path: /api/v1/health
          expected_status: 200
          expected_code: 0

report:
  suite_name: taichi-my-service
  output_dir: reports
  formats: [json, junit, summary]
```

For non-Go services, use the `custom` env kind (any startup command + health URL); see `configs/envs/` for Python / Rust / Java / Node / Ruby templates.

## Third-Party Plugin Skills

Write a test skill in any language without touching taichi source:

```yaml
skills:
  - name: my-check
    kind: plugin
    enabled: true
    raw:
      command: ./my-plugin
      args: ["--verbose"]
      timeout: 30s
      # Any extra fields are passed to the plugin via input.config
      endpoints:
        - /api/v1/custom
```

The plugin receives a JSON `PluginInput` on stdin and returns a JSON `PluginOutput` on stdout. SDKs are provided for [Python](sdks/python/), [Node](sdks/node/), and [Shell](sdks/shell/).

## AI Agent Integration

Taichi exposes an MCP server and ships skill packs so AI agents can generate configs, run tests, analyze failures, apply patches, and verify regressions end-to-end:

```
config-generator → test-runner → failure-analyzer → code-fixer → regression-runner
```

See the [Agent Integration Guide](docs/agent-integration.md) for details.

## Directory Structure

```
taichi/
├── cmd/taichi/          # Binary entry (cobra CLI)
├── pkg/                 # Public API (skill developers can import)
│   ├── framework/       # Core data models and engine
│   ├── autofix/         # Error detection and auto-fix
│   ├── env/             # Environment management (Go/Node binaries + any command process)
│   ├── registry/        # Skill registry (dynamic load/unload)
│   ├── skill/           # Skill interface contract (includes plugin subpackage)
│   ├── config/          # Config schema and loading
│   └── report/          # Report extension point
├── pkg/skill/           # Built-in skill implementations
│   ├── api/             # API testing
│   ├── grpc/            # gRPC smoke checks (health / dial / reflect)
│   ├── ui/              # UI / page testing
│   ├── static/          # Static resource testing
│   ├── regression/      # Regression testing
│   └── plugin/          # Third-party plugin skill loader
├── sdks/                # Multi-language plugin SDKs (Python / Node / Shell)
├── configs/             # Default config templates
│   ├── taichi.yaml      # Global default
│   ├── envs/            # Multi-language environment templates
│   └── plugin-demo.yaml # Plugin skill demo
├── examples/            # Plugin examples
├── skills/              # AI Agent skill packs (MCP integration)
├── docs/                # Design documents
└── reports/             # Test report output
```

## Documentation

- [Architecture Overview](docs/architecture.md)
- [Skill Interface Specification](docs/skill-interface.md)
- [Configuration Guide](docs/configuration.md)
- [Test Cases Specification](docs/test-cases.md)
- [Extension Guide](docs/extending.md)
- [Agent Integration Guide](docs/agent-integration.md)
- [Documentation Index](docs/README.md)

## Community

- [Contributing Guide](CONTRIBUTING.md) — development setup, commit conventions, PR workflow
- [Code of Conduct](CODE_OF_CONDUCT.md) — community standards
- [Security Policy](SECURITY.md) — vulnerability reporting
- [Issue Templates](.github/ISSUE_TEMPLATE/) — bug reports & feature requests
- [Discussions](https://github.com/tickraft/taichi/discussions) — Q&A and ideas

## License

Taichi is licensed under the [Apache License 2.0](LICENSE).

Contributions are accepted under the same license; no CLA is required.

---

<sub>
<b>Keywords:</b> test orchestration, automated testing, API testing, gRPC testing,
UI testing, regression testing, plugin-based testing, AI agent testing,
MCP test server, JUnit reports, continuous testing, auto-fix loop
</sub>
