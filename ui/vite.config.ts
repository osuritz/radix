import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { resolve } from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
  ],
  base: '/',
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Recharts + React bundle is ~580 KB raw / ~176 KB gzip — acceptable for a
    // localhost-only tool. Raise the warning limit to silence the noisy default.
    // Do NOT add code-splitting: this is a single-page dev tool, not a web app.
    chunkSizeWarningLimit: 800,
  },
  server: {
    proxy: {
      // Target must match metrics.port in radix.yml (default 9090).
      '/_metrics': {
        target: 'http://127.0.0.1:9090',
        changeOrigin: true,
      },
    },
  },
})
