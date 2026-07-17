---
name: "taichi-config-generator"
description: "通过分析项目源码自动生成 taichi 测试编排配置文件（taichi.yaml）。当用户请求为新项目生成测试配置、初始化 taichi 测试、或需要让 taichi 适配新项目时调用。Agent 扫描项目目录结构、识别项目类型、提取 HTTP 路由与健康检查端点，产出可直接使用的 taichi.yaml，后期可由人工微调。"
---

> 🌐 语言: [English](SKILL.md) | [中文](SKILL.zh.md)

# taichi 配置生成器 Skill

## 一、简介

本 Skill 让 AI Agent 通过静态分析项目源码与目录结构，自动生成 taichi 测试编排框架所需的配置文件（`taichi.yaml`）。taichi 是一个通用的自动化测试编排框架，通过配置文件描述被测项目、环境与技能，即可编排一次完整的测试运行。

生成的配置文件覆盖以下内容：
- 被测项目声明（项目名、根目录、环境绑定、技能启用列表）
- 环境定义（Go/Node 后端、Vite/Nuxt 前端的启动与就绪探测）
- 技能配置（api / grpc / ui / static / regression 五类内置技能的用例）
- 报告输出与自动修复配置
- 界面语言设置

生成结果可直接执行 `taichi run` 验证；用户可在此基础上手动增删用例、调整断言。

## 二、何时调用本 Skill

**强制调用场景**：
- 用户提及「生成 taichi 配置」「初始化测试配置」「为新项目添加 taichi 测试」
- 用户希望让 taichi 适配一个尚未配置的新项目
- 现有 `taichi.yaml` 缺失或损坏，需要重建
- 项目发生重大重构（路由全量变更、技术栈迁移），需要重新生成配置

**不应调用场景**：
- 项目已有 `taichi.yaml` 且仅需小范围调整（建议直接编辑）
- 仅需运行测试（改用 `taichi-test-runner`）
- 项目尚处于脚手架阶段，无实际可测路由（先实现核心功能再生成）

## 三、输入参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `project_root` | string | 是 | 被分析的项目根目录绝对路径 |
| `output_path` | string | 否 | 生成的配置文件输出路径，默认 `<project_root>/taichi.yaml` |
| `project_name` | string | 否 | 项目名，默认从 `go.mod` 的 module 名或 `package.json` 的 name 字段推断 |
| `env_kind` | string | 否 | 强制指定环境类型：`backend.go` / `backend.node` / `frontend.vite` / `frontend.nuxt`。不指定时自动检测 |
| `base_url` | string | 否 | 若被测服务已在外部运行，直接指定基址跳过启动（如 `http://localhost:8080`） |
| `locale` | string | 否 | 生成的配置文件中的 `locale` 字段值，默认 `auto` |

## 四、分析流程

Agent 应按以下步骤分析项目，逐步收集配置所需的字段。

### 4.1 项目类型识别

按优先级检查以下文件以确定项目类型：

| 检测信号 | 项目类型 | env.kind |
|----------|----------|----------|
| 根目录存在 `go.mod` 且 `cmd/` 下有 main 包 | Go 后端 | `backend.go` |
| 根目录存在 `package.json` 且含 `next`/`nuxt` 依赖 | Nuxt 前端 | `frontend.nuxt` |
| 根目录存在 `package.json` 且含 `vite` 依赖 | Vite 前端 | `frontend.vite` |
| 根目录存在 `package.json` 且 `scripts.start` 指向 node 服务 | Node 后端 | `backend.node` |

若同时存在前后端信号（如 monorepo），生成多个 project 与多个 env，分别对应前后端。

### 4.2 项目元信息提取

| 字段 | 提取规则 |
|------|----------|
| `project_name` | Go：`go.mod` 的 module 路径最后一段；Node：`package.json` 的 `name` 字段；其余：根目录名 |
| `project.root` | 相对 `taichi.yaml` 输出位置的项目根目录路径 |
| `build_target` | Go：`./cmd/<project_name>` 或扫描 `cmd/` 子目录；Node：无需 |
| `binary` | Go：`bin/<project_name>`；Node：无需 |
| `config_path` | 优先 `configs/config.yaml`，其次 `config.yaml`、`configs/config.yml`、`configs/<project_name>.yaml` |
| `config_flag` | Go：`--config`；Node：从 `scripts.start` 中提取，默认 `--config` |
| `addr_flag` | Go：`--addr`；Node：从启动脚本推断，默认 `--port` |
| `health_path` | 见 4.4 路由提取 |

### 4.3 前端就绪探测提取

仅当 env.kind 为 `frontend.*` 时：

| 字段 | 提取规则 |
|------|----------|
| `command` | `package.json` 的 `scripts.dev`，默认 `pnpm dev` |
| `cwd` | 前端源码所在子目录，默认 `.` |
| `ready_url` | 从 vite.config 的 `server.port` 推断，默认 `http://localhost:5173`（vite）或 `http://localhost:3000`（nuxt） |
| `ready_text` | 空字符串，仅以 HTTP 状态 < 500 判定就绪 |

### 4.4 HTTP 路由提取

**Go 项目**：扫描以下位置的路由注册代码，提取 `method` 与 `path`：

- `hertz`/`gin`/`echo`/`fiber`/`chi` 路由注册：`r.GET("/path", ...)`、`r.POST("/path", ...)`
- `net/http` 标准库：`http.HandleFunc("/path", ...)`
- 路由分组前缀：识别 `r.Group("/api/v1")` 并拼接子路由

**Node 项目**：扫描以下位置：

- `express`：`app.get("/path", ...)`、`app.post("/path", ...)`
- `fastify`：`fastify.get("/path", ...)`
- `koa-router`：`router.get("/path", ...)`

**健康检查识别**：优先识别以下路径作为 `health_path` 与回归测试用例：
- `/health`、`/healthz`、`/healthz/ready`、`/healthz/live`
- `/api/v1/health`、`/api/health`
- `/ping`、`/api/ping`
- `/ready`、`/readyz`

若未识别到任何健康检查端点，在生成的 api 技能用例中添加注释提示用户手动指定。

### 4.5 用例生成策略

#### api 技能

为每个识别到的 HTTP 路由生成一个测试用例：

```yaml
- name: <RouteName>          # 从 path 转为 PascalCase，如 /api/v1/users → UsersList
  method: <GET|POST|...>     # 从路由注册提取
  path: <path>               # 原始路径
  expected_status: 200       # GET 默认 200；POST/PUT 默认 200 或 201；DELETE 默认 200 或 204
  expected_code: 0           # 若项目响应体含 code 字段，默认 0 表示成功
```

对于含路径参数的路由（如 `/api/v1/users/:id`），生成一个使用占位 ID 的用例并标注 `expected_status: 404`（因占位 ID 通常不存在）。

#### ui 技能（仅前端项目）

从路由配置或页面目录提取页面路径：

- Vue Router：扫描 `router/routes.ts` 或 `router/index.ts`
- Next/Nuxt：扫描 `pages/` 目录结构
- 无路由配置时：仅测试根路径 `/`

```yaml
- path: <page-path>
  contains: [<html]       # 基础断言：响应包含 HTML 标签
  max_latency: 2s         # 可选
```

#### static 技能（仅前端项目）

```yaml
pages:
  - /                      # 首页
  - /nonexistent-page-12345  # SPA fallback 验证
assets:
  - /.gitkeep
  - /_nuxt/app.js          # nuxt
  - /assets/main.js        # vite（从 index.html 提取实际资源路径）
```

#### regression 技能

从 api 技能的用例中选取关键路径（健康检查、核心列表接口、首页）作为回归用例，去除断言细化字段，仅保留 `expected_status` 与 `expected_code`：

```yaml
- name: Health
  path: /api/v1/health
  expected_status: 200
  expected_code: 0
```

### 4.6 报告与语言配置

```yaml
report:
  suite_name: taichi-<project_name>
  output_dir: reports
  formats: [json, junit, summary]

autofix:
  enabled: false
  reports_dir: reports/errors

locale: auto                # 或根据用户系统环境推断
```

## 五、输出格式

Agent 应将生成的配置以 YAML 文件写入 `output_path`，并返回如下结构的 JSON 摘要：

```json
{
  "output_path": "/path/to/taichi.yaml",
  "project_name": "tickraft",
  "env_kind": "backend.go",
  "detected_routes": 18,
  "generated_cases": {
    "api": 18,
    "ui": 0,
    "static": 0,
    "regression": 5
  },
  "health_path": "/api/v1/health",
  "warnings": [
    "未识别到健康检查端点，已添加占位用例，请手动确认",
    "路由 /api/v1/users/:id 含路径参数，已生成 404 用例"
  ],
  "next_steps": [
    "执行 `taichi run -c <output_path>` 验证配置",
    "检查 warnings 中列出的待确认项",
    "根据业务语义补充 expected_field / expected_value 断言"
  ]
}
```

## 六、生成模板

以下是生成的 `taichi.yaml` 的标准结构模板。Agent 应按此结构产出，字段顺序与注释风格保持一致。

### 6.1 Go 后端项目模板

```yaml
# taichi 配置 —— <project_name> 项目
# 由 taichi-config-generator skill 自动生成，可手动修改。
# 配置 schema 详见 pkg/config/config.go 与 docs/configuration.md。

# ========== 被测项目 ==========
projects:
  - name: <project_name>
    root: <project_root_relative>
    env: <project_name>-backend
    skills: [api, regression]      # 按识别到的路由类型调整

# ========== 环境定义 ==========
envs:
  <project_name>-backend:
    kind: backend.go
    binary: bin/<project_name>
    build_target: ./cmd/<project_name>
    config_path: configs/config.yaml
    config_flag: --config
    addr_flag: --addr
    health_path: <health_path>
    healthy_timeout: 30s

# ========== 技能配置 ==========
skills:
  - name: api
    kind: api
    enabled: true
    priority: 0
    raw:
      timeout: 5s
      cases:
        - name: Health
          method: GET
          path: <health_path>
          expected_status: 200
          expected_code: 0
        # ... 其余路由生成的用例

  - name: regression
    kind: regression
    enabled: true
    priority: 30
    raw:
      timeout: 5s
      cases:
        - name: Health
          path: <health_path>
          expected_status: 200
          expected_code: 0
        # ... 其余关键路径回归用例

# ========== 报告输出 ==========
report:
  suite_name: taichi-<project_name>
  output_dir: reports
  formats: [json, junit, summary]

# ========== 自动修复 ==========
autofix:
  enabled: false
  reports_dir: reports/errors

# ========== 界面语言 ==========
locale: auto
```

### 6.2 Vite/Nuxt 前端项目模板

```yaml
# taichi 配置 —— <project_name> 前端项目
# 由 taichi-config-generator skill 自动生成，可手动修改。

projects:
  - name: <project_name>
    root: <project_root_relative>
    env: <project_name>-frontend
    skills: [ui, static, regression]

envs:
  <project_name>-frontend:
    kind: frontend.vite          # 或 frontend.nuxt
    command: pnpm dev
    cwd: .
    ready_url: http://localhost:5173
    ready_text: ""
    port: 5173

skills:
  - name: ui
    kind: ui
    enabled: true
    priority: 10
    raw:
      timeout: 5s
      pages:
        - path: /
          contains: [<html]
          max_latency: 2s
        # ... 其余页面

  - name: static
    kind: static
    enabled: true
    priority: 20
    raw:
      timeout: 5s
      pages:
        - /
        - /nonexistent-page-12345
      assets:
        - /.gitkeep
        - /assets/main.js

  - name: regression
    kind: regression
    enabled: true
    priority: 30
    raw:
      timeout: 5s
      cases:
        - name: Homepage
          path: /
          expected_status: 200
          skip_on_404: true

report:
  suite_name: taichi-<project_name>
  output_dir: reports
  formats: [json, junit, summary]

autofix:
  enabled: false
  reports_dir: reports/errors

locale: auto
```

### 6.3 全栈项目模板（前后端合并）

当项目同时包含 Go 后端与前端时，生成两个 project 与两个 env：

```yaml
projects:
  - name: <project_name>-api
    root: <backend_root>
    env: <project_name>-backend
    skills: [api, regression]
  - name: <project_name>-web
    root: <frontend_root>
    env: <project_name>-frontend
    skills: [ui, static, regression]

envs:
  <project_name>-backend:
    kind: backend.go
    # ... 同 6.1
  <project_name>-frontend:
    kind: frontend.vite
    # ... 同 6.2

skills:
  # ... api + ui + static + regression 合并配置
```

### 6.4 外部托管服务模板

当 `base_url` 参数指定时，env 中省略启动相关字段：

```yaml
envs:
  <project_name>-external:
    kind: backend.go           # kind 仍需指定，但 binary/build_target 可省略
    base_url: http://localhost:8080
    health_path: /api/v1/health
```

## 七、生成后验证

生成配置后，Agent 应执行以下验证步骤并将结果包含在输出摘要中：

1. **YAML 语法校验**：确保生成的文件可被 YAML 解析器加载
2. **taichi 加载校验**：执行 `taichi list -c <output_path>`，确认配置可被 taichi 正确加载
3. **路由合理性检查**：
   - 健康检查用例是否实际可访问（若服务已在运行）
   - 路径参数路由是否标注了 `expected_status: 404`
4. **warnings 汇总**：将所有需人工确认的项列入输出 JSON 的 `warnings` 数组

若 `taichi list` 失败，Agent 应修正配置中的明显错误（如路径错误、字段名拼写错误）并重试，最多 3 轮。

## 八、与其它 Skill 的衔接

| 衔接场景 | 下游 Skill | 触发条件 |
|----------|-----------|----------|
| 配置生成后首次运行测试 | `taichi-test-runner` | 用户确认配置无误后，执行 `taichi run -c <output_path>` |
| 首次运行出现失败 | `taichi-failure-analyzer` | `taichi run` 返回非零退出码 |
| 失败用例需修复 | `taichi-code-fixer` | failure-analyzer 产出根因后 |
| 修复后回归验证 | `taichi-regression-runner` | code-fixer 应用修复后 |

典型全流程：

```
config-generator → test-runner → failure-analyzer → code-fixer → regression-runner
```

## 九、输出自检清单

生成配置后，Agent 应逐条自检：

- [ ] 生成的 YAML 语法正确，可被 `taichi list -c <path>` 加载
- [ ] `projects[].name` 唯一且非空
- [ ] `projects[].env` 引用的环境在 `envs` 中已定义
- [ ] `envs` 中的 `kind` 字段值是支持的枚举值之一
- [ ] 后端环境的 `health_path` 已识别或标注待确认
- [ ] `skills` 中每个技能的 `name` 与 `kind` 一致
- [ ] api 技能的每个用例至少包含 `name`、`method`、`path`、`expected_status`
- [ ] 含路径参数的路由用例标注了 `expected_status: 404`
- [ ] regression 技能的用例从 api 关键路径中选取
- [ ] `report.formats` 至少包含 `json`
- [ ] `locale` 字段值为 `auto` / `zh-CN` / `en-US` 之一
- [ ] `warnings` 数组已列出所有需人工确认的项
- [ ] `next_steps` 已提示用户执行 `taichi run` 验证
