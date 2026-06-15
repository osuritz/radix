import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { resolve } from 'path'
import { getColorSchemeFoucScript } from './src/hooks/color-scheme/fouc-blocker'

// FOUC-blocker script generated from react-kit fouc-blocker.ts.
// Injected synchronously into <head> before any stylesheet or paint so the
// data-theme attribute is set before first render.
const foucScript = getColorSchemeFoucScript({
  storageKey: 'radix-metrics-theme',
  strategy: 'data-attribute',
})

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    {
      name: 'inject-fouc-blocker',
      transformIndexHtml(html) {
        return html.replace(
          /<!-- ?synchronous theme initialisation[\s\S]*?<\/script>/i,
          `<script>${foucScript}</script>`
        )
      },
    },
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
