<script setup lang="ts">
/**
 * Called numbers panel — the 5-column B-I-N-G-O number tracker shared by the
 * player and admin game views. `isCalled` decides the `called` highlight.
 * Mirrors the original `.numbers-panel` markup.
 */
import { BINGO_LETTERS, columnNumbers } from '@/lib/constants'

defineProps<{
  /** Count of called numbers (shown in the heading). */
  count: number
  /** Predicate: is this number currently called? */
  isCalled: (n: number) => boolean
}>()
</script>

<template>
  <div class="numbers-panel">
    <h3>Called Numbers ({{ count }} / 75)</h3>
    <div class="numbers-cols">
      <template v-for="(letter, li) in BINGO_LETTERS" :key="letter">
        <div>
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
