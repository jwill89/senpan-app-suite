<script setup lang="ts">
/**
 * App/plugin version readout for the admin sidebar footer. The app's version is this
 * build's (baked in); the server's (GET /api/v1/version) is fetched only to tell a tab
 * opened before a deploy that the server runs another version now, and that a reload
 * brings them back together. A failed probe is flagged too. The plugin's version is
 * fetched LIVE from the deployed Dalamud repo index (pluginmaster.json) so publishing
 * a new plugin refreshes it without an app release; it falls back to the bundled
 * changelog version when the index isn't reachable (e.g. local dev). Each version is a
 * button that opens its changelog (the plugin's also carries install steps).
 */
import { computed, onMounted, ref } from 'vue'
import { api } from '@/api/client'
import { APP_VERSION, serverDiffers } from '@/lib/version'
import { changelog, fetchLivePluginVersion, type ChangelogComponent } from '@/lib/changelog'
import ChangelogModal from '@/components/admin/ChangelogModal.vue'

/** The version the server runs, once fetched. */
const server = ref<string | null>(null)
const failed = ref(false)
/** Live plugin version from the deployed repo index; null until fetched (or on failure). */
const livePlugin = ref<string | null>(null)
/** Which changelog modal is open (null = closed). */
const openComponent = ref<ChangelogComponent | null>(null)

// .then/.catch (not async/await) so the rejection handler is attached
// synchronously - no unhandled-rejection window if the probe fails.
onMounted(() => {
  api
    .GET('/version')
    .then(({ data }) => {
      if (data) {
        server.value = data.version
      } else {
        failed.value = true
      }
    })
    .catch(() => {
      failed.value = true
    })
  // Live plugin version (what Dalamud serves). fetchLivePluginVersion never rejects -
  // it resolves to null on failure, so the computed falls back to the bundled value.
  // `void` marks it deliberately fire-and-forget (no rejection to handle).
  void fetchLivePluginVersion().then((v) => {
    livePlugin.value = v
  })
})

/** The server runs another version than this tab: it was opened before a deploy. */
const behind = computed(() => serverDiffers(server.value))

/** Live plugin version if reachable, else the version baked into the bundled changelog. */
const pluginVersion = computed(() => livePlugin.value ?? changelog.plugin.latest)
</script>

<template>
  <div class="app-versions" :class="{ 'app-versions--warn': behind || failed }">
    <span class="app-versions-row">
      <span class="app-versions-label">App</span>
      <button
        type="button"
        class="app-versions-val app-versions-link"
        title="View the App changelog"
        @click="openComponent = 'app'"
      >
        v{{ APP_VERSION }}
      </button>
    </span>
    <span class="app-versions-row">
      <span class="app-versions-label">Plugin</span>
      <button
        v-if="pluginVersion"
        type="button"
        class="app-versions-val app-versions-link"
        title="View the Plugin changelog + install steps"
        @click="openComponent = 'plugin'"
      >
        v{{ pluginVersion }}
      </button>
      <span v-else class="app-versions-val">-</span>
    </span>
    <span
      v-if="behind"
      class="app-versions-flag"
      :title="`This tab runs v${APP_VERSION}; the server runs v${server}. Reload to bring them together.`"
    >
      <font-awesome-icon :icon="['fas', 'triangle-exclamation']" /> server runs v{{ server }}:
      reload
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

.app-versions-row {
  display: flex;
  justify-content: space-between;
  gap: 10px;
}

.app-versions-label {
  opacity: 0.85;
}

.app-versions-val {
  font-family: Consolas, Monaco, monospace;
  color: var(--text);
}

/* Version numbers are buttons that open the changelog - reset button chrome and
   surface them as subtle links. */
.app-versions-link {
  background: none;
  border: none;
  padding: 0;
  margin: 0;
  cursor: pointer;
  text-decoration: underline;
  text-decoration-style: dotted;
  text-underline-offset: 2px;
}

.app-versions-link:hover,
.app-versions-link:focus-visible {
  color: var(--accent);
}

.app-versions--warn .app-versions-val {
  color: var(--warning);
}

.app-versions-flag {
  margin-top: 3px;
  color: var(--warning);
  font-weight: 600;
}
</style>
