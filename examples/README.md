# Taichi Examples

> 🌐 Languages: [English](README.md) | [中文](README.zh.md)

A collection of deliberately **buggy** projects for Taichi integration testing.
Each project contains intentional defects that Taichi skills are expected to detect,
exercising the full configuration surface (all env types and skill kinds).

## Project Overview

| Project | Language | Env Kind | Skills Covered | Intentional Bugs |
|---------|----------|----------|-----------------|------------------|
| `go-buggy-backend` | Go | `backend.go` | api, grpc, ui, static, regression, plugin | wrong health status, 500 on /users, gRPC NOT_SERVING, gRPC missing service, gRPC wrong price, missing HTML marker |
| `python-fastapi-buggy` | Python | `custom` | api, ui, static, regression | wrong health value, 500 on /items, wrong order total, missing HTML marker |
| `node-express-buggy` | JavaScript | `custom` | api, ui, static, regression | 500 on /health, 500 on /orders, wrong cart total, missing HTML marker |
| `frontend-vite-buggy` | JavaScript | `frontend.vite` | ui, static | missing `<div id="app">` marker |
| `external-buggy` | Python | `custom` + `base_url` | api, ui, static, regression | wrong health status, 500 on /users, wrong metrics code, missing HTML marker |

## Coverage Matrix

### Environment Types

| Env Kind | Example Project | Notes |
|---------|----------------|-------|
| `backend.go` | go-buggy-backend | Auto `go build`, auto free port, health path |
| `custom` (uvicorn) | python-fastapi-buggy | Process-based, ready_url health check |
| `custom` (node) | node-express-buggy | Process-based, ready_url health check |
| `frontend.vite` | frontend-vite-buggy | Vite dev server, ready_url polling |
| `custom` + `base_url` | external-buggy | External managed, no startup/stop |

### Skill Types

| Skill Kind | Example Projects | Cases |
|-----------|-----------------|-------|
| `api` | go-buggy-backend, python-fastapi-buggy, node-express-buggy, external-buggy | status / code / field / latency assertions |
| `grpc` | go-buggy-backend | health / dial / reflect cases (see [proto/product.proto](go-buggy-backend/proto/product.proto)) |
| `ui` | all projects | HTML contains + latency |
| `static` | all projects | pages + assets, SPA fallback |
| `regression` | go-buggy-backend, python-fastapi-buggy, node-express-buggy, external-buggy | critical path re-probe, skip_on_404 |
| `plugin` | go-buggy-backend | Go plugin ([cmd/grpc-check](go-buggy-backend/cmd/grpc-check)) calls gRPC RPCs |

## Quick Start

### Run all projects (master config)

```bash
# From the taichi root directory:
taichi run -c examples/taichi.yaml
```

### Run a single project

```bash
cd examples/go-buggy-backend && make build && taichi run -c taichi.yaml
cd examples/python-fastapi-buggy && pip install -r requirements.txt && taichi run -c taichi.yaml
cd examples/node-express-buggy && npm install && taichi run -c taichi.yaml
cd examples/frontend-vite-buggy && npm install && taichi run -c taichi.yaml
```

### External project (manual start required)

```bash
# Terminal 1: start the external service
cd examples/external-buggy && ./start.sh

# Terminal 2: run taichi against it
cd examples/external-buggy && taichi run -c taichi.yaml
```

### Filter by skill

```bash
taichi run -c examples/go-buggy-backend/taichi.yaml -s api,grpc
```

## Intentional Bugs Reference

### go-buggy-backend (Go + gRPC)
- `GET /api/v1/health` → `data.status = "unhealthy"` (expected: `healthy`)
- `GET /api/v1/users` → HTTP 500 (expected: 200)
- `GET /` → missing `<div id="app">` marker
- gRPC `Health/Check` → `NOT_SERVING` (expected: `SERVING`)
- gRPC `product.InventoryService` → declared in proto but NOT registered (reflection detects missing service)
- gRPC `ProductService.GetProduct(1)` → returns price=9.99 (expected: 99.99)
- `/favicon.ico` → not served

### python-fastapi-buggy (Python)
- `GET /health` → `data.status = "down"` (expected: `up`)
- `GET /api/v1/items` → HTTP 500 (expected: 200)
- `GET /api/v1/orders/1` → `data.total = 50.00` (expected: `100.00`)
- `GET /` → missing `<div id="app">` marker

### node-express-buggy (JavaScript)
- `GET /health` → HTTP 500 (expected: 200)
- `GET /api/v1/orders` → HTTP 500 (expected: 200)
- `GET /api/v1/cart/1` → `data.total = 30.00` (expected: `60.00`)
- `GET /` → missing `<div id="app">` marker

### frontend-vite-buggy (JavaScript)
- `GET /` → missing `<div id="app">` marker

### external-buggy (Python)
- `GET /api/v1/health` → `data.status = "degraded"` (expected: `healthy`)
- `GET /api/v1/users` → HTTP 500 (expected: 200)
- `GET /api/v1/metrics` → `code = 500` (expected: `0`)
- `GET /` → missing `<div id="app">` marker

## Verified Bug Detection Results

All 5 projects were run with Taichi and the bugs were confirmed detected in the test reports:

| Project | Total Tests | Passed | Failed | Bugs Detected |
|---------|------------|--------|--------|---------------|
| go-buggy-backend | 20 | 13 | 6 | health, users, gRPC health, gRPC reflect, gRPC price, ui |
| python-fastapi-buggy | 15 | 11 | 4 | health, items, order total, ui |
| node-express-buggy | 16 | 10 | 5 | health, orders, cart total, ui, regression health |
| frontend-vite-buggy | 6 | 5 | 1 | ui marker |
| external-buggy | 13 | 9 | 4 | health, users, metrics code, ui |

## Languages Covered

1. **Go** — `go-buggy-backend` (net/http + gRPC with .proto)
2. **Python** — `python-fastapi-buggy` (FastAPI) + `external-buggy` (stdlib http.server)
3. **JavaScript** — `node-express-buggy` (Express) + `frontend-vite-buggy` (Vite)
