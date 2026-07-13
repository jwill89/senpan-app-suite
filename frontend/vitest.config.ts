import { fileURLToPath, URL } from 'node:url'
import { readFileSync } from 'node:fs'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { changelogPlugin } from './config/changelog-plugin'

// Mirror vite.config.ts's __APP_VERSION__ define so code that reads the frontend
// version works (and can be asserted) under the test runner too.
const frontendVersion = JSON.parse(
  readFileSync(fileURLToPath(new URL('./package.json', import.meta.url)), 'utf-8'),
).version as string

// Vitest configuration, kept separate from `vite.config.ts` so the test runner
// doesn't pull in the production-only plugins (PWA service worker, bundle
// visualizer, dist-image stripping). Only the Vue SFC compiler + the `@` alias
// are needed to import and mount components/stores under test.
export default defineConfig({
  plugins: [vue(), changelogPlugin()],
  define: {
    __APP_VERSION__: JSON.stringify(frontendVersion),
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    // jsdom gives tests a DOM (localStorage, document, Image, fetch shimmed by
    // mocks) so stores and components behave like they do in the browser.
    environment: 'jsdom',
    // Test helpers (describe/it/expect/vi) are imported explicitly per file so
    // type-checking works without extra global type config.
    globals: false,
    // Component styles aren't needed for behavior tests; skipping keeps runs fast.
    css: false,
    // Auto-restore spies/mocks between tests so they don't leak across files.
    restoreMocks: true,
    clearMocks: true,
    // Global test setup (e.g. stubbing the globally-registered <font-awesome-icon>).
    setupFiles: ['./vitest.setup.ts'],
    include: ['src/**/*.test.ts'],
  },
})
