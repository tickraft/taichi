#!/usr/bin/env bash
# Example taichi plugin skill — a minimal plugin implemented in bash.
#
# Protocol:
#   stdin  → PluginInput JSON  (skill_name, project_name, base_url, reports_dir, config)
#   stdout ← PluginOutput JSON (cases[], error)
#   stderr ← free-form logs (taichi forwards them to its own logger)
#   exit 0 = plugin executed normally; exit ≠ 0 = plugin-level fatal error
#
# Usage: declare `kind: plugin` in taichi.yaml and point raw.command to this script.
#
#   skills:
#     - name: example-bash
#       kind: plugin
#       enabled: true
#       priority: 40
#       raw:
#         command: ./examples/plugin-example.sh
#         timeout: 10s
#         # The custom fields below are passed through to the config object of stdin JSON
#         checks:
#           - name: EchoCheck
#             expect: "ok"

set -euo pipefail

# Read stdin (PluginInput JSON)
input_json="$(cat)"

# Extract base_url (requires jq; falls back to python3 if jq is missing)
if command -v jq >/dev/null 2>&1; then
    base_url="$(echo "$input_json" | jq -r '.base_url // ""')"
else
    base_url="$(echo "$input_json" | python3 -c 'import sys,json;print(json.load(sys.stdin).get("base_url",""))' 2>/dev/null || echo "")"
fi

# Logs go to stderr (taichi will forward them)
echo "[example-bash] starting, base_url=$base_url" >&2

# Collect case JSON objects in an array
cases=()
cases+=('{"name":"PluginBootstrap","passed":true,"message":"插件启动成功","duration_ms":1}')

# If base_url is non-empty, try to access it and check for HTTP 200
if [ -n "$base_url" ]; then
    echo "[example-bash] probing $base_url ..." >&2
    http_code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "$base_url" 2>/dev/null || echo "000")"
    if [ "$http_code" = "200" ]; then
        cases+=('{"name":"BaseURLReachable","passed":true,"message":"基址可达 (HTTP 200)","duration_ms":50}')
    else
        cases+=("{\"name\":\"BaseURLReachable\",\"passed\":false,\"message\":\"基址不可达 (HTTP $http_code)\",\"duration_ms\":50,\"error\":\"unexpected status: $http_code\"}")
    fi
fi

# Join all cases with commas
joined=""
for c in "${cases[@]}"; do
    if [ -z "$joined" ]; then
        joined="$c"
    else
        joined="$joined,$c"
    fi
done

# Write PluginOutput JSON to stdout
echo "{\"cases\":[$joined]}"

exit 0
