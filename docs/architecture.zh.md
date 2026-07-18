# 架构设计

> 🌐 语言: [English](architecture.md) | [中文](architecture.zh.md)

## 一、设计目标

Taichi 是一个**通用的自动化测试编排框架**。核心设计目标：

1. **跨项目复用**：不绑定特定项目，通过配置描述任意被测服务
2. **技能扩展**：以 Skill 为扩展单元，支持 API / gRPC / UI / 静态 / 回归 / 自定义测试类型；第三方插件通过 JSON-over-stdio 协议接入，语言无关
3. **多环境支持**：原生支持 Go/Node 二进制后端、Vite/Nuxt 前端；`custom` 类型支持任意启动命令 + 健康检查 URL，覆盖 Python/Rust/Java/Ruby 等任意技术栈与外部托管服务
4. **低耦合**：技能间不直接引用，共享状态通过上下文传递
5. **可观测**：多格式报告（JSON / JUnit XML / 摘要）+ 自动修复 + 结构化日志

## 二、架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                      cmd/taichi (CLI)                        │
│   run / list / validate / version / mcp / copilot           │
│   子命令 + flag 解析                                         │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                   pkg/orchestrator                           │
│                                                             │
│  配置加载 → 项目选择 → 环境启停 → 技能执行 → 报告生成       │
└──────┬────────────┬─────────────┬────────────┬──────────────┘
       │            │             │            │
       ▼            ▼             ▼            ▼
┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐
│ pkg/env  │ │pkg/config│ │pkg/registry│ │pkg/report│
│ 环境管理  │ │ 配置加载  │ │ 技能注册中心│ │ 报告扩展点   │
└────┬─────┘ └──────────┘ └──────┬───┘ └──────┬───────┘
     │                          │             │
     ▼                          ▼             ▼
┌──────────┐            ┌──────────────┐  ┌──────────┐
│ backend  │            │ pkg/skill    │  │ framework│
│ frontend │            │ 接口契约+上下文│  │ 断言+报告│
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
│ 错误检测+修复引擎 │
└──────────────────┘
```

## 三、模块说明

### 3.1 pkg/framework — 测试核心

提供测试编排的基础能力，不依赖任何业务逻辑：

| 文件 | 职责 |
|------|------|
| `types.go` | TestCase / TestResult / TestSuite / AssertResult 类型定义 |
| `assert.go` | AssertionEngine：状态码、JSON 字段、JSONPath、HTML 包含、响应时延断言 |
| `reporter.go` | TestReporter：结果收集、JSON / JUnit XML / 摘要输出、Snapshot |
| `lifecycle.go` | ServiceLifecycle：按需构建二进制、空闲端口启动、健康检查、停止/重启。导出 `FreePort()` 作为空闲端口的唯一实现（`pkg/env` 复用） |

**关键设计**：`ServiceLifecycle` 通过 `ServiceConfig` 全配置化，不再硬编码项目路径，可服务任意 Go 二进制。

### 3.2 pkg/autofix — 自动修复

| 文件 | 职责 |
|------|------|
| `detector.go` | ErrorDetector：从 HTTP 响应分类错误（服务无响应 / 限流 / 5xx / 未知） |
| `engine.go` | FixRule 接口、FixEngine、3 内置规则（服务重启 / 限流退避 / 未知错误报告） |

**关键设计**：`FixContext.Lifecycle` 类型为 `ServiceRestarter` 接口而非具体类型，保持与 framework 解耦。

### 3.3 pkg/skill — 技能接口契约

定义所有技能必须实现的 `TestSkill` 接口与运行态上下文 `Context`：

- 生命周期：`Configure → Setup → Run → Teardown`
- 优先级：Critical(0) / High(10) / Normal(20) / Low(30)
- 上下文传递：BaseURL / Asserts / Reporter / Logger / FixEngine / Extra
- 辅助函数：HTTPRequest / RecordResult / AssertCommonEnvelope / Get* 配置读取

### 3.4 pkg/registry — 技能注册中心

并发安全的技能注册、查询、卸载、按配置筛选与优先级排序：

- `Register(s, overwrite)` / `Unregister(name)` / `Get(name)` / `List()`
- `Select(configs)`：按 `skill.Config` 筛选启用的技能，按 Priority 升序排序

### 3.5 pkg/env — 环境管理

| 实现 | 适用场景 |
|------|---------|
| `backend` | Go / Node 后端二进制，复用 framework.ServiceLifecycle |
| `frontend` | npm/pnpm dev server，子进程启动 + 等待 ready URL |
| `external`（base_url 非空） | 外部托管服务，跳过启动/停止 |

`Manager` 统一编排单一项目对应环境的启停。

### 3.6 pkg/config — 配置加载

YAML 配置 schema，viper 加载，含五大顶层结构：

```
projects: []Project      # 被测项目列表
envs: map[string]Env     # 环境定义
skills: []skill.Config   # 技能配置
report: Report            # 报告输出
autofix: Autofix          # 自动修复
```

### 3.7 pkg/report — 报告扩展点

- 内置格式（json / junit / summary）：由 `framework.TestReporter` 原生实现
- 自定义格式：实现 `Writer` 接口，注册到 `Registry`
- `Generate(reporter, registry, formats, pathFor)` 统一调度

### 3.8 pkg/orchestrator — 编排核心

协调一次完整测试运行的 9 步流程：

1. 加载配置（`config.Load`）
2. 选择项目（按 `--project` 或配置第一个）
3. 解析项目根目录（相对配置文件目录）
4. 启动环境（若配置了 env）
5. 准备技能配置（按项目 skills + filter 筛选）
6. 创建共享上下文资源（Reporter / Asserts / ReportsDir）
7. 创建自动修复引擎（若 `autofix.enabled`）
8. 依次执行技能（Configure → Setup → Run → Teardown，按优先级）
9. 生成报告

### 3.9 pkg/skill/* — 内置技能

内置技能实现位于 `pkg/skill/{api,grpc,ui,static,regression}`，由 `pkg/skill/builtin.Skills()` 聚合。（顶层 `skills/` 目录存放 AI Agent 的 `SKILL.md` 能力描述文件，并非 Go 实现。）

| 技能 | Kind | 优先级 | 验证内容 |
|------|------|--------|---------|
| api | api | Critical(0) | 状态码 + 统一响应契约(code/msg/request_id) + 指定字段值 + 响应时延 |
| grpc | grpc | Critical(0) | 配置驱动的 gRPC 冒烟检查（health / connectivity / reflection） |
| ui | ui | High(10) | 页面可访问性 + HTML 标记 + 关键字包含 + 首字节时延 |
| static | static | Normal(20) | 页面(200+html,404 skip) + 资源(200或404均pass) |
| regression | regression | Low(30) | 关键路径端点重探测，验证修复未引入回归 |

### 3.10 cmd/taichi — CLI

| 子命令 | 作用 |
|--------|------|
| `run` | 加载配置 → 注册内置技能 → 编排执行 → 打印摘要 → 生成报告 |
| `list` | 展示项目、环境、技能配置、报告与自动修复配置 |
| `validate` | 校验配置文件完整性（projects / envs / skills / 唯一性），不执行测试 |
| `version` | 打印版本、Go 运行时、目标平台 |
| `mcp` | 启动 MCP Server（基于 stdio 的 JSON-RPC），向 AI Agent 暴露 Taichi 工具 |
| `copilot` | 在外部 AI Agent（通过 `agent.Invoker` 接口）驱动下运行 测试 → 修复 → 回归 闭环 |

## 四、依赖关系

```
cmd/taichi → orchestrator, skill, pkg/skill/*, config, registry
orchestrator → config, env, framework, registry, report, skill, autofix
pkg/skill/* → skill, framework
env → config, framework
report → framework
autofix → (无外部依赖，ServiceRestarter 为接口)
```

**循环依赖规避**：`pkg/orchestrator` 不直接 import `pkg/skill/*`，而是通过 `RegisterBuiltinSkills([]skill.TestSkill)` 由 `cmd/taichi` 传入已构造的技能实例。内置技能实例的权威列表为 `pkg/skill/builtin` 中的 `builtin.Skills()` —— 是 `cmd/taichi`（run/list/copilot）与 `pkg/mcp` 共用的唯一来源，避免列表漂移。
