> 🌐 语言: [English](README.md) | [中文](README.zh.md)

# taichi Node.js 插件 SDK

为 Node.js 开发者提供 taichi 插件协议的封装，让你只需编写测试逻辑，无需关心
stdin 读取、JSON 解析与 stdout 输出。

## 协议概述

taichi 通过 `kind: plugin` 接入第三方测试插件。插件是任意可执行程序，taichi 与之
通过 JSON over stdin/stdout 通信：

| 方向 | 载体 | 内容 |
|------|------|------|
| taichi → 插件 | stdin | `PluginInput` JSON |
| 插件 → taichi | stdout | `PluginOutput` JSON |
| 插件 → taichi | stderr | 自由格式日志（taichi 转发到自身日志器） |

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

## 安装与依赖

- Node.js 14+
- **仅内置模块**（`fs` / `http` / `https`），无需 `npm install`

将 `taichi-plugin.js` 放到你的插件脚本可 require 的目录（同目录最简单）。

## SDK API

```js
const {
  runPlugin,
  readInput,
  writeOutput,
  endpoints,
  passCase,
  failCase,
  skipCase,
} = require('./taichi-plugin');
```

### 入口函数

```js
runPlugin((input) => {
  // input: { skill_name, project_name, base_url, reports_dir, config }
  return { cases: [passCase('Bootstrap')] };
});
```

`runPlugin(handler)` 自动完成：

1. 同步读取 stdin 并 `JSON.parse` 为 `PluginInput` 对象
2. 调用 `handler(input)` 获取 output（支持同步返回或 `async` 返回 Promise）
3. 序列化 output 写到 stdout，冲刷后 `exit 0`
4. handler 抛异常或 Promise reject 时，输出带 `error` 的 output 并 `exit 1`

> async handler：直接 `runPlugin(async (input) => { ... })` 即可，SDK 会等待
> Promise 完成。退出前会等待 stdout 冲刷，避免管道缓冲丢失输出。

### 用例构造辅助函数

| 函数 | 说明 |
|------|------|
| `passCase(name, message='ok')` | 构造通过用例 |
| `failCase(name, error, message='failed')` | 构造失败用例（error 必填） |
| `skipCase(name, message='skipped')` | 构造跳过用例（passed=false, skipped=true） |

### 底层函数

| 函数 | 说明 |
|------|------|
| `readInput()` | 同步读取并解析 stdin 的 `PluginInput` 对象 |
| `writeOutput(output, done?)` | 将 output 序列化为 JSON 写到 stdout；`done` 为写出完成回调 |
| `endpoints(input)` | 便捷读取 `input.config.endpoints` 端点列表（返回字符串数组） |

### output 对象形状

```js
{
  cases: [
    { name: 'GET /health', passed: true, message: 'HTTP 200', duration_ms: 12 },
    { name: 'GET /x', passed: false, error: 'unexpected status: 500', duration_ms: 8 }
  ],
  error: '' // 可选，非空表示插件级致命错误
}
```

空的可选字段会被省略，对齐 Go 端 `omitempty` 语义。

## 示例运行方式

示例 `example.js` 读取 `config.endpoints`，对 `base_url` + 端点做 HTTP GET，
状态码 2xx 视为通过。本地调试可直接管道喂入 input：

```bash
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health","/api/v1/users"]}}' | node sdks/node/example.js
```

预期输出形如：

```json
{"cases":[{"name":"GET /health","passed":true,"message":"HTTP 200","duration_ms":12}]}
```

调试时的日志会写到 stderr，可被 taichi 转发到自身日志器。

## 与 taichi 配置衔接

在 `taichi.yaml` 中声明 `kind: plugin`，`raw.command` 指向脚本，自定义字段会
透传到 `input.config`：

```yaml
skills:
  - name: my-check
    kind: plugin
    enabled: true
    raw:
      command: node sdks/node/example.js
      timeout: 30s
      endpoints: [/api/v1/users]   # 透传到 input.config
```

配置好后用 `taichi run -c <config>` 即可运行。
