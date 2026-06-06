<script setup lang="ts">
/**
 * Bingo board (5×5) with the B-I-N-G-O header. Used in three places:
 *  - Player view: interactive, shows stamp marks (mode="player").
 *  - Winner verification: static, highlights pattern-hit cells (mode="preview").
 *  - Card preview: static board (mode="preview").
 *
 * Keeps the original `.board-wrap` / `.board-header` / `.board-grid` /
 * `.board-cell` markup so all existing styles and themes apply unchanged.
 */
import type { StyleValue } from 'vue'

defineProps<{
  board: number[][]
  /** 'player' = interactive + stamps; 'preview' = static. */
  mode?: 'player' | 'preview'
  /** Adds the `.board-preview` modifier class (used in modals). */
  preview?: boolean
  /** Player mode: is the cell stamped? */
  isStamped?: (ri: number, ci: number) => boolean
  /** Player mode: per-cell class builder. */
  cellClass?: (ri: number, ci: number, cell: number) => (string | false)[]
  /** Preview mode: should this cell be highlighted as a pattern hit? */
  isCellMatch?: (ri: number, ci: number) => boolean
  /** Player mode: inline style for the stamp mark. */
  stampMarkStyle?: StyleValue
  /** Player mode: emoji to show for stamped cells. */
  stampEmoji?: string
  /** Player mode: custom stamp shape id ('custom' shows the image). */
  stampShape?: string
  /** Player mode: custom stamp image data URL. */
  customStampImage?: string | null
}>()

const emit = defineEmits<{ cellClick: [ri: number, ci: number, cell: number] }>()
</script>

<template>
  <div class="board-wrap" :class="{ 'board-preview': preview }">
    <div class="board-header">
      <span>B</span><span>I</span><span>N</span><span>G</span><span>O</span>
    </div>
    <div class="board-grid">
      <template v-for="(row, ri) in board" :key="ri">
        <!-- Player mode: interactive cells with stamps -->
        <template v-if="mode === 'player'">
          <div
            v-for="(cell, ci) in row"
            :key="ri + '-' + ci"
            :class="cellClass ? cellClass(ri, ci, cell) : ['board-cell', cell === 0 ? 'free' : '']"
            @click="emit('cellClick', ri, ci, cell)"
          >
            <span class="cell-num">{{ cell === 0 ? 'FREE' : cell }}</span>
            <div class="stamp-mark" :style="isStamped && isStamped(ri, ci) ? stampMarkStyle : {}">
              <img
                v-if="isStamped && isStamped(ri, ci) && stampShape === 'custom' && customStampImage"
                :src="customStampImage"
                class="stamp-custom-img"
                alt="stamp"
              />
              <template v-else>{{ isStamped && isStamped(ri, ci) ? stampEmoji : '' }}</template>
            </div>
          </div>
        </template>

        <!-- Preview mode: static cells, optional pattern-hit highlight -->
        <template v-else>
          <div
            v-for="(cell, ci) in row"
            :key="ri + '-' + ci"
            :class="[
              'board-cell',
              cell === 0 ? 'free' : '',
              isCellMatch && isCellMatch(ri, ci) ? 'pattern-hit' : '',
            ]"
            style="cursor: default"
          >
            <span class="cell-num">{{ cell === 0 ? 'FREE' : cell }}</span>
            <div v-if="cell === 0" class="stamp-mark"></div>
          </div>
        </template>
      </template>
    </div>
  </div>
</template>
