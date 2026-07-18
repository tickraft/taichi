# Taichi Documentation Index

> 🌐 Languages: [English](README.md) | [中文](README.zh.md)

> Taichi is a general-purpose automated test orchestration framework.

## Documentation Directory

| Document | Content | Languages |
|----------|---------|-----------|
| [Architecture Design](./architecture.md) | Overall architecture diagram, module breakdown, core flows, dependency relationships | [EN](./architecture.md) / [中文](./architecture.zh.md) |
| [Skill Interface Specification](./skill-interface.md) | TestSkill interface contract, lifecycle, context, priority, registration mechanism | [EN](./skill-interface.md) / [中文](./skill-interface.zh.md) |
| [Configuration Guide](./configuration.md) | Config file schema, environment types, field descriptions, template examples | [EN](./configuration.md) / [中文](./configuration.zh.md) |
| [Test Cases Specification](./test-cases.md) | Built-in skill case writing, assertion rules, unified response contract, best practices | [EN](./test-cases.md) / [中文](./test-cases.zh.md) |
| [Extension Guide](./extending.md) | Custom skills, custom environments, custom report formats, hook mechanism | [EN](./extending.md) / [中文](./extending.zh.md) |
| [Agent Integration Guide](./agent-integration.md) | Bidirectional integration with AI Agents, MCP Server, Copilot mode, failure context | [EN](./agent-integration.md) / [中文](./agent-integration.zh.md) |

## Quick Links

- Config file examples: [`configs/taichi.yaml`](../configs/taichi.yaml), [`configs/taichi.example.yaml`](../configs/taichi.example.yaml)
- Skill interface definition: [`pkg/skill/skill.go`](../pkg/skill/skill.go)
- Orchestrator core: [`pkg/orchestrator/orchestrator.go`](../pkg/orchestrator/orchestrator.go)
- CLI entry: [`cmd/taichi/`](../cmd/taichi/)

## Quick Start

```bash
# Build
make build

# List config
./bin/taichi list -c configs/taichi.yaml

# Run tests
./bin/taichi run -c configs/taichi.yaml

# Show version
./bin/taichi version
```

## Directory Structure Overview

```
taichi/
├── cmd/taichi/          # CLI entry (run / list / validate / version / mcp / copilot)
├── pkg/
│   ├── framework/       # Test core: types, assertions, reports, service lifecycle
│   ├── autofix/         # Auto-fix: error detection, fix rule engine
│   ├── skill/           # Skill interface contract and context
│   │   ├── api/         # API test skill implementation
│   │   ├── grpc/        # gRPC smoke check skill (health / dial / reflect)
│   │   ├── ui/          # UI / page test skill implementation
│   │   ├── static/      # Static resource test skill implementation
│   │   ├── regression/  # Regression test skill implementation
│   │   ├── plugin/      # Third-party plugin skill loader (JSON-over-stdio)
│   │   └── builtin/     # Canonical list of built-in skill instances (Skills())
│   ├── registry/        # Skill registry (concurrency-safe)
│   ├── env/             # Environment management (backend / frontend / custom)
│   ├── config/          # Config schema and loading
│   ├── report/          # Report extension point and multi-format output
│   ├── failure/         # Failure context contract (taichi ↔ AI Agent)
│   ├── agent/           # AI Agent invoker interface (CLI / HTTP)
│   ├── mcp/             # MCP Server (JSON-RPC over stdio)
│   └── orchestrator/    # Orchestration core (incl. copilot loop)
├── skills/              # AI Agent SKILL.md capability files (not Go code)
│   ├── test-runner/
│   ├── failure-analyzer/
│   ├── code-fixer/
│   ├── regression-runner/
│   └── config-generator/
├── sdks/                # Third-party plugin SDKs (Node / Python / Shell)
├── configs/             # Default config and templates
└── docs/                # This documentation directory
```
