> 🌐 Languages: [English](README.md) | [中文](README.zh.md)

# taichi Shell Plugin SDK

Provides bash developers with a wrapper for the taichi plugin protocol, so you only
need to write test logic without worrying about stdin reading, JSON parsing, or stdout
output. This library is designed to be used via `source`.

## Protocol Overview

taichi integrates third-party test plugins via `kind: plugin`. A plugin is any executable
program; taichi communicates with it via JSON over stdin/stdout:

| Direction | Carrier | Content |
|-----------|---------|---------|
| taichi → plugin | stdin | `PluginInput` JSON |
| plugin → taichi | stdout | `PluginOutput` JSON |
| plugin → taichi | stderr | Free-form logs (taichi forwards to its own logger) |

Exit code semantics: `exit 0` = normal execution (pass/fail is expressed by stdout JSON);
`exit ≠ 0` = plugin-level fatal error.

### PluginInput Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `skill_name` | string | Yes | Skill name |
| `project_name` | string | Yes | Project under test name |
| `base_url` | string | No | Base URL of the service under test |
| `reports_dir` | string | No | Report output directory |
| `config` | object | No | Plugin business config (remaining fields of the taichi.yaml raw section after removing command/args/env/workdir/timeout) |

### PluginOutput Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cases` | array | Yes | Test case results |
| `error` | string | No | Plugin-level fatal error message (non-empty means incomplete execution) |

### PluginCase Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Case name |
| `passed` | bool | Yes | Whether it passed |
| `skipped` | bool | No | Whether it was skipped (takes precedence over passed in skip statistics) |
| `message` | string | No | Human-readable description |
| `duration_ms` | int64 | No | Duration in milliseconds |
| `error` | string | No | Failure details |

## Dependencies

- **bash 4+** (uses associative arrays and features like `${arr[@]:-}`)
- **JSON parsing**: prefers `jq`; if `jq` is not available, automatically falls back to
  `python3`. If neither is present, the SDK cannot work.
- **HTTP probing** (only needed by the example): `curl`

Detection order: `jq` → `python3`. Installing `jq` is recommended for best performance
and reliability.

## SDK API

```bash
source ./taichi-plugin.sh
```

### Input Reading

| Function | Description |
|----------|-------------|
| `taichi_read_input` | Reads the `PluginInput` JSON from stdin into internal variables; also detects the JSON tool |
| `taichi_input_field <key>` | Extracts a top-level string field from input (e.g. `base_url`) |
| `taichi_input_endpoints` | Extracts `config.endpoints`, printing one endpoint per line |

### Case Construction

| Function | Description |
|----------|-------------|
| `taichi_emit_case <name> <passed:true\|false> [message] [error] [skipped:true\|false] [duration_ms]` | Appends a full case (most general form) |
| `taichi_pass <name> [message]` | Passed case |
| `taichi_fail <name> <error> [message]` | Failed case (error required) |
| `taichi_skip <name> [message]` | Skipped case (passed=false, skipped=true) |
| `taichi_set_error <msg>` | Sets a plugin-level fatal error message |

### Output

| Function | Description |
|----------|-------------|
| `taichi_emit_output` | Assembles the collected cases and optional error into `PluginOutput` JSON and writes to stdout |

> `taichi_emit_case` is the low-level general constructor; `taichi_pass` / `taichi_fail`
> / `taichi_skip` are convenience wrappers (without `duration_ms`). When you need to
> record duration, call `taichi_emit_case` directly and pass the 6th argument.

## Example Usage

The example `example.sh` reads `config.endpoints`, performs a curl GET against `base_url`
+ endpoint, and treats status code 2xx as pass. For local debugging, you can pipe input
directly:

```bash
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health","/api/v1/users"]}}' | bash sdks/shell/example.sh
```

Expected output looks like:

```json
{"cases":[{"name":"PluginBootstrap","passed":true,"message":"plugin started successfully"},{"name":"GET /health","passed":true,"message":"HTTP 200","duration_ms":45}]}
```

Debug logs are written to stderr and can be forwarded by taichi to its own logger.

## Integration with taichi Config

Declare `kind: plugin` in `taichi.yaml`, point `raw.command` to the script; custom
fields are passed through to `input.config`:

```yaml
skills:
  - name: my-check
    kind: plugin
    enabled: true
    raw:
      command: bash sdks/shell/example.sh
      timeout: 30s
      endpoints: [/api/v1/users]   # passed through to input.config
```

Once configured, run with `taichi run -c <config>`.

> `examples/plugin-example.sh` in the repo root is an earlier pure bash minimal example
> that does not use this SDK wrapper; it can serve as a protocol-level reference.
