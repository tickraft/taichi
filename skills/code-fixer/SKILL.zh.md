---
name: "taichi-code-fixer"
description: "根据失败分析结果修复项目源码。当 AI Agent 完成根因分析后需要生成代码修复、用户请求修复测试失败、或在 copilot 中需要产出 FixResult 供 Taichi 应用时调用。输入为 failure.Context JSON（通过 stdin 或文件路径），输出为 FixResult JSON（patch 或 direct 模式）。"
---

> 🌐 语言: [English](SKILL.md) | [中文](SKILL.zh.md)

# Taichi 代码修复器 Skill

## 一、简介

本 Skill 用于让 AI Agent 根据 Taichi 的失败上下文（`failure.Context`，下文简称 `Context`）分析失败原因并产出代码修复。Taichi 通过 `agent.Invoker` 接口将失败上下文交给 Agent，Agent 返回 `FixResult`，Taichi 再应用修复（patch 模式经 `git apply`，direct 模式仅校验）。

本 Skill 是 Taichi ↔ AI Agent 双向集成闭环中的「修复」环节，承接 `taichi-failure-analyzer` 的根因分析，产出供 `taichi-regression-runner` 验证的修复。

## 二、何时调用本 Skill

**强制调用场景**：
- `taichi-failure-analyzer` 已产出根因分析报告，需要落地为代码修改
- 用户提及「修复测试失败」「fix the bug」「生成补丁」「打 patch」
- copilot 中某轮测试失败，需要 Agent 产出 `FixResult` 供 Taichi 应用

**不应调用场景**：
- 尚未分析根因（先调用 `taichi-failure-analyzer`）
- 测试全部通过（无需修复）
- 仅需重新运行测试（改用 `taichi-regression-runner`）

## 三、输入参数

Agent 通过 `agent.Invoker` 接口接收 `failure.Context` JSON：

| 传递方式 | 说明 |
|---------|------|
| **CLIInvoker** | 通过 stdin 接收 `failure.Context` JSON，向 stdout 输出 `FixResult` JSON |
| **HTTPInvoker** | 通过 HTTP POST body 接收 `failure.Context` JSON，从响应体读取 `FixResult` JSON |
| **文件路径** | 接收失败上下文文件路径（`reports/failures-round-N-<timestamp>.json`），由 Agent 自行读取 |

`failure.Context` JSON 结构详见 [`taichi-failure-analyzer`](../failure-analyzer/SKILL.zh.md#四失败上下文格式failurecontext-json)。

## 四、修复模式

支持两种修复执行方式，由 Agent 在 `FixResult.mode` 中声明：

### 4.1 patch 模式（`mode: "patch"`）

Agent 生成 unified diff 补丁，Taichi 通过 `git apply`（回退到 `patch` 命令）应用到项目源码。

适用场景：Agent 无直接文件编辑能力，或需要 Taichi 统一应用与回滚。

### 4.2 direct 模式（`mode: "direct"`）

Agent 通过文件编辑工具直接修改源码文件，Taichi 仅验证修改后的文件存在且可读。

适用场景：Agent 具备文件编辑能力（如 IDE 内 Agent），Taichi 不介入文件写入。

## 五、输出格式（FixResult JSON）

Agent 必须向 stdout（CLIInvoker）或 HTTP 响应体（HTTPInvoker）输出如下 `FixResult` JSON：

```json
{
  "fixed": true,
  "mode": "patch",
  "patch": "--- a/internal/handler/home.go\n+++ b/internal/handler/home.go\n@@ -39,6 +39,11 @@\n func HomeHandler(c *Context) {\n+    if c.Config == nil {\n+        c.Config = DefaultConfig()\n+    }\n     theme := c.Config.Theme\n     render(c, theme)\n }\n",
  "modified_files": ["internal/handler/home.go"],
  "message": "修复了空指针引用",
  "analysis": "Health 端点因 nil 解引用 panic，在 handler.go:42 添加 nil 检查"
}
```

### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `fixed` | bool | 是 | `true` 表示 Agent 认为已成功修复；`false` 表示无法修复，Taichi 终止循环 |
| `mode` | string | 是 | 修复方式：`patch` 或 `direct` |
| `patch` | string | patch 模式必填 | unified diff 格式的补丁内容 |
| `modified_files` | string[] | 是 | 被修改的文件列表（两种模式均需填充，用于验证与回滚） |
| `message` | string | 是 | Agent 返回的人类可读描述 |
| `analysis` | string | 否 | Agent 对失败原因的分析说明 |

> 契约定义源码：[`pkg/agent/agent.go`](../../pkg/agent/agent.go)

### direct 模式示例

```json
{
  "fixed": true,
  "mode": "direct",
  "modified_files": ["internal/handler/home.go", "internal/config/default.go"],
  "message": "添加了配置缺失时的默认主题回退",
  "analysis": "home.go 在 config 为 nil 时 panic，已添加 nil 检查并引入 DefaultConfig"
}
```

## 六、patch 格式要求

`patch` 字段必须为标准 unified diff，遵循 git diff 风格：

1. **路径前缀**：使用 `a/` 与 `b/` 前缀（git diff 风格），路径相对于 `project_root` 解析
2. **文件头**：每个文件以 `--- a/<path>` 与 `+++ b/<path>` 开头
3. **差分行**：上下文行以空格开头，新增行以 `+` 开头，删除行以 `-` 开头
4. **块头**：`@@ -<start>,<len> +<start>,<len> @@` 格式
5. **单文件多块**：同一文件可有多个 `@@` 块
6. **多文件**：多个文件依次排列，每个文件独立 `---` / `+++` 头

示例：

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

Taichi 应用时优先调用 `git apply --whitespace=fix`，失败回退 `patch -p1`。路径中的 `a/` `b/` 前缀会被自动剥离（`-p1`）。

## 七、约束

1. **只修改相关文件**：仅修改与失败根因相关的文件，不引入无关变更
2. **不引入新依赖**：禁止为修复引入新的第三方依赖，优先使用标准库与项目已有依赖
3. **遵循项目代码规范**：
   - Go 代码遵循 `gofmt` 标准格式
   - 所有返回 `error` 的函数显式处理，错误包装使用 `%w`
   - 业务逻辑禁止 `panic`，通过 `error` 返回异常
   - 日志通过 `zap` 记录，禁止直接打印到 stdout
4. **最小化改动**：修复应聚焦于消除失败，不重构无关代码、不添加无关注释
5. **可回滚**：patch 模式下 Taichi 可通过 `git checkout` 回滚；direct 模式下 Agent 应确保修改可逆

## 八、与其他 Skill 的衔接

- **上游**：`taichi-failure-analyzer` 提供根因与涉及文件
- **下游**：`taichi-regression-runner` 验证修复是否消除失败且未引入新问题

## 九、失败处理

| 场景 | Agent 应对 |
|------|-----------|
| 无法定位根因 | 返回 `fixed: false`，`message` 说明无法修复，Taichi 终止当前轮次 |
| 修复会引入风险 | 返回 `fixed: false`，`message` 说明风险，建议人工介入 |
| patch 应用失败 | Taichi 记录 `ApplyError`，copilot 进入下一轮或终止 |

## 十、输出自检清单

- [ ] `fixed` 与 `mode` 字段已正确填写
- [ ] patch 模式下 `patch` 字段为标准 unified diff，路径带 `a/` `b/` 前缀
- [ ] `modified_files` 列出所有改动文件（相对 `project_root`）
- [ ] 仅修改与失败相关的文件，无无关变更
- [ ] 未引入新依赖，未违反项目代码规范
- [ ] patch 可通过 `git apply --check` 验证
