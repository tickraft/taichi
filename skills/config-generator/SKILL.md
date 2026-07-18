---
name: "taichi-config-generator"
description: "Automatically generates Taichi test orchestration config files (taichi.yaml) by analyzing project source code. Invoke when the user requests generating test config for a new project, initializing Taichi tests, or adapting Taichi to a new project. The Agent scans the project directory structure, identifies project type, extracts HTTP routes and health-check endpoints, and produces a ready-to-use taichi.yaml that can be fine-tuned manually afterwards."
---

> 🌐 Languages: [English](SKILL.md) | [中文](SKILL.zh.md)

# Taichi Config Generator Skill

## 1. Overview

This Skill lets an AI Agent automatically generate the config file (`taichi.yaml`) required by the Taichi test orchestration framework through static analysis of the project source code and directory structure. Taichi is a general-purpose automated test orchestration framework: by describing the project under test, environments, and skills in a config file, it can orchestrate a complete test run.

The generated config covers the following:
- Project under test declaration (project name, root directory, environment binding, enabled skills list)
- Environment definitions (startup and readiness probes for Go/Node backends, Vite/Nuxt frontends)
- Skill config (cases for the five built-in skills: api / grpc / ui / static / regression)
- Report output and autofix config
- UI locale setting

The generated result can be verified directly by running `taichi run`; users can then manually add/remove cases and adjust assertions.

## 2. When to Invoke This Skill

**Mandatory invocation scenarios**:
- The user mentions "generate Taichi config", "initialize test config", "add Taichi tests for a new project"
- The user wants Taichi to adapt to a new project that has not been configured yet
- The existing `taichi.yaml` is missing or corrupted and needs to be rebuilt
- The project has undergone major refactoring (full route changes, tech stack migration) and the config needs to be regenerated

**Do not invoke scenarios**:
- The project already has a `taichi.yaml` and only needs minor adjustments (just edit it directly)
- Only need to run tests (use `taichi-test-runner` instead)
- The project is still at the scaffolding stage with no real testable routes (implement core functionality first)

## 3. Input Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_root` | string | Yes | Absolute path of the project root directory to analyze |
| `output_path` | string | No | Output path of the generated config file; default `<project_root>/taichi.yaml` |
| `project_name` | string | No | Project name; inferred from the `go.mod` module name or the `name` field in `package.json` by default |
| `env_kind` | string | No | Force the environment type: `backend.go` / `backend.node` / `frontend.vite` / `frontend.nuxt`. Auto-detected if not specified |
| `base_url` | string | No | If the service under test is already running externally, specify the base URL to skip startup (e.g. `http://localhost:8080`) |
| `locale` | string | No | Value of the `locale` field in the generated config; default `auto` |

## 4. Analysis Flow

The Agent should analyze the project in the following steps, progressively collecting the fields needed for the config.

### 4.1 Project Type Identification

Check the following files in priority order to determine the project type:

| Detection signal | Project type | env.kind |
|------------------|--------------|----------|
| `go.mod` in root and a main package under `cmd/` | Go backend | `backend.go` |
| `package.json` in root with `next`/`nuxt` dependency | Nuxt frontend | `frontend.nuxt` |
| `package.json` in root with `vite` dependency | Vite frontend | `frontend.vite` |
| `package.json` in root with `scripts.start` pointing to a node service | Node backend | `backend.node` |

If both frontend and backend signals exist (e.g. monorepo), generate multiple projects and multiple envs, one for each.

### 4.2 Project Metadata Extraction

| Field | Extraction rule |
|-------|-----------------|
| `project_name` | Go: last segment of `go.mod` module path; Node: `name` field of `package.json`; others: root directory name |
| `project.root` | Project root path relative to the `taichi.yaml` output location |
| `build` | Go: `go build -o bin/<project_name> ./cmd/<project_name>` (or scan `cmd/` subdirectories); also accepts any shell command via `sh -c` (e.g. `make build`); Node: not needed |
| `binary` | Go: `bin/<project_name>`; Node: not needed |
| `config_path` | Prefer `configs/config.yaml`, then `config.yaml`, `configs/config.yml`, `configs/<project_name>.yaml` |
| `config_flag` | Go: `--config`; Node: extract from `scripts.start`, default `--config` |
| `addr_flag` | Go: `--addr`; Node: infer from startup script, default `--port` |
| `health_path` | See 4.4 route extraction |

### 4.3 Frontend Readiness Probe Extraction

Only when env.kind is `frontend.*`:

| Field | Extraction rule |
|-------|-----------------|
| `command` | `scripts.dev` of `package.json`, default `pnpm dev` |
| `cwd` | Subdirectory where frontend source resides, default `.` |
| `ready_url` | Inferred from `server.port` in vite.config, default `http://localhost:5173` (vite) or `http://localhost:3000` (nuxt) |
| `ready_text` | Empty string; readiness is determined solely by HTTP status < 500 |

### 4.4 HTTP Route Extraction

**Go projects**: Scan the following locations for route registration code and extract `method` and `path`:

- `hertz`/`gin`/`echo`/`fiber`/`chi` route registration: `r.GET("/path", ...)`, `r.POST("/path", ...)`
- `net/http` standard library: `http.HandleFunc("/path", ...)`
- Route group prefixes: detect `r.Group("/api/v1")` and concatenate sub-routes

**Node projects**: Scan the following locations:

- `express`: `app.get("/path", ...)`, `app.post("/path", ...)`
- `fastify`: `fastify.get("/path", ...)`
- `koa-router`: `router.get("/path", ...)`

**Health check identification**: Preferentially recognize the following paths as `health_path` and regression test cases:
- `/health`, `/healthz`, `/healthz/ready`, `/healthz/live`
- `/api/v1/health`, `/api/health`
- `/ping`, `/api/ping`
- `/ready`, `/readyz`

If no health check endpoint is identified, add a comment in the generated api skill cases prompting the user to specify one manually.

### 4.5 Case Generation Strategy

#### api skill

Generate a test case for each identified HTTP route:

```yaml
- name: <RouteName>          # PascalCase from path, e.g. /api/v1/users → UsersList
  method: <GET|POST|...>     # extracted from route registration
  path: <path>               # original path
  expected_status: 200       # GET defaults to 200; POST/PUT defaults to 200 or 201; DELETE defaults to 200 or 204
  expected_code: 0           # if the project response body has a code field, 0 means success by default
```

For routes with path parameters (e.g. `/api/v1/users/:id`), generate a case using a placeholder ID and mark `expected_status: 404` (since placeholder IDs usually do not exist).

#### ui skill (frontend projects only)

Extract page paths from route configuration or the pages directory:

- Vue Router: scan `router/routes.ts` or `router/index.ts`
- Next/Nuxt: scan the `pages/` directory structure
- When no route config exists: only test the root path `/`

```yaml
- path: <page-path>
  contains: [<html]       # basic assertion: response contains an HTML tag
  max_latency: 2s         # optional
```

#### static skill (frontend projects only)

```yaml
pages:
  - /                      # homepage
  - /nonexistent-page-12345  # SPA fallback verification
assets:
  - /.gitkeep
  - /_nuxt/app.js          # nuxt
  - /assets/main.js        # vite (extract real asset paths from index.html)
```

#### regression skill

Select key paths from the api skill cases (health check, core list endpoints, homepage) as regression cases, remove detailed assertion fields, keeping only `expected_status` and `expected_code`:

```yaml
- name: Health
  path: /api/v1/health
  expected_status: 200
  expected_code: 0
```

### 4.6 Report and Locale Config

```yaml
report:
  suite_name: taichi-<project_name>
  output_dir: reports
  formats: [json, junit, summary]

autofix:
  enabled: false
  reports_dir: reports/errors

locale: auto                # or inferred from the user's system environment
```

## 5. Output Format

The Agent should write the generated config as a YAML file to `output_path` and return a JSON summary with the following structure:

```json
{
  "output_path": "/path/to/taichi.yaml",
  "project_name": "tickraft",
  "env_kind": "backend.go",
  "detected_routes": 18,
  "generated_cases": {
    "api": 18,
    "ui": 0,
    "static": 0,
    "regression": 5
  },
  "health_path": "/api/v1/health",
  "warnings": [
    "No health check endpoint detected; a placeholder case has been added, please confirm manually",
    "Route /api/v1/users/:id contains a path parameter; a 404 case has been generated"
  ],
  "next_steps": [
    "Run `taichi run -c <output_path>` to verify the config",
    "Review the items listed in warnings that need confirmation",
    "Add expected_field / expected_value assertions based on business semantics"
  ]
}
```

## 6. Generation Templates

The following is the standard structure template of the generated `taichi.yaml`. The Agent should produce output following this structure, keeping field order and comment style consistent.

### 6.1 Go Backend Project Template

```yaml
# taichi config — <project_name> project
# Auto-generated by the taichi-config-generator skill; safe to edit manually.
# See pkg/config/config.go and docs/configuration.md for the config schema.

# ========== Project under test ==========
projects:
  - name: <project_name>
    root: <project_root_relative>
    env: <project_name>-backend
    skills: [api, regression]      # adjust based on detected route types

# ========== Environment definitions ==========
envs:
  <project_name>-backend:
    kind: backend.go
    binary: bin/<project_name>
    build: go build -o bin/<project_name> ./cmd/<project_name>
    config_path: configs/config.yaml
    config_flag: --config
    addr_flag: --addr
    health_path: <health_path>
    healthy_timeout: 30s

# ========== Skill config ==========
skills:
  - name: api
    kind: api
    enabled: true
    priority: 0
    raw:
      timeout: 5s
      cases:
        - name: Health
          method: GET
          path: <health_path>
          expected_status: 200
          expected_code: 0
        # ... other cases generated from routes

  - name: regression
    kind: regression
    enabled: true
    priority: 30
    raw:
      timeout: 5s
      cases:
        - name: Health
          path: <health_path>
          expected_status: 200
          expected_code: 0
        # ... other regression cases for key paths

# ========== Report output ==========
report:
  suite_name: taichi-<project_name>
  output_dir: reports
  formats: [json, junit, summary]

# ========== Autofix ==========
autofix:
  enabled: false
  reports_dir: reports/errors

# ========== UI locale ==========
locale: auto
```

### 6.2 Vite/Nuxt Frontend Project Template

```yaml
# taichi config — <project_name> frontend project
# Auto-generated by the taichi-config-generator skill; safe to edit manually.

projects:
  - name: <project_name>
    root: <project_root_relative>
    env: <project_name>-frontend
    skills: [ui, static, regression]

envs:
  <project_name>-frontend:
    kind: frontend.vite          # or frontend.nuxt
    command: pnpm dev
    cwd: .
    ready_url: http://localhost:5173
    ready_text: ""
    port: 5173

skills:
  - name: ui
    kind: ui
    enabled: true
    priority: 10
    raw:
      timeout: 5s
      pages:
        - path: /
          contains: [<html]
          max_latency: 2s
        # ... other pages

  - name: static
    kind: static
    enabled: true
    priority: 20
    raw:
      timeout: 5s
      pages:
        - /
        - /nonexistent-page-12345
      assets:
        - /.gitkeep
        - /assets/main.js

  - name: regression
    kind: regression
    enabled: true
    priority: 30
    raw:
      timeout: 5s
      cases:
        - name: Homepage
          path: /
          expected_status: 200
          skip_on_404: true

report:
  suite_name: taichi-<project_name>
  output_dir: reports
  formats: [json, junit, summary]

autofix:
  enabled: false
  reports_dir: reports/errors

locale: auto
```

### 6.3 Full-stack Project Template (frontend + backend merged)

When the project contains both a Go backend and a frontend, generate two projects and two envs:

```yaml
projects:
  - name: <project_name>-api
    root: <backend_root>
    env: <project_name>-backend
    skills: [api, regression]
  - name: <project_name>-web
    root: <frontend_root>
    env: <project_name>-frontend
    skills: [ui, static, regression]

envs:
  <project_name>-backend:
    kind: backend.go
    # ... same as 6.1
  <project_name>-frontend:
    kind: frontend.vite
    # ... same as 6.2

skills:
  # ... merged config of api + ui + static + regression
```

### 6.4 Externally Hosted Service Template

When the `base_url` parameter is specified, omit startup-related fields in env:

```yaml
envs:
  <project_name>-external:
    kind: backend.go           # kind is still required, but binary/build can be omitted
    base_url: http://localhost:8080
    health_path: /api/v1/health
```

## 7. Post-generation Validation

After generating the config, the Agent should perform the following validation steps and include the results in the output summary:

1. **YAML syntax validation**: Ensure the generated file can be loaded by a YAML parser
2. **Taichi load validation**: Run `taichi list -c <output_path>` to confirm the config can be loaded correctly by Taichi
3. **Route sanity check**:
   - Whether the health check case is actually reachable (if the service is already running)
   - Whether path-parameter routes are annotated with `expected_status: 404`
4. **warnings summary**: List all items requiring manual confirmation in the `warnings` array of the output JSON

If `taichi list` fails, the Agent should fix obvious errors in the config (e.g. wrong paths, misspelled field names) and retry, up to 3 rounds.

## 8. Integration with Other Skills

| Handoff scenario | Downstream Skill | Trigger condition |
|------------------|------------------|-------------------|
| First test run after config generation | `taichi-test-runner` | After the user confirms the config is correct, run `taichi run -c <output_path>` |
| Failures on first run | `taichi-failure-analyzer` | `taichi run` returns a non-zero exit code |
| Failed cases need fixing | `taichi-code-fixer` | After failure-analyzer produces root cause |
| Regression verification after fix | `taichi-regression-runner` | After code-fixer applies the fix |

Typical full pipeline:

```
config-generator → test-runner → failure-analyzer → code-fixer → regression-runner
```

## 9. Output Self-check List

After generating the config, the Agent should self-check item by item:

- [ ] The generated YAML has correct syntax and can be loaded by `taichi list -c <path>`
- [ ] `projects[].name` is unique and non-empty
- [ ] The environment referenced by `projects[].env` is defined in `envs`
- [ ] The `kind` field value in `envs` is one of the supported enum values
- [ ] The `health_path` of the backend environment has been identified or marked as pending confirmation
- [ ] In `skills`, each skill's `name` matches its `kind`
- [ ] Each case of the api skill contains at least `name`, `method`, `path`, `expected_status`
- [ ] Cases for routes with path parameters are annotated with `expected_status: 404`
- [ ] Cases of the regression skill are selected from api key paths
- [ ] `report.formats` includes at least `json`
- [ ] The `locale` field value is one of `auto` / `zh-CN` / `en-US`
- [ ] The `warnings` array lists all items requiring manual confirmation
- [ ] `next_steps` prompts the user to run `taichi run` for verification
