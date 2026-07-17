#!/usr/bin/env bash
# taichi plugin SDK (bash implementation, can be sourced).
#
# Wraps the stdin/stdout JSON exchange of the taichi plugin protocol so that
# plugin authors can focus on the test logic. Protocol fields are kept
# strictly aligned with PluginInput / PluginOutput / PluginCase in
# taichi/pkg/skill/plugin/skill.go.
#
# Protocol overview:
#   stdin  → PluginInput  JSON (skill_name, project_name, base_url, reports_dir, config)
#   stdout ← PluginOutput JSON (cases[], error)
#   stderr ← free-form logs (taichi forwards them to its own logger)
#   exit 0 = plugin executed normally; exit ≠ 0 = plugin-level fatal error
#
# Dependencies: bash 4+; JSON parsing prefers jq, falls back to python3 when missing.
#
# Typical usage:
#   source ./taichi-plugin.sh
#   taichi_read_input
#   base_url="$(taichi_input_field base_url)"
#   taichi_pass "Bootstrap"
#   taichi_emit_output
#
# Note: this file is designed to be sourced; it does not set `set -e` or other
# options itself, to avoid polluting the caller's shell.

# Prevent resetting collected cases and state when sourced repeatedly.
if [ -z "${_TAICHI_PLUGIN_SOURCED:-}" ]; then
  _TAICHI_PLUGIN_SOURCED=1
  # Internal case array; stores the JSON object string of each case.
  _TAICHI_CASES=()
  # Plugin-level fatal error message (when non-empty, emit_output writes it to the error field).
  _TAICHI_ERROR=""
  # Raw stdin JSON.
  _TAICHI_INPUT_RAW=""
  # Detected JSON tool: jq or python3.
  _TAICHI_JSON_TOOL=""
fi

# Detect an available JSON tool. Prefers jq, falls back to python3; returns non-zero if neither is found.
taichi_detect_tool() {
  if [ -n "$_TAICHI_JSON_TOOL" ]; then
    return 0
  fi
  if command -v jq >/dev/null 2>&1; then
    _TAICHI_JSON_TOOL="jq"
    return 0
  fi
  if command -v python3 >/dev/null 2>&1; then
    _TAICHI_JSON_TOOL="python3"
    return 0
  fi
  echo "[taichi-plugin] neither jq nor python3 found, cannot parse JSON" >&2
  return 1
}

# Read the PluginInput JSON from stdin into the internal variable _TAICHI_INPUT_RAW.
taichi_read_input() {
  taichi_detect_tool || return 1
  _TAICHI_INPUT_RAW="$(cat)"
}

# Extract a top-level string field from input (e.g. skill_name / project_name / base_url / reports_dir).
# Usage: taichi_input_field <key>
taichi_input_field() {
  local key="$1"
  if [ -z "$_TAICHI_INPUT_RAW" ]; then
    echo ""
    return 0
  fi
  if [ "$_TAICHI_JSON_TOOL" = "jq" ]; then
    echo "$_TAICHI_INPUT_RAW" | jq -r --arg k "$key" '.[$k] // ""' 2>/dev/null || echo ""
  else
    echo "$_TAICHI_INPUT_RAW" | python3 -c \
      'import sys,json
d=json.load(sys.stdin)
v=d.get(sys.argv[1],"")
print("" if v is None else v)' "$key" 2>/dev/null || echo ""
  fi
}

# Extract input.config.endpoints, printing one endpoint per line.
taichi_input_endpoints() {
  if [ -z "$_TAICHI_INPUT_RAW" ]; then
    return 0
  fi
  if [ "$_TAICHI_JSON_TOOL" = "jq" ]; then
    echo "$_TAICHI_INPUT_RAW" | jq -r '.config.endpoints[]?' 2>/dev/null || true
  else
    echo "$_TAICHI_INPUT_RAW" | python3 -c \
      'import sys,json
d=json.load(sys.stdin)
c=d.get("config") or {}
eps=c.get("endpoints") or []
for e in eps:
    print(str(e))' 2>/dev/null || true
  fi
}

# Set the plugin-level fatal error message (when non-empty, emit_output writes it to the error field).
taichi_set_error() {
  _TAICHI_ERROR="$1"
}

# Internal: build the JSON object string for a single case.
# Args: <name> <passed:true|false> [message] [error] [skipped:true|false] [duration_ms]
# String fields are safely escaped via the selected JSON tool.
_taichi_build_case() {
  local name="$1" passed="$2" message="${3:-}" error="${4:-}" skipped="${5:-false}" duration_ms="${6:-0}"
  if [ "$_TAICHI_JSON_TOOL" = "jq" ]; then
    jq -cn \
      --arg name "$name" \
      --argjson passed "$([ "$passed" = "true" ] && echo true || echo false)" \
      --argjson skipped "$([ "$skipped" = "true" ] && echo true || echo false)" \
      --arg message "$message" \
      --arg error "$error" \
      --argjson duration_ms "$duration_ms" \
      '{name:$name, passed:$passed}
       + (if $skipped then {skipped:true} else {} end)
       + (if ($message|length)>0 then {message:$message} else {} end)
       + (if ($error|length)>0 then {error:$error} else {} end)
       + (if $duration_ms>0 then {duration_ms:$duration_ms} else {} end)'
  else
    python3 -c \
      'import sys, json
name, passed, message, error, skipped, duration_ms = sys.argv[1:7]
out = {"name": name, "passed": passed == "true"}
if skipped == "true":
    out["skipped"] = True
if message:
    out["message"] = message
if error:
    out["error"] = error
try:
    dm = int(duration_ms)
except ValueError:
    dm = 0
if dm:
    out["duration_ms"] = dm
print(json.dumps(out, ensure_ascii=False))' \
      "$name" "$passed" "$message" "$error" "$skipped" "$duration_ms"
  fi
}

# Append a case to the internal array.
# Usage: taichi_emit_case <name> <passed:true|false> [message] [error] [skipped:true|false] [duration_ms]
taichi_emit_case() {
  local name="$1" passed="$2" message="${3:-}" error="${4:-}" skipped="${5:-false}" duration_ms="${6:-0}"
  taichi_detect_tool || return 1
  local obj
  obj="$(_taichi_build_case "$name" "$passed" "$message" "$error" "$skipped" "$duration_ms")"
  _TAICHI_CASES+=("$obj")
}

# Shortcut: passing case.
# Usage: taichi_pass <name> [message]
taichi_pass() {
  local name="$1" message="${2:-ok}"
  taichi_emit_case "$name" true "$message" "" false 0
}

# Shortcut: failing case; `error` is required.
# Usage: taichi_fail <name> <error> [message]
taichi_fail() {
  local name="$1" error="$2" message="${3:-failed}"
  taichi_emit_case "$name" false "$message" "$error" false 0
}

# Shortcut: skipped case. passed=false, skipped=true.
# Usage: taichi_skip <name> [message]
taichi_skip() {
  local name="$1" message="${2:-skipped}"
  taichi_emit_case "$name" false "$message" "" true 0
}

# Output the full PluginOutput JSON to stdout.
# The internal case array is joined with commas; the error field is appended when non-empty.
taichi_emit_output() {
  local joined=""
  local c
  for c in "${_TAICHI_CASES[@]:-}"; do
    [ -z "$c" ] && continue
    if [ -z "$joined" ]; then
      joined="$c"
    else
      joined="$joined,$c"
    fi
  done

  if [ -n "$_TAICHI_ERROR" ]; then
    # The error field must be JSON-escaped; use the selected tool to generate {"error":"..."} then strip the outer braces.
    local err_json err_inner
    if [ "$_TAICHI_JSON_TOOL" = "jq" ]; then
      err_json="$(jq -cn --arg e "$_TAICHI_ERROR" '{error:$e}')"
    else
      err_json="$(python3 -c \
        'import sys,json
print(json.dumps({"error":sys.argv[1]},ensure_ascii=False))' "$_TAICHI_ERROR")"
    fi
    err_inner="${err_json#\{}"
    err_inner="${err_inner%\}}"
    if [ -n "$joined" ]; then
      echo "{\"cases\":[$joined],$err_inner}"
    else
      echo "{\"cases\":[],$err_inner}"
    fi
  else
    echo "{\"cases\":[$joined]}"
  fi
}
