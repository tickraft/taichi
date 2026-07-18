> 🌐 语言: [English](README.md) | [中文](README.zh.md)

# Taichi 插件 SDK 总览

Taichi 的插件协议是语言无关的 JSON-over-stdio：Taichi 启动插件进程，将
`PluginInput` JSON 写入插件 stdin，插件执行测试后将 `PluginOutput` JSON 写入
stdout，stderr 日志由 Taichi 转发到自身日志器。`exit 0` 表示正常执行（通过/失败
由 stdout JSON 表达），`exit ≠ 0` 表示插件级致命错误。本目录提供常用语言的 SDK，
封装 stdin 读取、JSON 解析与 stdout 输出，降低用非 Go 语言编写第三方测试插件的
门槛，让插件开发者只关注测试逻辑本身。

协议字段定义与 Go 端 `taichi/pkg/skill/plugin/skill.go` 中的
`PluginInput` / `PluginOutput` / `PluginCase` 严格对齐，各语言 SDK 行为一致。

## 各语言 SDK 一览

| 语言 | 子目录 | 入口函数 | 示例运行命令 |
|------|--------|----------|--------------|
| Python | `python/` | `run_plugin(handler)` | `python3 sdks/python/example.py` |
| Node.js | `node/` | `runPlugin(handler)` | `node sdks/node/example.js` |
| Shell (bash) | `shell/` | `source taichi-plugin.sh` 后调用 `taichi_read_input` / `taichi_emit_output` | `bash sdks/shell/example.sh` |

三个示例插件行为一致：读取 `input.config.endpoints`，对 `base_url` + 端点做
HTTP GET，状态码 2xx 视为通过，否则失败，输出 `PluginOutput`。各自仅依赖语言
标准库 / 系统工具（Python `urllib`、Node 内置 `http`/`https`、Shell 的 `curl`），
无需安装第三方包。

## 本地调试

无需启动 Taichi 即可调试插件，直接用管道喂入 `PluginInput` JSON：

```bash
# Python
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health"]}}' | python3 sdks/python/example.py

# Node.js
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health"]}}' | node sdks/node/example.js

# Shell
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health"]}}' | bash sdks/shell/example.sh
```

## 与 Taichi 配置衔接

在 `taichi.yaml` 中声明 `kind: plugin`，`raw.command` 指向插件脚本，`raw` 中的
自定义字段（去除 `command`/`args`/`env`/`workdir`/`timeout` 后）会透传到
`input.config`：

```yaml
skills:
  - name: my-check
    kind: plugin
    enabled: true
    raw:
      command: python3 sdks/python/example.py   # 换成 node/bash 同理
      timeout: 30s
      endpoints: [/api/v1/users]                # 透传到 input.config
```

配置好后用 `taichi run -c <config>` 即可运行。

## 参考

- 仓库根目录的 [`examples/plugin-example.sh`](../examples/plugin-example.sh) 是
  更早的纯 bash 最小示例，未使用本目录的 SDK 封装，可作为协议层的对照参考，
  帮助理解 SDK 在协议之上做了哪些封装。
- Go 端协议实现：`taichi/pkg/skill/plugin/skill.go`
