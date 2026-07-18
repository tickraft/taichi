# <img src="assets/logo.svg" width="44" align="absmiddle" alt="Taichi logo"> Taichi · 插件驱动的测试编排框架

[![Test](https://github.com/tickraft/taichi/actions/workflows/test.yaml/badge.svg)](https://github.com/tickraft/taichi/actions/workflows/test.yaml)
[![Release](https://github.com/tickraft/taichi/actions/workflows/release.yaml/badge.svg)](https://github.com/tickraft/taichi/actions/workflows/release.yaml)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![GitHub release](https://img.shields.io/github/v/release/tickraft/taichi?include_prereleases)](https://github.com/tickraft/taichi/releases)
[![GitHub Downloads](https://img.shields.io/github/downloads/tickraft/taichi/total?logo=github)](https://github.com/tickraft/taichi/releases)
[![GitHub Issues](https://img.shields.io/github/issues/tickraft/taichi?logo=github)](https://github.com/tickraft/taichi/issues)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen?logo=github)](CONTRIBUTING.zh.md)

> 🌐 语言: [English](README.md) | [中文](README.zh.md)

一个语言无关、插件驱动的测试编排框架 —— 用单一 CLI 运行 API、gRPC、UI、静态资源、回归测试，通过 JSON-over-stdio 协议用任意语言编写第三方插件技能，集成 AI Agent 实现自动修复闭环，输出 JUnit/JSON/HTML 多格式报告。

## 为什么选择 Taichi

- **一个二进制，多个项目**：在单个 YAML 中配置多个项目（Go 服务、Python/Rust/Java 应用、Vite 前端…），一条命令运行全部测试套件。
- **以技能为扩展单元**：五类内置技能覆盖常见场景；自定义 Go 技能实现 `TestSkill` 接口即可，第三方插件可用任意语言通过 JSON-over-stdio 协议接入 —— 无需重新编译 taichi。
- **AI 原生闭环**：`copilot` 命令串联 测试 → 失败分析 → Agent 补丁 → 回归验证，实现自动修复与验证工作流。
- **多格式报告**：开箱即用 JSON、JUnit XML、可读摘要，并提供 `report.Writer` 扩展点支持自定义输出（HTML、Slack、飞书…）。
- **生命周期感知的环境管理**：自动启动/停止后端二进制与开发服务器，等待健康检查通过后执行测试，结束自动清理。

## 内置技能

| 技能 | Kind | 说明 |
|------|------|------|
| API | `api` | HTTP 状态码、统一响应契约（`code`/`msg`/`request_id`）、字段值、时延断言 |
| gRPC | `grpc` | 健康检查、连通性 dial、反射服务发现 |
| UI | `ui` | 页面可达性、HTML 标记、TTFB |
| Static | `static` | 页面与静态资源可用性 |
| Regression | `regression` | 修复后关键路径回归验证 |
| Plugin | `plugin` | 任意外部进程通过 JSON-over-stdio 接入（提供 Python/Node/Shell SDK） |

## 安装

### 从源码构建（需 Go 1.26+）

```bash
git clone https://github.com/tickraft/taichi.git
cd taichi
make build
# 二进制：./bin/taichi
```

### 安装到 GOBIN

```bash
go install github.com/tickraft/taichi/cmd/taichi@latest
```

### 下载预编译二进制

见 [Releases](https://github.com/tickraft/taichi/releases)，提供跨平台二进制（Linux / macOS / Windows，amd64 / arm64）及 SHA256 校验文件。

## 快速开始

```bash
# 1. 编译二进制
make build

# 2. 查看已配置的项目、环境与技能
./bin/taichi list --config configs/taichi.yaml

# 3. 运行配置中第一个项目的全部测试
./bin/taichi run --config configs/taichi.yaml

# 4. 仅运行 api 技能
./bin/taichi run --config configs/taichi.yaml --skill api

# 5. 按名称指定项目运行（-p）
./bin/taichi run --config configs/taichi.yaml -p tickraft

# 6. 覆盖报告输出目录
./bin/taichi run --config configs/taichi.yaml --reports-dir /tmp/taichi-reports

# 7. 校验配置文件（不执行测试）
./bin/taichi validate -c configs/taichi.yaml
```

## 最小配置示例

一个项目、一个环境、一个 API 技能的最小配置：

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
    build_target: ./cmd/my-service
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

非 Go 服务请使用 `custom` 环境类型（任意启动命令 + 健康 URL）；`configs/envs/` 提供 Python / Rust / Java / Node / Ruby 等模板。

## 第三方插件技能

用任意语言编写测试技能，无需修改 Taichi 源码：

```yaml
skills:
  - name: my-check
    kind: plugin
    enabled: true
    raw:
      command: ./my-plugin
      args: ["--verbose"]
      timeout: 30s
      # 以下任意自定义字段会通过 input.config 透传给插件
      endpoints:
        - /api/v1/custom
```

插件从 stdin 读取 JSON `PluginInput`，向 stdout 输出 JSON `PluginOutput`。提供 [Python](sdks/python/)、[Node](sdks/node/)、[Shell](sdks/shell/) SDK。

## AI Agent 集成

Taichi 暴露 MCP 服务器并提供技能包，让 AI Agent 端到端完成生成配置、运行测试、分析失败、应用补丁、回归验证：

```
config-generator → test-runner → failure-analyzer → code-fixer → regression-runner
```

详见 [Agent 集成指南](docs/agent-integration.zh.md)。

## 目录结构

```
taichi/
├── cmd/taichi/          # 二进制入口（cobra CLI）
├── pkg/                 # 对外公开 API（技能开发者可 import）
│   ├── framework/       # 核心数据模型与引擎
│   ├── autofix/         # 错误检测与自动修复
│   ├── env/             # 环境管理（Go/Node 二进制 + 任意命令进程式）
│   ├── registry/        # 技能注册中心（动态加载 / 卸载）
│   ├── skill/           # 技能接口契约（含 plugin 子包）
│   ├── config/          # 配置 schema 与加载
│   └── report/          # 报告扩展点
├── pkg/skill/           # 内置技能实现
│   ├── api/             # API 测试
│   ├── grpc/            # gRPC 冒烟检查（health / dial / reflect）
│   ├── ui/              # UI / 页面测试
│   ├── static/          # 静态资源测试
│   ├── regression/      # 回归测试
│   └── plugin/          # 第三方插件技能加载器
├── sdks/                # 多语言插件 SDK（Python / Node / Shell）
├── configs/             # 默认配置模板
│   ├── taichi.yaml      # 全局默认
│   ├── envs/            # 多语言环境模板
│   └── plugin-demo.yaml # 插件技能示例
├── examples/            # 插件示例
├── skills/              # AI Agent 技能包（MCP 集成）
├── docs/                # 设计文档
└── reports/             # 测试报告输出
```

## 文档

- [架构总览](docs/architecture.zh.md)
- [技能对接接口规范](docs/skill-interface.zh.md)
- [环境配置指南](docs/configuration.zh.md)
- [测试用例编写规范](docs/test-cases.zh.md)
- [框架扩展开发文档](docs/extending.zh.md)
- [Agent 集成指南](docs/agent-integration.zh.md)
- [文档索引](docs/README.zh.md)

## 社区

- [贡献指南](CONTRIBUTING.zh.md) — 开发环境、提交规范、PR 流程
- [行为准则](CODE_OF_CONDUCT.zh.md) — 社区行为标准
- [安全策略](SECURITY.zh.md) — 漏洞报告流程
- [Issue 模板](.github/ISSUE_TEMPLATE/) — Bug 报告与功能请求
- [Discussions](https://github.com/tickraft/taichi/discussions) — 问答与想法

## 协议

Taichi 采用 [Apache License 2.0](LICENSE) 开源协议。

贡献内容在同一协议下授权，无需签署 CLA。

---

<sub>
<b>关键词 / Keywords：</b>测试编排 test orchestration、自动化测试 automated testing、
API 测试、gRPC 测试、UI 测试、回归测试 regression testing、插件化测试 plugin-based testing、
AI Agent 测试、MCP 测试服务器 MCP test server、JUnit 报告 JUnit reports、
持续测试 continuous testing、自动修复闭环 auto-fix loop
</sub>
