#!/usr/bin/env bash
# taichi bash plugin example: HTTP GET probe against input.config.endpoints.
#
# A status code of 2xx is treated as pass, otherwise fail. Uses curl for
# probing and jq/python3 for JSON parsing.
#
# Local debugging:
#   echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health"]}}' | bash example.sh

set -euo pipefail

# Locate the SDK library relative to this script's directory and source it.
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=taichi-plugin.sh
source "$script_dir/taichi-plugin.sh"

# Read PluginInput.
taichi_read_input

base_url="$(taichi_input_field base_url)"

echo "[example] starting, base_url=$base_url" >&2

# Bootstrap self-check case (demonstrates the taichi_pass shortcut).
taichi_pass "PluginBootstrap" "插件启动成功"

# Without base_url we cannot issue requests; record as failure and exit.
if [ -z "$base_url" ]; then
  taichi_fail "EndpointsProbe" "base_url is empty" "missing base_url"
  taichi_emit_output
  exit 0
fi

# Read the endpoint list (one per line).
endpoints="$(taichi_input_endpoints)"
if [ -z "$endpoints" ]; then
  # Demonstrate the taichi_skip shortcut.
  taichi_skip "EndpointsProbe" "no endpoints configured"
  taichi_emit_output
  exit 0
fi

# Millisecond timestamp (use python3 for cross-platform support; macOS `date` does not support %N).
now_ms() {
  python3 -c 'import time;print(int(time.time()*1000))'
}

# Probe each endpoint. Here we need duration_ms, so we use taichi_emit_case directly.
while IFS= read -r ep; do
  [ -z "$ep" ] && continue
  url="${base_url%/}/${ep#/}"
  echo "[example] probing $url" >&2

  start_ms="$(now_ms)"
  # curl returns 000 on failure or timeout.
  http_code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$url" 2>/dev/null || echo "000")"
  end_ms="$(now_ms)"
  duration_ms=$((end_ms - start_ms))

  case_name="GET $ep"
  if [ "$http_code" = "000" ]; then
    taichi_emit_case "$case_name" false "request failed" "connection failed (curl non-zero exit or timeout)" false "$duration_ms"
  elif [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
    taichi_emit_case "$case_name" true "HTTP $http_code" "" false "$duration_ms"
  else
    taichi_emit_case "$case_name" false "HTTP $http_code" "unexpected status: $http_code" false "$duration_ms"
  fi
done <<< "$endpoints"

taichi_emit_output
exit 0
