/**
 * Deliberately broken Express service for taichi integration testing.
 *
 * Intentional defects for taichi skills to detect:
 *   - GET /health          returns HTTP 500 (should be 200)
 *   - GET /api/v1/orders   returns HTTP 500 (should be 200)
 *   - GET /api/v1/cart/1   returns total=30.00 (expected 60.00)
 *   - GET /                serves HTML missing the expected <div id="app"> marker
 *   - /favicon.ico         is not served (static asset failure)
 *   - SPA fallback         returns 404 instead of index.html
 *
 * Correct endpoints (for contrast):
 *   - GET /api/v1/status   returns version 0.1.0
 *   - GET /api/v1/cart/2   returns total=45.00 (correct)
 *
 * Run with:
 *   npm install
 *   npm start
 */

const express = require('express');

const app = express();
const PORT = process.env.PORT || 3000;

// BUG: /health returns 500 instead of 200.
app.get('/health', (_req, res) => {
  res.status(500).json({
    code: 500,
    msg: 'database connection failed',
    request_id: 'buggy-express-health-001',
    data: { status: 'down' }, // BUG: should be "up"
  });
});

// BUG: /api/v1/orders returns 500 with wrong code.
app.get('/api/v1/orders', (_req, res) => {
  res.status(500).json({
    code: 500,
    msg: 'internal error',
    request_id: 'buggy-express-orders-002',
  });
});

// BUG: /api/v1/cart/1 returns total=30.00 instead of 60.00.
// /api/v1/cart/2 returns the correct total for contrast.
app.get('/api/v1/cart/:id', (req, res) => {
  const id = parseInt(req.params.id, 10);
  const carts = {
    1: { id: 1, total: 30.00, currency: 'USD' }, // BUG: should be 60.00
    2: { id: 2, total: 45.00, currency: 'USD' },
  };
  const cart = carts[id];
  if (!cart) {
    res.status(404).json({
      code: 1004,
      msg: `cart ${id} not found`,
      request_id: 'buggy-express-cart-404',
    });
    return;
  }
  res.status(200).json({
    code: 0,
    msg: 'ok',
    request_id: `buggy-express-cart-${id}`,
    data: cart,
  });
});

// Correct endpoint for contrast: returns a valid status report.
app.get('/api/v1/status', (_req, res) => {
  res.status(200).json({
    code: 0,
    msg: 'ok',
    request_id: 'buggy-express-status-003',
    data: { version: '0.1.0', region: 'us-east-1', ready: true },
  });
});

// Correct endpoint for contrast: returns a valid metrics summary.
app.get('/api/v1/metrics', (_req, res) => {
  res.status(200).json({
    code: 0,
    msg: 'ok',
    request_id: 'buggy-express-metrics-004',
    data: { requests: 1024, errors: 0, uptime: 3600 },
  });
});

// BUG: homepage HTML is missing the expected <div id="app"> marker.
app.get('/', (_req, res) => {
  res.type('html');
  res.send('<!DOCTYPE html><html><head><title>Buggy Express</title></head><body><h1>Hello</h1></body></html>');
});

// BUG: SPA fallback returns 404 instead of serving index.html.
// A correct SPA would catch-all and serve the same index.html.

// NOTE: /favicon.ico and /assets/* are intentionally not served (static asset failure).

app.listen(PORT, () => {
  console.log(`Buggy Express server listening on port ${PORT}`);
});
