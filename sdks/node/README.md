> 🌐 Languages: [English](README.md) | [中文](README.zh.md)

# Taichi Node.js Plugin SDK

Provides Node.js developers with a wrapper for the Taichi plugin protocol, so you only
need to write test logic without worrying about stdin reading, JSON parsing, or stdout
output.

## Protocol Overview

Taichi integrates third-party test plugins via `kind: plugin`. A plugin is any executable
program; Taichi communicates with it via JSON over stdin/stdout:

| Direction | Carrier | Content |
|-----------|---------|---------|
| taichi → plugin | stdin | `PluginInput` JSON |
| plugin → taichi | stdout | `PluginOutput` JSON |
| plugin → taichi | stderr | Free-form logs (Taichi forwards to its own logger) |

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

- Node.js 14+
- **Built-in modules only** (`fs` / `http` / `https`), no `npm install` required

Place `taichi-plugin.js` in a directory your plugin script can require from (same
directory is simplest).

## SDK API

```js
const {
  runPlugin,
  readInput,
  writeOutput,
  endpoints,
  passCase,
  failCase,
  skipCase,
} = require('./taichi-plugin');
```

### Entry Function

```js
runPlugin((input) => {
  // input: { skill_name, project_name, base_url, reports_dir, config }
  return { cases: [passCase('Bootstrap')] };
});
```

`runPlugin(handler)` automatically:

1. Synchronously reads stdin and `JSON.parse` it into a `PluginInput` object
2. Calls `handler(input)` to get the output (supports both synchronous return and `async` returning a Promise)
3. Serializes the output to stdout, flushes, then `exit 0`
4. If the handler throws or the Promise rejects, outputs an output with `error` and `exit 1`

> async handler: just use `runPlugin(async (input) => { ... })`; the SDK awaits the
> Promise. stdout is flushed before exit to avoid pipe buffering losing output.

### Case Construction Helpers

| Function | Description |
|----------|-------------|
| `passCase(name, message='ok')` | Construct a passed case |
| `failCase(name, error, message='failed')` | Construct a failed case (error required) |
| `skipCase(name, message='skipped')` | Construct a skipped case (passed=false, skipped=true) |

### Low-level Functions

| Function | Description |
|----------|-------------|
| `readInput()` | Synchronously reads and parses the `PluginInput` object from stdin |
| `writeOutput(output, done?)` | Serializes output to JSON and writes to stdout; `done` is a completion callback |
| `endpoints(input)` | Conveniently reads the `input.config.endpoints` endpoint list (returns a string array) |

### output Object Shape

```js
{
  cases: [
    { name: 'GET /health', passed: true, message: 'HTTP 200', duration_ms: 12 },
    { name: 'GET /x', passed: false, error: 'unexpected status: 500', duration_ms: 8 }
  ],
  error: '' // optional, non-empty means plugin-level fatal error
}
```

Empty optional fields are omitted, aligning with Go's `omitempty` semantics.

## Example Usage

The example `example.js` reads `config.endpoints`, performs HTTP GET against `base_url` +
endpoint, and treats status code 2xx as pass. For local debugging, you can pipe input
directly:

```bash
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health","/api/v1/users"]}}' | node sdks/node/example.js
```

Expected output looks like:

```json
{"cases":[{"name":"GET /health","passed":true,"message":"HTTP 200","duration_ms":12}]}
```

Debug logs are written to stderr and can be forwarded by Taichi to its own logger.

## Integration with Taichi Config

Declare `kind: plugin` in `taichi.yaml`, point `raw.command` to the script; custom
fields are passed through to `input.config`:

```yaml
skills:
  - name: my-check
    kind: plugin
    enabled: true
    raw:
      command: node sdks/node/example.js
      timeout: 30s
      endpoints: [/api/v1/users]   # passed through to input.config
```

Once configured, run with `taichi run -c <config>`.
