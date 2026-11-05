import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  // Relative paths so the hub also works behind a reverse-proxy subpath.
  base: './',
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: { '/api': 'http://127.0.0.1:8420' },
  },
  test: {
    include: ['src/**/*.test.ts'],
  },
})
