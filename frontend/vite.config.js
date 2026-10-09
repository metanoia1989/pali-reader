import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// In development the SPA talks to the Go API on :8090 through this proxy, so
// the browser sees one origin and cookies and paths behave as they do in
// production. In production nginx does the same job.
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: process.env.API_TARGET || 'http://127.0.0.1:8090',
        changeOrigin: true,
      },
    },
  },
  build: {
    target: 'es2020',
    sourcemap: false,
    chunkSizeWarningLimit: 900,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['vue', 'vue-router', 'pinia'],
        },
      },
    },
  },
})
