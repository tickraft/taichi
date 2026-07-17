---
name: "taichi-regression-runner"
description: "Runs regression tests after a code fix to verify the fix did not introduce new problems. Invoke when an AI Agent has completed a code fix and needs to verify, the user requests regression testing, or a copilot needs to re-run tests after a fix is applied. Inputs are the config file path, project name, and timeout; it calls the regression skill to run regression cases, with the pass criterion being all regression cases passing (Failed=0)."
---

> 🌐 Languages: [English](SKILL.md) | [中文](SKILL.zh.md)

# taichi Regression Runner Skill

## 1. Overview

This Skill lets an AI Agent run regression tests after a code fix, verifying that the fix eliminated the original failures without introducing new problems. taichi has a built-in `regression` skill (`pkg/skill/regression`); by filtering with `--skill regression`, only regression cases are executed.

This Skill is the "regression verification" stage of the taichi ↔ AI Agent bidirectional integration loop. It consumes the fix output from `taichi-code-fixer` and confirms the loop succeeded.

## 2. When to Invoke This Skill

**Mandatory invocation scenarios**:
- `taichi-code-fixer` has applied a fix (`fixed: true`) and the fix effect needs to be verified
- The user mentions "regression test", "regression", "verify the fix", "re-run tests"
- In a copilot, after a fix is applied, proceed to the next round of test verification

**Do not invoke scenarios**:
- No fix has been applied yet (call `taichi-code-fixer` first)
- Full test suite is needed rather than only regression cases (use `taichi-test-runner` without the `--skill` filter)
- Only analyzing failure causes (use `taichi-failure-analyzer`)

## 3. Input Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `config` | string | Yes | taichi config file path (YAML), e.g. `configs/taichi.yaml` |
| `project` | string | No | Project under test name. Empty means run the first project in the config |
| `timeout` | duration | No | Total timeout for this run (e.g. `30m`, `90s`). `0` means unlimited |
| `reports-dir` | string | No | Override the report output directory in the config |
| `log-level` | string | No | Log level: `debug` / `info` / `warn` / `error`; default `info` |

## 4. Invocation Methods

### 4.1 CLI Invocation

```bash
taichi run -c <config> --skill regression [--project <name>] [--timeout <dur>] [--reports-dir <dir>] [--log-level <level>]
```

Example:

```bash
# Run regression tests for the tickraft project, timeout 15 minutes
taichi run -c configs/taichi.yaml --project tickraft --skill regression --timeout 15m
```

Exit code: `0` if all pass; `1` if there are failures or runtime errors.

### 4.2 MCP Invocation

Call the `taichi_regression` tool exposed by the MCP Server:

```json
{
  "config": "configs/taichi.yaml",
  "project": "tickraft",
  "timeout": "15m"
}
```

## 5. Pass Criteria

The pass criteria for regression tests:

| Criterion | Standard |
|-----------|----------|
| `summary.failed` | `== 0` (all regression cases pass) |
| Exit code | `== 0` |

Meeting both criteria means regression passed; the copilot marks `Fixed=true` and ends the loop.

If `summary.failed > 0`, it means the fix did not fully take effect or introduced a new problem. The copilot will:
1. Regenerate the failure context
2. Enter the next analyze-fix round (if `max-rounds` is not exceeded)
3. Or return the final failure result after exhausting rounds

## 6. Output Format

The regression test result is identical to that of `taichi-test-runner`; see [`taichi-test-runner` output format](../test-runner/SKILL.md#5-output-format).

Structured result (JSON):

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

### Field Descriptions

| Field | Type | Description |
|-------|------|-------------|
| `project_name` | string | Project under test name |
| `base_url` | string | Base URL of the service under test |
| `duration` | string | Total execution duration |
| `summary.total` | int | Total number of regression cases |
| `summary.passed` | int | Number of passed cases |
| `summary.failed` | int | Number of failed cases (must be 0 for regression to pass) |
| `summary.skipped` | int | Number of skipped cases |
| `skill_results[]` | array | Per-skill execution results (only `regression` in the regression scenario) |

## 7. Integration with Other Skills

- **Upstream**: Triggered after `taichi-code-fixer` applies a fix
- **Loop exit**: Regression pass (`failed == 0`) means the entire test → analyze → fix → regression loop succeeded

```
taichi-test-runner ──failure──► taichi-failure-analyzer ──► taichi-code-fixer ──► taichi-regression-runner
       ▲                                                                            │
       └────────────────────── regression still fails, return to start ─────────────┘
```

## 8. Output Self-check List

- [ ] Used `--skill regression` filter (avoid running unrelated skills)
- [ ] Checked `summary.failed == 0` and the exit code
- [ ] On failure, regenerated the failure context for the next round of analysis
- [ ] Timeout setting has margin (regression may take longer than a single test round)
