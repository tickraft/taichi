# Test Cases Specification

> 🌐 Languages: [English](test-cases.md) | [中文](test-cases.zh.md)

## 1. Built-in Skill Case Writing

### 1.1 API Skill

Validates HTTP endpoints: status code + unified response contract + specified field values + response latency.

```yaml
skills:
  - name: api
    kind: api
    enabled: true
    priority: 0
    raw:
      timeout: 5s               # per-request timeout
      cases:
        - name: Health          # required, case name (for report display)
          method: GET           # default GET
          path: /api/v1/health  # required, path (appended to BaseURL)
          headers:              # optional, custom request headers
            CF-IPCountry: CN
          expected_status: 200  # default 200
          expected_code: 0      # default 0; when non-0, validates the unified response code field
          expected_field: data.status  # optional, validates a JSONPath field
          expected_value: healthy       # used with expected_field
          max_latency: 500ms    # optional, response latency upper bound
```

**Validation order**: status code → unified response contract (code/msg/request_id) → specified field value → response latency. Any failure means the case fails.

### 1.2 UI Skill

Validates page accessibility, HTML markup, keyword contains, first-byte latency.

```yaml
skills:
  - name: ui
    kind: ui
    enabled: true
    priority: 10
    raw:
      timeout: 5s
      pages:
        - path: /                      # required, page path
          contains: [<html, <div id="app"]  # optional, substrings the response body must contain
          max_latency: 2s              # optional, response latency upper bound
```

**Validation order**: status code (200) → HTML markup contains → response latency.

### 1.3 Static Resource Skill

Validates static resource accessibility and SPA fallback.

```yaml
skills:
  - name: static
    kind: static
    enabled: true
    priority: 20
    raw:
      timeout: 5s
      pages:            # page type: 200 + <html marker; 404 treated as skip
        - /
        - /nonexistent-page-12345
      assets:           # asset type: 200 or 404 both treated as pass
        - /.gitkeep
        - /_nuxt/app.js
```

Difference between **pages** and **assets**:
- `pages`: expects 200 + HTML; on 404 records as skipped (resource not built)
- `assets`: 200 or 404 both treated as pass (missing resource does not block)

### 1.4 Regression Test Skill

After auto-fix or other skill execution, re-probes key path endpoints.

```yaml
skills:
  - name: regression
    kind: regression
    enabled: true
    priority: 30
    raw:
      timeout: 5s
      cases:
        - name: Health           # required
          path: /api/v1/health   # required
          expected_status: 200   # default 200
          expected_code: 0       # default 0; when non-0, validates unified response code
          skip_on_404: false     # default false; when true, 404 is treated as skip
```

Differences from the API skill:
- Smaller, more stable case set (key paths only)
- Lowest priority (runs last)
- Any failure means overall regression failure

## 2. Unified Response Contract

Taichi has built-in validation for the tickraft system's unified response contract: all API responses should contain top-level fields `code` / `msg` / `request_id`.

```
{
  "code": 0,           # business code, 0 means success
  "msg": "ok",         # human-readable message
  "request_id": "xxx", # request trace ID
  "data": { ... }      # business data
}
```

Validated via `skill.AssertCommonEnvelope(asserts, body, expectedCode)`. Auto-triggered when the case configures `expected_code` and the response body contains a `code` field.

## 3. Assertion Engine

`framework.AssertionEngine` provides the following assertion methods (source: [pkg/framework/assert.go](../pkg/framework/assert.go)):

| Method | Validated Content |
|--------|-------------------|
| `AssertStatusCode(resp, expected)` | HTTP status code equals expected |
| `AssertJSONField(body, field, expected)` | JSON top-level field value equals expected |
| `AssertJSONFieldsExist(body, fields...)` | JSON top-level fields exist |
| `AssertJSONPath(body, path, expected)` | JSONPath (dot-separated, e.g. `data.region`) field value |
| `AssertHTMLContains(body, subs...)` | Response body contains all given substrings |
| `AssertResponseTime(elapsed, max)` | Response latency does not exceed upper bound |

All assertions return `AssertResult{Passed bool, Message string}`.

## 4. Test Result Types

```go
type TestResult struct {
    Name     string        // case name
    Passed   bool          // whether passed
    Skipped  bool          // whether skipped (not counted as failure)
    Message  string        // result description
    Duration time.Duration // execution duration
    Error    error         // underlying error (if any)
}
```

- **Pass**: `Passed=true`
- **Fail**: `Passed=false, Skipped=false`
- **Skip**: `Passed=false, Skipped=true` (environment limitation, not counted as failure)

## 5. Result Naming Convention

To distinguish sources in reports, built-in skills use prefix naming:

| Skill | Result Name Prefix | Example |
|-------|-------------------|---------|
| api | none | `Health` |
| ui | `ui:` | `ui:/product` |
| static | `page:` / `asset:` | `page:/`, `asset:/.gitkeep` |
| regression | `regression:` | `regression:Health` |

Custom skills should follow a similar convention.

## 6. Config Value Types

When reading config from the `raw` map, note the types after YAML parsing:

| YAML Syntax | Parsed Type | Read Function |
|-------------|-------------|---------------|
| `5s` | `string` | `GetDuration` (supports string parsing) |
| `200` | `int` | `GetInt` |
| `true` | `bool` | `GetBool` |
| `healthy` | `string` | `GetString` |
| `[a, b]` | `[]any` | skill converts itself |
| `{key: val}` | `map[string]any` | skill converts itself |

## 7. Best Practices

### 7.1 Case Naming
- Use business-semantic names (`Health`, `RegionDetect`), avoid `test1`, `case_a`
- Names serve as unique identifiers in reports and should be readable and unique

### 7.2 Priority Assignment
- Critical smoke path: `priority: 0` (Critical)
- Regular features: `priority: 10` (High)
- Auxiliary validation: `priority: 20` (Normal)
- Regression / long-running: `priority: 30` (Low)

### 7.3 Skip Strategy
- 404s from unbuilt frontends should use `skip_on_404: true` to avoid false positives
- Environment-related flaky cases should be split into separate skills for easy `-s` filtering

### 7.4 Timeout Settings
- Per-request timeout (`timeout`): recommended 5s
- Health check ready timeout (`healthy_timeout`): recommended 30s
- Orchestration total timeout (`--timeout`): estimate by case count to avoid infinite waits

### 7.5 Report Format
- CI environments: recommend `junit` format (parseable by CI systems)
- Local debugging: recommend `summary` format (human-readable)
- Archival analysis: recommend `json` format (structured)

## 8. Case Writing Example

For a complete example, see [`configs/taichi.yaml`](../configs/taichi.yaml) (the tickraft project's 7 API + 7 UI + static + 6 regression cases).
