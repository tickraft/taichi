#!/usr/bin/env node
'use strict';

/**
 * taichi Node.js plugin example: HTTP GET probe against input.config.endpoints.
 *
 * A status code of 2xx is treated as pass, otherwise fail. Uses only Node
 * built-in http/https modules; no third-party dependencies required.
 *
 * Local debugging:
 *   echo '{"skill_name":"x","project_name":"demo","base_url":"http://127.0.0.1:8000","config":{"endpoints":["/health"]}}' | node example.js
 */

const http = require('http');
const https = require('https');

const {
  runPlugin,
  endpoints,
  passCase,
  failCase,
} = require('./taichi-plugin');

// Probe timeout for a single endpoint (milliseconds).
const PROBE_TIMEOUT_MS = 5000;

// Issue a GET against a single URL and return { status, error }.
function probeUrl(url) {
  return new Promise((resolve) => {
    const lib = url.startsWith('https:') ? https : http;
    const req = lib.get(url, { timeout: PROBE_TIMEOUT_MS }, (res) => {
      // Consume the response body to release the socket.
      res.resume();
      resolve({ status: res.statusCode || 0, error: null });
    });
    req.on('error', (err) => {
      const code = err && err.code ? err.code : 'Error';
      const msg = err && err.message ? err.message : String(err);
      resolve({ status: 0, error: `${code}: ${msg}` });
    });
    req.on('timeout', () => {
      req.destroy(new Error('request timeout'));
    });
  });
}

// Probe a single endpoint and construct a case (with duration).
async function probe(baseUrl, endpoint) {
  const url = baseUrl.replace(/\/+$/, '') + '/' + endpoint.replace(/^\/+/, '');
  const caseName = `GET ${endpoint}`;
  const start = Date.now();
  const { status, error } = await probeUrl(url);
  const durationMs = Date.now() - start;

  if (error) {
    return {
      name: caseName,
      passed: false,
      message: 'request failed',
      error,
      duration_ms: durationMs,
    };
  }
  if (status >= 200 && status < 300) {
    return {
      name: caseName,
      passed: true,
      message: `HTTP ${status}`,
      duration_ms: durationMs,
    };
  }
  return {
    name: caseName,
    passed: false,
    message: `HTTP ${status}`,
    error: `unexpected status: ${status}`,
    duration_ms: durationMs,
  };
}

async function handler(input) {
  const eps = endpoints(input);
  const baseUrl = input.base_url;

  // Skip when no endpoints are configured, to avoid running empty.
  if (eps.length === 0) {
    return {
      cases: [
        {
          name: 'EndpointsProbe',
          passed: false,
          skipped: true,
          message: 'no endpoints configured',
        },
      ],
    };
  }

  // Without base_url we cannot issue requests; record as failure.
  if (!baseUrl) {
    return {
      cases: [failCase('EndpointsProbe', 'base_url is empty', 'missing base_url')],
    };
  }

  const cases = [];
  for (const ep of eps) {
    process.stderr.write(`[example] probing ${baseUrl}${ep}\n`);
    cases.push(await probe(baseUrl, ep));
  }
  return { cases };
}

runPlugin(handler);
