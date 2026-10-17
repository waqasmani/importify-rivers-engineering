import { fileURLToPath, URL } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

// During development the dashboard runs on :5173 and proxies API calls to the Go server so the
// session cookie stays first-party. In production the built files are served by Caddy (or by the
// API itself with REA_WEB_DIR) on the same origin as the API.
const apiTarget = process.env.REA_API_URL ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: {
    port: 5173,
    proxy: { '/api': { target: apiTarget, changeOrigin: false }, '/healthz': apiTarget },
  },
  build: {
    target: 'es2022',
    sourcemap: false,
    chunkSizeWarningLimit: 1500,
  },
  test: {
    environment: 'happy-dom',
    include: ['tests/unit/**/*.spec.ts'],
    setupFiles: ['tests/unit/setup.ts'],
    globals: false,
  },
})
