// region: config
/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // The dev server passes these paths on to the api. The browser sees
    // one origin, http://localhost:5173, which means no CORS is involved.
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/healthz': 'http://127.0.0.1:8080',
    },
  },
  test: {
    // jsdom gives each test file a browser document to render into.
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
    // Only unit and component tests. Playwright runs e2e/ itself.
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
// endregion: config
