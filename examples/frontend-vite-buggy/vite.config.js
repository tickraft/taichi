import { defineConfig } from 'vite';

// Minimal Vite config for the buggy frontend example.
// The dev server runs on port 5174 to avoid clashing with other examples.
export default defineConfig({
  server: {
    port: 5174,
    host: '127.0.0.1',
    strictPort: true,
  },
});
