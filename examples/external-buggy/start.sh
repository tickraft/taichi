#!/usr/bin/env bash
# Start the external-buggy service for taichi integration testing.
#
# This service is "externally managed" — taichi does NOT start or stop it.
# Run this script in one terminal, then run `taichi run` in another.
#
# Usage:
#   ./start.sh                # default port 18090
#   ./start.sh --port 18091   # custom port
#   PORT=18091 ./start.sh     # custom port via env var
#
# Press Ctrl+C to stop the service.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Allow overriding the port via env var or the first argument.
if [ -n "${PORT:-}" ]; then
    ARGS="--port $PORT"
elif [ $# -gt 0 ]; then
    ARGS="$*"
else
    ARGS=""
fi

echo "[start.sh] starting external-buggy service..."
echo "[start.sh] once ready, run: taichi run -c taichi.yaml"
exec python3 server.py $ARGS
