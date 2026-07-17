> 🌐 语言: [English](README.md) | [中文](README.zh.md)

# taichi Python 插件 SDK

为 Python 开发者提供 taichi 插件协议的封装，让你只需编写测试逻辑，无需关心
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

- Python 3.8+
- **仅标准库**，无需 `pip install` 任何第三方包

将 `taichi_plugin.py` 放到你的插件脚本可 import 的目录（同目录最简单），
或加入 `PYTHONPATH`。

## SDK API

### 数据类

```python
from taichi_plugin import PluginInput, PluginOutput, PluginCase
```

- `PluginInput`：字段 `skill_name` / `project_name` / `base_url` / `reports_dir` / `config`（dict）。
  提供 `PluginInput.from_dict(d)` 从字典构造，以及 `input.endpoints()` 便捷读取
  `config["endpoints"]` 端点列表。
- `PluginCase`：字段 `name` / `passed` / `skipped` / `message` / `duration_ms` / `error`，
  `to_dict()` 输出协议 JSON（省略空可选字段，对齐 Go omitempty）。
- `PluginOutput`：字段 `cases`（list） / `error`，`to_dict()` 输出协议 JSON。

### 入口函数

```python
import taichi_plugin

def handler(input: taichi_plugin.PluginInput) -> taichi_plugin.PluginOutput:
    ...

if __name__ == "__main__":
    taichi_plugin.run_plugin(handler)
```

`run_plugin(handler)` 自动完成：

1. `json.load(sys.stdin)` 读取并解析 `PluginInput`
2. 调用 `handler(input)` 获取 `PluginOutput`
3. `json.dump(sys.stdout)` 写出结果，`exit 0`
4. handler 抛异常时输出带 `error` 的 `PluginOutput` 并 `exit 1`

### 用例构造辅助函数

| 函数 | 说明 |
|------|------|
| `pass_case(name, message="ok")` | 构造通过用例 |
| `fail_case(name, error, message="failed")` | 构造失败用例（error 必填） |
| `skip_case(name, message="skipped")` | 构造跳过用例（passed=False, skipped=True） |

### 底层函数

| 函数 | 说明 |
|------|------|
| `read_input(stream=sys.stdin)` | 读取并解析 `PluginInput` |
| `write_output(output, stream=sys.stdout)` | 写出 `PluginOutput` JSON |

## 示例运行方式

示例 `example.py` 读取 `config.endpoints`，对 `base_url` + 端点做 HTTP GET，
状态码 2xx 视为通过。本地调试可直接管道喂入 input：

```bash
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health","/api/v1/users"]}}' | python3 sdks/python/example.py
```

预期输出形如：

```json
{"cases":[{"name":"GET /health","passed":true,"message":"HTTP 200","duration_ms":12},{"name":"GET /api/v1/users","passed":true,"message":"HTTP 200","duration_ms":8}]}
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
      command: python3 sdks/python/example.py
      timeout: 30s
      endpoints: [/api/v1/users]   # 透传到 input.config
```

配置好后用 `taichi run -c <config>` 即可运行。
