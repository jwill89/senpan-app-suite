<script setup lang="ts">
/**
 * Miniature 5×5 pattern preview. Renders the pattern grid as small cells where
 * `on` cells are highlighted. Mirrors the `.pattern-mini` markup used on the
 * player view, game screens, and pattern editor list.
 */
defineProps<{
  /** 5×5 boolean grid; true = required cell. */
  patternData: boolean[][]
  /** Extra classes for the wrapper (e.g. 'pattern-mini-sm'). */
  size?: string
  /** Render as inline-grid (used in the game-start checkbox list). */
  inline?: boolean
}>()
</script>

<template>
  <component
    :is="inline ? 'span' : 'div'"
    class="pattern-mini"
    :class="size"
    :style="inline ? 'display:inline-grid' : undefined"
  >
    <template v-for="(row, ri) in patternData" :key="ri">
      <component
        :is="inline ? 'span' : 'div'"
        v-for="(cell, ci) in row"
        :key="ri + '-' + ci"
        :class="['pattern-mini-cell', cell ? 'on' : '']"
      />
    </template>
  </component>
</template>
