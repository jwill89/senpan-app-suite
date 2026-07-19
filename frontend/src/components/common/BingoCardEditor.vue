<script setup lang="ts">
/**
 * Editable 5×5 bingo card grid for the public Personal Card Requests page. Each
 * column accepts only its own number range (B 1–15 … O 61–75); the centre cell is
 * the fixed FREE space. Out-of-range or duplicated cells are highlighted so the
 * player can see exactly what to fix. "Generate Random" fills a valid random card
 * (still fully editable afterwards). v-model is the 5×5 number grid (0 = blank/FREE).
 */
import { computed } from 'vue'
import { BINGO_LETTERS, columnRange, randomBoard } from '@/lib/constants'

const props = defineProps<{ modelValue: number[][] }>()
const emit = defineEmits<{ 'update:modelValue': [board: number[][]] }>()

function setCell(row: number, col: number, raw: string): void {
  const n = parseInt(raw, 10)
  const next = props.modelValue.map((r) => [...r])
  next[row][col] = Number.isNaN(n) ? 0 : n
  emit('update:modelValue', next)
}

function randomize(): void {
  emit('update:modelValue', randomBoard())
}

function isFree(row: number, col: number): boolean {
  return row === 2 && col === 2
}

// Cells that are out of their column range or duplicated within the column, so the
// grid can flag exactly what needs fixing. Blanks (0) and the FREE centre are skipped.
const invalidCells = computed(() => {
  const bad = new Set<string>()
  for (let col = 0; col < 5; col++) {
    const [lo, hi] = columnRange(col)
    const rowsByValue = new Map<number, number[]>()
    for (let row = 0; row < 5; row++) {
      if (isFree(row, col)) continue
      const n = props.modelValue[row][col]
      if (n === 0) continue // blank — incomplete, not yet an error
      if (n < lo || n > hi) bad.add(`${row}-${col}`)
      const rows = rowsByValue.get(n) || []
      rows.push(row)
      rowsByValue.set(n, rows)
    }
    for (const rows of rowsByValue.values()) {
      if (rows.length > 1) rows.forEach((row) => bad.add(`${row}-${col}`))
    }
  }
  return bad
})
</script>

<template>
  <div class="card-editor">
    <div class="card-editor-header">
      <div v-for="letter in BINGO_LETTERS" :key="letter" class="ce-letter">{{ letter }}</div>
    </div>
    <div class="card-editor-grid">
      <template v-for="row in 5" :key="row">
        <template v-for="col in 5" :key="`${row}-${col}`">
          <div v-if="isFree(row - 1, col - 1)" class="ce-cell ce-free">FREE</div>
          <input
            v-else
            class="ce-cell"
            :class="{ 'ce-invalid': invalidCells.has(`${row - 1}-${col - 1}`) }"
            type="number"
            inputmode="numeric"
            :min="columnRange(col - 1)[0]"
            :max="columnRange(col - 1)[1]"
            :value="modelValue[row - 1][col - 1] || ''"
            :aria-label="`Column ${BINGO_LETTERS[col - 1]}, row ${row}`"
            @input="setCell(row - 1, col - 1, ($event.target as HTMLInputElement).value)"
          />
        </template>
      </template>
    </div>
    <button type="button" class="btn-view btn-sm mt" @click="randomize">
      <font-awesome-icon :icon="['fas', 'dice']" /> Generate Random
    </button>
  </div>
</template>

<style scoped>
/* Centre the whole editor: the grids centre via margin-auto, and text-align
   centres the inline "Generate Random" button under them. */
.card-editor {
  text-align: center;
}
.card-editor-header,
.card-editor-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 6px;
  max-width: 340px;
  margin-inline: auto;
}
.card-editor-header {
  margin-bottom: 6px;
}
.ce-letter {
  text-align: center;
  font-weight: 700;
  color: var(--highlight);
}
.ce-cell {
  aspect-ratio: 1;
  text-align: center;
  font-size: 1rem;
  padding: 0;
}
.ce-free {
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 0.75rem;
  background: var(--board-free-bg);
  color: var(--text-on-accent);
  border-radius: var(--radius, 6px);
}
.ce-invalid {
  border-color: var(--danger) !important;
  outline: 1px solid var(--danger);
}
</style>
