<script setup lang="ts">
/**
 * Toast notification — fixed-position success/error/info popup.
 * Mirrors the original `toast` markup; driven by the UI store.
 */
import { useUiStore } from '@/stores/ui'

const ui = useUiStore()
</script>

<template>
  <div v-if="ui.toast.show" :class="['toast', 'toast-' + ui.toast.type]">
    {{ ui.toast.message }}
  </div>
</template>
