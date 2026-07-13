<script setup lang="ts">
/**
 * Frontend/backend/plugin version readout for the admin sidebar footer. The SPA's
 * build version (baked in) and the backend's (fetched from GET /api/version) are
 * shown so operators can confirm a deploy left the web halves compatible; a MAJOR
 * mismatch (or a failed probe) is flagged. The plugin's version comes from the
 * bundled changelog (its latest released entry). Each version is a button that
 * opens that component's changelog (the plugin's also carries install steps).
 */
import { computed, onMounted, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { FRONTEND_VERSION, versionsCompatible } from '@/lib/version'
import { changelog, type ChangelogComponent } from '@/lib/changelog'
import ChangelogModal from '@/components/admin/ChangelogModal.vue'

const backend = ref<string | null>(null)
const failed = ref(false)
/** Which component's changelog modal is open (null = closed). */
const openComponent = ref<ChangelogComponent | null>(null)

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

/** Latest released plugin version (from the bundled changelog). */
const pluginVersion = computed(() => changelog.plugin.latest)
</script>

<template>
  <div class="app-versions" :class="{ 'app-versions--warn': incompatible || failed }">
    <span class="app-versions__row">
      <span class="app-versions__label">Frontend</span>
      <button
        type="button"
        class="app-versions__val app-versions__link"
        title="View the Frontend changelog"
        @click="openComponent = 'frontend'"
      >
        v{{ FRONTEND_VERSION }}
      </button>
    </span>

    <span class="app-versions__row">
      <span class="app-versions__label">Backend</span>
      <button
        v-if="backend"
        type="button"
        class="app-versions__val app-versions__link"
        title="View the Backend changelog"
        @click="openComponent = 'backend'"
      >
        v{{ backend }}
      </button>
      <span v-else class="app-versions__val">{{ failed ? 'unknown' : '…' }}</span>
    </span>

    <span class="app-versions__row">
      <span class="app-versions__label">Plugin</span>
      <button
        v-if="pluginVersion"
        type="button"
        class="app-versions__val app-versions__link"
        title="View the Plugin changelog + install steps"
        @click="openComponent = 'plugin'"
      >
        v{{ pluginVersion }}
      </button>
      <span v-else class="app-versions__val">—</span>
    </span>

    <span
      v-if="incompatible"
      class="app-versions__flag"
      title="Frontend and backend major versions differ — check compatibility."
    >
      <font-awesome-icon :icon="['fas', 'triangle-exclamation']" /> version mismatch
    </span>

    <ChangelogModal v-if="openComponent" :component="openComponent" @close="openComponent = null" />
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
/* Version numbers are buttons that open the changelog — reset button chrome and
   surface them as subtle links. */
.app-versions__link {
  background: none;
  border: none;
  padding: 0;
  margin: 0;
  cursor: pointer;
  text-decoration: underline;
  text-decoration-style: dotted;
  text-underline-offset: 2px;
}
.app-versions__link:hover,
.app-versions__link:focus-visible {
  color: var(--accent);
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
