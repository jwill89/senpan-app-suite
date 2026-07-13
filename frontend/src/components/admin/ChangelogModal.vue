<script setup lang="ts">
/**
 * Changelog viewer, opened from the admin sidebar version readout. A version list
 * (left) navigates a detail pane (right) that renders one release at a time with
 * proper typography — intro paragraph plus labelled change-groups (Added / Fixed /
 * …) as badge + bullet list. The plugin additionally gets a first-class "How to
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
  return known.includes(key) ? `cl-badge--${key}` : 'cl-badge--default'
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
    /* clipboard blocked — the URL is still visible to select manually */
  }
}
</script>

<template>
  <ModalOverlay
    :aria-label="title"
    :box-style="{ maxWidth: '880px', width: '95vw' }"
    @close="$emit('close')"
  >
    <div class="cl">
      <header class="cl__head">
        <h3 class="cl__title">
          <font-awesome-icon :icon="['fad', 'clipboard-list']" /> {{ title }}
        </h3>
        <span v-if="section.latest" class="cl__latest">Latest v{{ section.latest }}</span>
      </header>

      <div class="cl__body">
        <!-- Navigation rail: install (plugin) + version list -->
        <nav class="cl__rail" aria-label="Versions">
          <button
            v-if="hasInstall"
            type="button"
            class="cl__railitem cl__railitem--install"
            :class="{ 'is-active': showInstall }"
            @click="selected = 'install'"
          >
            <font-awesome-icon :icon="['fas', 'download']" />
            <span>How to install</span>
          </button>
          <div v-if="hasInstall" class="cl__raildiv">Changelog</div>

          <button
            v-for="e in entries"
            :key="e.version"
            type="button"
            class="cl__railitem"
            :class="{ 'is-active': selected === e.version }"
            @click="selected = e.version"
          >
            <span class="cl__railver">v{{ e.version }}</span>
            <span v-if="e.date" class="cl__raildate">{{ e.date }}</span>
          </button>
        </nav>

        <!-- Detail pane -->
        <div class="cl__detail">
          <!-- Plugin install steps -->
          <section v-if="showInstall" class="cl__install">
            <h4 class="cl__detailtitle">Installing the plugin (Dalamud)</h4>
            <p class="cl__lead">
              <strong>Senpan Admin Companion</strong> is a Dalamud plugin for <strong>FFXIV</strong>
              that lets staff drive app services from in-game. Install it from the custom repository
              below.
            </p>

            <div class="cl__repo">
              <span class="cl__repolabel">Repository URL</span>
              <div class="cl__reporow">
                <code class="cl__repourl">{{ PLUGIN_REPO_URL }}</code>
                <button type="button" class="btn-neutral btn-sm cl__copy" @click="copyRepoUrl">
                  <font-awesome-icon :icon="['fas', copied ? 'circle-check' : 'copy']" />
                  {{ copied ? 'Copied' : 'Copy' }}
                </button>
              </div>
            </div>

            <ol class="cl__steps">
              <li v-for="(step, i) in PLUGIN_INSTALL_STEPS" :key="i">
                <span class="cl__steptitle">{{ step.title }}</span>
                <MarkdownText flow class="cl__stepdetail md" :source="step.detail" />
              </li>
            </ol>

            <p class="cl__note">
              <font-awesome-icon :icon="['fas', 'circle-info']" /> Custom-repo plugins don’t
              auto-update on their own — re-open <code>/xlplugins</code> to pull a new version when
              one is released.
            </p>
          </section>

          <!-- Version entry -->
          <article v-else-if="currentEntry" class="cl__entry">
            <h4 class="cl__detailtitle">
              v{{ currentEntry.version }}
              <span v-if="currentEntry.date" class="cl__entrydate">{{ currentEntry.date }}</span>
            </h4>

            <MarkdownText
              v-if="currentEntry.intro"
              flow
              class="cl__lead md"
              :source="currentEntry.intro"
            />

            <section v-for="(g, gi) in currentEntry.groups" :key="gi" class="cl__group">
              <span class="cl-badge" :class="badgeClass(g.label)">{{ g.label }}</span>
              <MarkdownText flow class="cl__grouplist md" :source="g.body" />
            </section>
          </article>

          <p v-else class="cl__empty">No changelog available.</p>
        </div>
      </div>

      <footer class="cl__foot">
        <button class="btn-neutral" @click="$emit('close')">Close</button>
      </footer>
    </div>
  </ModalOverlay>
</template>

<style scoped>
.cl {
  display: flex;
  flex-direction: column;
  min-height: 0;
  max-height: 80vh;
}

/* Header */
.cl__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--control-border);
}
.cl__title {
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}
.cl__latest {
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-muted);
  white-space: nowrap;
}

/* Two-pane body */
.cl__body {
  display: flex;
  gap: 14px;
  min-height: 0;
  flex: 1;
  padding: 12px 0;
}

/* Left rail */
.cl__rail {
  flex: 0 0 172px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  overflow-y: auto;
  padding-right: 8px;
  border-right: 1px solid var(--control-border);
}
.cl__railitem {
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
.cl__railitem:hover {
  background: var(--panel-raised-bg);
}
.cl__railitem.is-active {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--text-on-accent);
}
.cl__railitem--install {
  flex-direction: row;
  align-items: center;
  gap: 7px;
  font-weight: 600;
}
.cl__railver {
  font-family: 'Consolas', 'Monaco', monospace;
  font-weight: 600;
  font-size: 0.86rem;
}
.cl__raildate {
  font-size: 0.68rem;
  opacity: 0.75;
}
.cl__raildiv {
  margin: 8px 4px 3px;
  font-size: 0.66rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-muted);
}

/* Right detail */
.cl__detail {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding-right: 6px;
}
.cl__detailtitle {
  margin: 0 0 10px;
  display: flex;
  align-items: baseline;
  gap: 10px;
  font-size: 1.15rem;
}
.cl__entrydate,
.cl__note {
  font-size: 0.78rem;
  color: var(--text-muted);
  font-weight: 400;
}
.cl__lead {
  color: var(--text-muted);
  margin-bottom: 16px;
  line-height: 1.55;
}

/* Change-group: badge + list */
.cl__group {
  margin-bottom: 16px;
}
.cl-badge {
  display: inline-block;
  margin-bottom: 6px;
  padding: 1px 9px;
  border: 1px solid currentColor;
  border-radius: 999px;
  background: color-mix(in srgb, currentColor 14%, transparent);
  font-size: 0.7rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.cl-badge--added {
  color: #3fb950;
}
.cl-badge--fixed {
  color: #d29922;
}
.cl-badge--changed {
  color: #539bf5;
}
.cl-badge--security {
  color: #db61a2;
}
.cl-badge--removed {
  color: #f85149;
}
.cl-badge--deprecated,
.cl-badge--default {
  color: var(--text-muted);
}

/* Install view */
.cl__repo {
  margin-bottom: 16px;
  padding: 10px 12px;
  border: 1px solid var(--control-border);
  border-radius: var(--radius);
  background: var(--panel-raised-bg);
}
.cl__repolabel {
  display: block;
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-muted);
  margin-bottom: 6px;
}
.cl__reporow {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.cl__repourl {
  flex: 1;
  min-width: 220px;
  padding: 6px 8px;
  border-radius: 6px;
  background: var(--panel-bg);
  font-size: 0.82rem;
  word-break: break-all;
}
.cl__copy {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.cl__steps {
  margin: 0 0 14px;
  padding-left: 22px;
  display: flex;
  flex-direction: column;
  gap: 9px;
}
.cl__steps li {
  line-height: 1.5;
}
.cl__steptitle {
  font-weight: 600;
}
.cl__stepdetail {
  color: var(--text-muted);
}
.cl__note {
  padding-top: 6px;
}
.cl__empty {
  color: var(--text-muted);
}

/* Footer */
.cl__foot {
  display: flex;
  justify-content: flex-end;
  padding-top: 10px;
  border-top: 1px solid var(--control-border);
}

/* ── Rendered-markdown typography (v-html output) ─────────────────────────── */
.md :deep(p) {
  margin: 0 0 8px;
  line-height: 1.55;
}
.md :deep(p:last-child) {
  margin-bottom: 0;
}
.cl__grouplist.md :deep(ul) {
  margin: 0;
  padding-left: 20px;
  display: flex;
  flex-direction: column;
  gap: 7px;
}
.cl__grouplist.md :deep(li) {
  line-height: 1.55;
}
.cl__grouplist.md :deep(ul ul) {
  margin-top: 6px;
  gap: 4px;
}
.md :deep(code) {
  padding: 1px 5px;
  border-radius: 5px;
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
  .cl__body {
    flex-direction: column;
  }
  .cl__rail {
    flex: 0 0 auto;
    flex-direction: row;
    max-height: none;
    overflow-x: auto;
    overflow-y: hidden;
    padding: 0 0 8px;
    border-right: none;
    border-bottom: 1px solid var(--control-border);
  }
  .cl__railitem {
    width: auto;
    white-space: nowrap;
  }
  .cl__raildiv {
    display: none;
  }
}
</style>
