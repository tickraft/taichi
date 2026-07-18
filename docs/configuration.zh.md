# 环境配置指南

> 🌐 语言: [English](configuration.md) | [中文](configuration.zh.md)

## 一、配置文件概览

Taichi 使用单一 YAML 配置文件描述被测项目、环境、技能与输出。配置 schema 定义于 [pkg/config/config.go](../pkg/config/config.go)。

五大顶层结构：

```yaml
projects: []     # 被测项目列表
envs: {}         # 环境定义映射
skills: []       # 技能配置列表
report: {}       # 报告输出
autofix: {}      # 自动修复
```

## 二、项目（Project）

```yaml
projects:
  - name: tickraft              # 必填，项目唯一标识
    root: ../../tickraft        # 项目根目录（相对配置文件所在目录，或绝对路径）
    env: tickraft-backend       # 引用 envs 中的键
    skills: [api, regression]   # 启用的技能名列表；空表示启用所有 enabled 技能
```

- `root` 为空时表示被测服务由外部托管（仅通过 `env.base_url` 访问）
- `skills` 为空或省略时，启用配置中所有 `enabled: true` 的技能

## 三、环境（Env）

### 3.1 环境类型（Kind）

| Kind | 值 | 说明 |
|------|-----|------|
| `EnvKindBackendGo` | `backend.go` | Go 后端：自动 go build + 启动二进制 |
| `EnvKindBackendNode` | `backend.node` | Node 后端：启动 node 二进制 |
| `EnvKindFrontendVite` | `frontend.vite` | Vite 前端：启动 dev server |
| `EnvKindFrontendNuxt` | `frontend.nuxt` | Nuxt 前端：启动 dev server |
| `EnvKindCustom` | `custom` | 自定义：复用 process 实现，通过 `command` 启动任意语言服务进程并以 `ready_url` 轮询就绪 |

> **重要约束**：env 名（`envs` 的 map 键）**不可含点号（`.`）**，否则会被 viper 当作嵌套键分隔符。使用连字符（`-`）分隔，如 `tickraft-backend`。

### 3.2 后端环境字段

```yaml
envs:
  tickraft-backend:
    kind: backend.go
    binary: bin/tickraft           # 二进制路径（相对项目根）；不存在时自动构建
    build_target: ./cmd/tickraft   # go build 目标
    config_path: configs/config.yaml  # 配置文件路径（相对项目根）
    config_flag: --config         # 配置参数名
    addr_flag: --addr             # 监听地址参数名
    health_path: /api/v1/health   # 健康检查路径
    healthy_timeout: 30s          # 等待就绪超时
    port: 0                       # 0=自动空闲端口；填具体值则固定端口
    args: [--debug]               # 附加命令行参数
    env: [LOG_LEVEL=debug]        # 附加环境变量（KEY=VALUE）
    base_url: ""                  # 非空时跳过启动，直接用此 URL（外部托管）
```

启动命令拼装：`<binary> <config_flag> <config_path> <addr_flag> :<port> <args...>`

### 3.3 前端环境字段

```yaml
envs:
  vite-local:
    kind: frontend.vite
    command: pnpm dev             # 启动命令（拆分为程序+参数；保留双引号包裹的含空格段，如 `npm "run dev" --port 5173`）
    cwd: .                        # 工作目录（相对项目根）
    ready_url: http://localhost:5173  # 轮询此 URL 直到 2xx/4xx
    ready_text: ""                # 非空时还要求响应体包含此子串
    healthy_timeout: 60s          # 等待就绪超时（默认 60s）；同样适用于 custom 环境
    port: 5173
    base_url: ""                  # 非空时跳过启动
```

前端环境启动子进程，捕获 stdout/stderr 到日志文件，轮询 `ready_url` 直到返回 2xx/4xx 状态码（且响应体包含 `ready_text`，若配置）。最长等待时长由 `healthy_timeout` 控制（默认 60s）。

### 3.4 外部托管环境

当 `base_url` 非空时，Taichi 跳过启动/停止，直接使用此 URL 访问被测服务：

```yaml
envs:
  external:
    kind: backend.go    # kind 仅占位
    base_url: https://api.example.com
```

## 四、技能配置（skill.Config）

```yaml
skills:
  - name: api            # 必填，与技能 Name() 一致
    kind: api            # 技能大类
    enabled: true        # 是否启用
    priority: 0          # 执行优先级（数值小先执行）
    raw:                 # 技能专属配置（技能自行解析）
      timeout: 5s
      cases: [...]
```

`raw` 字段的内容由各技能自定义，详见[测试用例编写规范](./test-cases.zh.md)。

### 4.1 gRPC 技能配置

内置 `grpc` 技能（`kind: grpc`，包路径 `github.com/tickraft/taichi/pkg/skill/grpc`）对目标服务执行配置驱动的 gRPC 冒烟检查。它专注于就绪/冒烟检查；若需带编译好的 protobuf 桩代码进行完整的 unary/streaming RPC 测试，请使用第三方插件技能（`kind: plugin`）编写小型 Go 助手实现。

`raw` 下字段：

| 字段 | 说明 | 默认值 |
|------|------|--------|
| `target` | 默认目标地址 `host:port`；可被 case 级 `target` 覆盖 | — |
| `insecure` | 使用明文 h2c（不启用 TLS）。为 `false` 时使用 TLS 传输凭证 | `true` |
| `timeout` | 连接 / 调用超时 | `5s` |
| `cases` | gRPC 用例列表（见下） | — |

每条 case 支持的字段：

| 字段 | 说明 |
|------|------|
| `name` | 用例名（必填；用作结果名） |
| `type` | 用例类型：`health` / `dial` / `reflect`（默认 `health`） |
| `target` | 单条用例的 `host:port`；缺省回退到技能级 `target` |
| `insecure` | 单条用例的 TLS 覆盖（布尔）；缺省回退到技能级 `insecure` |
| `expected_status` | 仅 `health`：期望的服务状态（`SERVING` / `NOT_SERVING` / `UNKNOWN` / `SERVICE_UNKNOWN`）；默认 `SERVING` |
| `expected_services` | 仅 `reflect`：必须暴露的完全限定服务名列表 |
| `max_latency` | 可选，该用例的时延上限（如 `2s`）；为空则跳过时延断言 |

用例类型：

- **`health`** — 调用 `grpc.health.v1.Health/Check` 并断言服务状态。
- **`dial`** — 建立 gRPC 连接，验证服务可达性。
- **`reflect`** — 查询 `grpc.reflection.v1.ServerReflection`，断言暴露的服务列表包含全部 `expected_services`。

示例：

```yaml
skills:
  - name: grpc
    kind: grpc
    enabled: true
    raw:
      target: 127.0.0.1:9090
      insecure: true
      timeout: 5s
      cases:
        - name: HealthServing
          type: health
          expected_status: SERVING
          max_latency: 2s
        - name: ServerReachable
          type: dial
        - name: ExposesExpectedServices
          type: reflect
          expected_services:
            - myapp.UserService
            - myapp.OrderService
```

## 五、报告配置（Report）

```yaml
report:
  suite_name: taichi-tickraft    # JUnit testsuite 名与 testcase classname
  output_dir: reports           # 报告输出目录
  formats: [json, junit, summary]  # 启用的格式；空/省略时默认三种全开
```

| 格式 | 值 | 输出文件 |
|------|-----|---------|
| JSON | `json` | `<project>-<timestamp>.json` |
| JUnit XML | `junit` | `<project>-<timestamp>.xml` |
| 摘要 | `summary` | `<project>-<timestamp>.txt`（空路径时输出到 stdout） |

## 六、自动修复配置（Autofix）

```yaml
autofix:
  enabled: false          # 默认关闭
  reports_dir: reports/errors  # 错误报告 JSON 输出目录
```

启用后，技能失败时将尝试：
1. **服务无响应** → 重启服务（最多 2 次）
2. **限流（429）** → 等待退避后通知重试
3. **5xx / 未知错误** → 写错误报告 JSON 供人工分析

## 七、环境配置模板

完整模板见 [`configs/taichi.example.yaml`](../configs/taichi.example.yaml)，覆盖所有环境类型与技能选项。

### 7.1 快速新建项目配置

1. 复制 `configs/taichi.example.yaml` 为你的 `taichi.yaml`
2. 修改 `projects[0].name`、`root`、`env`
3. 在 `envs` 下定义对应环境
4. 按需裁剪 `skills`
5. 运行 `taichi run -c taichi.yaml`

### 7.2 多项目配置

一个配置文件可包含多个项目，通过 `--project` 选择：

```bash
taichi run -c taichi.yaml -p my-backend
taichi run -c taichi.yaml -p my-frontend
```

## 八、配置校验规则

Taichi 在加载时自动校验：

- `projects[*].name` 不可为空
- `projects[*].env` 引用的键必须存在于 `envs`
- `skills[*].name` 不可为空且不可重复

校验失败时返回明确错误信息，不执行测试。

## 九、CLI 配置覆盖

`run` 命令支持覆盖配置：

| Flag | 作用 |
|------|------|
| `-c, --config` | 配置文件路径（默认 `configs/taichi.yaml`） |
| `-p, --project` | 指定本次运行的项目名 |
| `-s, --skill` | 只运行指定技能（可重复） |
| `--reports-dir` | 覆盖报告输出目录 |
| `--timeout` | 本次运行总超时 |
| `--log-level` | 日志级别（debug/info/warn/error） |
