import { defineConfig } from 'vitest/config'
import { resolve } from 'path'

/**
 * Vitest configuration — kept separate from vite.config.ts so the FOUC-blocker
 * plugin (which reads the filesystem at import time) is never executed in the
 * test runner, and the test tsconfig can differ from the app tsconfig.
 */
export default defineConfig({
  test: {
    environment: 'happy-dom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
  },
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
})
