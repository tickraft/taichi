#!/usr/bin/env python3
"""taichi Python plugin example: HTTP GET probe against input.config.endpoints.

A status code of 2xx is treated as pass, otherwise fail. Depends only on the
standard library urllib; no third-party packages required.

Local debugging:

    echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health"]}}' | python3 example.py
"""

from __future__ import annotations

import sys
import time
import urllib.error
import urllib.request

import taichi_plugin

# Probe timeout for a single endpoint (seconds).
PROBE_TIMEOUT = 5.0


def probe(base_url: str, endpoint: str) -> taichi_plugin.PluginCase:
    """Issue a GET to base_url + endpoint; 2xx is treated as pass."""
    # Build the full URL, avoiding duplicate slashes.
    url = base_url.rstrip("/") + "/" + endpoint.lstrip("/")
    case_name = f"GET {endpoint}"
    start = time.monotonic()

    status = 0
    err_msg = ""
    try:
        with urllib.request.urlopen(url, timeout=PROBE_TIMEOUT) as resp:
            status = resp.status
    except urllib.error.HTTPError as exc:
        # HTTP error codes (4xx/5xx) still expose a status; treat as business failure, not exception.
        status = exc.code
    except Exception as exc:  # noqa: BLE001 - network-layer exceptions are uniformly recorded as failures
        err_msg = f"{type(exc).__name__}: {exc}"

    duration_ms = int((time.monotonic() - start) * 1000)

    if err_msg:
        return taichi_plugin.PluginCase(
            name=case_name,
            passed=False,
            message="request failed",
            error=err_msg,
            duration_ms=duration_ms,
        )
    if 200 <= status < 300:
        return taichi_plugin.PluginCase(
            name=case_name,
            passed=True,
            message=f"HTTP {status}",
            duration_ms=duration_ms,
        )
    return taichi_plugin.PluginCase(
        name=case_name,
        passed=False,
        message=f"HTTP {status}",
        error=f"unexpected status: {status}",
        duration_ms=duration_ms,
    )


def handler(plugin_input: taichi_plugin.PluginInput) -> taichi_plugin.PluginOutput:
    """Plugin business entry: read endpoint config and probe each one."""
    endpoints = plugin_input.endpoints()
    base_url = plugin_input.base_url

    # Skip when no endpoints are configured, to avoid running empty.
    if not endpoints:
        return taichi_plugin.PluginOutput(
            cases=[
                taichi_plugin.skip_case(
                    "EndpointsProbe", message="no endpoints configured"
                )
            ]
        )

    # Without base_url we cannot issue requests; record as failure.
    if not base_url:
        return taichi_plugin.PluginOutput(
            cases=[
                taichi_plugin.fail_case(
                    "EndpointsProbe",
                    error="base_url is empty",
                    message="missing base_url",
                )
            ]
        )

    cases = []
    for ep in endpoints:
        print(f"[example] probing {base_url}{ep}", file=sys.stderr)
        cases.append(probe(base_url, ep))
    return taichi_plugin.PluginOutput(cases=cases)


if __name__ == "__main__":
    taichi_plugin.run_plugin(handler)
