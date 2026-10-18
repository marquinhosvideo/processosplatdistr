/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// Em dev o Vite (:5173) repassa API, BFF e página pública para o server
// (:8080) sem trocar o Origin, para o CSRF ver a origem do APP_BASE_URL.
const backend = 'http://localhost:8080';
const proxied = ['/api/', '/auth/', '/s/', '/healthz', '/readyz'];

export default defineConfig({
  plugins: [react()],
  build: {
    // O binário Go embute este diretório (internal/web, go:embed all:dist).
    outDir: '../internal/web/dist',
    emptyOutDir: true,
    sourcemap: false,
    // Nada de data: em script, fonte ou CSS: a CSP do SPA só libera 'self'.
    assetsInlineLimit: 0,
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: Object.fromEntries(proxied.map((p) => [p, { target: backend, changeOrigin: false }])),
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}', 'lint/**/*.test.js'],
    restoreMocks: true,
    unstubGlobals: true,
    css: false,
  },
});
