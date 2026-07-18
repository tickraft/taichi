# Taichi 示例工程

> 🌐 语言: [English](README.md) | [中文](README.zh.md)

一组**故意有问题**的工程，用于 Taichi 集成测试。每个工程都包含有意植入的缺陷，
供 Taichi skill 检测，覆盖全部配置用例（所有 env 类型与 skill 种类）。

## 工程总览

| 工程 | 语言 | Env 类型 | 覆盖 Skill | 故意植入的缺陷 |
|------|------|---------|-----------|---------------|
| `go-buggy-backend` | Go | `backend.go` | api, grpc, ui, static, regression, plugin | health 状态错误、/users 500、gRPC NOT_SERVING、gRPC 服务缺失、gRPC 价格错误、HTML 缺标记 |
| `python-fastapi-buggy` | Python | `custom` | api, ui, static, regression | health 值错误、/items 500、订单金额错误、HTML 缺标记 |
| `node-express-buggy` | JavaScript | `custom` | api, ui, static, regression | /health 500、/orders 500、购物车金额错误、HTML 缺标记 |
| `frontend-vite-buggy` | JavaScript | `frontend.vite` | ui, static | 缺少 `<div id="app">` 标记 |
| `external-buggy` | Python | `custom` + `base_url` | api, ui, static, regression | health 状态错误、/users 500、metrics code 错误、HTML 缺标记 |

## 覆盖矩阵

### 环境类型

| Env 类型 | 示例工程 | 说明 |
|---------|---------|------|
| `backend.go` | go-buggy-backend | 自动 `go build`、自动分配端口、health 路径检查 |
| `custom`（uvicorn） | python-fastapi-buggy | 进程式启动、ready_url 健康检查 |
| `custom`（node） | node-express-buggy | 进程式启动、ready_url 健康检查 |
| `frontend.vite` | frontend-vite-buggy | Vite 开发服务器、ready_url 轮询 |
| `custom` + `base_url` | external-buggy | 外部托管、不启停进程 |

### Skill 类型

| Skill 类型 | 示例工程 | 测试用例 |
|-----------|---------|---------|
| `api` | go-buggy-backend, python-fastapi-buggy, node-express-buggy, external-buggy | 状态码 / code / 字段 / 延迟断言 |
| `grpc` | go-buggy-backend | health / dial / reflect 用例（见 [proto/product.proto](go-buggy-backend/proto/product.proto)） |
| `ui` | 全部工程 | HTML 包含检查 + 延迟 |
| `static` | 全部工程 | pages + assets、SPA 回退 |
| `regression` | go-buggy-backend, python-fastapi-buggy, node-express-buggy, external-buggy | 关键路径复测、skip_on_404 |
| `plugin` | go-buggy-backend | Go 插件（[cmd/grpc-check](go-buggy-backend/cmd/grpc-check)）调用 gRPC 业务方法 |

## 快速开始

### 运行全部工程（主配置）

```bash
# 在 taichi 根目录下执行：
taichi run -c examples/taichi.yaml
```

### 运行单个工程

```bash
cd examples/go-buggy-backend && make build && taichi run -c taichi.yaml
cd examples/python-fastapi-buggy && pip install -r requirements.txt && taichi run -c taichi.yaml
cd examples/node-express-buggy && npm install && taichi run -c taichi.yaml
cd examples/frontend-vite-buggy && npm install && taichi run -c taichi.yaml
```

### 外部工程（需手动启动）

```bash
# 终端 1：启动外部服务
cd examples/external-buggy && ./start.sh

# 终端 2：运行 Taichi 测试
cd examples/external-buggy && taichi run -c taichi.yaml
```

### 按 skill 过滤

```bash
taichi run -c examples/go-buggy-backend/taichi.yaml -s api,grpc
```

## 故意植入的缺陷一览

### go-buggy-backend（Go + gRPC）
- `GET /api/v1/health` → `data.status = "unhealthy"`（期望：`healthy`）
- `GET /api/v1/users` → HTTP 500（期望：200）
- `GET /` → 缺少 `<div id="app">` 标记
- gRPC `Health/Check` → `NOT_SERVING`（期望：`SERVING`）
- gRPC `product.InventoryService` → proto 中声明但未注册（反射检测服务缺失）
- gRPC `ProductService.GetProduct(1)` → 返回 price=9.99（期望：99.99）
- `/favicon.ico` → 未提供

### python-fastapi-buggy（Python）
- `GET /health` → `data.status = "down"`（期望：`up`）
- `GET /api/v1/items` → HTTP 500（期望：200）
- `GET /api/v1/orders/1` → `data.total = 50.00`（期望：`100.00`）
- `GET /` → 缺少 `<div id="app">` 标记

### node-express-buggy（JavaScript）
- `GET /health` → HTTP 500（期望：200）
- `GET /api/v1/orders` → HTTP 500（期望：200）
- `GET /api/v1/cart/1` → `data.total = 30.00`（期望：`60.00`）
- `GET /` → 缺少 `<div id="app">` 标记

### frontend-vite-buggy（JavaScript）
- `GET /` → 缺少 `<div id="app">` 标记

### external-buggy（Python）
- `GET /api/v1/health` → `data.status = "degraded"`（期望：`healthy`）
- `GET /api/v1/users` → HTTP 500（期望：200）
- `GET /api/v1/metrics` → `code = 500`（期望：`0`）
- `GET /` → 缺少 `<div id="app">` 标记

## 已验证的 BUG 检测结果

全部 5 个工程均已通过 Taichi 实跑，测试报告确认 BUG 被正确识别：

| 工程 | 测试总数 | 通过 | 失败 | 检测到的 BUG |
|------|---------|------|------|-------------|
| go-buggy-backend | 20 | 13 | 6 | health、users、gRPC health、gRPC reflect、gRPC price、ui |
| python-fastapi-buggy | 15 | 11 | 4 | health、items、order total、ui |
| node-express-buggy | 16 | 10 | 5 | health、orders、cart total、ui、regression health |
| frontend-vite-buggy | 6 | 5 | 1 | ui 标记 |
| external-buggy | 13 | 9 | 4 | health、users、metrics code、ui |

## 覆盖的编程语言

1. **Go** — `go-buggy-backend`（net/http + gRPC，含 .proto 文件）
2. **Python** — `python-fastapi-buggy`（FastAPI）+ `external-buggy`（标准库 http.server）
3. **JavaScript** — `node-express-buggy`（Express）+ `frontend-vite-buggy`（Vite）
