# 测试用例编写规范

> 🌐 语言: [English](test-cases.md) | [中文](test-cases.zh.md)

## 一、内置技能用例写法

### 1.1 API 技能

验证 HTTP 端点：状态码 + 统一响应契约 + 指定字段值 + 响应时延。

```yaml
skills:
  - name: api
    kind: api
    enabled: true
    priority: 0
    raw:
      timeout: 5s               # 单请求超时
      cases:
        - name: Health          # 必填，用例名（用于报告展示）
          method: GET           # 默认 GET
          path: /api/v1/health  # 必填，路径（拼接到 BaseURL 后）
          headers:              # 可选，自定义请求头
            CF-IPCountry: CN
          expected_status: 200  # 默认 200
          expected_code: 0      # 默认 0；非 0 时验证统一响应 code 字段
          expected_field: data.status  # 可选，验证 JSONPath 字段
          expected_value: healthy       # 与 expected_field 配合
          max_latency: 500ms    # 可选，响应时延上限
```

**验证顺序**：状态码 → 统一响应契约(code/msg/request_id) → 指定字段值 → 响应时延。任一失败即该用例失败。

### 1.2 UI 技能

验证页面可访问性、HTML 标记、关键字包含、首字节时延。

```yaml
skills:
  - name: ui
    kind: ui
    enabled: true
    priority: 10
    raw:
      timeout: 5s
      pages:
        - path: /                      # 必填，页面路径
          contains: [<html, <div id="app"]  # 可选，响应体须包含的子串
          max_latency: 2s              # 可选，响应时延上限
```

**验证顺序**：状态码(200) → HTML 标记包含 → 响应时延。

### 1.3 静态资源技能

验证静态资源可访问性、SPA fallback。

```yaml
skills:
  - name: static
    kind: static
    enabled: true
    priority: 20
    raw:
      timeout: 5s
      pages:            # 页面类：200 + <html 标记；404 视为 skip
        - /
        - /nonexistent-page-12345
      assets:           # 资源类：200 或 404 均视为 pass
        - /.gitkeep
        - /_nuxt/app.js
```

**pages** 与 **assets** 的区别：
- `pages`：期望 200 + HTML；404 时记为 skipped（资源未构建）
- `assets`：200 或 404 均视为 pass（资源不存在不阻断）

### 1.4 回归测试技能

在自动修复或其他技能执行后，重新探测关键路径端点。

```yaml
skills:
  - name: regression
    kind: regression
    enabled: true
    priority: 30
    raw:
      timeout: 5s
      cases:
        - name: Health           # 必填
          path: /api/v1/health   # 必填
          expected_status: 200   # 默认 200
          expected_code: 0       # 默认 0；非 0 时验证统一响应 code
          skip_on_404: false     # 默认 false；true 时 404 视为跳过
```

与 API 技能的区别：
- 用例集更小、更稳定（仅关键路径）
- 优先级最低（最后执行）
- 失败即整体回归失败

## 二、统一响应契约

Taichi 内置对 tickraft 体系统一响应契约的验证：所有 API 响应应包含顶层字段 `code` / `msg` / `request_id`。

```
{
  "code": 0,           # 业务码，0 表示成功
  "msg": "ok",         # 人类可读消息
  "request_id": "xxx", # 请求追踪 ID
  "data": { ... }      # 业务数据
}
```

通过 `skill.AssertCommonEnvelope(asserts, body, expectedCode)` 验证。当用例配置了 `expected_code` 且响应体含 `code` 字段时自动触发。

## 三、断言引擎

`framework.AssertionEngine` 提供以下断言方法（源码：[pkg/framework/assert.go](../pkg/framework/assert.go)）：

| 方法 | 验证内容 |
|------|---------|
| `AssertStatusCode(resp, expected)` | HTTP 状态码等于期望值 |
| `AssertJSONField(body, field, expected)` | JSON 顶层字段值等于期望值 |
| `AssertJSONFieldsExist(body, fields...)` | JSON 顶层字段存在 |
| `AssertJSONPath(body, path, expected)` | JSONPath（点分隔，如 `data.region`）字段值 |
| `AssertHTMLContains(body, subs...)` | 响应体包含所有给定子串 |
| `AssertResponseTime(elapsed, max)` | 响应时延不超过上限 |

所有断言返回 `AssertResult{Passed bool, Message string}`。

## 四、测试结果类型

```go
type TestResult struct {
    Name     string        // 用例名
    Passed   bool          // 是否通过
    Skipped  bool          // 是否跳过（不计入失败）
    Message  string        // 结果描述
    Duration time.Duration // 执行耗时
    Error    error         // 底层错误（如有）
}
```

- **通过**：`Passed=true`
- **失败**：`Passed=false, Skipped=false`
- **跳过**：`Passed=false, Skipped=true`（环境限制，不计入失败）

## 五、结果命名约定

为便于在报告中区分来源，内置技能采用前缀命名：

| 技能 | 结果名前缀 | 示例 |
|------|-----------|------|
| api | 无 | `Health` |
| ui | `ui:` | `ui:/product` |
| static | `page:` / `asset:` | `page:/`、`asset:/.gitkeep` |
| regression | `regression:` | `regression:Health` |

自定义技能建议遵循类似约定。

## 六、配置值类型

从 `raw` map 读取配置时，注意 YAML 解析后的类型：

| YAML 写法 | 解析类型 | 读取函数 |
|-----------|---------|---------|
| `5s` | `string` | `GetDuration`（支持字符串解析） |
| `200` | `int` | `GetInt` |
| `true` | `bool` | `GetBool` |
| `healthy` | `string` | `GetString` |
| `[a, b]` | `[]any` | 技能自行转换 |
| `{key: val}` | `map[string]any` | 技能自行转换 |

## 七、最佳实践

### 7.1 用例命名
- 使用业务语义命名（`Health`、`RegionDetect`），避免 `test1`、`case_a`
- 名称在报告中作为唯一标识，应可读且唯一

### 7.2 优先级分配
- 关键冒烟路径：`priority: 0`（Critical）
- 常规功能：`priority: 10`（High）
- 辅助验证：`priority: 20`（Normal）
- 回归/长耗时：`priority: 30`（Low）

### 7.3 跳过策略
- 前端未构建导致的 404 应使用 `skip_on_404: true`，避免误报
- 环境相关的不稳定用例应拆分为独立技能，便于按 `-s` 过滤

### 7.4 超时设置
- 单请求超时（`timeout`）建议 5s
- 健康检查就绪超时（`healthy_timeout`）建议 30s
- 编排总超时（`--timeout`）按用例数估算，避免无限等待

### 7.5 报告格式
- CI 环境推荐 `junit` 格式（可被 CI 系统解析）
- 本地调试推荐 `summary` 格式（人类可读）
- 归档分析推荐 `json` 格式（结构化）

## 八、用例编写示例

完整示例参考 [`configs/taichi.yaml`](../configs/taichi.yaml)（tickraft 项目的 7 API + 7 UI + 静态 + 6 回归用例）。
