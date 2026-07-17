---
name: "taichi-failure-analyzer"
description: "读取 taichi 生成的失败上下文，分析测试失败根因。当 taichi 测试运行存在失败用例、用户请求分析测试失败原因、定位 bug 根因、或在 copilot 中需要为修复提供根因分析时调用。输入为失败上下文 JSON 文件路径，输出为根因分析报告（失败用例、根因、涉及文件、建议修复方式）。"
---

> 🌐 语言: [English](SKILL.md) | [中文](SKILL.zh.md)

# taichi 失败分析器 Skill

## 一、简介

本 Skill 用于让 AI Agent 读取 taichi 在测试失败后生成的结构化失败上下文（`FailureContext`），结合服务日志与项目源码，分析失败根因，产出可执行的根因分析报告。

本 Skill 是 taichi ↔ AI Agent 双向集成闭环中的「根因分析」环节，承接 `taichi-test-runner` 产出的失败上下文，为 `taichi-code-fixer` 提供修复依据。

## 二、何时调用本 Skill

**强制调用场景**：
- `taichi-test-runner` 运行结果 `summary.failed > 0`
- 用户提及「分析失败」「为什么测试失败」「定位 bug」「根因分析」
- copilot 中某轮测试存在失败，需要为修复决策提供依据
- 失败上下文文件已生成在 `reports/` 目录

**不应调用场景**：
- 测试全部通过（无需分析）
- 仅需重新运行测试（改用 `taichi-test-runner` / `taichi-regression-runner`）
- 仅需应用修复补丁（改用 `taichi-code-fixer`）

## 三、输入参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `failure_context_path` | string | 是 | 失败上下文 JSON 文件路径，由 taichi 在测试失败后生成到 `reports/` 目录，命名形如 `failures-round-1-<timestamp>.json` |

## 四、失败上下文格式（FailureContext JSON）

taichi 在测试存在失败用例时，将结构化失败上下文写入 `reports/` 目录，作为 taichi ↔ AI Agent 的信息交换契约。完整结构如下：

```json
{
  "project_name": "tickraft",
  "base_url": "http://127.0.0.1:8080",
  "timestamp": "2026-07-17T10:30:00Z",
  "project_root": "/Users/derekray/workspace/auzekalabs/tickraft",
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

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `project_name` | string | 被测项目名 |
| `base_url` | string | 被测服务基址（未启动环境时为空） |
| `timestamp` | string | 失败上下文生成时间（RFC3339，UTC） |
| `project_root` | string | 被测项目根目录的绝对路径，用于定位源码 |
| `env_log_path` | string | 环境（服务）日志文件路径，用于检查服务端错误 |
| `reports_dir` | string | 报告输出目录，包含本次运行的 JSON / JUnit XML / 摘要 |
| `total_cases` | int | 本次运行的用例总数 |
| `passed_cases` | int | 通过的用例数 |
| `failed_cases[]` | array | 失败用例详情列表 |
| `failed_cases[].skill_name` | string | 产出该用例的技能名（如 `api`、`ui`） |
| `failed_cases[].name` | string | 用例标识符 |
| `failed_cases[].message` | string | 失败的人类可读描述 |
| `failed_cases[].error` | string | 底层错误的字符串形式（如有） |
| `failed_cases[].duration` | string | 用例执行耗时 |

> 契约定义源码：[`pkg/failure/failure.go`](../../pkg/failure/failure.go)

## 五、分析步骤

收到失败上下文后，按以下顺序执行：

### Step 1：读取失败上下文

```
读取 failure_context_path → 解析 FailureContext JSON
                          → 提取 project_name / project_root / env_log_path
                          → 遍历 failed_cases 获取每个失败用例的 name / message / error
```

### Step 2：检查服务日志

```
读取 env_log_path 指向的服务日志
  → 检索失败时间窗内的 ERROR / panic / stack trace
  → 关联 failed_cases 中的 error 信息
  → 识别服务端异常（nil 解引用、连接拒绝、超时、配置缺失等）
```

若 `env_log_path` 为空（未启动环境），跳过本步并在报告中注明。

### Step 3：定位相关源码

```
以 project_root 为根
  → 根据 failed_cases[].name 中的模块/路由线索定位 handler / 业务代码
  → 根据 stack trace 中的文件:行号定位具体位置
  → 读取相关源码片段，理解控制流
```

### Step 4：分析根因

```
综合 用例失败信息 + 服务日志 + 源码上下文
  → 判定根因类别（nil 解引用 / 逻辑错误 / 配置缺失 / 依赖不可用 / 断言期望不符 / 环境问题）
  → 评估影响范围（是否影响其他用例）
  → 给出建议修复方式（具体到文件与行号）
```

## 六、输出格式（根因分析报告）

分析完成后，输出结构化根因分析报告。该报告作为 `taichi-code-fixer` 的输入依据：

```json
{
  "project_name": "tickraft",
  "analyzed_at": "2026-07-17T10:31:00Z",
  "total_failures": 2,
  "root_causes": [
    {
      "case_name": "ui:home_page_render",
      "skill_name": "ui",
      "root_cause": "首页 handler 因配置缺失导致 nil 解引用 panic",
      "category": "nil_dereference",
      "evidence": [
        "env_log: 2026-07-17T10:29:55Z panic: runtime error: invalid memory address",
        "source: internal/handler/home.go:42 访问了 config.Theme 而未判空"
      ],
      "affected_files": [
        "internal/handler/home.go"
      ],
      "suggested_fix": "在 home.go:42 添加 nil 检查，配置缺失时回退默认主题",
      "confidence": "high"
    }
  ],
  "summary": "2 个失败用例均源于首页 handler 的 nil 解引用，修复 home.go 即可同时解决。"
}
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `project_name` | string | 被测项目名 |
| `analyzed_at` | string | 分析完成时间（RFC3339） |
| `total_failures` | int | 失败用例总数 |
| `root_causes[]` | array | 根因列表，每个失败用例一项 |
| `root_causes[].case_name` | string | 失败用例名 |
| `root_causes[].skill_name` | string | 技能名 |
| `root_causes[].root_cause` | string | 根因一句话描述 |
| `root_causes[].category` | string | 根因类别（`nil_dereference` / `logic_error` / `config_missing` / `dependency_unavailable` / `assertion_mismatch` / `env_issue` / `unknown`） |
| `root_causes[].evidence[]` | string[] | 证据链（日志片段、源码位置） |
| `root_causes[].affected_files[]` | string[] | 涉及的源码文件（相对 project_root） |
| `root_causes[].suggested_fix` | string | 建议修复方式 |
| `root_causes[].confidence` | string | 置信度（`high` / `medium` / `low`） |
| `summary` | string | 总体结论 |

## 七、与其他 Skill 的衔接

- **上游**：`taichi-test-runner` 产出失败上下文文件
- **下游**：`taichi-code-fixer` 基于本报告的 `affected_files` 与 `suggested_fix` 生成修复

## 八、输出自检清单

- [ ] 是否成功解析了失败上下文 JSON
- [ ] 是否检查了 `env_log_path` 指向的服务日志
- [ ] 是否以 `project_root` 为基准定位源码（避免路径错误）
- [ ] 每个失败用例是否都给出了根因与涉及文件
- [ ] 根因类别是否准确归类
- [ ] 建议修复方式是否具体到文件与行号
