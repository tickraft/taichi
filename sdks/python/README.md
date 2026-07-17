> 🌐 Languages: [English](README.md) | [中文](README.zh.md)

# taichi Python Plugin SDK

Provides Python developers with a wrapper for the taichi plugin protocol, so you only
need to write test logic without worrying about stdin reading, JSON parsing, or stdout
output.

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

## Installation and Dependencies

- Python 3.8+
- **Standard library only**, no `pip install` of any third-party packages required

Place `taichi_plugin.py` in a directory your plugin script can import from (same
directory is simplest), or add it to `PYTHONPATH`.

## SDK API

### Data Classes

```python
from taichi_plugin import PluginInput, PluginOutput, PluginCase
```

- `PluginInput`: fields `skill_name` / `project_name` / `base_url` / `reports_dir` / `config` (dict).
  Provides `PluginInput.from_dict(d)` to construct from a dict, and `input.endpoints()` to
  conveniently read the `config["endpoints"]` endpoint list.
- `PluginCase`: fields `name` / `passed` / `skipped` / `message` / `duration_ms` / `error`;
  `to_dict()` outputs the protocol JSON (omits empty optional fields, aligning with Go omitempty).
- `PluginOutput`: fields `cases` (list) / `error`; `to_dict()` outputs the protocol JSON.

### Entry Function

```python
import taichi_plugin

def handler(input: taichi_plugin.PluginInput) -> taichi_plugin.PluginOutput:
    ...

if __name__ == "__main__":
    taichi_plugin.run_plugin(handler)
```

`run_plugin(handler)` automatically:

1. Reads and parses `PluginInput` via `json.load(sys.stdin)`
2. Calls `handler(input)` to get `PluginOutput`
3. Writes the result via `json.dump(sys.stdout)`, then `exit 0`
4. If the handler raises an exception, outputs a `PluginOutput` with `error` and `exit 1`

### Case Construction Helpers

| Function | Description |
|----------|-------------|
| `pass_case(name, message="ok")` | Construct a passed case |
| `fail_case(name, error, message="failed")` | Construct a failed case (error required) |
| `skip_case(name, message="skipped")` | Construct a skipped case (passed=False, skipped=True) |

### Low-level Functions

| Function | Description |
|----------|-------------|
| `read_input(stream=sys.stdin)` | Read and parse `PluginInput` |
| `write_output(output, stream=sys.stdout)` | Write out `PluginOutput` JSON |

## Example Usage

The example `example.py` reads `config.endpoints`, performs HTTP GET against `base_url` +
endpoint, and treats status code 2xx as pass. For local debugging, you can pipe input
directly:

```bash
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health","/api/v1/users"]}}' | python3 sdks/python/example.py
```

Expected output looks like:

```json
{"cases":[{"name":"GET /health","passed":true,"message":"HTTP 200","duration_ms":12},{"name":"GET /api/v1/users","passed":true,"message":"HTTP 200","duration_ms":8}]}
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
      command: python3 sdks/python/example.py
      timeout: 30s
      endpoints: [/api/v1/users]   # passed through to input.config
```

Once configured, run with `taichi run -c <config>`.
