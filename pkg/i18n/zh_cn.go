package i18n

// zhCNMessages is the Simplified Chinese language pack.
//
// It covers user-visible text such as CLI command descriptions, run output,
// orchestration logs, and report summaries. Internal error messages (e.g. errors
// wrapped by fmt.Errorf) are kept in English for log searchability and
// cross-language collaboration.
//
// Key naming convention: `<domain>.<subdomain>.<name>`, e.g. `cli.run.short`,
// `orchestrator.run.env_start`.
func init() {
	Register(ZhCN, map[string]string{
		// ===== root command =====
		"cli.root.short": "taichi 测试编排框架",
		"cli.root.long": `taichi 是一个通用的自动化测试编排框架。

它提供技能（Skill）扩展机制、多环境生命周期管理、自动修复与多格式报告输出。

通过配置文件描述被测项目、环境与技能，即可编排一次完整的测试运行。
内置 API / gRPC / UI / Static / Regression 五类技能，亦可通过实现
pkg/skill.TestSkill 接口注册自定义技能。

taichi 支持与 AI Agent（如 Trae IDE）双向集成：
  - 作为 MCP Server 暴露 taichi 工具给 AI Agent 调用
  - 在 copilot 模式下调用 AI Agent 进行代码修复，实现测试→修复→回归闭环

快速开始：
  taichi run --config configs/taichi.yaml
  taichi list --config configs/taichi.yaml
  taichi mcp --config configs/taichi.yaml
  taichi copilot --config configs/taichi.yaml --agent-cli trae
  taichi version
`,
		"cli.root.flag.config":    "配置文件路径（YAML）",
		"cli.root.flag.log_level": "日志级别：debug / info / warn / error",
		"cli.root.flag.locale":    "界面语言：auto / zh-CN / en-US（auto 根据系统环境检测）",

		// ===== run command =====
		"cli.run.short": "按配置文件执行一次测试编排",
		"cli.run.long": `加载配置文件，启动被测项目对应的环境，按优先级依次执行启用的技能，
收集结果并生成 JSON / JUnit XML / 人类可读摘要报告。

退出码：全部通过返回 0；存在失败或运行错误返回 1。`,
		"cli.run.flag.project":     "指定本次运行的项目名（默认配置中的第一个项目）",
		"cli.run.flag.skill":       "只运行指定技能（可重复，如 -s api -s ui）",
		"cli.run.flag.reports_dir": "覆盖配置中的报告输出目录",
		"cli.run.flag.timeout":     "本次运行的总超时（0 表示不限）",

		// run output
		"cli.run.output.header":          "=== taichi run ===",
		"cli.run.output.project":         "项目",
		"cli.run.output.baseurl":         "基址",
		"cli.run.output.duration":        "耗时",
		"cli.run.output.summary":         "摘要",
		"cli.run.output.summary_format":  "总数=%d 通过=%d 失败=%d 跳过=%d",
		"cli.run.output.envlog":          "环境日志",
		"cli.run.output.skills":          "技能",
		"cli.run.output.failed_count":    "%d 个测试失败",
		"cli.run.error.register_builtin": "注册内置技能失败",

		// ===== list command =====
		"cli.list.short": "列出配置中的项目、环境与已注册技能",
		"cli.list.long": `加载配置文件，展示：
  - 被测项目列表及其启用的环境与技能
  - 已定义的环境清单
  - taichi 内置与已注册的自定义技能

便于在运行前确认配置是否符合预期。`,
		"cli.list.section.projects":   "=== 项目 (%d) ===",
		"cli.list.section.envs":       "=== 环境 (%d) ===",
		"cli.list.section.skill_cfgs": "=== 技能配置 (%d) ===",
		"cli.list.section.registered": "=== 已注册技能 (%d) ===",
		"cli.list.section.report":     "=== 报告配置 ===",
		"cli.list.section.autofix":    "=== 自动修复 ===",
		"cli.list.label.root":         "root",
		"cli.list.label.env":          "env",
		"cli.list.label.skills":       "skills",
		"cli.list.label.skills_all":   "（全部已配置）",
		"cli.list.label.port":         "port",
		"cli.list.label.base_url":     "base_url",
		"cli.list.label.binary":       "binary",
		"cli.list.label.build":        "build",
		"cli.list.label.health":       "health",
		"cli.list.label.command":      "command",
		"cli.list.label.ready":        "ready",
		"cli.list.label.kind":         "kind",
		"cli.list.label.priority":     "priority",
		"cli.list.label.state":        "state",
		"cli.list.state.enabled":      "已启用",
		"cli.list.state.disabled":     "已禁用",
		"cli.list.label.suite_name":   "suite_name",
		"cli.list.label.output_dir":   "output_dir",
		"cli.list.label.formats":      "formats",
		"cli.list.label.enabled":      "enabled",
		"cli.list.label.reports_dir":  "reports_dir",

		// ===== validate command =====
		"cli.validate.short": "校验配置文件",
		"cli.validate.long": `加载并校验 taichi 配置文件，不启动环境、不执行技能。

便于在运行前捕获配置语法与完整性错误。`,
		"cli.validate.ok": "配置文件校验通过",

		// ===== version command =====
		"cli.version.short":  "打印 taichi 版本信息",
		"cli.version.long":   "打印 taichi 二进制的版本号、Go 运行时版本与目标平台。",
		"cli.version.format": "taichi %s (go %s %s/%s)",

		// ===== mcp command =====
		"cli.mcp.short": "作为 MCP Server 运行，向 AI Agent 暴露 taichi 工具",
		"cli.mcp.long": `作为 MCP（Model Context Protocol）Server 运行，通过 stdin/stdout
与 AI Agent 通信。

AI Agent（如 Trae IDE）可通过 MCP 协议调用以下工具：
  - taichi_run        执行一次测试编排
  - taichi_list       列出配置中的项目、环境与技能
  - taichi_failures   获取最近一次运行的失败上下文
  - taichi_regression 执行回归测试

协议基于 JSON-RPC 2.0 over stdio，零第三方依赖。`,
		"cli.mcp.flag.config": "MCP Server 启动时加载的配置文件路径",
		"cli.mcp.error.serve": "MCP server 错误",
		"cli.mcp.log.serving": "taichi MCP server serving on stdio (locale=%s)",

		// MCP tool descriptions
		"mcp.tool.taichi_run.desc":        "执行一次完整的测试编排。返回 JSON 格式的测试摘要与失败用例列表。",
		"mcp.tool.taichi_list.desc":       "列出配置文件中的项目、环境与已注册技能。",
		"mcp.tool.taichi_failures.desc":   "获取最近一次运行的失败上下文（JSON），用于 AI Agent 分析。",
		"mcp.tool.taichi_regression.desc": "对已修复代码执行回归测试。",
		"mcp.tool.param.config_path":      "配置文件路径",
		"mcp.tool.param.project":          "项目名（空表示配置中的第一个项目）",
		"mcp.tool.param.skills":           "技能过滤列表",
		"mcp.tool.param.reports_dir":      "报告输出目录",
		"mcp.tool.param.timeout_seconds":  "总超时（秒）",

		// ===== copilot command =====
		"cli.copilot.short": "AI Agent 协作的测试→修复→回归全自动闭环",
		"cli.copilot.long": `AI Agent 协作的测试→修复→回归全自动闭环：

1. 运行测试（orchestrator.Run）
2. 若全部通过，直接返回
3. 若存在失败：
   a. 构建失败上下文（JSON）
   b. 调用 AI Agent 分析与修复
   c. 应用修复（patch 或 direct 模式）
   d. 重新构建并运行回归测试
   e. 回归通过则返回成功，否则递增轮次回到步骤 3
4. 超过最大轮次仍有失败，返回最后的失败结果

Agent 调用方式（二选一）：
  --agent-cli trae --agent-args "agent fix"
  --agent-endpoint http://localhost:8080/fix

Agent 脚本协议：
  stdin:  FailureContext JSON
  stdout: FixResult JSON {"fixed":true,"mode":"patch","patch":"...","message":"..."}`,
		"cli.copilot.flag.project":           "指定本次运行的项目名",
		"cli.copilot.flag.skill":             "只运行指定技能（可重复）",
		"cli.copilot.flag.reports_dir":       "覆盖配置中的报告输出目录",
		"cli.copilot.flag.timeout":           "本次运行的总超时（0 表示不限）",
		"cli.copilot.flag.max_rounds":        "最大修复轮次（默认 3）",
		"cli.copilot.flag.agent_cli":         "AI Agent 命令行（如 trae），通过 stdin/stdout 交换 JSON",
		"cli.copilot.flag.agent_args":        "AI Agent 命令参数（可重复）",
		"cli.copilot.flag.agent_endpoint":    "AI Agent HTTP 端点（与 --agent-cli 互斥）",
		"cli.copilot.flag.agent_token":       "HTTP 模式下的 Bearer 认证令牌",
		"cli.copilot.flag.agent_timeout":     "单次 Agent 调用超时",
		"cli.copilot.error.no_invoker":       "未配置 Agent 调用器：请使用 --agent-cli 或 --agent-endpoint",
		"cli.copilot.error.register":         "注册内置技能失败",
		"cli.copilot.error.failed_after":     "%d 个测试在 %d 轮修复后仍失败",
		"cli.copilot.error.mutual_exclusive": "--agent-cli 与 --agent-endpoint 互斥",

		// copilot output
		"cli.copilot.output.header":         "=== taichi copilot ===",
		"cli.copilot.output.total_duration": "总耗时",
		"cli.copilot.output.rounds":         "修复轮次",
		"cli.copilot.output.fixed":          "已修复",
		"cli.copilot.output.final_result":   "最终结果",
		"cli.copilot.output.project":        "项目",
		"cli.copilot.output.baseurl":        "基址",
		"cli.copilot.output.duration":       "耗时",
		"cli.copilot.output.summary":        "摘要",
		"cli.copilot.output.summary_format": "总数=%d 通过=%d 失败=%d 跳过=%d",
		"cli.copilot.output.fix_rounds":     "修复轮次详情",
		"cli.copilot.output.round":          "轮次",
		"cli.copilot.output.failures":       "失败数",
		"cli.copilot.output.agent_error":    "Agent 错误",
		"cli.copilot.output.fixed_label":    "已修复",
		"cli.copilot.output.mode":           "模式",
		"cli.copilot.output.message":        "消息",
		"cli.copilot.output.analysis":       "分析",
		"cli.copilot.output.apply_error":    "应用错误",

		// ===== orchestrator logs =====
		"orchestrator.run.env_start":              "正在为项目 %q 启动环境 %q",
		"orchestrator.run.env_stop_error":         "停止环境失败: %v",
		"orchestrator.run.skills_missing":         "以下技能未注册: %s",
		"orchestrator.run.skill_running":          "正在运行技能 %s",
		"orchestrator.run.skill_configure_failed": "技能 %s 配置失败: %v",
		"orchestrator.run.skill_setup_failed":     "技能 %s 启动失败: %v",
		"orchestrator.run.skill_run_error":        "技能 %s 运行错误: %v",
		"orchestrator.run.skill_teardown_failed":  "技能 %s 清理失败: %v",
		"orchestrator.run.reports_error":          "生成报告失败: %v",
		"orchestrator.run.plugin_registered":      "已自动注册插件技能 %q",

		// copilot orchestration logs
		"copilot.round.initial":              "copilot: 第 1 轮 - 运行初始测试",
		"copilot.round.all_passed":           "copilot: 第 1 轮全部通过，无需修复",
		"copilot.round.no_invoker":           "copilot: 检测到测试失败，但未配置 Agent 调用器",
		"copilot.round.write_fc_error":       "copilot: 写入失败上下文失败: %v",
		"copilot.round.fc_written":           "copilot: 失败上下文已写入 %s",
		"copilot.round.invoking_agent":       "copilot: 第 %d 轮 - 调用 Agent %s 分析 %d 个失败",
		"copilot.round.agent_failed":         "copilot: 第 %d 轮 - Agent 调用失败: %v",
		"copilot.round.agent_cannot_fix":     "copilot: 第 %d 轮 - Agent 无法修复: %s",
		"copilot.round.agent_fixed":          "copilot: 第 %d 轮 - Agent 报告已修复（mode=%s）: %s",
		"copilot.round.apply_failed":         "copilot: 第 %d 轮 - 应用修复失败: %v",
		"copilot.round.applied":              "copilot: 第 %d 轮 - 修复已成功应用",
		"copilot.round.regression":           "copilot: 第 %d 轮 - 运行回归测试",
		"copilot.round.regression_passed":    "copilot: 经 %d 轮修复后全部测试通过",
		"copilot.round.regression_remaining": "copilot: 第 %d 轮 - 回归仍有 %d 个失败",
		"copilot.round.exhausted":            "copilot: 已用尽 %d 轮，仍有 %d 个失败",

		// ===== reporter output =====
		"reporter.summary.title":    "=== 测试摘要 ===",
		"reporter.summary.total":    "总数",
		"reporter.summary.passed":   "通过",
		"reporter.summary.failed":   "失败",
		"reporter.summary.skipped":  "跳过",
		"reporter.summary.duration": "耗时",
		"reporter.table.test":       "测试",
		"reporter.table.status":     "状态",
		"reporter.table.duration":   "耗时",
		"reporter.table.message":    "消息",
		"reporter.status.pass":      "通过",
		"reporter.status.fail":      "失败",
		"reporter.status.skip":      "跳过",
		"reporter.test_failed":      "测试失败",

		// ===== env logs =====
		"env.frontend.command_empty":   "前端环境 %q: command 为空",
		"env.frontend.ready_url_empty": "前端环境 %q: ready_url 为空",
		"env.frontend.start_failed":    "启动前端 %q 失败",
		"env.frontend.not_ready":       "前端 %q 在 60s 内未就绪",
		"env.frontend.cmd_empty":       "前端环境 %q: command 为空",
		"env.frontend.build_empty":     "环境 %q: build 为空或无法解析",
		"env.frontend.build_failed":    "环境 %q: build 执行失败",
		"env.unknown_kind":             "未知的环境类型 %q",

		// ===== agent errors =====
		"agent.cli.invoke_failed":  "调用 Agent CLI 失败: %w",
		"agent.cli.timeout":        "Agent CLI 调用超时（%s）",
		"agent.cli.bad_exit":       "Agent CLI 退出码 %d",
		"agent.cli.parse_failed":   "解析 Agent 响应失败: %w",
		"agent.cli.empty_stdout":   "Agent CLI 未输出任何内容",
		"agent.http.invoke_failed": "调用 Agent HTTP 端点失败: %w",
		"agent.http.bad_status":    "Agent HTTP 返回状态码 %d",
		"agent.http.parse_failed":  "解析 Agent HTTP 响应失败: %w",
		"agent.http.empty_body":    "Agent HTTP 响应体为空",

		// patch
		"agent.patch.git_apply_failed":   "git apply 失败: %s",
		"agent.patch.patch_apply_failed": "patch -p1 失败: %s",
		"agent.patch.empty":              "修复 patch 为空",
		"agent.patch.verify_failed":      "验证 direct 修复失败: %v",
		"agent.patch.unknown_mode":       "未知的修复模式: %s",
		"agent.patch.no_modified_files":  "direct 模式未提供 modified_files",
		"agent.patch.file_missing":       "修改的文件不存在: %s",
	})
}
