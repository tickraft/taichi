---
name: "taichi-code-fixer"
description: "Fixes project source code based on failure analysis results. Invoke when an AI Agent has completed root cause analysis and needs to generate code fixes, the user requests fixing test failures, or a copilot needs to produce a FixResult for Taichi to apply. The input is failure.Context JSON (via stdin or file path); the output is FixResult JSON (patch or direct mode)."
---

> 🌐 Languages: [English](SKILL.md) | [中文](SKILL.zh.md)

# Taichi Code Fixer Skill

## 1. Overview

This Skill lets an AI Agent analyze failure causes based on Taichi's failure context (`failure.Context`, referred to as `Context` below) and produce code fixes. Taichi hands the failure context to the Agent via the `agent.Invoker` interface; the Agent returns a `FixResult`, and Taichi then applies the fix (patch mode via `git apply`; direct mode only validates).

This Skill is the "fix" stage of the Taichi ↔ AI Agent bidirectional integration loop. It consumes the root cause analysis from `taichi-failure-analyzer` and produces fixes for `taichi-regression-runner` to verify.

## 2. When to Invoke This Skill

**Mandatory invocation scenarios**:
- `taichi-failure-analyzer` has produced a root cause analysis report that needs to be materialized into code changes
- The user mentions "fix test failure", "fix the bug", "generate a patch", "apply a patch"
- A copilot round has a test failure and needs the Agent to produce a `FixResult` for Taichi to apply

**Do not invoke scenarios**:
- Root cause has not been analyzed yet (call `taichi-failure-analyzer` first)
- All tests passed (no fix needed)
- Only need to re-run tests (use `taichi-regression-runner`)

## 3. Input Parameters

The Agent receives `failure.Context` JSON via the `agent.Invoker` interface:

| Delivery method | Description |
|-----------------|-------------|
| **CLIInvoker** | Receives `failure.Context` JSON via stdin, outputs `FixResult` JSON to stdout |
| **HTTPInvoker** | Receives `failure.Context` JSON via HTTP POST body, reads `FixResult` JSON from the response body |
| **File path** | Receives the failure context file path (`reports/failures-round-N-<timestamp>.json`); the Agent reads it itself |

The `failure.Context` JSON structure is detailed in [`taichi-failure-analyzer`](../failure-analyzer/SKILL.md#4-failure-context-format-failurecontext-json).

## 4. Fix Modes

Two fix execution methods are supported, declared by the Agent in `FixResult.mode`:

### 4.1 patch mode (`mode: "patch"`)

The Agent generates a unified diff patch; Taichi applies it to the project source via `git apply` (falling back to the `patch` command).

Applicable when: the Agent has no direct file editing capability, or needs Taichi to uniformly apply and roll back.

### 4.2 direct mode (`mode: "direct"`)

The Agent directly modifies source files via file editing tools; Taichi only verifies that the modified files exist and are readable.

Applicable when: the Agent has file editing capability (e.g. an in-IDE Agent); Taichi does not intervene in file writing.

## 5. Output Format (FixResult JSON)

The Agent must output the following `FixResult` JSON to stdout (CLIInvoker) or the HTTP response body (HTTPInvoker):

```json
{
  "fixed": true,
  "mode": "patch",
  "patch": "--- a/internal/handler/home.go\n+++ b/internal/handler/home.go\n@@ -39,6 +39,11 @@\n func HomeHandler(c *Context) {\n+    if c.Config == nil {\n+        c.Config = DefaultConfig()\n+    }\n     theme := c.Config.Theme\n     render(c, theme)\n }\n",
  "modified_files": ["internal/handler/home.go"],
  "message": "Fixed nil pointer dereference",
  "analysis": "Health endpoint panicked due to nil dereference; added nil check at handler.go:42"
}
```

### Field Descriptions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `fixed` | bool | Yes | `true` means the Agent believes the fix was successful; `false` means it cannot be fixed, and Taichi terminates the loop |
| `mode` | string | Yes | Fix method: `patch` or `direct` |
| `patch` | string | Required for patch mode | Patch content in unified diff format |
| `modified_files` | string[] | Yes | List of modified files (required in both modes, used for validation and rollback) |
| `message` | string | Yes | Human-readable description returned by the Agent |
| `analysis` | string | No | The Agent's analysis of the failure cause |

> Contract definition source: [`pkg/agent/agent.go`](../../pkg/agent/agent.go)

### direct mode example

```json
{
  "fixed": true,
  "mode": "direct",
  "modified_files": ["internal/handler/home.go", "internal/config/default.go"],
  "message": "Added default theme fallback when config is missing",
  "analysis": "home.go panics when config is nil; added nil check and introduced DefaultConfig"
}
```

## 6. Patch Format Requirements

The `patch` field must be a standard unified diff, following the git diff style:

1. **Path prefixes**: Use `a/` and `b/` prefixes (git diff style); paths are resolved relative to `project_root`
2. **File headers**: Each file starts with `--- a/<path>` and `+++ b/<path>`
3. **Diff lines**: Context lines start with a space, added lines start with `+`, deleted lines start with `-`
4. **Hunk headers**: `@@ -<start>,<len> +<start>,<len> @@` format
5. **Multiple hunks per file**: The same file can have multiple `@@` hunks
6. **Multiple files**: Multiple files are arranged sequentially, each with its own `---` / `+++` header

Example:

```diff
--- a/internal/handler/home.go
+++ b/internal/handler/home.go
@@ -39,6 +39,11 @@ func HomeHandler(c *Context) {
 func HomeHandler(c *Context) {
+    if c.Config == nil {
+        c.Config = DefaultConfig()
+    }
     theme := c.Config.Theme
     render(c, theme)
 }
```

Taichi first calls `git apply --whitespace=fix` when applying, falling back to `patch -p1`. The `a/` `b/` prefixes in paths are automatically stripped (`-p1`).

## 7. Constraints

1. **Only modify relevant files**: Only modify files related to the failure root cause; do not introduce unrelated changes
2. **No new dependencies**: Do not introduce new third-party dependencies for the fix; prefer the standard library and existing project dependencies
3. **Follow project code conventions**:
   - Go code follows `gofmt` standard formatting
   - All functions returning `error` must explicitly handle errors; use `%w` for error wrapping
   - Business logic must not `panic`; return exceptions via `error`
   - Logging via `zap`; do not print directly to stdout
4. **Minimize changes**: Fixes should focus on eliminating the failure; do not refactor unrelated code or add irrelevant comments
5. **Rollbackable**: In patch mode, Taichi can roll back via `git checkout`; in direct mode, the Agent should ensure changes are reversible

## 8. Integration with Other Skills

- **Upstream**: `taichi-failure-analyzer` provides root cause and affected files
- **Downstream**: `taichi-regression-runner` verifies whether the fix eliminates the failure without introducing new problems

## 9. Failure Handling

| Scenario | Agent response |
|----------|----------------|
| Cannot locate root cause | Return `fixed: false`; `message` explains it cannot be fixed; Taichi terminates the current round |
| Fix would introduce risk | Return `fixed: false`; `message` explains the risk and recommends manual intervention |
| Patch application fails | Taichi records `ApplyError`; copilot proceeds to the next round or terminates |

## 10. Output Self-check List

- [ ] `fixed` and `mode` fields are correctly filled
- [ ] In patch mode, the `patch` field is a standard unified diff with `a/` `b/` path prefixes
- [ ] `modified_files` lists all changed files (relative to `project_root`)
- [ ] Only files related to the failure are modified; no unrelated changes
- [ ] No new dependencies introduced; no project code conventions violated
- [ ] The patch can be validated with `git apply --check`
