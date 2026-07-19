<script setup lang="ts">
/**
 * Called numbers panel — the 5-column B-I-N-G-O number tracker shared by the
 * player and admin game views. `isCalled` decides the `called` highlight.
 * Mirrors the original `.numbers-panel` markup.
 *
 * `activeColumns` (from patternColumns) marks which columns the current game can
 * actually draw from; a column no pattern uses (so no number in it will ever be
 * called this game) gets a dim overlay to show it won't be used. Omit it — or pass
 * all-true — to leave every column normal (e.g. when there's no active game).
 */
import { BINGO_LETTERS, columnNumbers } from '@/lib/constants'

const props = defineProps<{
  /** Count of called numbers (shown in the heading). */
  count: number
  /** Predicate: is this number currently called? */
  isCalled: (n: number) => boolean
  /** Per-column active flags (index 0=B … 4=O); inactive columns are dimmed. */
  activeColumns?: boolean[]
}>()

/** A column is dimmed only when activeColumns is provided and marks it inactive. */
function isDimmed(li: number): boolean {
  return !!props.activeColumns && !props.activeColumns[li]
}
</script>

<template>
  <div class="numbers-panel">
    <h3>Called Numbers ({{ count }} / 75)</h3>
    <div class="numbers-cols">
      <template v-for="(letter, li) in BINGO_LETTERS" :key="letter">
        <div :class="['numbers-col', isDimmed(li) ? 'col-unused' : '']">
          <div class="numbers-col-header">{{ letter }}</div>
          <div
            v-for="n in columnNumbers(li)"
            :key="n"
            :class="['num-cell', isCalled(n) ? 'called' : '']"
          >
            {{ n }}
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
/* A column whose numbers won't be drawn this game (no pattern uses it): a simple
   ~45% dark block over the whole column so it clearly reads as "not in play". */
.numbers-col {
  position: relative;
}
.numbers-col.col-unused::after {
  content: '';
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  border-radius: 6px;
  pointer-events: none;
}
</style>
