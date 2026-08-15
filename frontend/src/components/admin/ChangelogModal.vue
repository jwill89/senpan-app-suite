<script setup lang="ts">
/**
 * Changelog viewer, opened from the admin sidebar version readout. A version list
 * (left) navigates a detail pane (right) that renders one release at a time with
 * proper typography - intro paragraph plus labelled change-groups (Added / Fixed /
 * ...) as badge + bullet list. The plugin additionally gets a first-class "How to
 * install" view (Dalamud steps + the copyable repo URL), kept separate from the
 * changelog rather than mixed into it.
 */
import { computed, ref } from 'vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import {
  changelog,
  CHANGELOG_LABELS,
  PLUGIN_INSTALL_STEPS,
  PLUGIN_REPO_URL,
  type ChangelogComponent,
} from '@/lib/changelog'

const props = defineProps<{ component: ChangelogComponent }>()
defineEmits<{ close: [] }>()

const section = computed(() => changelog[props.component])
const entries = computed(() => section.value.entries)
const title = computed(() => `${CHANGELOG_LABELS[props.component]} Changelog`)
const hasInstall = computed(() => props.component === 'plugin')

// Selected rail item: a version string, or 'install' for the plugin's steps view.
// The plugin opens on its install steps (that's usually why you'd click it); the
// web components open on their newest release.
const selected = ref<string>(hasInstall.value ? 'install' : (entries.value[0]?.version ?? ''))

const showInstall = computed(() => selected.value === 'install')
const currentEntry = computed(() => entries.value.find((e) => e.version === selected.value) ?? null)

/** Category class for a change-group label (colours the badge). */
function badgeClass(label: string): string {
  const key = label.trim().toLowerCase().split(/\s+/)[0]
  const known = ['added', 'fixed', 'changed', 'security', 'removed', 'deprecated']
  return known.includes(key) ? `changelog-badge--${key}` : 'changelog-badge--default'
}

// Copy-the-repo-URL affordance with a brief "Copied!" confirmation.
const copied = ref(false)
let copyTimer: ReturnType<typeof setTimeout> | null = null
async function copyRepoUrl(): Promise<void> {
  try {
    await navigator.clipboard.writeText(PLUGIN_REPO_URL)
    copied.value = true
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = setTimeout(() => {
      copied.value = false
    }, 1600)
  } catch {
    /* clipboard blocked - the URL is still visible to select manually */
  }
}
</script>

<template>
  <ModalOverlay
    :aria-label="title"
    :box-style="{ maxWidth: '880px', width: '95vw' }"
    @close="$emit('close')"
  >
    <div class="changelog">
      <header class="changelog-head">
        <h3 class="changelog-title">
          <font-awesome-icon :icon="['fad', 'clipboard-list']" /> {{ title }}
        </h3>
        <span v-if="section.latest" class="changelog-latest">Latest v{{ section.latest }}</span>
      </header>

      <div class="changelog-body">
        <!-- Navigation rail: install (plugin) + version list -->
        <nav class="changelog-rail" aria-label="Versions">
          <button
            v-if="hasInstall"
            type="button"
            class="changelog-rail-item changelog-rail-item--install"
            :class="{ 'is-active': showInstall }"
            @click="selected = 'install'"
          >
            <font-awesome-icon :icon="['fas', 'download']" />
            <span>How to install</span>
          </button>
          <div v-if="hasInstall" class="changelog-rail-divider">Changelog</div>

          <button
            v-for="e in entries"
            :key="e.version"
            type="button"
            class="changelog-rail-item"
            :class="{ 'is-active': selected === e.version }"
            @click="selected = e.version"
          >
            <span class="changelog-rail-version">v{{ e.version }}</span>
            <span v-if="e.date" class="changelog-rail-date">{{ e.date }}</span>
          </button>
        </nav>

        <!-- Detail pane -->
        <div class="changelog-detail">
          <!-- Plugin install steps -->
          <section v-if="showInstall" class="changelog-install">
            <h4 class="changelog-detail-title">Installing the plugin (Dalamud)</h4>
            <p class="changelog-lead">
              <strong>Senpan Admin Companion</strong> is a Dalamud plugin for <strong>FFXIV</strong>
              that lets staff drive app services from in-game. Install it from the custom repository
              below.
            </p>

            <div class="changelog-repo">
              <span class="changelog-repo-label">Repository URL</span>
              <div class="changelog-repo-row">
                <code class="changelog-repo-url">{{ PLUGIN_REPO_URL }}</code>
                <button
                  type="button"
                  class="btn-neutral btn-sm changelog-copy"
                  @click="copyRepoUrl"
                >
                  <font-awesome-icon :icon="['fas', copied ? 'circle-check' : 'copy']" />
                  {{ copied ? 'Copied' : 'Copy' }}
                </button>
              </div>
            </div>

            <ol class="changelog-steps">
              <li v-for="(step, i) in PLUGIN_INSTALL_STEPS" :key="i">
                <span class="changelog-step-title">{{ step.title }}</span>
                <MarkdownText flow class="changelog-step-detail md" :source="step.detail" />
              </li>
            </ol>

            <p class="changelog-note">
              <font-awesome-icon :icon="['fas', 'circle-info']" /> Custom-repo plugins don’t
              auto-update on their own - re-open <code>/xlplugins</code> to pull a new version when
              one is released.
            </p>
          </section>

          <!-- Version entry -->
          <article v-else-if="currentEntry" class="changelog-entry">
            <h4 class="changelog-detail-title">
              v{{ currentEntry.version }}
              <span v-if="currentEntry.date" class="changelog-entry-date">{{
                currentEntry.date
              }}</span>
            </h4>

            <MarkdownText
              v-if="currentEntry.intro"
              flow
              class="changelog-lead md"
              :source="currentEntry.intro"
            />

            <section v-for="(g, gi) in currentEntry.groups" :key="gi" class="changelog-group">
              <span class="changelog-badge" :class="badgeClass(g.label)">{{ g.label }}</span>
              <MarkdownText flow class="changelog-group-list md" :source="g.body" />
            </section>
          </article>

          <p v-else class="changelog-empty">No changelog available.</p>
        </div>
      </div>

      <footer class="changelog-foot">
        <button class="btn-neutral" @click="$emit('close')">Close</button>
      </footer>
    </div>
  </ModalOverlay>
</template>

<style scoped>
.changelog {
  display: flex;
  flex-direction: column;
  min-height: 0;
  max-height: 80vh;
}

/* Header */
.changelog-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--control-border);
}
.changelog-title {
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}
.changelog-latest {
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-muted);
  white-space: nowrap;
}

/* Two-pane body */
.changelog-body {
  display: flex;
  gap: 14px;
  min-height: 0;
  flex: 1;
  padding: 12px 0;
}

/* Left rail */
.changelog-rail {
  flex: 0 0 172px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  overflow-y: auto;
  padding-right: 8px;
  border-right: 1px solid var(--control-border);
}
.changelog-rail-item {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 1px;
  width: 100%;
  padding: 6px 10px;
  border: 1px solid transparent;
  border-radius: var(--radius);
  background: none;
  color: var(--text);
  cursor: pointer;
  text-align: left;
  transition:
    background 0.12s,
    border-color 0.12s;
}
.changelog-rail-item:hover {
  background: var(--panel-raised-bg);
}
.changelog-rail-item.is-active {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--text-on-accent);
}
.changelog-rail-item--install {
  flex-direction: row;
  align-items: center;
  gap: 7px;
  font-weight: 600;
}
.changelog-rail-version {
  font-family: 'Consolas', 'Monaco', monospace;
  font-weight: 600;
  font-size: 0.86rem;
}
.changelog-rail-date {
  font-size: 0.68rem;
  opacity: 0.75;
}
.changelog-rail-divider {
  margin: 8px 4px 3px;
  font-size: 0.66rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-muted);
}

/* Right detail */
.changelog-detail {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding-right: 6px;
}
.changelog-detail-title {
  margin: 0 0 10px;
  display: flex;
  align-items: baseline;
  gap: 10px;
  font-size: 1.15rem;
}
.changelog-entry-date,
.changelog-note {
  font-size: 0.78rem;
  color: var(--text-muted);
  font-weight: 400;
}
.changelog-lead {
  color: var(--text-muted);
  margin-bottom: 16px;
  line-height: 1.55;
}

/* Change-group: badge + list */
.changelog-group {
  margin-bottom: 16px;
}
/* Change-type badge - pill chrome from the `.badge` object; the type modifiers
   below set `color`, which drives both the outline and the tinted fill. */
.changelog-badge {
  margin-bottom: 6px;
  border: 1px solid currentColor;
  background: color-mix(in srgb, currentColor 14%, transparent);
}
.changelog-badge--added {
  color: #3fb950;
}
.changelog-badge--fixed {
  color: #d29922;
}
.changelog-badge--changed {
  color: #539bf5;
}
.changelog-badge--security {
  color: #db61a2;
}
.changelog-badge--removed {
  color: #f85149;
}
.changelog-badge--deprecated,
.changelog-badge--default {
  color: var(--text-muted);
}

/* Install view */
.changelog-repo {
  margin-bottom: 16px;
  padding: 10px 12px;
  border: 1px solid var(--control-border);
  border-radius: var(--radius);
  background: var(--panel-raised-bg);
}
.changelog-repo-label {
  display: block;
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-muted);
  margin-bottom: 6px;
}
.changelog-repo-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.changelog-repo-url {
  flex: 1;
  min-width: 220px;
  padding: 6px 8px;
  border-radius: 0;
  background: var(--panel-bg);
  font-size: 0.82rem;
  word-break: break-all;
}
.changelog-copy {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.changelog-steps {
  margin: 0 0 14px;
  padding-left: 22px;
  display: flex;
  flex-direction: column;
  gap: 9px;
}
.changelog-steps li {
  line-height: 1.5;
}
.changelog-step-title {
  font-weight: 600;
}
.changelog-step-detail {
  color: var(--text-muted);
}
.changelog-note {
  padding-top: 6px;
}
.changelog-empty {
  color: var(--text-muted);
}

/* Footer */
.changelog-foot {
  display: flex;
  justify-content: flex-end;
  padding-top: 10px;
  border-top: 1px solid var(--control-border);
}

/* -- Rendered-markdown typography (v-html output) --------------------------- */
.md :deep(p) {
  margin: 0 0 8px;
  line-height: 1.55;
}
.md :deep(p:last-child) {
  margin-bottom: 0;
}
.changelog-group-list.md :deep(ul) {
  margin: 0;
  padding-left: 20px;
  display: flex;
  flex-direction: column;
  gap: 7px;
}
.changelog-group-list.md :deep(li) {
  line-height: 1.55;
}
.changelog-group-list.md :deep(ul ul) {
  margin-top: 6px;
  gap: 4px;
}
.md :deep(code) {
  padding: 1px 5px;
  border-radius: 0;
  background: var(--panel-raised-bg);
  font-size: 0.86em;
}
.md :deep(a) {
  color: var(--accent);
  text-decoration: underline;
  text-underline-offset: 2px;
}
.md :deep(strong) {
  color: var(--text);
}
.md :deep(blockquote) {
  margin: 8px 0;
  padding: 4px 12px;
  border-left: 3px solid var(--control-border);
  color: var(--text-muted);
}

/* Stack the panes on narrow screens: rail becomes a horizontal scroller. */
@media (max-width: 640px) {
  .changelog-body {
    flex-direction: column;
  }
  .changelog-rail {
    flex: 0 0 auto;
    flex-direction: row;
    max-height: none;
    overflow-x: auto;
    overflow-y: hidden;
    padding: 0 0 8px;
    border-right: none;
    border-bottom: 1px solid var(--control-border);
  }
  .changelog-rail-item {
    width: auto;
    white-space: nowrap;
  }
  .changelog-rail-divider {
    display: none;
  }
}
</style>
