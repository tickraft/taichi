"""Deliberately broken HTTP service for taichi's "external" env integration test.

This service is **externally managed**: taichi does NOT start or stop it.
Instead, the user runs `start.sh` (or `python3 server.py`) manually, and taichi
probes it via `base_url` declared in the config.

Intentional bugs (for taichi skills to detect):

- ``GET /api/v1/health``   returns ``data.status = "degraded"`` (expected ``"healthy"``)
- ``GET /api/v1/users``    returns HTTP 500 (expected 200)
- ``GET /``                 serves HTML missing the ``<div id="app">`` marker
- ``GET /api/v1/metrics``  returns code 500 instead of 0

Correct endpoints (for contrast):

- ``GET /api/v1/version``  returns ``data.version = "1.0.0"``
- ``GET /api/v1/ready``    returns HTTP 200

Usage::

    python3 server.py                # listens on 127.0.0.1:18090
    python3 server.py --port 18090  # custom port
"""

from __future__ import annotations

import argparse
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def make_handler() -> type[BaseHTTPRequestHandler]:
    class BuggyHandler(BaseHTTPRequestHandler):
        def _send_json(self, status: int, body: dict) -> None:
            payload = json.dumps(body).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

        def _send_html(self, html: str) -> None:
            payload = html.encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

        def log_message(self, fmt: str, *args: object) -> None:
            # Log to stderr so it doesn't interfere with taichi's stdout report.
            sys.stderr.write("[external-buggy] " + (fmt % args) + "\n")

        def do_GET(self) -> None:  # noqa: N802 - http.server API
            path = self.path.split("?", 1)[0]
            routes = {
                "/api/v1/health": self._handle_health,
                "/api/v1/users": self._handle_users,
                "/api/v1/version": self._handle_version,
                "/api/v1/metrics": self._handle_metrics,
                "/api/v1/ready": self._handle_ready,
                "/": self._handle_home,
            }
            handler = routes.get(path)
            if handler is None:
                self._send_json(404, {
                    "code": 1004,
                    "msg": "not found",
                    "request_id": "ext-not-found",
                })
                return
            handler()

        # --- Bug: health status is "degraded" instead of "healthy" ---
        def _handle_health(self) -> None:
            self._send_json(200, {
                "code": 0,
                "msg": "ok",
                "request_id": "ext-health-001",
                "data": {"status": "degraded"},  # BUG: should be "healthy"
            })

        # --- Bug: /api/v1/users returns 500 ---
        def _handle_users(self) -> None:
            self._send_json(500, {
                "code": 500,
                "msg": "database unreachable",
                "request_id": "ext-users-002",
            })

        # --- Correct: version endpoint ---
        def _handle_version(self) -> None:
            self._send_json(200, {
                "code": 0,
                "msg": "ok",
                "request_id": "ext-version-003",
                "data": {"version": "1.0.0", "region": "us-east-1"},
            })

        # --- Bug: /api/v1/metrics returns code 500 ---
        def _handle_metrics(self) -> None:
            self._send_json(200, {
                "code": 500,  # BUG: should be 0
                "msg": "metrics collection failed",
                "request_id": "ext-metrics-004",
                "data": {},
            })

        # --- Correct: readiness endpoint ---
        def _handle_ready(self) -> None:
            self._send_json(200, {
                "code": 0,
                "msg": "ok",
                "request_id": "ext-ready-005",
                "data": {"ready": True},
            })

        # --- Bug: homepage HTML missing <div id="app"> marker ---
        def _handle_home(self) -> None:
            # BUG: missing <div id="app"> that the UI skill checks for.
            self._send_html(
                "<!DOCTYPE html><html><head><title>External Buggy</title>"
                "</head><body><h1>Hello</h1></body></html>"
            )

    return BuggyHandler


def main() -> None:
    parser = argparse.ArgumentParser(description="Buggy external service for taichi")
    parser.add_argument("--host", default="127.0.0.1", help="listen host")
    parser.add_argument("--port", type=int, default=18090, help="listen port")
    args = parser.parse_args()

    server = ThreadingHTTPServer((args.host, args.port), make_handler())
    print(f"[external-buggy] listening on http://{args.host}:{args.port}", file=sys.stderr)
    print("[external-buggy] press Ctrl+C to stop", file=sys.stderr)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n[external-buggy] shutting down", file=sys.stderr)
        server.shutdown()


if __name__ == "__main__":
    main()
