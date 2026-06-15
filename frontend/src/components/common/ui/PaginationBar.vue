<script setup lang="ts">
/**
 * Prev / page-indicator / Next control for paginated admin tables. The parent
 * owns the page state; this emits `go` with the requested 1-based page number.
 * Renders nothing when there is only a single page.
 */
const props = defineProps<{ page: number; totalPages: number }>()
const emit = defineEmits<{ go: [page: number] }>()
</script>

<template>
  <div v-if="totalPages > 1" class="pagination-bar">
    <button
      class="btn-neutral btn-sm"
      :disabled="props.page <= 1"
      @click="emit('go', props.page - 1)"
    >
      ‹ Prev
    </button>
    <span class="text-dim text-xs">Page {{ props.page }} / {{ props.totalPages }}</span>
    <button
      class="btn-neutral btn-sm"
      :disabled="props.page >= props.totalPages"
      @click="emit('go', props.page + 1)"
    >
      Next ›
    </button>
  </div>
</template>
