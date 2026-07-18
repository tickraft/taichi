# 框架扩展开发文档

> 🌐 语言: [English](extending.md) | [中文](extending.zh.md)

## 一、扩展点总览

Taichi 提供四个扩展点，按使用频率排序：

| 扩展点 | 接口 | 包 | 典型场景 |
|--------|------|-----|---------|
| 自定义技能 | `skill.TestSkill` | `pkg/skill` | 新增测试类型（如 gRPC、Playwright E2E） |
| 第三方插件技能 | 任意语言 + JSON-over-stdio | `pkg/skill/plugin` + `sdks/` | 用 Python/Node/Shell 等编写测试技能，无需编译进 Taichi |
| 自定义环境 | `env.Environment` | `pkg/env` | 新增环境类型（如 Docker、K8s port-forward）；`custom` 类型已支持任意启动命令 |
| 自定义报告格式 | `report.Writer` | `pkg/report` | HTML 报告、Slack 通知、飞书卡片 |
| 修复规则 | `autofix.FixRule` | `pkg/autofix` | 新增自动修复策略 |

> **内置技能**：Taichi 内置五个技能 —— `api` / `grpc` / `ui` / `static` / `regression`。其中 `grpc` 技能（`pkg/skill/grpc`，`kind: grpc`）仅覆盖配置驱动的冒烟检查（health / connectivity / reflection），刻意不做动态 protobuf 消息构造。若需带编译好的 protobuf 桩代码进行完整的 unary/streaming RPC 测试，请实现第三方插件技能（`kind: plugin`），用小型 Go 助手导入生成的 client 包。

> **插件进程环境**：插件进程会继承 Taichi 父进程的全部环境变量（`os.Environ()`），并叠加为该插件配置的 `env` 字段。请注意敏感变量的隔离。

## 二、自定义技能

### 2.1 实现 TestSkill 接口

```go
package grpc

import (
    "time"
    "github.com/tickraft/taichi/pkg/framework"
    "github.com/tickraft/taichi/pkg/skill"
)

// Skill 是 gRPC 测试技能。
type Skill struct {
    cfg    skill.Config
    cases  []grpcCase
    timeout time.Duration
}

type grpcCase struct {
    Service string
    Method  string
    // ...
}

// Name 返回技能唯一标识符。
func (s *Skill) Name() string { return "grpc" }

// Kind 返回技能大类。
func (s *Skill) Kind() skill.Kind { return skill.KindCustom }

// Priority 返回执行优先级。
func (s *Skill) Priority() skill.Priority { return skill.PriorityNormal }

// Configure 解析 raw 配置。
func (s *Skill) Configure(cfg skill.Config) error {
    s.cfg = cfg
    s.timeout = skill.GetDuration(cfg.Raw, "timeout", skill.DefaultHTTPTimeout)
    // 解析 cases...
    return nil
}

// Setup 准备资源（如建立 gRPC 连接）。
func (s *Skill) Setup(ctx *skill.Context) error {
    return nil
}

// Run 执行测试，结果通过 ctx.Reporter.Record 上报。
func (s *Skill) Run(ctx *skill.Context) skill.Result {
    start := time.Now()
    for _, c := range s.cases {
        caseStart := time.Now()
        // 执行 gRPC 调用...
        passed := true
        msg := "ok"
        skill.RecordResult(ctx.Reporter, "grpc:"+c.Service, caseStart, passed, msg, nil)
    }
    return skill.Result{
        SkillName: s.Name(),
        Duration:  time.Since(start),
        Summary:   ctx.Reporter.Summary(),
    }
}

// Teardown 释放资源。
func (s *Skill) Teardown(ctx *skill.Context) error {
    return nil
}
```

### 2.2 注册技能

#### 方式一：编译到 Taichi 二进制

在 `pkg/skill/builtin/builtin.go` 的 `Skills()` 中追加（`cmd/taichi` 与 `pkg/mcp` 共用的唯一来源）：

```go
func Skills() []skill.TestSkill {
    return []skill.TestSkill{
        &api.Skill{},
        &grpc.Skill{},
        &ui.Skill{},
        &static.Skill{},
        &regression.Skill{},
        &mySkill.Skill{},  // 新增
    }
}
```

#### 方式二：运行时动态注册

```go
o := orchestrator.New()
o.RegisterBuiltinSkills(builtin.Skills())
// 动态注册自定义技能
o.Registry().Register(&mySkill.Skill{}, true)
```

#### 方式三：通过注册中心卸载

```go
o.Registry().Unregister("static")  // 卸载静态资源技能
```

### 2.3 配置文件对接

在 `taichi.yaml` 的 `skills` 下增加配置：

```yaml
skills:
  - name: grpc
    kind: custom
    enabled: true
    priority: 20
    raw:
      timeout: 10s
      cases:
        - service: helloworld.Greeter
          method: SayHello
          expected_code: 0
```

技能名（`name`）必须与 `Skill.Name()` 返回值一致。

### 2.4 技能间数据传递

通过 `ctx.Extra` map 传递弱类型数据（键名加技能名前缀避免冲突）：

```go
// 生产者技能
ctx.Extra["screenshot.last_dir"] = "/tmp/shots"

// 消费者技能
if dir, ok := ctx.Extra["screenshot.last_dir"].(string); ok {
    // 使用 dir
}
```

## 三、自定义环境

### 3.1 实现 Environment 接口

```go
type Environment interface {
    Start(ctx context.Context) (string, error)  // 启动并返回 BaseURL
    Stop(ctx context.Context) error              // 停止
    BaseURL() string                             // 当前 BaseURL
    LogPath() string                             // 日志路径（如有）
}
```

### 3.2 注册自定义环境

`env.New()` 按 `spec.Kind` 分发。要支持新 Kind：

1. 在 `pkg/config/config.go` 增加常量：`EnvKindDocker EnvKind = "docker"`
2. 在 `pkg/env/env.go` 的 `New()` 增加 case：

```go
func New(spec EnvSpec, projectRoot string) (Environment, error) {
    switch spec.Kind {
    case config.EnvKindBackendGo, config.EnvKindBackendNode:
        return newBackend(spec, projectRoot), nil
    case config.EnvKindFrontendVite, config.EnvKindFrontendNuxt:
        return newFrontend(spec, projectRoot), nil
    case config.EnvKindDocker:
        return newDocker(spec, projectRoot), nil
    // ...
    }
}
```

3. 实现环境类型：

```go
type dockerEnv struct {
    spec        EnvSpec
    projectRoot string
    containerID string
    baseURL     string
}

func newDocker(spec EnvSpec, projectRoot string) *dockerEnv {
    return &dockerEnv{spec: spec, projectRoot: projectRoot}
}

func (d *dockerEnv) Start(ctx context.Context) (string, error) {
    // docker run -d -p <port>:<port> <image>
    // 返回 BaseURL
}

func (d *dockerEnv) Stop(ctx context.Context) error {
    // docker stop <containerID>
}
```

## 四、自定义报告格式

### 4.1 实现 Writer 接口

```go
type Writer interface {
    Format() report.Format
    Write(w io.Writer, summary framework.TestSummary, results []framework.TestResult) error
}
```

### 4.2 注册并使用

```go
// 在 cmd/taichi/run.go 中注册
reg := report.NewRegistry()
reg.Register(&htmlWriter{})

// 修改 orchestrator 调用，传入 registry（需扩展 Options）
```

### 4.3 HTML 报告示例

```go
type htmlWriter struct{}

func (htmlWriter) Format() report.Format { return "html" }

func (htmlWriter) Write(w io.Writer, summary framework.TestSummary, results []framework.TestResult) error {
    fmt.Fprintf(w, "<html><body><h1>Test Report</h1>")
    fmt.Fprintf(w, "<p>Total: %d, Passed: %d, Failed: %d</p>",
        summary.Total, summary.Passed, summary.Failed)
    // 渲染结果表格...
    fmt.Fprintf(w, "</body></html>")
    return nil
}
```

配置中启用：

```yaml
report:
  formats: [json, junit, html]
```

## 五、自定义修复规则

### 5.1 实现 FixRule 接口

```go
type FixRule interface {
    Match(detected ErrorType) bool
    Apply(ctx *FixContext) FixResult
    Name() string
}
```

### 5.2 注册规则

```go
engine := autofix.NewFixEngine(lifecycle, reportsDir)
engine.Register(&myFixRule{})
```

### 5.3 示例：数据库重连规则

```go
type DBReconnectRule struct{}

func (DBReconnectRule) Match(detected autofix.ErrorType) bool {
    return detected == autofix.ErrorTypeServerError
}

func (DBReconnectRule) Name() string { return "DBReconnectRule" }

func (DBReconnectRule) Apply(ctx *autofix.FixContext) autofix.FixResult {
    // 尝试重连数据库
    if reconnectDB() {
        return autofix.FixResult{
            RuleName: "DBReconnectRule",
            Fixed:    true,
            Retry:    true,
            Message:  "database reconnected",
        }
    }
    return autofix.FixResult{RuleName: "DBReconnectRule", Fixed: false}
}
```

规则按注册顺序匹配，首个 `Match` 返回 true 的规则胜出。

## 六、钩子机制

### 6.1 技能级钩子

实现 `skill.Hook` 接口获得全局生命周期通知：

```go
func (s *MySkill) BeforeAll(ctx *skill.Context) error {
    // 所有技能执行前：初始化共享资源
    return nil
}

func (s *MySkill) AfterAll(ctx *skill.Context) error {
    // 所有技能执行后：清理共享资源
    return nil
}
```

### 6.2 编排级扩展点

编排器 `Run` 方法的 9 步流程提供天然的扩展点：

| 步骤 | 扩展方式 |
|------|---------|
| 配置加载 | 自定义 `config.Load` 或预处理配置文件 |
| 项目选择 | 通过 `--project` flag |
| 环境启停 | 自定义 `Environment` 实现 |
| 技能选择 | 通过 `--skill` flag 或项目 `skills` 字段 |
| 技能执行 | 注册自定义技能 |
| 报告生成 | 注册自定义 `report.Writer` |

## 七、跨项目复用

Taichi 作为独立 Go module，可被任意项目引用：

### 7.1 作为库引用

```go
// 在其他项目的 go.mod
require github.com/tickraft/taichi v0.1.0
replace github.com/tickraft/taichi => ../taichi  // 本地开发
```

```go
import (
    "github.com/tickraft/taichi/pkg/framework"
    "github.com/tickraft/taichi/pkg/skill"
)
```

### 7.2 作为二进制使用

无需代码集成，仅编写配置文件：

```bash
taichi run -c my-project-taichi.yaml
```

### 7.3 tickraft 集成示例

tickraft 项目通过如下方式集成 Taichi：
- tickraft 的测试 runner 改为 import `github.com/tickraft/taichi/pkg/{framework,autofix}`
- `tickraft/go.mod` 通过 `replace` 指令引用本地 taichi

详见 [tickraft/tests/runner/main_test.go](../../tickraft/tests/runner/main_test.go)。

## 八、扩展开发检查清单

- [ ] 技能 `Name()` 返回值与配置 `name` 字段一致
- [ ] 技能 `Kind()` 返回值与配置 `kind` 字段一致
- [ ] `Configure` 正确解析 `Raw` 字段，缺失字段有合理默认值
- [ ] `Run` 通过 `ctx.Reporter.Record` 上报每条结果
- [ ] `Teardown` 释放所有资源（连接、文件、子进程）
- [ ] 结果名加技能名前缀避免冲突
- [ ] 无 `fmt.Println`，日志走 `ctx.Logger`
- [ ] 无 `panic`，错误通过 `error` 返回
- [ ] 并发安全（如使用共享资源）
