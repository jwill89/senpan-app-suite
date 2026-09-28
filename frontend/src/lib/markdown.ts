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

// Two configured instances share one lazy import: `md` converts single newlines
// to <br> (breaks: true - for user-entered game details/raffle copy where a line
// break is intended), while `mdFlow` follows standard markdown (soft newlines are
// spaces) - right for prose that's soft-wrapped in source, like CHANGELOG.md.
let md: MarkdownRenderer | null = null
let mdFlow: MarkdownRenderer | null = null
let loadPromise: Promise<void> | null = null
const ready = ref(false)

/** Kicks off the one-time dynamic import of markdown-it (idempotent). */
function ensureLoaded(): Promise<void> {
  if (loadPromise) return loadPromise
  loadPromise = import('markdown-it').then(({ default: MarkdownIt }) => {
    const inline = new MarkdownIt({ html: false, breaks: true, linkify: true })
    const flow = new MarkdownIt({ html: false, breaks: false, linkify: true })
    // markdown-it 15 ships linkify-it 6, which flipped `fuzzyLink` to false by
    // default - so a bare "www.senpan.cafe" stopped becoming a link while an
    // explicit https:// one still worked. Authors write both, and the rendering
    // an admin already knows shouldn't change under them because a transitive
    // default moved, so keep the pre-15 behavior explicitly.
    inline.linkify.set({ fuzzyLink: true })
    flow.linkify.set({ fuzzyLink: true })
    md = inline
    mdFlow = flow
    ready.value = true
  })
  return loadPromise
}

/**
 * Matches a fenced block or an inline code span FIRST, then a <br> tag. Ordering
 * is the point: the alternation consumes code before the tag can match inside it,
 * so a <br> being shown as an example in a code sample stays literal.
 */
const CODE_SPAN_OR_BREAK_TAG = /(```[\s\S]*?```|~~~[\s\S]*?~~~|`[^`\n]*`)|<br\s*\/?>/gi

/**
 * Turns literal <br> tags in the SOURCE into real newlines.
 *
 * Raw HTML is disabled (html: false) so the parser escapes anything tag-shaped,
 * which is what we want for safety - but it meant a description holding a literal
 * "<br />" rendered those five characters to the reader instead of breaking the
 * line. Text arrives that way from more than one direction: pasted from somewhere
 * that emitted HTML, or serialized by a WYSIWYG editor that writes hard breaks as
 * tags.
 *
 * Rewriting them to newlines rather than allowing raw HTML keeps the escape intact
 * for every OTHER tag: this promotes one inert, unambiguous tag to the line break
 * it was always meant to be, and nothing else. `breaks: true` then renders the
 * newline as a <br> the parser itself produced.
 */
export function normalizeHardBreaks(text: string): string {
  return text.replace(CODE_SPAN_OR_BREAK_TAG, (_match, code: string | undefined) =>
    code === undefined ? '\n' : code,
  )
}

/** Shared reactive renderer factory over one of the configured instances. */
function useRenderer(get: () => MarkdownRenderer | null) {
  void ensureLoaded()
  function render(text: string | null | undefined): string {
    // Touch `ready` so the rendering effect re-runs once the parser loads.
    if (!ready.value || !text) return ''
    return get()?.render(normalizeHardBreaks(text)) ?? ''
  }
  return { render, ready }
}

/**
 * Reactive markdown renderer (single newline -> <br>). Use in <script setup>:
 *   const { render: renderMarkdown } = useMarkdown()
 * then bind `v-html="renderMarkdown(text)"`. Triggers the lazy load on first
 * use and re-renders when the parser becomes available.
 */
export function useMarkdown() {
  return useRenderer(() => md)
}

/**
 * Reactive markdown renderer with standard soft-break handling (single newlines
 * join into flowing paragraphs). For rendering prose that is soft-wrapped in its
 * source, such as CHANGELOG.md.
 */
export function useMarkdownFlow() {
  return useRenderer(() => mdFlow)
}
