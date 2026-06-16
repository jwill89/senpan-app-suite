<script setup lang="ts">
/**
 * WYSIWYG markdown editor (Milkdown "Crepe") exposed as a v-model component.
 *
 * The bound value is **markdown** — that's what we store and what Discord
 * renders natively when a reading list is published. So the editor is limited
 * to a Discord-safe subset: inline formatting + headings, quotes, lists,
 * dividers and links. Block types Discord can't render (tables, images, LaTeX,
 * fenced-code editors, task lists) are left out so an author can't silently
 * produce markdown that breaks once published.
 *
 * Formatting is reached the modern WYSIWYG way: a floating selection toolbar
 * (bold/italic/strike/link) plus a `/` slash menu and markdown input rules
 * (`# `, `> `, `- `) for block structure.
 *
 * Built via Crepe's tree-shakable `CrepeBuilder` rather than the all-in-one
 * `Crepe` class: we import only the features we use, so the code-mirror
 * (≈1.2 MB of language parsers) and LaTeX/KaTeX features are dropped from the
 * bundle entirely. The base commonmark/gfm editing comes from the builder.
 *
 * The library + its CSS are dynamically imported on mount so this (still
 * sizeable) editor stays out of the initial load — only fetched when an admin
 * opens a view that uses it. Colors are mapped to the app theme variables so
 * the editor follows the active theme (including custom themes).
 */
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { CrepeBuilder as CrepeBuilderType } from '@milkdown/crepe/builder'

const props = withDefaults(
  defineProps<{ modelValue: string; minHeight?: string; placeholder?: string }>(),
  { minHeight: '180px', placeholder: '' },
)
const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'blur'): void
}>()

const el = ref<HTMLDivElement | null>(null)
let crepe: CrepeBuilderType | null = null
/** Push an external markdown value into the editor (set after create()). */
let applyExternal: ((markdown: string) => void) | null = null
let disposed = false
// Gate emits until the initial document has loaded, so we don't echo the
// starting value back to the parent (which would mark a pristine form dirty).
let ready = false

onMounted(async () => {
  const [
    { CrepeBuilder },
    { toolbar },
    { blockEdit },
    { listItem },
    { linkTooltip },
    { placeholder },
    { cursor },
    { replaceAll },
  ] = await Promise.all([
    import('@milkdown/crepe/builder'),
    import('@milkdown/crepe/feature/toolbar'),
    import('@milkdown/crepe/feature/block-edit'),
    import('@milkdown/crepe/feature/list-item'),
    import('@milkdown/crepe/feature/link-tooltip'),
    import('@milkdown/crepe/feature/placeholder'),
    import('@milkdown/crepe/feature/cursor'),
    import('@milkdown/kit/utils'),
    import('@milkdown/crepe/theme/common/style.css'),
    import('@milkdown/crepe/theme/classic.css'),
  ])
  if (disposed || !el.value) return

  const builder = new CrepeBuilder({
    root: el.value,
    defaultValue: props.modelValue || '',
  })
  builder
    .addFeature(cursor)
    .addFeature(listItem)
    .addFeature(linkTooltip)
    .addFeature(placeholder, { text: props.placeholder })
    .addFeature(toolbar)
    .addFeature(blockEdit, {
      // Discord-safe slash menu: keep headings/quote/divider and bullet/ordered
      // lists; drop task lists and the whole advanced group (image/code/table/
      // math) since Discord can't render them.
      listGroup: { taskList: null },
      advancedGroup: null,
    })

  builder.on((listener) => {
    listener.markdownUpdated((_ctx, markdown) => {
      if (ready) emit('update:modelValue', markdown)
    })
    listener.blur(() => emit('blur'))
  })

  await builder.create()
  if (disposed) {
    void builder.destroy()
    return
  }
  crepe = builder
  applyExternal = (markdown) => {
    builder.editor.action(replaceAll(markdown))
  }
  ready = true
})

onBeforeUnmount(() => {
  disposed = true
  void crepe?.destroy()
  crepe = null
  applyExternal = null
})

// External updates (e.g. an AniList fill or a form reset) sync into the editor,
// but only when the value actually differs so we don't fight the user's cursor.
watch(
  () => props.modelValue,
  (v) => {
    if (crepe && applyExternal && v !== crepe.getMarkdown()) applyExternal(v || '')
  },
)
</script>

<template>
  <div ref="el" class="md-editor" :style="{ '--md-min-height': minHeight }"></div>
</template>

<style scoped>
/*
 * Map Crepe's design tokens onto the app theme variables so the editor follows
 * the active theme (including custom themes). CSS variables inherit into the
 * `.milkdown` subtree, so setting them on the wrapper is enough; element-level
 * tweaks (min-height, frame) use :deep().
 */
.md-editor {
  --crepe-color-background: var(--panel-bg);
  --crepe-color-surface: var(--panel-bg);
  --crepe-color-surface-low: var(--panel-raised-bg);
  --crepe-color-on-background: var(--text);
  --crepe-color-on-surface: var(--text);
  --crepe-color-on-surface-variant: var(--text-muted);
  --crepe-color-primary: var(--accent);
  --crepe-color-secondary: var(--panel-raised-bg);
  --crepe-color-on-secondary: var(--text);
  --crepe-color-outline: var(--panel-raised-bg);
  --crepe-color-hover: var(--panel-raised-bg);
  --crepe-color-selected: var(--panel-raised-bg);
  --crepe-color-inline-area: var(--panel-raised-bg);
  --crepe-font-default: inherit;
  --crepe-font-title: inherit;

  border: 1px solid var(--panel-raised-bg);
  border-radius: var(--radius);
  background: var(--panel-bg);
}

.md-editor :deep(.milkdown) {
  background: transparent;
  border-radius: var(--radius);
}

.md-editor :deep(.milkdown .ProseMirror) {
  min-height: var(--md-min-height, 180px);
  padding: 0.6rem 0.85rem;
  outline: none;
}
</style>
