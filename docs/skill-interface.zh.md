# 技能对接接口规范

> 🌐 语言: [English](skill-interface.md) | [中文](skill-interface.zh.md)

## 一、TestSkill 接口

所有测试技能必须实现 `pkg/skill.TestSkill` 接口：

```go
type TestSkill interface {
    // Name 返回技能唯一标识符，应与 Config.Name 一致。
    Name() string
    // Kind 返回技能大类。
    Kind() Kind
    // Configure 接收技能配置，技能应在此时解析 Raw 字段并初始化内部状态。
    // 返回 error 表示配置无效，技能将被禁用。
    Configure(cfg Config) error
    // Priority 返回执行优先级。配置中显式设置时优先使用配置值。
    Priority() Priority
    // Setup 在 Run 之前执行，用于准备资源（如启动浏览器、加载用例集）。
    Setup(ctx *Context) error
    // Run 是技能执行核心，将测试结果通过 ctx.Reporter.Record 上报。
    // 返回 error 表示技能级致命错误；用例级失败应通过 TestResult.Passed=false 表达。
    Run(ctx *Context) Result
    // Teardown 在 Run 之后（无论成功或失败）执行，用于释放资源。
    // 返回的 error 仅记录日志，不影响最终结果。
    Teardown(ctx *Context) error
}
```

源码：[pkg/skill/skill.go](../pkg/skill/skill.go)

## 二、生命周期

```
Configure(cfg)     ← 配置注入（仅一次）
    │
    ▼
Setup(ctx)         ← 资源准备（每次运行）
    │
    ▼
Run(ctx) → result  ← 执行核心（每次运行）
    │
    ▼
Teardown(ctx)      ← 资源释放（每次运行，即使 Run 失败）
```

- `Configure` 在技能注册后、执行前调用一次，用于解析配置
- `Setup` / `Run` / `Teardown` 在每次编排运行时调用
- `Teardown` 即使 `Run` 返回 error 也会被调用

## 三、技能大类（Kind）

| 常量 | 值 | 用途 |
|------|-----|------|
| `KindAPI` | `api` | RESTful API 测试 |
| `KindUI` | `ui` | UI / 页面测试 |
| `KindStatic` | `static` | 静态资源测试 |
| `KindRegression` | `regression` | 回归测试 |
| `KindCustom` | `custom` | 用户自定义 |

## 四、优先级（Priority）

数值小先执行。同一项目内技能按优先级升序依次执行。

| 常量 | 值 | 建议场景 |
|------|-----|---------|
| `PriorityCritical` | 0 | 关键路径（API 冒烟） |
| `PriorityHigh` | 10 | 高优先级（UI 页面） |
| `PriorityNormal` | 20 | 正常（静态资源） |
| `PriorityLow` | 30 | 可延后（回归、长耗时） |

配置中 `priority` 字段非零时覆盖技能默认优先级。

## 五、Config 配置视图

由 Taichi 配置文件加载后注入技能：

```go
type Config struct {
    Name     string         // 唯一标识符
    Kind     Kind           // 大类
    Enabled  bool           // 是否参与执行
    Priority Priority       // 执行顺序
    Raw      map[string]any // 技能专属配置（技能自行解析）
}
```

`Raw` 字段是技能自由定义的配置 map，技能在 `Configure` 中用 `skill.GetString` / `GetInt` / `GetDuration` / `GetBool` 等辅助函数读取。

## 六、Context 运行态上下文

在 `Setup` / `Run` / `Teardown` 期间传递：

```go
type Context struct {
    Ctx         context.Context      // 生命周期控制，支持优雅取消
    ProjectName string               // 被测项目名
    BaseURL     string               // 被测服务基址
    Asserts     *framework.AssertionEngine  // 断言引擎
    Reporter    *framework.TestReporter     // 结果收集
    ReportsDir  string               // 输出目录（截图、har 等）
    Logger      Logger               // 结构化日志
    FixEngine   FixEngineAccessor    // 自动修复（可选）
    Extra       map[string]any       // 技能间弱类型数据传递
}
```

**Extra 使用约定**：键名应加技能名前缀避免冲突，如 `"ui.screenshot_dir"`。

## 七、Result 产出

```go
type Result struct {
    SkillName string
    Duration  time.Duration
    Summary   framework.TestSummary  // 该技能产出结果的聚合统计
    Error     error                  // 非 nil 表示技能未完整执行
}
```

- 用例级失败：通过 `ctx.Reporter.Record(TestResult{Passed: false, ...})` 上报
- 技能级致命错误：通过 `Result.Error` 表达

## 八、注册机制

### 8.1 通过编排器注册

```go
o := orchestrator.New()
skills := []skill.TestSkill{
    &mySkill.Skill{},
}
o.RegisterBuiltinSkills(skills)
```

### 8.2 通过注册中心直接操作

```go
reg := registry.NewRegistry()
reg.Register(&mySkill.Skill{}, true)   // overwrite=true
reg.Unregister("my-skill")
s, err := reg.Get("my-skill")
list := reg.List()                       // 按名称排序
selected, missing := reg.Select(configs) // 按配置筛选+优先级排序
```

注册中心并发安全（`sync.RWMutex`）。

## 九、辅助函数

`pkg/skill` 提供公共辅助函数，技能可直接调用：

| 函数 | 用途 |
|------|------|
| `HTTPRequest(client, method, url, headers)` | 执行 HTTP 请求，返回响应、正文、错误 |
| `RecordResult(reporter, name, start, passed, msg, err)` | 记录结果并自动计算耗时 |
| `AssertCommonEnvelope(asserts, body, expectedCode)` | 验证统一响应契约（code/msg/request_id） |
| `GetString(raw, key, fallback)` | 从配置 map 读字符串 |
| `GetInt(raw, key, fallback)` | 从配置 map 读整数 |
| `GetDuration(raw, key, fallback)` | 从配置 map 读时长 |
| `GetBool(raw, key, fallback)` | 从配置 map 读布尔 |

## 十、可选钩子接口

实现 `Hook` 接口可获得更细粒度的生命周期通知：

```go
type Hook interface {
    BeforeAll(ctx *Context) error  // 所有技能执行前
    AfterAll(ctx *Context) error   // 所有技能执行后
}
```

## 十一、Logger 接口

技能应通过 `ctx.Logger` 记录日志，禁止 `fmt.Println`：

```go
type Logger interface {
    Infof(format string, args ...any)
    Warnf(format string, args ...any)
    Errorf(format string, args ...any)
}
```

与 `zap.SugaredLogger` 子集兼容。测试场景可用 `skill.NoOpLogger{}`。
