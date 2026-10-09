// A build config for the render harness only: the app's own config chunks the
// vendor bundle for the browser, which does not apply when the output is a
// Node script.
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: {
    ssr: true,
    // The harness uses top-level await; the app's browser target predates it.
    target: 'esnext',
    // Inside the project so the external `vue` import resolves against the
    // project's own node_modules.
    outDir: '.ssr',
    emptyOutDir: true,
    rollupOptions: { output: { manualChunks: undefined } },
  },
})
