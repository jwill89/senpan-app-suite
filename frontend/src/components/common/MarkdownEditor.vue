<script setup lang="ts">
/**
 * WYSIWYG markdown editor (Toast UI Editor) exposed as a v-model component.
 *
 * The bound value is **markdown** — the editor starts in WYSIWYG mode (with a
 * Markdown toggle) and emits markdown via getMarkdown(). Markdown is what we
 * store and what Discord renders natively when a reading list is published.
 *
 * The library + its CSS are dynamically imported on mount so the (sizeable)
 * editor bundle stays out of the initial load — it's only fetched when an admin
 * actually opens a view that uses the editor. A dark/light theme is chosen from
 * the current page background so it blends with the active app theme.
 */
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type ToastEditor from '@toast-ui/editor'

const props = withDefaults(
  defineProps<{ modelValue: string; minHeight?: string; placeholder?: string }>(),
  { minHeight: '180px', placeholder: '' },
)
const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'blur'): void
}>()

const el = ref<HTMLDivElement | null>(null)
let editor: ToastEditor | null = null
let disposed = false

/** Rough luminance check of the page background to pick the editor theme. */
function bgIsDark(): boolean {
  const bg = getComputedStyle(document.body).backgroundColor
  const m = bg.match(/\d+(\.\d+)?/g)
  if (!m || m.length < 3) return true
  const [r, g, b] = m.map(Number)
  return 0.299 * r + 0.587 * g + 0.114 * b < 140
}

onMounted(async () => {
  const [{ default: Editor }] = await Promise.all([
    import('@toast-ui/editor'),
    import('@toast-ui/editor/dist/toastui-editor.css'),
    import('@toast-ui/editor/dist/theme/toastui-editor-dark.css'),
  ])
  if (disposed || !el.value) return
  // The dark theme CSS only applies under a `.toastui-editor-dark` ancestor.
  if (bgIsDark()) el.value.classList.add('toastui-editor-dark')

  editor = new Editor({
    el: el.value,
    initialValue: props.modelValue || '',
    initialEditType: 'wysiwyg',
    previewStyle: 'vertical',
    height: 'auto',
    minHeight: props.minHeight,
    placeholder: props.placeholder,
    autofocus: false,
    usageStatistics: false,
    toolbarItems: [
      ['heading', 'bold', 'italic', 'strike'],
      ['hr', 'quote'],
      ['ul', 'ol'],
      ['link'],
    ],
  })
  editor.on('change', () => {
    if (editor) emit('update:modelValue', editor.getMarkdown())
  })
  // Surface blur so callers can persist-on-blur (e.g. autosave) like a textarea.
  editor.on('blur', () => emit('blur'))
})

onBeforeUnmount(() => {
  disposed = true
  editor?.destroy()
  editor = null
})

// External updates (e.g. an AniList fill or a form reset) sync into the editor,
// but only when the value actually differs so we don't fight the user's cursor.
watch(
  () => props.modelValue,
  (v) => {
    if (editor && v !== editor.getMarkdown()) editor.setMarkdown(v || '', false)
  },
)
</script>

<template>
  <div ref="el" class="md-editor"></div>
</template>

<style scoped>
/*
 * Skin Toast UI Editor with the app's theme variables so it follows the active
 * theme (including custom themes) rather than only its bundled light/dark CSS.
 * The base light/dark stylesheet is still loaded (for the toolbar icon sprites
 * and markdown-mode syntax colors); these overrides re-map the surfaces, text,
 * borders, and accent to --surface / --text / --primary / etc.
 */
.md-editor :deep(.toastui-editor-defaultUI) {
  border: 1px solid var(--surface2);
  border-radius: var(--radius);
}

/* Toolbar */
.md-editor :deep(.toastui-editor-defaultUI-toolbar) {
  background: var(--surface);
  border-bottom: 1px solid var(--surface2);
}
.md-editor :deep(.toastui-editor-toolbar-group) {
  border-color: var(--surface2);
}

/* Editing surfaces + text */
.md-editor :deep(.toastui-editor-ww-container),
.md-editor :deep(.toastui-editor-md-container),
.md-editor :deep(.toastui-editor-main .toastui-editor-md-preview) {
  background: var(--surface);
}
.md-editor :deep(.toastui-editor-contents),
.md-editor :deep(.toastui-editor-contents p),
.md-editor :deep(.toastui-editor-contents h1),
.md-editor :deep(.toastui-editor-contents h2),
.md-editor :deep(.toastui-editor-contents h3),
.md-editor :deep(.toastui-editor-contents li),
.md-editor :deep(.ProseMirror) {
  color: var(--text);
}
.md-editor :deep(.toastui-editor-contents a) {
  color: var(--primary);
}
.md-editor :deep(.ProseMirror .placeholder),
.md-editor :deep(.toastui-editor-md-container .placeholder) {
  color: var(--text-dim);
}

/* Bottom mode switch (Markdown / WYSIWYG) */
.md-editor :deep(.toastui-editor-mode-switch) {
  background: var(--surface);
  border-top: 1px solid var(--surface2);
}
.md-editor :deep(.toastui-editor-mode-switch .tab-item) {
  color: var(--text-dim);
  background: var(--surface);
  border-color: var(--surface2);
}
.md-editor :deep(.toastui-editor-mode-switch .tab-item.active) {
  color: var(--text);
  border-bottom-color: var(--surface);
}
</style>
