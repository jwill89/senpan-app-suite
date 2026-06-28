<script setup lang="ts">
/**
 * Compact frontend/backend version readout for the admin sidebar footer. Shows
 * the SPA's build version (baked in) alongside the backend's (fetched from
 * GET /api/version) so operators can confirm a deploy left the two halves
 * compatible. A mismatch in MAJOR version (or a failed probe) is flagged.
 */
import { computed, onMounted, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { FRONTEND_VERSION, versionsCompatible } from '@/lib/version'

const backend = ref<string | null>(null)
const failed = ref(false)

// .then/.catch (not async/await) so the rejection handler is attached
// synchronously — no unhandled-rejection window if the probe fails.
onMounted(() => {
  endpoints.system
    .version()
    .then((r) => {
      backend.value = r.backend
    })
    .catch(() => {
      failed.value = true
    })
})

/** Incompatible only once we actually know the backend version. */
const incompatible = computed(
  () => backend.value !== null && !versionsCompatible(FRONTEND_VERSION, backend.value),
)
</script>

<template>
  <div class="app-versions" :class="{ 'app-versions--warn': incompatible || failed }">
    <span class="app-versions__row">
      <span class="app-versions__label">Frontend</span>
      <span class="app-versions__val">v{{ FRONTEND_VERSION }}</span>
    </span>
    <span class="app-versions__row">
      <span class="app-versions__label">Backend</span>
      <span class="app-versions__val">{{ backend ? `v${backend}` : failed ? 'unknown' : '…' }}</span>
    </span>
    <span
      v-if="incompatible"
      class="app-versions__flag"
      title="Frontend and backend major versions differ — check compatibility."
    >
      <font-awesome-icon :icon="['fas', 'triangle-exclamation']" /> version mismatch
    </span>
  </div>
</template>

<style scoped>
.app-versions {
  margin-top: 8px;
  padding: 8px 12px;
  border-top: 1px solid var(--control-border);
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 0.72rem;
  color: var(--text-muted);
}
.app-versions__row {
  display: flex;
  justify-content: space-between;
  gap: 10px;
}
.app-versions__label {
  opacity: 0.85;
}
.app-versions__val {
  font-family: 'Consolas', 'Monaco', monospace;
  color: var(--text);
}
.app-versions--warn .app-versions__val {
  color: var(--warning);
}
.app-versions__flag {
  margin-top: 3px;
  color: var(--warning);
  font-weight: 600;
}
</style>
