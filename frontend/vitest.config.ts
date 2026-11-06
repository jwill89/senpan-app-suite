import { fileURLToPath, URL } from 'node:url'
import { readFileSync } from 'node:fs'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { changelogPlugin } from './config/changelog-plugin.ts'
import { vitestDefaults } from '../shared/test/web/vitest.js'

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
    // The shared test settings (shared-tooling's test/web): jsdom, mocks and stubbed
    // globals put back after each test, and v8 coverage over all of src.
    // `npm run test:coverage` fails below the floor: a little under what the tests
    // reached when it was set (2026-11-06: 35.9% of statements, 25.8% of branches,
    // 28.5% of functions, 35.9% of lines). Raise it as tests land; never lower it.
    ...vitestDefaults({
      environment: 'jsdom',
      include: ['src/**/*.test.ts'],
      // Stubs the globally registered <font-awesome-icon> for every mount.
      setupFiles: ['./vitest.setup.ts'],
      floor: { statements: 35.5, branches: 25.5, functions: 28, lines: 35.5 },
    }),
    // Test helpers (describe/it/expect/vi) are imported explicitly per file so
    // type-checking works without extra global type config.
    globals: false,
    // Component styles aren't needed for behavior tests; skipping keeps runs fast.
    css: false,
    // restoreMocks puts spies back but keeps a vi.fn()'s recorded calls; the
    // assertions on call counts expect each test to start from none.
    clearMocks: true,
  },
})
