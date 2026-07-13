<script setup lang="ts">
import { useMarkdown, useMarkdownFlow } from '@/lib/markdown'

// Renders sanitized Markdown from `source`. markdown-it runs with `html: false`
// (see src/lib/markdown.ts), so any raw HTML in the source is escaped and the
// rendered output is safe to bind. Centralizing the render here keeps the single
// trusted-HTML (v-html) boundary in one reviewed place instead of scattering it
// across views. Renders '' until the parser lazy-loads, then updates reactively.
//
// `flow` switches off the single-newline→<br> behaviour (standard markdown, where
// soft newlines are spaces) — use it for source that's soft-wrapped as prose,
// e.g. the changelog. Default (off) preserves the game-details/raffle behaviour.
const props = defineProps<{ source: string; flow?: boolean }>()

const { render } = props.flow ? useMarkdownFlow() : useMarkdown()
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -->
  <div v-html="render(source)"></div>
</template>
