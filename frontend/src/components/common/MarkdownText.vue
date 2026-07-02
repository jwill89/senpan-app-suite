<script setup lang="ts">
import { useMarkdown } from '@/lib/markdown'

// Renders sanitized Markdown from `source`. markdown-it runs with `html: false`
// (see src/lib/markdown.ts), so any raw HTML in the source is escaped and the
// rendered output is safe to bind. Centralizing the render here keeps the single
// trusted-HTML (v-html) boundary in one reviewed place instead of scattering it
// across views. Renders '' until the parser lazy-loads, then updates reactively.
defineProps<{ source: string }>()

const { render } = useMarkdown()
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -->
  <div v-html="render(source)"></div>
</template>
