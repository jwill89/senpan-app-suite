/// <reference types="vite/client" />
/// <reference types="vite-plugin-pwa/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const component: DefineComponent<Record<string, never>, Record<string, never>, any>
  export default component
}

// Side-effect CSS import shipped by vue3-emoji-picker via its package exports
// map (a bare specifier, not a `.css` path, so vite/client's glob doesn't cover it).
declare module 'vue3-emoji-picker/css'

// Frontend semver, injected from package.json at build time (see vite.config.ts
// `define` + vitest.config.ts). Read it via `lib/version.ts`, not directly.
declare const __APP_VERSION__: string
