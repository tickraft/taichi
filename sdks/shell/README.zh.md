> 🌐 语言: [English](README.md) | [中文](README.zh.md)

# Taichi Shell 插件 SDK

为 bash 开发者提供 Taichi 插件协议的封装，让你只需编写测试逻辑，无需关心
stdin 读取、JSON 解析与 stdout 输出。本库设计为被 `source` 使用。

## 协议概述

Taichi 通过 `kind: plugin` 接入第三方测试插件。插件是任意可执行程序，Taichi 与之
通过 JSON over stdin/stdout 通信：

| 方向 | 载体 | 内容 |
|------|------|------|
| taichi → 插件 | stdin | `PluginInput` JSON |
| 插件 → taichi | stdout | `PluginOutput` JSON |
| 插件 → taichi | stderr | 自由格式日志（Taichi 转发到自身日志器） |

退出码语义：`exit 0` = 正常执行（通过/失败由 stdout JSON 表达）；`exit ≠ 0` =
插件级致命错误。

### PluginInput 字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `skill_name` | string | 是 | 技能名 |
| `project_name` | string | 是 | 被测项目名 |
| `base_url` | string | 否 | 被测服务基址 |
| `reports_dir` | string | 否 | 报告输出目录 |
| `config` | object | 否 | 插件业务配置（taichi.yaml raw 段去除 command/args/env/workdir/timeout 后的剩余字段） |

### PluginOutput 字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `cases` | array | 是 | 测试用例结果 |
| `error` | string | 否 | 插件级致命错误消息（非空表示未完整执行） |

### PluginCase 字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 用例名 |
| `passed` | bool | 是 | 是否通过 |
| `skipped` | bool | 否 | 是否跳过（优先于 passed 计入跳过统计） |
| `message` | string | 否 | 人类可读描述 |
| `duration_ms` | int64 | 否 | 耗时毫秒 |
| `error` | string | 否 | 失败详情 |

## 依赖

- **bash 4+**（使用关联数组与 `${arr[@]:-}` 等特性）
- **JSON 解析**：优先使用 `jq`；若系统中没有 `jq`，自动回退到 `python3`。
  二者都没有时 SDK 无法工作。
- **HTTP 探测**（仅示例需要）：`curl`

检测顺序：`jq` → `python3`。建议安装 `jq` 以获得最佳性能与可靠性。

## SDK API

```bash
source ./taichi-plugin.sh
```

### 输入读取

| 函数 | 说明 |
|------|------|
| `taichi_read_input` | 读取 stdin 的 `PluginInput` JSON 到内部变量；同时检测 JSON 工具 |
| `taichi_input_field <key>` | 提取 input 顶层字符串字段（如 `base_url`） |
| `taichi_input_endpoints` | 提取 `config.endpoints`，每行打印一个端点 |

### 用例构造

| 函数 | 说明 |
|------|------|
| `taichi_emit_case <name> <passed:true\|false> [message] [error] [skipped:true\|false] [duration_ms]` | 追加一条完整 case（最通用形式） |
| `taichi_pass <name> [message]` | 通过用例 |
| `taichi_fail <name> <error> [message]` | 失败用例（error 必填） |
| `taichi_skip <name> [message]` | 跳过用例（passed=false, skipped=true） |
| `taichi_set_error <msg>` | 设置插件级致命错误消息 |

### 输出

| 函数 | 说明 |
|------|------|
| `taichi_emit_output` | 将已收集的用例与可选 error 拼装为 `PluginOutput` JSON 写到 stdout |

> `taichi_emit_case` 是底层通用构造函数，`taichi_pass` / `taichi_fail` /
> `taichi_skip` 是其便捷封装（不带 `duration_ms`）。需要记录耗时时直接调用
> `taichi_emit_case` 并传入第 6 个参数。

## 示例运行方式

示例 `example.sh` 读取 `config.endpoints`，对 `base_url` + 端点做 curl GET，
状态码 2xx 视为通过。本地调试可直接管道喂入 input：

```bash
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health","/api/v1/users"]}}' | bash sdks/shell/example.sh
```

预期输出形如：

```json
{"cases":[{"name":"PluginBootstrap","passed":true,"message":"插件启动成功"},{"name":"GET /health","passed":true,"message":"HTTP 200","duration_ms":45}]}
```

调试时的日志会写到 stderr，可被 Taichi 转发到自身日志器。

## 与 Taichi 配置衔接

在 `taichi.yaml` 中声明 `kind: plugin`，`raw.command` 指向脚本，自定义字段会
透传到 `input.config`：

```yaml
skills:
  - name: my-check
    kind: plugin
    enabled: true
    raw:
      command: bash sdks/shell/example.sh
      timeout: 30s
      endpoints: [/api/v1/users]   # 透传到 input.config
```

配置好后用 `taichi run -c <config>` 即可运行。

> 仓库根目录的 `examples/plugin-example.sh` 是更早的纯 bash 最小示例，
> 未使用本 SDK 封装，可作为协议层的对照参考。
