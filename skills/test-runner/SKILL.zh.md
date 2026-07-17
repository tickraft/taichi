---
name: "taichi-test-runner"
description: "通过 taichi CLI 或 MCP Server 执行自动化测试。当用户请求运行 taichi 测试编排、执行项目测试、查看测试结果摘要、或在 AI Agent 工作流中触发一次测试运行时调用。输入为配置文件路径、项目名、技能过滤与超时，输出为结构化测试结果摘要（通过/失败/跳过数、耗时、各技能结果）。"
---

> 🌐 语言: [English](SKILL.md) | [中文](SKILL.zh.md)

# taichi 测试运行器 Skill

## 一、简介

本 Skill 用于让 AI Agent 通过 taichi 执行一次完整的自动化测试编排。taichi 是一个通用的测试编排框架，按配置文件描述被测项目、环境与技能，编排一次完整的测试运行，并生成 JSON / JUnit XML / 人类可读摘要报告。

本 Skill 是 taichi ↔ AI Agent 双向集成闭环中的「测试执行」环节，产出 `TestResults` 供后续的失败分析与修复消费。

## 二、何时调用本 Skill

**强制调用场景**：
- 用户提及「跑测试」「run tests」「执行 taichi」「验证项目」
- AI Agent 工作流中需要触发一次测试运行以获取基线结果
- 修复代码后需要重新执行测试以确认修复效果（与 `taichi-regression-runner` 配合）
- 需要按技能过滤运行特定类型测试（仅 API、仅 UI 等）

**不应调用场景**：
- 仅分析失败原因（改用 `taichi-failure-analyzer`）
- 仅修复代码（改用 `taichi-code-fixer`）
- 单纯查看配置（改用 `taichi list` 命令）

## 三、输入参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `config` | string | 是 | taichi 配置文件路径（YAML），如 `configs/taichi.yaml` |
| `project` | string | 否 | 被测项目名。空表示运行配置中的第一个项目 |
| `skill` | string[] | 否 | 技能过滤，仅运行指定技能（可重复，如 `api`、`ui`）。空表示运行项目配置的全部技能 |
| `timeout` | duration | 否 | 本次运行的总超时（如 `30m`、`90s`）。`0` 表示不限，监听 SIGINT/SIGTERM 优雅取消 |
| `reports-dir` | string | 否 | 覆盖配置中的报告输出目录 |
| `log-level` | string | 否 | 日志级别：`debug` / `info` / `warn` / `error`，默认 `info` |

## 四、调用方式

### 4.1 CLI 调用

```bash
taichi run -c <config> [--project <name>] [--skill <name>] [--timeout <dur>] [--reports-dir <dir>] [--log-level <level>]
```

示例：

```bash
# 运行配置中的第一个项目，全部技能
taichi run -c configs/taichi.yaml

# 仅运行 tickraft 项目的 api 与 ui 技能，超时 30 分钟
taichi run -c configs/taichi.yaml --project tickraft --skill api --skill ui --timeout 30m
```

退出码：全部通过返回 `0`；存在失败或运行错误返回 `1`。

### 4.2 MCP 调用

调用 MCP Server 暴露的 `taichi_run` 工具：

```json
{
  "config": "configs/taichi.yaml",
  "project": "tickraft",
  "skill": ["api", "ui"],
  "timeout": "30m"
}
```

## 五、输出格式

### 5.1 控制台摘要

CLI 在 stdout 输出人类可读摘要：

```
=== taichi run ===
Project:  tickraft
BaseURL:  http://127.0.0.1:8080
Duration: 12.34s
Summary:  total=24 passed=22 failed=2 skipped=0
EnvLog:   /tmp/taichi-tickraft-env.log

Skills:
  - api          OK     3.21s (total=10 passed=10 failed=0 skipped=0)
  - ui           FAIL   9.13s (total=14 passed=12 failed=2 skipped=0)
```

### 5.2 结构化结果（JSON）

结果同时以 JSON 文件写入 `reports/<project>-<timestamp>.json`，结构如下：

```json
{
  "project_name": "tickraft",
  "base_url": "http://127.0.0.1:8080",
  "duration": "12.34s",
  "summary": {
    "total": 24,
    "passed": 22,
    "failed": 2,
    "skipped": 0
  },
  "skill_results": [
    {
      "skill_name": "api",
      "duration": "3.21s",
      "summary": {
        "total": 10,
        "passed": 10,
        "failed": 0,
        "skipped": 0
      },
      "error": null
    },
    {
      "skill_name": "ui",
      "duration": "9.13s",
      "summary": {
        "total": 14,
        "passed": 12,
        "failed": 2,
        "skipped": 0
      },
      "error": null
    }
  ]
}
```

### 5.3 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `project_name` | string | 被测项目名 |
| `base_url` | string | 被测服务基址（未启动环境时为空） |
| `duration` | string | 总执行耗时 |
| `summary.total` | int | 用例总数 |
| `summary.passed` | int | 通过数 |
| `summary.failed` | int | 失败数 |
| `summary.skipped` | int | 跳过数 |
| `skill_results[]` | array | 各技能执行结果 |
| `skill_results[].skill_name` | string | 技能名（`api` / `ui` / `static` / `regression`） |
| `skill_results[].duration` | string | 技能执行耗时 |
| `skill_results[].summary` | object | 该技能结果聚合统计 |
| `skill_results[].error` | string \| null | 非 null 表示技能级致命错误 |

## 六、与失败上下文的衔接

当 `summary.failed > 0` 时，taichi 会在 `reports/` 目录生成 `FailureContext` JSON 文件（命名形如 `failures-round-1-<timestamp>.json`），供 `taichi-failure-analyzer` 消费。Agent 应在测试结束后检查退出码与 `summary.failed`：

- `failed == 0`：测试通过，无需后续动作
- `failed > 0`：调用 `taichi-failure-analyzer` 读取失败上下文进行分析

## 七、退出码与错误处理

| 退出码 | 含义 | Agent 应对 |
|--------|------|-----------|
| `0` | 全部用例通过 | 流程结束，可输出成功摘要 |
| `1` | 存在失败用例或运行错误 | 检查 `reports/` 下的失败上下文，进入分析-修复流程 |

运行级错误（配置加载失败、环境启动失败、无技能被选中）会以非零退出码返回并打印错误信息到 stderr，Agent 应捕获 stderr 内容并向上报告，而非进入修复流程。

## 八、输出自检清单

- [ ] 是否传入了有效的配置文件路径（`-c`）
- [ ] 技能过滤名是否与配置中的 `skills[].name` 一致
- [ ] 超时设置是否合理（避免过短导致中断）
- [ ] 是否检查了退出码与 `summary.failed` 字段
- [ ] 失败时是否定位到 `reports/` 下的失败上下文文件供下游消费
