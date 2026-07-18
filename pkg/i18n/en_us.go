package i18n

// enUSMessages is the American English language pack (also the default fallback language).
//
// Keys are fully aligned with zhCNMessages; values are the English translations.
func init() {
	Register(EnUS, map[string]string{
		// ===== root command =====
		"cli.root.short": "taichi test orchestration framework",
		"cli.root.long": `taichi is a general-purpose automated test orchestration framework.

It provides a Skill extension mechanism, multi-environment lifecycle
management, auto-fix, and multi-format report output.

Describe the project under test, environments, and skills via a config
file to orchestrate a complete test run. Built-in skills: API / gRPC /
UI / Static / Regression. Register custom skills by implementing the
pkg/skill.TestSkill interface.

taichi supports bidirectional integration with AI Agents (e.g. Trae IDE):
  - Acts as an MCP Server exposing taichi tools to AI Agents
  - In copilot mode, invokes an AI Agent for code fixes, completing a
    test → fix → regression loop

Quick start:
  taichi run --config configs/taichi.yaml
  taichi list --config configs/taichi.yaml
  taichi mcp --config configs/taichi.yaml
  taichi copilot --config configs/taichi.yaml --agent-cli trae
  taichi version
`,
		"cli.root.flag.config":    "Config file path (YAML)",
		"cli.root.flag.log_level": "Log level: debug / info / warn / error",
		"cli.root.flag.locale":    "UI language: auto / zh-CN / en-US (auto detects from system environment)",

		// ===== run command =====
		"cli.run.short": "Run a test orchestration according to the config file",
		"cli.run.long": `Loads the config file, starts the environment for the project under test,
runs enabled skills in priority order, collects results, and generates
JSON / JUnit XML / human-readable summary reports.

Exit code: 0 if all pass; 1 if any failure or runtime error occurs.`,
		"cli.run.flag.project":     "Project name for this run (defaults to the first project in config)",
		"cli.run.flag.skill":       "Run only specified skills (repeatable, e.g. -s api -s ui)",
		"cli.run.flag.reports_dir": "Override the report output directory in config",
		"cli.run.flag.timeout":     "Total timeout for this run (0 means no limit)",

		// run output
		"cli.run.output.header":          "=== taichi run ===",
		"cli.run.output.project":         "Project",
		"cli.run.output.baseurl":         "BaseURL",
		"cli.run.output.duration":        "Duration",
		"cli.run.output.summary":         "Summary",
		"cli.run.output.summary_format":  "total=%d passed=%d failed=%d skipped=%d",
		"cli.run.output.envlog":          "EnvLog",
		"cli.run.output.skills":          "Skills",
		"cli.run.output.failed_count":    "%d test(s) failed",
		"cli.run.error.register_builtin": "register builtin skills",

		// ===== list command =====
		"cli.list.short": "List projects, environments, and registered skills in the config",
		"cli.list.long": `Loads the config file and shows:
  - Projects under test with their environment and skills
  - Defined environments
  - taichi built-in and registered custom skills

Useful for verifying the config before a run.`,
		"cli.list.section.projects":   "=== Projects (%d) ===",
		"cli.list.section.envs":       "=== Environments (%d) ===",
		"cli.list.section.skill_cfgs": "=== Skill Configs (%d) ===",
		"cli.list.section.registered": "=== Registered Skills (%d) ===",
		"cli.list.section.report":     "=== Report ===",
		"cli.list.section.autofix":    "=== Autofix ===",
		"cli.list.label.root":         "root",
		"cli.list.label.env":          "env",
		"cli.list.label.skills":       "skills",
		"cli.list.label.skills_all":   "(all configured)",
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
		"cli.list.state.enabled":      "enabled",
		"cli.list.state.disabled":     "disabled",
		"cli.list.label.suite_name":   "suite_name",
		"cli.list.label.output_dir":   "output_dir",
		"cli.list.label.formats":      "formats",
		"cli.list.label.enabled":      "enabled",
		"cli.list.label.reports_dir":  "reports_dir",

		// ===== validate command =====
		"cli.validate.short": "Validate a configuration file",
		"cli.validate.long": `Load and validate a taichi configuration file without starting environments or executing skills.

Useful for catching config syntax and integrity errors before a run.`,
		"cli.validate.ok": "configuration is valid",

		// ===== version command =====
		"cli.version.short":  "Print taichi version information",
		"cli.version.long":   "Print the taichi binary version, Go runtime version, and target platform.",
		"cli.version.format": "taichi %s (go %s %s/%s)",

		// ===== mcp command =====
		"cli.mcp.short": "Run as an MCP Server exposing taichi tools to AI Agents",
		"cli.mcp.long": `Runs as an MCP (Model Context Protocol) Server communicating with
AI Agents via stdin/stdout.

AI Agents (e.g. Trae IDE) can invoke the following tools via MCP:
  - taichi_run        Run a test orchestration
  - taichi_list       List projects, environments, and skills in the config
  - taichi_failures   Get the failure context of the most recent run
  - taichi_regression Run regression tests

The protocol is JSON-RPC 2.0 over stdio with zero third-party deps.`,
		"cli.mcp.flag.config": "Config file path loaded when the MCP server starts",
		"cli.mcp.error.serve": "MCP server error",
		"cli.mcp.log.serving": "taichi MCP server serving on stdio (locale=%s)",

		// MCP tool descriptions
		"mcp.tool.taichi_run.desc":        "Run a complete test orchestration. Returns a JSON test summary and failed case list.",
		"mcp.tool.taichi_list.desc":       "List projects, environments, and registered skills in the config file.",
		"mcp.tool.taichi_failures.desc":   "Get the failure context (JSON) of the most recent run for AI Agent analysis.",
		"mcp.tool.taichi_regression.desc": "Run regression tests against fixed code.",
		"mcp.tool.param.config_path":      "Config file path",
		"mcp.tool.param.project":          "Project name (empty means the first project in config)",
		"mcp.tool.param.skills":           "Skill filter list",
		"mcp.tool.param.reports_dir":      "Report output directory",
		"mcp.tool.param.timeout_seconds":  "Total timeout in seconds",

		// ===== copilot command =====
		"cli.copilot.short": "AI Agent collaborative test → fix → regression fully automated loop",
		"cli.copilot.long": `AI Agent collaborative test → fix → regression fully automated loop:

1. Run tests (orchestrator.Run)
2. If all pass, return immediately
3. If there are failures:
   a. Build a failure context (JSON)
   b. Invoke the AI Agent to analyze and fix
   c. Apply the fix (patch or direct mode)
   d. Re-run tests (regression)
   e. If regression passes, return success; otherwise increment the
      round and go back to step 3
4. If failures remain after MaxRounds, return the final failure result

Agent invocation (one of):
  --agent-cli trae --agent-args "agent fix"
  --agent-endpoint http://localhost:8080/fix

Agent script protocol:
  stdin:  FailureContext JSON
  stdout: FixResult JSON {"fixed":true,"mode":"patch","patch":"...","message":"..."}`,
		"cli.copilot.flag.project":           "Project name for this run",
		"cli.copilot.flag.skill":             "Run only specified skills (repeatable)",
		"cli.copilot.flag.reports_dir":       "Override the report output directory in config",
		"cli.copilot.flag.timeout":           "Total timeout for this run (0 means no limit)",
		"cli.copilot.flag.max_rounds":        "Maximum fix rounds (default 3)",
		"cli.copilot.flag.agent_cli":         "AI Agent command line (e.g. trae), exchanges JSON via stdin/stdout",
		"cli.copilot.flag.agent_args":        "AI Agent command args (repeatable)",
		"cli.copilot.flag.agent_endpoint":    "AI Agent HTTP endpoint (mutually exclusive with --agent-cli)",
		"cli.copilot.flag.agent_token":       "Bearer token for HTTP mode",
		"cli.copilot.flag.agent_timeout":     "Single Agent invocation timeout",
		"cli.copilot.error.no_invoker":       "no agent invoker configured: use --agent-cli or --agent-endpoint",
		"cli.copilot.error.register":         "register builtin skills",
		"cli.copilot.error.failed_after":     "%d test(s) still failing after %d round(s)",
		"cli.copilot.error.mutual_exclusive": "--agent-cli and --agent-endpoint are mutually exclusive",

		// copilot output
		"cli.copilot.output.header":         "=== taichi copilot ===",
		"cli.copilot.output.total_duration": "Total Duration",
		"cli.copilot.output.rounds":         "Rounds",
		"cli.copilot.output.fixed":          "Fixed",
		"cli.copilot.output.final_result":   "Final Result",
		"cli.copilot.output.project":        "Project",
		"cli.copilot.output.baseurl":        "BaseURL",
		"cli.copilot.output.duration":       "Duration",
		"cli.copilot.output.summary":        "Summary",
		"cli.copilot.output.summary_format": "total=%d passed=%d failed=%d skipped=%d",
		"cli.copilot.output.fix_rounds":     "Fix Rounds",
		"cli.copilot.output.round":          "Round",
		"cli.copilot.output.failures":       "Failures",
		"cli.copilot.output.agent_error":    "Agent Error",
		"cli.copilot.output.fixed_label":    "Fixed",
		"cli.copilot.output.mode":           "Mode",
		"cli.copilot.output.message":        "Message",
		"cli.copilot.output.analysis":       "Analysis",
		"cli.copilot.output.apply_error":    "Apply Error",

		// ===== orchestrator logs =====
		"orchestrator.run.env_start":              "starting env %q for project %q",
		"orchestrator.run.env_stop_error":         "stop env: %v",
		"orchestrator.run.skills_missing":         "skills not registered: %s",
		"orchestrator.run.skill_running":          "running skill %s",
		"orchestrator.run.skill_configure_failed": "skill %s configure failed: %v",
		"orchestrator.run.skill_setup_failed":     "skill %s setup failed: %v",
		"orchestrator.run.skill_run_error":        "skill %s run error: %v",
		"orchestrator.run.skill_teardown_failed":  "skill %s teardown failed: %v",
		"orchestrator.run.reports_error":          "generate reports: %v",
		"orchestrator.run.plugin_registered":      "auto-registered plugin skill %q",

		// copilot orchestration logs
		"copilot.round.initial":              "copilot: round 1 - running initial tests",
		"copilot.round.all_passed":           "copilot: all tests passed in round 1, no fix needed",
		"copilot.round.no_invoker":           "copilot: test failures detected but no agent invoker configured",
		"copilot.round.write_fc_error":       "copilot: write failure context: %v",
		"copilot.round.fc_written":           "copilot: failure context written to %s",
		"copilot.round.invoking_agent":       "copilot: round %d - invoking agent %s to analyze %d failure(s)",
		"copilot.round.agent_failed":         "copilot: round %d - agent invocation failed: %v",
		"copilot.round.agent_cannot_fix":     "copilot: round %d - agent could not fix: %s",
		"copilot.round.agent_fixed":          "copilot: round %d - agent reports fixed (mode=%s): %s",
		"copilot.round.apply_failed":         "copilot: round %d - apply fix failed: %v",
		"copilot.round.applied":              "copilot: round %d - fix applied successfully",
		"copilot.round.regression":           "copilot: round %d - running regression tests",
		"copilot.round.regression_passed":    "copilot: all tests passed after %d round(s) of fix",
		"copilot.round.regression_remaining": "copilot: round %d - regression still has %d failure(s)",
		"copilot.round.exhausted":            "copilot: exhausted %d round(s), %d failure(s) remain",

		// ===== reporter output =====
		"reporter.summary.title":    "=== Test Summary ===",
		"reporter.summary.total":    "Total",
		"reporter.summary.passed":   "Passed",
		"reporter.summary.failed":   "Failed",
		"reporter.summary.skipped":  "Skipped",
		"reporter.summary.duration": "Duration",
		"reporter.table.test":       "TEST",
		"reporter.table.status":     "STATUS",
		"reporter.table.duration":   "DURATION",
		"reporter.table.message":    "MESSAGE",
		"reporter.status.pass":      "PASS",
		"reporter.status.fail":      "FAIL",
		"reporter.status.skip":      "SKIP",
		"reporter.test_failed":      "test failed",

		// ===== env logs =====
		"env.frontend.command_empty":   "frontend env %q: command is empty",
		"env.frontend.ready_url_empty": "frontend env %q: ready_url is empty",
		"env.frontend.start_failed":    "start frontend %q",
		"env.frontend.not_ready":       "frontend %q did not become ready within 60s",
		"env.frontend.cmd_empty":       "frontend env %q: command is empty",
		"env.frontend.build_empty":     "env %q: build is empty or unparseable",
		"env.frontend.build_failed":    "env %q: build failed",
		"env.unknown_kind":             "unknown env kind %q",

		// ===== agent errors =====
		"agent.cli.invoke_failed":  "invoke agent CLI: %w",
		"agent.cli.timeout":        "agent CLI invocation timeout (%s)",
		"agent.cli.bad_exit":       "agent CLI exit code %d",
		"agent.cli.parse_failed":   "parse agent response: %w",
		"agent.cli.empty_stdout":   "agent CLI produced no output",
		"agent.http.invoke_failed": "invoke agent HTTP endpoint: %w",
		"agent.http.bad_status":    "agent HTTP returned status %d",
		"agent.http.parse_failed":  "parse agent HTTP response: %w",
		"agent.http.empty_body":    "agent HTTP response body is empty",

		// patch
		"agent.patch.git_apply_failed":   "git apply failed: %s",
		"agent.patch.patch_apply_failed": "patch -p1 failed: %s",
		"agent.patch.empty":              "fix patch is empty",
		"agent.patch.verify_failed":      "verify direct fix failed: %v",
		"agent.patch.unknown_mode":       "unknown fix mode: %s",
		"agent.patch.no_modified_files":  "direct mode did not provide modified_files",
		"agent.patch.file_missing":       "modified file does not exist: %s",
	})
}
