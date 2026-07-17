"""taichi plugin SDK (Python implementation).

Wraps the stdin/stdout JSON exchange of the taichi plugin protocol so that
plugin authors can focus on the test logic itself. Protocol fields are kept
strictly aligned with PluginInput / PluginOutput / PluginCase in
taichi/pkg/skill/plugin/skill.go.

Protocol overview:
  stdin  → PluginInput  JSON (skill_name, project_name, base_url, reports_dir, config)
  stdout ← PluginOutput JSON (cases[], error)
  stderr ← free-form logs (taichi forwards them to its own logger)
  exit 0 = plugin executed normally (pass/fail is expressed via stdout JSON);
          exit ≠ 0 = plugin-level fatal error

Typical usage:

    import taichi_plugin

    def handler(input: taichi_plugin.PluginInput) -> taichi_plugin.PluginOutput:
        cases = [taichi_plugin.pass_case("Bootstrap")]
        return taichi_plugin.PluginOutput(cases=cases)

    if __name__ == "__main__":
        taichi_plugin.run_plugin(handler)

Depends only on the Python standard library; no third-party packages required.
"""

from __future__ import annotations

import json
import sys
from dataclasses import dataclass, field
from typing import Any, Callable, Dict, List

# Type signature of a handler: takes PluginInput and returns PluginOutput.
PluginHandler = Callable[["PluginInput"], "PluginOutput"]


@dataclass
class PluginInput:
    """Input that taichi writes to the plugin's stdin.

    Field names use snake_case and are explicitly mapped to JSON keys via
    from_dict. The `config` field is the passthrough of the `raw` section in
    taichi.yaml after removing command/args/env/workdir/timeout.
    """

    skill_name: str = ""
    project_name: str = ""
    base_url: str = ""
    reports_dir: str = ""
    config: Dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "PluginInput":
        """Construct PluginInput from a parsed JSON dict; missing fields use defaults."""
        config = data.get("config") or {}
        if not isinstance(config, dict):
            config = {}
        return cls(
            skill_name=data.get("skill_name", "") or "",
            project_name=data.get("project_name", "") or "",
            base_url=data.get("base_url", "") or "",
            reports_dir=data.get("reports_dir", "") or "",
            config=config,
        )

    def endpoints(self) -> List[str]:
        """Convenience accessor for config["endpoints"].

        `endpoints: [/a, /b]` in taichi.yaml is passed through to
        config.endpoints. Returns an empty list when the value is not a list
        or is missing; elements are coerced to strings.
        """
        raw = self.config.get("endpoints")
        if not isinstance(raw, list):
            return []
        return [str(item) for item in raw]


@dataclass
class PluginCase:
    """Result of a single test case.

    `skipped` and `passed` are independent: when `skipped` is true, the taichi
    reporter counts it under skip statistics; otherwise it is counted as
    pass/fail according to `passed`. `skip_case` defaults to passed=False,
    skipped=True.
    """

    name: str
    passed: bool = False
    skipped: bool = False
    message: str = ""
    duration_ms: int = 0
    error: str = ""

    def to_dict(self) -> Dict[str, Any]:
        """Serialize to a protocol JSON dict, omitting empty optional fields to align with Go omitempty."""
        out: Dict[str, Any] = {
            "name": self.name,
            "passed": self.passed,
        }
        if self.skipped:
            out["skipped"] = True
        if self.message:
            out["message"] = self.message
        if self.duration_ms:
            out["duration_ms"] = self.duration_ms
        if self.error:
            out["error"] = self.error
        return out


@dataclass
class PluginOutput:
    """Output that the plugin writes to stdout.

    `cases` is always present (even if empty); a non-empty `error` indicates a
    plugin-level fatal error, in which case taichi marks the skill as not
    fully executed.
    """

    cases: List[PluginCase] = field(default_factory=list)
    error: str = ""

    def to_dict(self) -> Dict[str, Any]:
        out: Dict[str, Any] = {"cases": [c.to_dict() for c in self.cases]}
        if self.error:
            out["error"] = self.error
        return out


def pass_case(name: str, message: str = "ok") -> PluginCase:
    """Construct a passing case."""
    return PluginCase(name=name, passed=True, message=message)


def fail_case(name: str, error: str, message: str = "failed") -> PluginCase:
    """Construct a failing case; `error` is required to describe the failure."""
    return PluginCase(name=name, passed=False, message=message, error=error)


def skip_case(name: str, message: str = "skipped") -> PluginCase:
    """Construct a skipped case. passed=False, skipped=True."""
    return PluginCase(name=name, passed=False, skipped=True, message=message)


def read_input(stream=None) -> PluginInput:
    """Read and parse PluginInput JSON from `stream`; defaults to sys.stdin."""
    stream = stream if stream is not None else sys.stdin
    data = json.load(stream)
    if not isinstance(data, dict):
        raise ValueError("plugin input must be a JSON object")
    return PluginInput.from_dict(data)


def write_output(output: PluginOutput, stream=None) -> None:
    """Serialize PluginOutput as JSON and write it to `stream`; defaults to sys.stdout."""
    stream = stream if stream is not None else sys.stdout
    json.dump(output.to_dict(), stream, ensure_ascii=False)
    stream.write("\n")


def run_plugin(handler: PluginHandler) -> None:
    """Plugin main entry point.

    Automatically: reads stdin → parses PluginInput → invokes handler →
    writes PluginOutput to stdout. When the handler raises an exception,
    outputs a PluginOutput with an `error` field and exits with code 1,
    indicating a plugin-level fatal error.

    Normal execution exits with code 0; pass/fail is expressed via the
    `cases` in stdout JSON.
    """
    try:
        plugin_input = read_input()
    except Exception as exc:  # noqa: BLE001 - protocol-level fallback; input errors are treated as fatal
        write_output(PluginOutput(error=f"read plugin input failed: {exc}"))
        sys.exit(1)

    try:
        output = handler(plugin_input)
    except Exception as exc:  # noqa: BLE001 - convert handler exceptions to plugin-level errors
        write_output(
            PluginOutput(
                error=f"plugin handler raised: {type(exc).__name__}: {exc}"
            )
        )
        sys.exit(1)

    if output is None:
        output = PluginOutput(cases=[])

    write_output(output)
    sys.exit(0)
