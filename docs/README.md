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
├── cmd/taichi/          # CLI entry (run / list / version)
├── pkg/
│   ├── framework/       # Test core: types, assertions, reports, service lifecycle
│   ├── autofix/         # Auto-fix: error detection, fix rule engine
│   ├── skill/           # Skill interface contract and context
│   ├── registry/        # Skill registry (concurrency-safe)
│   ├── env/             # Environment management (backend / frontend)
│   ├── config/          # Config schema and loading
│   ├── report/          # Report extension point and multi-format output
│   └── orchestrator/    # Orchestration core
├── skills/              # Built-in skill implementations
│   ├── api/             # API test skill
│   ├── grpc/            # gRPC smoke check skill (health / dial / reflect)
│   ├── ui/              # UI / page test skill
│   ├── static/          # Static resource test skill
│   └── regression/      # Regression test skill
├── configs/             # Default config and templates
└── docs/                # This documentation directory
```
