/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';

const monacoEsm = fileURLToPath(new URL('./node_modules/monaco-editor/esm/vs', import.meta.url));

export default defineConfig({
  plugins: [react()],
  // Relative asset URLs so the bundle works under wails://, file-less
  // embedding, and the browser `serve` mode alike.
  base: './',
  resolve: {
    alias: { '@monaco': monacoEsm },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 6000,
    target: 'es2022',
  },
  worker: { format: 'es' },
  server: {
    port: 5173,
    strictPort: true,
    // `npm run dev` against a running `codediver serve` (default port).
    proxy: {
      '/api': 'http://127.0.0.1:47474',
      '/mcp': 'http://127.0.0.1:47474',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['./src/test-setup.ts'],
    css: false,
  },
});
