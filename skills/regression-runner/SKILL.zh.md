---
name: "taichi-regression-runner"
description: "在代码修复后运行回归测试，验证修复未引入新问题。当 AI Agent 完成代码修复后需要验证、用户请求回归测试、或在 copilot 中修复应用后需要重新执行测试时调用。输入为配置文件路径、项目名、超时，调用 regression 技能执行回归用例，验证标准为所有回归用例通过（Failed=0）。"
---

> 🌐 语言: [English](SKILL.md) | [中文](SKILL.zh.md)

# Taichi 回归测试运行器 Skill

## 一、简介

本 Skill 用于让 AI Agent 在代码修复后运行回归测试，验证修复消除了原有失败且未引入新问题。Taichi 内置 `regression` 技能（`pkg/skill/regression`），通过 `--skill regression` 过滤即可仅执行回归用例。

本 Skill 是 Taichi ↔ AI Agent 双向集成闭环中的「回归验证」环节，承接 `taichi-code-fixer` 的修复产出，确认闭环成功。

## 二、何时调用本 Skill

**强制调用场景**：
- `taichi-code-fixer` 已应用修复（`fixed: true`），需要验证修复效果
- 用户提及「回归测试」「regression」「验证修复」「重新跑测试」
- copilot 中修复应用后，进入下一轮测试验证

**不应调用场景**：
- 尚未执行修复（先调用 `taichi-code-fixer`）
- 需要全量测试而非仅回归用例（改用 `taichi-test-runner` 不带 `--skill` 过滤）
- 仅分析失败原因（改用 `taichi-failure-analyzer`）

## 三、输入参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `config` | string | 是 | Taichi 配置文件路径（YAML），如 `configs/taichi.yaml` |
| `project` | string | 否 | 被测项目名。空表示运行配置中的第一个项目 |
| `timeout` | duration | 否 | 本次运行的总超时（如 `30m`、`90s`）。`0` 表示不限 |
| `reports-dir` | string | 否 | 覆盖配置中的报告输出目录 |
| `log-level` | string | 否 | 日志级别：`debug` / `info` / `warn` / `error`，默认 `info` |

## 四、调用方式

### 4.1 CLI 调用

```bash
taichi run -c <config> --skill regression [--project <name>] [--timeout <dur>] [--reports-dir <dir>] [--log-level <level>]
```

示例：

```bash
# 对 tickraft 项目执行回归测试，超时 15 分钟
taichi run -c configs/taichi.yaml --project tickraft --skill regression --timeout 15m
```

退出码：全部通过返回 `0`；存在失败或运行错误返回 `1`。

### 4.2 MCP 调用

调用 MCP Server 暴露的 `taichi_regression` 工具：

```json
{
  "config": "configs/taichi.yaml",
  "project": "tickraft",
  "timeout": "15m"
}
```

## 五、验证标准

回归测试通过的判定标准：

| 判定项 | 标准 |
|--------|------|
| `summary.failed` | `== 0`（所有回归用例通过） |
| 退出码 | `== 0` |

满足以上两项即视为回归通过，copilot 标记 `Fixed=true` 并结束闭环。

若 `summary.failed > 0`，表示修复未完全生效或引入了新问题，copilot 将：
1. 重新生成失败上下文
2. 进入下一轮分析-修复（若未超过 `max-rounds`）
3. 或在耗尽轮次后返回最终失败结果

## 六、输出格式

回归测试结果与 `taichi-test-runner` 完全相同，详见 [`taichi-test-runner` 输出格式](../test-runner/SKILL.md#五输出格式)。

结构化结果（JSON）：

```json
{
  "project_name": "tickraft",
  "base_url": "http://127.0.0.1:8080",
  "duration": "8.76s",
  "summary": {
    "total": 12,
    "passed": 12,
    "failed": 0,
    "skipped": 0
  },
  "skill_results": [
    {
      "skill_name": "regression",
      "duration": "8.76s",
      "summary": {
        "total": 12,
        "passed": 12,
        "failed": 0,
        "skipped": 0
      },
      "error": null
    }
  ]
}
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `project_name` | string | 被测项目名 |
| `base_url` | string | 被测服务基址 |
| `duration` | string | 总执行耗时 |
| `summary.total` | int | 回归用例总数 |
| `summary.passed` | int | 通过数 |
| `summary.failed` | int | 失败数（回归通过要求为 0） |
| `summary.skipped` | int | 跳过数 |
| `skill_results[]` | array | 各技能执行结果（回归场景仅 `regression` 一项） |

## 七、与其他 Skill 的衔接

- **上游**：`taichi-code-fixer` 应用修复后触发
- **闭环出口**：回归通过（`failed == 0`）即整个 test → analyze → fix → regression 闭环成功

```
taichi-test-runner ──失败──► taichi-failure-analyzer ──► taichi-code-fixer ──► taichi-regression-runner
       ▲                                                                            │
       └────────────────────── 回归仍有失败则回到起点 ──────────────────────────────┘
```

## 八、输出自检清单

- [ ] 是否使用 `--skill regression` 过滤（避免运行无关技能）
- [ ] 是否检查了 `summary.failed == 0` 与退出码
- [ ] 失败时是否重新生成失败上下文供下一轮分析
- [ ] 超时设置是否留有余量（回归可能比单轮测试耗时）
