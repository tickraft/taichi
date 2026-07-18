"""Deliberately broken FastAPI service for taichi integration testing.

Intentional defects for taichi skills to detect:

- ``GET /health``           returns ``data.status = "down"`` (expected ``"up"``)
- ``GET /api/v1/items``     returns HTTP 500 (expected 200)
- ``GET /api/v1/orders/{id}`` returns a wrong total for id=1 (expected 100.00, returns 50.00)
- ``GET /``                  serves HTML missing the expected ``<div id="app">`` marker
- ``/favicon.ico``           is not served (static asset failure)

Correct endpoints (for contrast):

- ``GET /api/v1/version``   returns ``data.version = "0.1.0"``
- ``GET /api/v1/status``     returns ``data.region = "us-east-1"``

Run with::

    pip install -r requirements.txt
    uvicorn app.main:app --host 127.0.0.1 --port 8000
"""

from fastapi import FastAPI
from fastapi.responses import HTMLResponse, JSONResponse

app = FastAPI(title="Buggy FastAPI", version="0.1.0")


@app.get("/health")
def health() -> JSONResponse:
    """Health check endpoint.

    BUG: returns ``status: down`` instead of ``status: up``.
    """
    return JSONResponse(
        status_code=200,
        content={
            "code": 0,
            "msg": "ok",
            "request_id": "buggy-fastapi-health-001",
            "data": {"status": "down"},  # BUG: should be "up"
        },
    )


@app.get("/api/v1/items")
def list_items() -> JSONResponse:
    """Items listing endpoint.

    BUG: returns HTTP 500 with a server error.
    """
    return JSONResponse(
        status_code=500,
        content={
            "code": 500,
            "msg": "internal error: database unreachable",
            "request_id": "buggy-fastapi-items-002",
        },
    )


@app.get("/api/v1/orders/{order_id}")
def get_order(order_id: int) -> JSONResponse:
    """Order detail endpoint.

    BUG: order id=1 returns total=50.00 instead of 100.00.
    Order id=2 returns the correct total for contrast.
    """
    orders = {
        1: {"id": 1, "total": 50.00, "currency": "USD"},  # BUG: should be 100.00
        2: {"id": 2, "total": 75.00, "currency": "USD"},
    }
    order = orders.get(order_id)
    if order is None:
        return JSONResponse(
            status_code=404,
            content={
                "code": 1004,
                "msg": f"order {order_id} not found",
                "request_id": "buggy-fastapi-order-404",
            },
        )
    return JSONResponse(
        status_code=200,
        content={
            "code": 0,
            "msg": "ok",
            "request_id": f"buggy-fastapi-order-{order_id}",
            "data": order,
        },
    )


@app.get("/api/v1/version")
def version() -> JSONResponse:
    """Version endpoint (correct, for contrast)."""
    return JSONResponse(
        status_code=200,
        content={
            "code": 0,
            "msg": "ok",
            "request_id": "buggy-fastapi-version-003",
            "data": {"version": "0.1.0", "region": "us-east-1"},
        },
    )


@app.get("/api/v1/status")
def status() -> JSONResponse:
    """Status endpoint (correct, for contrast)."""
    return JSONResponse(
        status_code=200,
        content={
            "code": 0,
            "msg": "ok",
            "request_id": "buggy-fastapi-status-004",
            "data": {"region": "us-east-1", "ready": True},
        },
    )


@app.get("/", response_class=HTMLResponse)
def index() -> str:
    """Homepage.

    BUG: the HTML body is missing the ``<div id="app">`` marker
    that the UI skill checks for.
    """
    return "<!DOCTYPE html><html><head><title>Buggy FastAPI</title></head><body><h1>Hello</h1></body></html>"
