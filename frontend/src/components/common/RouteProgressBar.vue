<script setup lang="ts">
/**
 * Thin top-of-viewport progress bar shown while a route navigation is in flight
 * (async guard + lazy chunk fetch). Reads `ui.routeLoading`, which the router
 * guards toggle. Indeterminate animation — styled with theme tokens in app.css.
 */
import { useUiStore } from '@/stores/ui'

const ui = useUiStore()
</script>

<template>
  <div v-if="ui.routeLoading" class="route-progress" role="progressbar" aria-label="Loading">
    <div class="route-progress-bar"></div>
  </div>
</template>
