<script setup lang="ts">
/**
 * WYSIWYG markdown editor (Milkdown "Crepe") exposed as a v-model component.
 *
 * The bound value is **markdown** - that's what we store and what Discord
 * renders natively when a reading list is published. So the editor is limited
 * to a Discord-safe subset: inline formatting + headings, quotes, lists,
 * dividers and links. Block types Discord can't render (tables, images, LaTeX,
 * fenced-code editors, task lists) are left out so an author can't silently
 * produce markdown that breaks once published.
 *
 * Formatting is reached from a persistent **top bar** (Crepe's `topBar` feature)
 * rather than the floating selection toolbar, so the controls are visible before
 * you select anything - plus a `/` slash menu and markdown input rules (`# `,
 * `> `, `- `) for block structure.
 *
 * The top bar ships with buttons for features this editor deliberately does NOT
 * enable (image, table, code block, math) and for task lists Discord can't render,
 * so `buildTopBar` prunes them - the same Discord-safe set the slash menu keeps.
 * The emptied "block" group is reused for an emoji button rather than left empty,
 * because the bar renders a divider per group whether or not the group has items.
 *
 * Built via Crepe's tree-shakable `CrepeBuilder` rather than the all-in-one
 * `Crepe` class: we import only the features we use, so the code-mirror
 * (~1.2 MB of language parsers) and LaTeX/KaTeX features are dropped from the
 * bundle entirely. The base commonmark/gfm editing comes from the builder.
 *
 * The library + its CSS are dynamically imported on mount so this (still
 * sizeable) editor stays out of the initial load - only fetched when an admin
 * opens a view that uses it. Colors are mapped to the app theme variables so
 * the editor follows the active theme (including custom themes).
 */
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { icon as faIcon } from '@fortawesome/fontawesome-svg-core'
import type { CrepeBuilder as CrepeBuilderType } from '@milkdown/crepe/builder'
import type { TopBarFeatureConfig } from '@milkdown/crepe/feature/top-bar'
import EmojiPickerModal from '@/components/common/EmojiPickerModal.vue'

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

// Read `disposed` through a call so control-flow analysis doesn't treat it as
// still-false after the awaits below - it can flip to true if the component
// unmounts while the editor is loading.
const isDisposed = (): boolean => disposed

/** Whether the emoji picker (opened from the top bar) is showing. */
const emojiOpen = ref(false)
/** Inserts markdown inline at the cursor; set once the editor exists. */
let insertInline: ((markdown: string) => void) | null = null

/**
 * The emoji button's icon, as the HTML string Crepe's <Icon> renders (it
 * sanitizes and sets innerHTML). Pulled from the app's FontAwesome library so the
 * button matches the rest of the bar; if the lookup ever misses, fall back to a
 * plain glyph rather than rendering an empty button.
 *
 * The cast is deliberate: FA types `icon()` as returning `Icon`, but it really
 * returns undefined for a name that isn't in the library - so without this the
 * fallback reads as dead code and the editor would throw on a missing icon.
 */
const faceSmile = faIcon({ prefix: 'fad', iconName: 'face-smile' }) as
  ReturnType<typeof faIcon> | undefined
const emojiIcon = faceSmile?.html[0] ?? '<span>🙂</span>'

/**
 * Inserts the picked emoji at the cursor as a literal character - NOT a
 * `:shortcode:`. A shortcode would be escaped by the markdown serializer
 * (":video\_game:") and has to be un-escaped again before Discord can resolve it;
 * the character itself survives every hop and renders in our own preview too.
 *
 * The insert waits for the modal to actually unmount. ModalOverlay restores focus
 * to whatever was focused when it opened - here the editor - with a raw
 * `.focus()`, and focusing a contenteditable that way collapses the caret to the
 * start of the block. Inserting first meant that restoration landed AFTER our own
 * `view.focus()` and undid it, which is why the caret kept jumping to the start of
 * the line. Going second makes the editor's selection the last word.
 */
async function onEmojiSelect(emoji: string): Promise<void> {
  emojiOpen.value = false
  if (!emoji) return
  await nextTick()
  insertInline?.(emoji)
}

// Derive the builder types from Crepe's own config rather than re-declaring them:
// `GroupBuilder`/`TopBarItem` are internal paths, but the public feature config
// already names them through buildTopBar's signature.
type BuildTopBar = NonNullable<TopBarFeatureConfig['buildTopBar']>
type TopBarBuilder = Parameters<BuildTopBar>[0]
type TopBarGroup = ReturnType<TopBarBuilder['getGroup']>

/**
 * Looks a default group up by key, tolerating its absence: `getGroup` THROWS on an
 * unknown key, and a Crepe upgrade that renames a group would otherwise take the
 * whole editor down at creation. Missing means "nothing to prune here".
 */
function topBarGroup(builder: TopBarBuilder, key: string): TopBarGroup | null {
  try {
    return builder.getGroup(key)
  } catch {
    return null
  }
}

/**
 * Trims Crepe's default top bar to the Discord-safe set and adds the emoji button.
 * Kept: the heading selector, bold/italic/strikethrough/code, bullet + ordered
 * lists, link, quote and divider. Dropped: task lists (Discord has none) and
 * image/table/code-block/math, which are buttons for features this editor never
 * enables - clicking one would either do nothing or produce markdown that breaks
 * when published. Mirrors the slash menu's exclusions.
 */
const buildTopBar: BuildTopBar = (builder) => {
  const drop = (key: string, ...items: string[]): void => {
    const g = topBarGroup(builder, key)
    if (g) g.group.items = g.group.items.filter((i) => !items.includes(i.key))
  }
  drop('list', 'task-list')
  drop('insert', 'image', 'table')

  // The "block" group holds only code-block + math, so it would empty out - and an
  // empty group still renders its divider. Reuse the slot for the emoji button.
  const block = topBarGroup(builder, 'block')
  if (block) {
    block.clear()
    block.addItem('emoji', {
      icon: emojiIcon,
      active: () => false,
      onRun: () => {
        emojiOpen.value = true
      },
    })
  }
}

onMounted(async () => {
  const [
    { CrepeBuilder },
    { topBar },
    { blockEdit },
    { listItem },
    { linkTooltip },
    { placeholder },
    { cursor },
    { replaceAll, insert },
    { editorViewCtx },
  ] = await Promise.all([
    import('@milkdown/crepe/builder'),
    import('@milkdown/crepe/feature/top-bar'),
    import('@milkdown/crepe/feature/block-edit'),
    import('@milkdown/crepe/feature/list-item'),
    import('@milkdown/crepe/feature/link-tooltip'),
    import('@milkdown/crepe/feature/placeholder'),
    import('@milkdown/crepe/feature/cursor'),
    import('@milkdown/kit/utils'),
    import('@milkdown/kit/core'),
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
    .addFeature(topBar, { buildTopBar })
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
  if (isDisposed()) {
    void builder.destroy()
    return
  }
  crepe = builder
  applyExternal = (markdown) => {
    builder.editor.action(replaceAll(markdown))
  }
  // `inline` = true so the emoji joins the current paragraph instead of starting
  // a block of its own.
  //
  // Then hand focus back to the editor. The picker is a modal, so it TAKES focus:
  // the transaction still lands in the right place (ProseMirror keeps its own
  // selection while blurred), but the DOM selection is stale, and a contenteditable
  // that regains focus without one drops the caret at the start of the block - so
  // typing after picking an emoji ran on at the beginning of the line. `view.focus()`
  // writes the state's selection back to the DOM, which puts the caret after the
  // emoji that was just inserted.
  insertInline = (markdown) => {
    builder.editor.action((ctx) => {
      insert(markdown, true)(ctx)
      ctx.get(editorViewCtx).focus()
    })
  }
  ready = true
})

onBeforeUnmount(() => {
  disposed = true
  void crepe?.destroy()
  crepe = null
  applyExternal = null
  insertInline = null
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

  <!-- Opened from the top bar's emoji button; inserts the character at the cursor.
       The picker itself is lazy inside the modal, so it costs nothing until used. -->
  <EmojiPickerModal v-if="emojiOpen" @select="onEmojiSelect" @close="emojiOpen = false" />
</template>

<style scoped>
/*
 * Map Crepe's design tokens onto the app theme variables so the editor follows
 * the active theme (including custom themes). The classic theme defines these
 * variables ON `.milkdown` itself, so the overrides must live on `.milkdown`
 * too - a rule on the parent `.md-editor` gets shadowed by the child's own
 * definitions, leaving the content text at the theme's dark default instead of
 * --text. Widgets inside `.milkdown` (top bar, slash menu) inherit these.
 */
.md-editor {
  border: 1px solid var(--panel-raised-bg);
  border-radius: var(--radius);
  background: var(--panel-bg);
}

.md-editor :deep(.milkdown) {
  --crepe-color-background: var(--panel-bg);
  /* The top bar and floating widgets (slash menu) sit on the raised surface so
     they stand out from the editor body. */
  --crepe-color-surface: var(--panel-raised-bg);
  --crepe-color-surface-low: var(--panel-raised-bg);
  --crepe-color-on-background: var(--text);
  --crepe-color-on-surface: var(--text);
  --crepe-color-on-surface-variant: var(--text-muted);
  --crepe-color-primary: var(--accent);
  --crepe-color-secondary: var(--panel-raised-bg);
  --crepe-color-on-secondary: var(--text);
  /* Crepe uses --outline as a FOREGROUND color for top-bar/handle icons and
     dividers, so it must be a readable text tone - not a background color, or
     the icons are invisible (and vanish entirely when hover swaps the bg). */
  --crepe-color-outline: var(--text-muted);
  --crepe-color-hover: color-mix(in srgb, var(--text) 8%, var(--panel-raised-bg));
  --crepe-color-selected: color-mix(in srgb, var(--text) 14%, var(--panel-raised-bg));
  --crepe-color-inline-area: var(--panel-raised-bg);
  --crepe-font-default: inherit;
  --crepe-font-title: inherit;

  background: transparent;
  border-radius: var(--radius);
}

.md-editor :deep(.milkdown .ProseMirror) {
  min-height: var(--md-min-height, 180px);
  padding: 0.6rem 0.85rem;
  outline: none;
}
</style>
