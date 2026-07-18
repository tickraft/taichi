> 🌐 Languages: [English](README.md) | [中文](README.zh.md)

# Taichi Plugin SDK Overview

Taichi's plugin protocol is a language-agnostic JSON-over-stdio protocol: Taichi starts
the plugin process, writes `PluginInput` JSON to the plugin's stdin, the plugin executes
tests and writes `PluginOutput` JSON to stdout, and stderr logs are forwarded by Taichi
to its own logger. `exit 0` means normal execution (pass/fail is expressed by the stdout
JSON); `exit ≠ 0` means a plugin-level fatal error. This directory provides SDKs for
common languages, wrapping stdin reading, JSON parsing, and stdout output to lower the
barrier for writing third-party test plugins in non-Go languages, so plugin developers
can focus on the test logic itself.

The protocol field definitions strictly align with `PluginInput` / `PluginOutput` /
`PluginCase` in `taichi/pkg/skill/plugin/skill.go` on the Go side; all language SDKs
behave consistently.

## SDKs by Language

| Language | Subdirectory | Entry function | Example run command |
|----------|--------------|----------------|---------------------|
| Python | `python/` | `run_plugin(handler)` | `python3 sdks/python/example.py` |
| Node.js | `node/` | `runPlugin(handler)` | `node sdks/node/example.js` |
| Shell (bash) | `shell/` | `source taichi-plugin.sh` then call `taichi_read_input` / `taichi_emit_output` | `bash sdks/shell/example.sh` |

All three example plugins behave consistently: they read `input.config.endpoints`,
perform HTTP GET against `base_url` + endpoint, treat status code 2xx as pass, otherwise
fail, and output `PluginOutput`. Each relies only on the language's standard library /
system tools (Python `urllib`, Node built-in `http`/`https`, Shell `curl`), with no
third-party packages required.

## Local Debugging

You can debug plugins without starting Taichi by piping `PluginInput` JSON directly:

```bash
# Python
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health"]}}' | python3 sdks/python/example.py

# Node.js
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health"]}}' | node sdks/node/example.js

# Shell
echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health"]}}' | bash sdks/shell/example.sh
```

## Integration with Taichi Config

Declare `kind: plugin` in `taichi.yaml`, point `raw.command` to the plugin script; custom
fields in `raw` (after removing `command`/`args`/`env`/`workdir`/`timeout`) are passed
through to `input.config`:

```yaml
skills:
  - name: my-check
    kind: plugin
    enabled: true
    raw:
      command: python3 sdks/python/example.py   # swap to node/bash similarly
      timeout: 30s
      endpoints: [/api/v1/users]                # passed through to input.config
```

Once configured, run with `taichi run -c <config>`.

## References

- [`examples/plugin-example.sh`](../examples/plugin-example.sh) in the repo root is an
  earlier pure bash minimal example that does not use the SDK wrappers in this directory.
  It serves as a protocol-level reference to help understand what the SDK encapsulates
  on top of the protocol.
- Go-side protocol implementation: `taichi/pkg/skill/plugin/skill.go`
