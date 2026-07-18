# Taichi 与 AI Agent 集成指南

> 🌐 语言: [English](agent-integration.md) | [中文](agent-integration.zh.md)

> 本文档描述 Taichi 测试编排框架与 AI Agent（如 Trae IDE Agent、自定义 Agent）的双向集成架构、调用接口与修复闭环。

Taichi 与 AI Agent 的集成是**双向**的，两个方向互补，共同构成「测试 → 分析 → 修复 → 回归」的全自动闭环。失败上下文（`Context`）作为两侧的信息交换契约。

### 方向 1：Taichi 作为 MCP Server 被 AI Agent 调用（Agent 主导）

AI Agent 处于主导地位，按需调用 Taichi 暴露的 MCP 工具执行测试、列出配置、读取失败、运行回归。Agent 自行编排分析、修复与验证流程。

- 适用：Agent 工作流中需要灵活触发测试，Agent 具备完整的分析-修复能力
- 协议：MCP（Model Context Protocol）或 CLI 子进程调用
- 信息流：Agent → taichi（调用工具）→ Taichi 返回结果 → Agent 决策

### 方向 2：Taichi 在编排中调用 AI Agent 修复（Taichi 主导的 copilot 模式）

Taichi 处于主导地位，在测试失败时主动通过 `agent.Invoker` 接口调用 AI Agent 进行分析与修复，修复后自动回归，循环直至通过或耗尽轮次。

- 适用：CI / 命令行场景，希望 Taichi 自驱完成「测试-修复-回归」闭环
- 协议：CLI（stdin/stdout JSON）或 HTTP API
- 信息流：taichi → Agent（传递 `Context`）→ Agent 返回 `FixResult` → Taichi 应用并回归

### 失败上下文：信息交换契约

无论哪个方向，Taichi 与 Agent 之间都以 `Context` JSON 作为失败信息的标准载体，以 `FixResult` JSON 作为修复产出的标准载体。两者定义于：

- `pkg/failure/failure.go`：`Context` / `FailedCase`
- `pkg/agent/agent.go`：`FixResult` / `FixMode` / `Invoker` 接口

## 二、架构图

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

**闭环流程**：

```
taichi run (测试)
   │
   ├─ 全部通过 ──► 结束（成功）
   │
   └─ 存在失败 ──► 生成失败上下文 ──► Agent.AnalyzeAndFix
                                                    │
                                                    ▼
                                           应用 FixResult
                                           (git apply / verify)
                                                    │
                                                    ▼
                                           taichi run (回归)
                                                    │
                                                    ├─ 通过 ──► 结束（修复成功）
                                                    │
                                                    └─ 仍有失败 ──► 下一轮（≤ max-rounds）
```

## 三、MCP Server 使用

Taichi 可作为 MCP Server 运行，向 AI Agent 暴露测试编排能力。

### 3.1 启动 MCP Server

```bash
taichi mcp -c configs/taichi.yaml
```

`-c` 指定配置文件，MCP Server 在 stdio 上监听 MCP 协议消息，供 Agent 客户端连接。

### 3.2 暴露的工具

| 工具名 | 用途 | 关键参数 | 返回格式 |
|--------|------|---------|---------|
| `taichi_run` | 执行一次测试编排 | `config_path`、`project`、`skills`、`timeout` | 测试结果摘要（见 3.3） |
| `taichi_list` | 列出配置中的项目、环境与已注册技能 | `config_path` | 项目/环境/技能清单 |
| `taichi_failures` | 读取最近一次运行的失败上下文 | `config_path`、`reports_dir` | `Context` JSON |
| `taichi_regression` | 执行回归测试（仅 `regression` 技能） | `config_path`、`project`、`timeout` | 测试结果摘要 |

### 3.3 工具参数与返回格式

#### taichi_run

请求参数：

```json
{
  "config_path": "configs/taichi.yaml",
  "project": "tickraft",
  "skills": ["api", "ui"],
  "timeout": "30m"
}
```

返回（测试结果摘要）：

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

请求参数：

```json
{ "config_path": "configs/taichi.yaml" }
```

返回项目、环境与已注册技能清单（名称、类型、启用状态、优先级）。

#### taichi_failures

请求参数：

```json
{ "config_path": "configs/taichi.yaml", "reports_dir": "reports" }
```

返回最近一次运行的 `Context` JSON（结构见[第七章](#七失败上下文格式)）。若无失败返回空对象。

#### taichi_regression

请求参数：

```json
{
  "config_path": "configs/taichi.yaml",
  "project": "tickraft",
  "timeout": "15m"
}
```

返回与 `taichi_run` 相同格式的测试结果摘要，`skill_results` 仅含 `regression` 一项。

## 四、Copilot 使用

Taichi 主导的 copilot 模式通过 `taichi copilot` 命令触发，自动完成「测试 → 失败 → Agent 修复 → 回归测试 → 循环」。

### 4.1 CLI 调用

```bash
taichi copilot -c configs/taichi.yaml \
  --agent-cli trae \
  --agent-args "agent fix" \
  --max-rounds 3
```

### 4.2 流程

1. **测试**：执行一次完整测试编排（等价于 `taichi run`）
2. **失败判定**：若全部通过，直接返回成功；若存在失败，进入修复循环
3. **构建失败上下文**：将失败用例封装为 `Context`，写入 `reports/failures-round-<N>-<timestamp>.json`
4. **调用 Agent**：通过 `agent.Invoker` 将 `Context` 交给 AI Agent 分析与修复
5. **应用修复**：
   - `patch` 模式：`git apply` 应用 Agent 生成的 unified diff
   - `direct` 模式：校验 Agent 修改后的文件存在且可读
6. **回归测试**：重新执行测试，验证修复效果
7. **循环或结束**：
   - 回归通过 → 标记 `Fixed=true`，结束
   - 回归仍有失败 → 轮次 +1，回到步骤 3（不超过 `max-rounds`）
   - 耗尽轮次或 Agent 返回 `fixed=false` → 返回最终失败结果

### 4.3 配置项说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-c, --config` | 配置文件路径 | `configs/taichi.yaml` |
| `--agent-cli` | AI Agent 命令行可执行文件（如 `trae`、`python3`） | 必填 |
| `--agent-args` | 传给 Agent 命令的参数 | 空 |
| `--max-rounds` | 最大修复轮次 | `3` |
| `--project` | 被测项目名 | 配置中第一个项目 |
| `--timeout` | 单轮测试超时 | 不限 |
| `--log-level` | 日志级别 | `info` |

> 实现：[`pkg/orchestrator/copilot.go`](../pkg/orchestrator/copilot.go) 的 `RunCopilot`。默认最大轮次常量 `defaultMaxRounds = 3`。

## 五、Agent Invoker 实现

Taichi 通过 `agent.Invoker` 接口抽象 AI Agent 的调用方式，内置两种实现，并支持自定义。

### 5.1 Invoker 接口

```go
// 定义于 pkg/agent/agent.go
type Invoker interface {
    // AnalyzeAndFix 分析失败上下文并返回修复结果。
    AnalyzeAndFix(ctx context.Context, fc *failure.Context) (*FixResult, error)
    // Name 返回调用器的人类可读名称。
    Name() string
}
```

### 5.2 CLIInvoker：命令行调用

通过子进程调用 AI Agent。Agent 脚本需**从 stdin 读取 `Context` JSON，向 stdout 输出 `FixResult` JSON**。

```go
invoker := &agent.CLIInvoker{
    Command: "trae",            // Agent 可执行文件
    Args:    []string{"agent", "fix"},  // Agent 参数
    Timeout: 5 * time.Minute,   // 单次调用超时，0 表示默认 5 分钟
    WorkDir: "/path/to/project", // 命令执行工作目录，空表示当前目录
}
```

契约：
- stdin：`Context` JSON（紧凑格式）
- stdout：`FixResult` JSON（紧凑格式）
- stderr：Agent 自由输出（Taichi 仅在出错时记录）
- 退出码：非零视为调用失败

### 5.3 HTTPInvoker：HTTP API 调用

通过 HTTP POST 调用 AI Agent 服务。

```go
invoker := &agent.HTTPInvoker{
    Endpoint: "https://agent.example.com/api/fix",  // Agent HTTP 端点
    Token:    "bearer-token",        // Bearer 认证令牌（可选）
    Timeout:  5 * time.Minute,       // 请求超时，0 表示默认 5 分钟
    Client:   customHTTPClient,      // 自定义 HTTP 客户端（可选）
}
```

契约：
- 请求：`POST <Endpoint>`，`Content-Type: application/json`，body 为 `Context` JSON
- 认证：若 `Token` 非空，附加 `Authorization: Bearer <Token>` 头
- 响应：HTTP 200，body 为 `FixResult` JSON；非 200 视为失败

### 5.4 自定义 Invoker

实现 `agent.Invoker` 接口即可接入任意 Agent 后端（如 gRPC、消息队列、内置 LLM 调用）：

```go
type MyInvoker struct{}

func (m *MyInvoker) Name() string { return "my-agent" }

func (m *MyInvoker) AnalyzeAndFix(ctx context.Context, fc *failure.Context) (*agent.FixResult, error) {
    // 1. 将 fc 序列化并发送给自建 Agent 服务
    // 2. 等待 Agent 分析与修复
    // 3. 解析返回并构造 FixResult
    return &agent.FixResult{
        Fixed:         true,
        Mode:          agent.FixModePatch,
        Patch:         patchContent,
        ModifiedFiles: []string{"internal/handler/home.go"},
        Message:       "修复了 nil 解引用",
        Analysis:      "home.go:42 添加 nil 检查",
    }, nil
}
```

注入 copilot 编排：

```go
result, err := o.RunCopilot(ctx, orchestrator.CopilotOptions{
    Options:    opts,
    MaxRounds:  3,
    Invoker:    &MyInvoker{},
})
```

## 六、修复模式

Taichi 通过 `agent.PatchApplier` 应用 Agent 的修复产出，支持两种模式。

### 6.1 patch 模式

Agent 生成 unified diff 补丁，Taichi 应用到项目源码。

- **应用方式**：优先 `git apply --whitespace=fix`，失败回退 `patch -p1 --no-backup-if-mismatch`
- **路径解析**：patch 中路径带 `a/` `b/` 前缀（git diff 风格），相对于 `project_root` 解析
- **回滚**：可通过 `git checkout` 回滚工作区

`FixResult` 示例：

```json
{
  "fixed": true,
  "mode": "patch",
  "patch": "--- a/internal/handler/home.go\n+++ b/internal/handler/home.go\n@@ -39,6 +39,9 @@\n func HomeHandler(c *Context) {\n+    if c.Config == nil {\n+        c.Config = DefaultConfig()\n+    }\n     theme := c.Config.Theme\n }\n",
  "modified_files": ["internal/handler/home.go"],
  "message": "修复了空指针引用"
}
```

### 6.2 direct 模式

Agent 通过文件编辑工具直接修改源码文件，Taichi 仅验证修改有效（文件存在且可读，非目录）。

- **适用**：Agent 具备文件编辑能力（如 IDE 内 Agent）
- **验证**：Taichi 调用 `PatchApplier.VerifyDirectFix` 检查 `modified_files` 列表
- **不介入写入**：Taichi 不修改任何文件，仅校验

`FixResult` 示例：

```json
{
  "fixed": true,
  "mode": "direct",
  "modified_files": ["internal/handler/home.go", "internal/config/default.go"],
  "message": "添加了配置缺失时的默认主题回退"
}
```

> 实现：[`pkg/agent/patch.go`](../pkg/agent/patch.go) 的 `PatchApplier.ApplyResult` 按 `Mode` 分发。

## 七、失败上下文格式

`Context` 是 Taichi ↔ AI Agent 的信息交换契约，定义于 [`pkg/failure/failure.go`](../pkg/failure/failure.go)。

### 7.1 JSON 结构

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

### 7.2 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `project_name` | string | 被测项目名 |
| `base_url` | string | 被测服务基址（未启动环境时为空） |
| `timestamp` | string | 生成时间（RFC3339，UTC） |
| `project_root` | string | 被测项目根目录绝对路径，Agent 据此定位源码 |
| `env_log_path` | string | 环境（服务）日志路径，Agent 据此检查服务端错误 |
| `reports_dir` | string | 报告输出目录，含 JSON / JUnit XML / 摘要 |
| `total_cases` | int | 用例总数 |
| `passed_cases` | int | 通过数 |
| `failed_cases[]` | array | 失败用例详情 |
| `failed_cases[].skill_name` | string | 技能名（`api` / `ui` / `static` / `regression`） |
| `failed_cases[].name` | string | 用例标识符 |
| `failed_cases[].message` | string | 失败的人类可读描述 |
| `failed_cases[].error` | string | 底层错误字符串（如有） |
| `failed_cases[].duration` | string | 用例执行耗时 |

### 7.3 生成与消费

- **生成**：Taichi 在测试存在失败时，由 `failure.FromResults` 从测试结果快照构建，通过 `WriteToFile` 写入 `reports/failures-round-<N>-<timestamp>.json`
- **消费**：Agent 通过文件路径读取，或经 `CLIInvoker` stdin / `HTTPInvoker` body 接收
- **可序列化**：`Context` 实现 JSON 序列化/反序列化，`ReadFromFile` 可从文件还原

## 八、Skill 文件索引

`taichi/skills/` 下存放面向 AI Agent 的 SKILL.md 文件，描述 Agent 在集成闭环各环节的能力契约。通过 `scripts/link-skills.sh` 可软链到 `.trae/skills/` 供 Trae IDE Agent 发现。

| Skill 目录 | 名称 | 用途 | 闭环环节 |
|-----------|------|------|---------|
| [`skills/config-generator/`](../skills/config-generator/SKILL.md) | `taichi-config-generator` | 分析项目源码自动生成 taichi.yaml 配置文件 | 配置初始化 |
| [`skills/test-runner/`](../skills/test-runner/SKILL.md) | `taichi-test-runner` | 通过 CLI 或 MCP 执行自动化测试，产出测试结果摘要 | 测试执行 |
| [`skills/failure-analyzer/`](../skills/failure-analyzer/SKILL.md) | `taichi-failure-analyzer` | 读取失败上下文，结合日志与源码分析失败根因 | 根因分析 |
| [`skills/code-fixer/`](../skills/code-fixer/SKILL.md) | `taichi-code-fixer` | 根据失败上下文产出修复（patch 或 direct 模式） | 代码修复 |
| [`skills/regression-runner/`](../skills/regression-runner/SKILL.md) | `taichi-regression-runner` | 修复后运行回归测试，验证修复未引入新问题 | 回归验证 |

### 闭环编排

```
taichi-config-generator ──► taichi-test-runner ──失败──► taichi-failure-analyzer ──► taichi-code-fixer ──► taichi-regression-runner
         ▲                          ▲                                                                                    │
         │ 新项目首次接入            └──────────────────────── 回归仍有失败则回到起点 ──────────────────────────────────────┘
         └──────── 重大重构后重建配置 ──────────────────────────────────────────────────────────────────────────────────────┘
```

### 软链到 Trae IDE

```bash
# 将 taichi/skills/ 下的 Skill 软链到 .trae/skills/，使 Trae Agent 可发现
bash taichi/scripts/link-skills.sh
```

软链后，Skill 在 `.trae/skills/` 下以 `taichi-<skill-name>` 命名（如 `taichi-test-runner`），Trae IDE 的 AI Agent 可按需调用。
