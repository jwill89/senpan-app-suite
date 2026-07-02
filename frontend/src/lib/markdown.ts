/**
 * Markdown rendering via markdown-it (replaces the CDN `marked.js`).
 *
 * markdown-it (~100 KB) is loaded LAZILY via dynamic import the first time any
 * markdown is rendered, so it stays out of the initial bundle and only its
 * consumers (raffle detail, game details) pull it in. Until the parser chunk
 * has loaded, `renderMarkdown` returns '' and re-renders automatically once
 * ready (it reads the reactive `ready` flag, so the v-html updates).
 *
 * Configured to match the previous behavior: `breaks: true` so single newlines
 * become <br>; raw HTML disabled for safety; linkify on.
 */
import { ref } from 'vue'

type MarkdownRenderer = { render: (src: string) => string }

let md: MarkdownRenderer | null = null
let loadPromise: Promise<void> | null = null
const ready = ref(false)

/** Kicks off the one-time dynamic import of markdown-it (idempotent). */
function ensureLoaded(): Promise<void> {
  if (loadPromise) return loadPromise
  loadPromise = import('markdown-it').then(({ default: MarkdownIt }) => {
    md = new MarkdownIt({
      html: false, // do not allow raw HTML (matches marked default safety posture)
      breaks: true, // convert \n to <br>
      linkify: true,
    })
    ready.value = true
  })
  return loadPromise
}

/**
 * Reactive markdown renderer. Use in <script setup>:
 *   const { render: renderMarkdown } = useMarkdown()
 * then bind `v-html="renderMarkdown(text)"`. Triggers the lazy load on first
 * use and re-renders when the parser becomes available.
 */
export function useMarkdown() {
  void ensureLoaded()
  function render(text: string | null | undefined): string {
    // Touch `ready` so the rendering effect re-runs once the parser loads.
    if (!ready.value || !md) return ''
    if (!text) return ''
    return md.render(text)
  }
  return { render, ready }
}
