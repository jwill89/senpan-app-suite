<script setup lang="ts">
/**
 * Win patterns panel (player view) - shows the active win patterns for the
 * current game. Mirrors the original `.patterns-panel` block on the player view.
 */
import PatternMini from '@/components/common/PatternMini.vue'
import type { GamePattern } from '@/api/client'

defineProps<{ patterns: GamePattern[] }>()
</script>

<template>
  <div v-if="patterns && patterns.length" class="patterns-panel">
    <h3>Win Pattern{{ patterns.length > 1 ? 's' : '' }}</h3>
    <div class="pattern-cards">
      <div v-for="(p, i) in patterns" :key="i" class="pattern-card">
        <span :title="p.name">{{ p.name }}</span>
        <PatternMini :pattern-data="p.patternData" />
      </div>
    </div>
  </div>
</template>
