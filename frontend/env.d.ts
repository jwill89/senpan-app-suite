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

// Parsed CHANGELOG.md, provided at build/dev time by config/changelog-plugin.ts
// (registered in vite.config.ts + vitest.config.ts). Read it via lib/changelog.ts.
declare module 'virtual:changelog' {
  /** One labelled change-group (Added / Fixed / …); `body` is its bullet markdown. */
  export interface ChangelogGroup {
    label: string
    body: string
  }
  /** One released version: its number, date, intro paragraph, and change-groups. */
  export interface ChangelogEntry {
    version: string
    date: string
    intro: string
    groups: ChangelogGroup[]
  }
  export interface ChangelogSection {
    /** The section's newest version, e.g. "3.7.0" (its first `### [version]`). */
    latest: string
    /** Version entries, newest first. */
    entries: ChangelogEntry[]
  }
  export const changelog: {
    frontend: ChangelogSection
    backend: ChangelogSection
    plugin: ChangelogSection
  }
}

// Frontend semver, injected from package.json at build time (see vite.config.ts
// `define` + vitest.config.ts). Read it via `lib/version.ts`, not directly.
declare const __APP_VERSION__: string

// Cloudflare Turnstile (loaded at runtime from challenges.cloudflare.com — see
// components/common/TurnstileWidget.vue). Minimal surface of the global API.
interface TurnstileRenderOptions {
  sitekey: string
  callback?: (token: string) => void
  'expired-callback'?: () => void
  'error-callback'?: () => void
  theme?: 'auto' | 'light' | 'dark'
}
interface TurnstileApi {
  render: (el: HTMLElement, opts: TurnstileRenderOptions) => string
  reset: (widgetId?: string) => void
  remove: (widgetId?: string) => void
}
interface Window {
  turnstile?: TurnstileApi
}
