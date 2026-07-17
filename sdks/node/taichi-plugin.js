'use strict';

/**
 * taichi plugin SDK (Node.js implementation, CommonJS).
 *
 * Wraps the stdin/stdout JSON exchange of the taichi plugin protocol so that
 * plugin authors can focus on the test logic. Protocol fields are kept
 * strictly aligned with PluginInput / PluginOutput / PluginCase in
 * taichi/pkg/skill/plugin/skill.go.
 *
 * Protocol overview:
 *   stdin  → PluginInput  JSON (skill_name, project_name, base_url, reports_dir, config)
 *   stdout ← PluginOutput JSON (cases[], error)
 *   stderr ← free-form logs (taichi forwards them to its own logger)
 *   exit 0 = plugin executed normally; exit ≠ 0 = plugin-level fatal error
 *
 * Typical usage:
 *   const { runPlugin, passCase } = require('./taichi-plugin');
 *   runPlugin((input) => {
 *     return { cases: [passCase('Bootstrap')] };
 *   });
 *
 * Uses only Node built-in modules; no third-party dependencies required.
 */

const fs = require('fs');

// Synchronously read all of fd 0 (stdin).
// Node's process.stdin is an async stream by default; here we use
// readFileSync(0) for synchronous reads so the handler can consume it synchronously.
function readStdinSync() {
  try {
    return fs.readFileSync(0, 'utf8');
  } catch (err) {
    // In some pipe scenarios reading may fail; return empty and let JSON.parse report the error upstream.
    return '';
  }
}

/**
 * Read and parse the PluginInput JSON from stdin.
 * @returns {{skill_name:string, project_name:string, base_url:string, reports_dir:string, config:object}}
 */
function readInput() {
  const raw = readStdinSync();
  if (!raw.trim()) {
    throw new Error('plugin input is empty');
  }
  const obj = JSON.parse(raw);
  if (typeof obj !== 'object' || obj === null || Array.isArray(obj)) {
    throw new Error('plugin input must be a JSON object');
  }
  const config =
    obj.config && typeof obj.config === 'object' && !Array.isArray(obj.config)
      ? obj.config
      : {};
  return {
    skill_name: typeof obj.skill_name === 'string' ? obj.skill_name : '',
    project_name: typeof obj.project_name === 'string' ? obj.project_name : '',
    base_url: typeof obj.base_url === 'string' ? obj.base_url : '',
    reports_dir: typeof obj.reports_dir === 'string' ? obj.reports_dir : '',
    config,
  };
}

/**
 * Read the endpoint list from input.config.endpoints; elements are coerced to strings.
 * @param {object} input return value of readInput()
 * @returns {string[]}
 */
function endpoints(input) {
  const raw = input && input.config ? input.config.endpoints : null;
  if (!Array.isArray(raw)) return [];
  return raw.map((item) => String(item));
}

// Normalize a single case into a protocol JSON dict, omitting empty optional fields to align with Go omitempty.
function caseToDict(c) {
  const out = { name: String(c.name), passed: Boolean(c.passed) };
  if (c.skipped) out.skipped = true;
  if (c.message) out.message = String(c.message);
  if (c.duration_ms) out.duration_ms = Number(c.duration_ms);
  if (c.error) out.error = String(c.error);
  return out;
}

/**
 * Serialize a PluginOutput object as JSON and write it to stdout.
 * @param {{cases:Array, error?:string}} output
 * @param {() => void} [done] callback invoked after the write completes (used to flush the buffer before safe exit)
 */
function writeOutput(output, done) {
  const cases = Array.isArray(output.cases)
    ? output.cases.map(caseToDict)
    : [];
  const out = { cases };
  if (output && typeof output.error === 'string' && output.error) {
    out.error = output.error;
  }
  const data = JSON.stringify(out) + '\n';
  if (typeof done === 'function') {
    process.stdout.write(data, () => done());
  } else {
    process.stdout.write(data);
  }
}

/**
 * Construct a passing case.
 * @param {string} name
 * @param {string} [message='ok']
 * @returns {{name:string, passed:boolean, message:string}}
 */
function passCase(name, message = 'ok') {
  return { name, passed: true, message };
}

/**
 * Construct a failing case; `error` is required.
 * @param {string} name
 * @param {string} error
 * @param {string} [message='failed']
 * @returns {{name:string, passed:boolean, message:string, error:string}}
 */
function failCase(name, error, message = 'failed') {
  return { name, passed: false, message, error };
}

/**
 * Construct a skipped case. passed=false, skipped=true.
 * @param {string} name
 * @param {string} [message='skipped']
 * @returns {{name:string, passed:boolean, skipped:boolean, message:string}}
 */
function skipCase(name, message = 'skipped') {
  return { name, passed: false, skipped: true, message };
}

// Format an error thrown by a handler into a protocol error string.
function formatHandlerError(err) {
  const name = err && err.name ? err.name : 'Error';
  const msg = err && err.message ? err.message : String(err);
  return `plugin handler raised: ${name}: ${msg}`;
}

/**
 * Plugin main entry point.
 *
 * Automatically: reads stdin → parses PluginInput → invokes handler →
 * writes PluginOutput to stdout. When the handler throws (or the returned
 * Promise rejects), outputs a PluginOutput with an `error` field and exits
 * with code 1, indicating a plugin-level fatal error.
 *
 * The handler may either synchronously return an output object or return a
 * Promise (async handler). Before exit, stdout is flushed to avoid losing
 * buffered output on the pipe.
 * @param {(input:object) => ({cases:Array, error?:string} | Promise<{cases:Array, error?:string}>)} handler
 */
function runPlugin(handler) {
  let input;
  try {
    input = readInput();
  } catch (err) {
    writeOutput(
      { cases: [], error: `read plugin input failed: ${err.message}` },
      () => process.exit(1)
    );
    return;
  }

  let result;
  try {
    result = handler(input);
  } catch (err) {
    writeOutput(
      { cases: [], error: formatHandlerError(err) },
      () => process.exit(1)
    );
    return;
  }

  // Support async handlers: when a Promise is returned, await its result asynchronously.
  if (result && typeof result.then === 'function') {
    result.then(
      (output) => {
        writeOutput(output || { cases: [] }, () => process.exit(0));
      },
      (err) => {
        writeOutput(
          { cases: [], error: formatHandlerError(err) },
          () => process.exit(1)
        );
      }
    );
    return;
  }

  writeOutput(result || { cases: [] }, () => process.exit(0));
}

module.exports = {
  readInput,
  writeOutput,
  runPlugin,
  endpoints,
  passCase,
  failCase,
  skipCase,
  caseToDict,
};
