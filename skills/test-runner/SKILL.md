---
name: "taichi-test-runner"
description: "Executes automated tests via the Taichi CLI or MCP Server. Invoke when the user requests running a Taichi test orchestration, executing project tests, viewing a test result summary, or triggering a test run within an AI Agent workflow. Inputs are the config file path, project name, skill filter, and timeout; the output is a structured test result summary (passed/failed/skipped counts, duration, per-skill results)."
---

> 🌐 Languages: [English](SKILL.md) | [中文](SKILL.zh.md)

# Taichi Test Runner Skill

## 1. Overview

This Skill lets an AI Agent execute a complete automated test orchestration via Taichi. Taichi is a general-purpose test orchestration framework: it reads a config file describing the project under test, environments, and skills, then orchestrates a full test run and produces JSON / JUnit XML / human-readable summary reports.

This Skill is the "test execution" stage of the Taichi ↔ AI Agent bidirectional integration loop, producing `TestResults` for downstream failure analysis and fix consumption.

## 2. When to Invoke This Skill

**Mandatory invocation scenarios**:
- The user mentions "run tests", "execute taichi", "verify the project"
- An AI Agent workflow needs to trigger a test run to obtain a baseline result
- After fixing code, tests need to be re-run to confirm the fix effect (in conjunction with `taichi-regression-runner`)
- Need to run a specific type of test filtered by skill (API only, UI only, etc.)

**Do not invoke scenarios**:
- Only analyzing failure causes (use `taichi-failure-analyzer` instead)
- Only fixing code (use `taichi-code-fixer` instead)
- Simply viewing the config (use the `taichi list` command)

## 3. Input Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `config` | string | Yes | taichi config file path (YAML), e.g. `configs/taichi.yaml` |
| `project` | string | No | Project under test name. Empty means run the first project in the config |
| `skill` | string[] | No | Skill filter; only run the specified skills (repeatable, e.g. `api`, `ui`). Empty means run all skills configured for the project |
| `timeout` | duration | No | Total timeout for this run (e.g. `30m`, `90s`). `0` means unlimited; listens for SIGINT/SIGTERM for graceful cancellation |
| `reports-dir` | string | No | Override the report output directory in the config |
| `log-level` | string | No | Log level: `debug` / `info` / `warn` / `error`; default `info` |

## 4. Invocation Methods

### 4.1 CLI Invocation

```bash
taichi run -c <config> [--project <name>] [--skill <name>] [--timeout <dur>] [--reports-dir <dir>] [--log-level <level>]
```

Examples:

```bash
# Run the first project in the config with all skills
taichi run -c configs/taichi.yaml

# Run only the api and ui skills of the tickraft project, timeout 30 minutes
taichi run -c configs/taichi.yaml --project tickraft --skill api --skill ui --timeout 30m
```

Exit code: `0` if all pass; `1` if there are failures or runtime errors.

### 4.2 MCP Invocation

Call the `taichi_run` tool exposed by the MCP Server:

```json
{
  "config": "configs/taichi.yaml",
  "project": "tickraft",
  "skill": ["api", "ui"],
  "timeout": "30m"
}
```

## 5. Output Format

### 5.1 Console Summary

The CLI prints a human-readable summary to stdout:

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

### 5.2 Structured Result (JSON)

The result is also written as a JSON file to `reports/<project>-<timestamp>.json`, with the following structure:

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

### 5.3 Field Descriptions

| Field | Type | Description |
|-------|------|-------------|
| `project_name` | string | Project under test name |
| `base_url` | string | Base URL of the service under test (empty when no environment is started) |
| `duration` | string | Total execution duration |
| `summary.total` | int | Total number of cases |
| `summary.passed` | int | Number of passed cases |
| `summary.failed` | int | Number of failed cases |
| `summary.skipped` | int | Number of skipped cases |
| `skill_results[]` | array | Per-skill execution results |
| `skill_results[].skill_name` | string | Skill name (`api` / `ui` / `static` / `regression`) |
| `skill_results[].duration` | string | Skill execution duration |
| `skill_results[].summary` | object | Aggregated statistics for that skill |
| `skill_results[].error` | string \| null | Non-null indicates a skill-level fatal error |

## 6. Handoff with Failure Context

When `summary.failed > 0`, Taichi generates a `failure.Context` JSON file in the `reports/` directory (named like `failures-round-1-<timestamp>.json`) for `taichi-failure-analyzer` to consume. After the test finishes, the Agent should check the exit code and `summary.failed`:

- `failed == 0`: Tests passed; no further action needed
- `failed > 0`: Call `taichi-failure-analyzer` to read the failure context for analysis

## 7. Exit Codes and Error Handling

| Exit code | Meaning | Agent response |
|-----------|---------|----------------|
| `0` | All cases passed | End the flow; can output a success summary |
| `1` | There are failed cases or runtime errors | Inspect the failure context under `reports/` and enter the analyze-fix flow |

Runtime-level errors (config load failure, environment startup failure, no skill selected) are returned with a non-zero exit code and an error message printed to stderr. The Agent should capture the stderr content and report it upward, rather than entering the fix flow.

## 8. Output Self-check List

- [ ] A valid config file path (`-c`) was provided
- [ ] Skill filter names match the `skills[].name` in the config
- [ ] The timeout setting is reasonable (avoid being too short and causing interruption)
- [ ] The exit code and the `summary.failed` field were checked
- [ ] On failure, the failure context file under `reports/` was located for downstream consumption
