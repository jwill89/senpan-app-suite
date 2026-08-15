<script setup lang="ts">
/**
 * Structured theme editor: one row per design token (a colour swatch + value
 * field), grouped by role, with a collapsible live preview that exercises every
 * token (surfaces, text, buttons, status, the bingo board, and a modal/shadow).
 * A theme is just a set of token overrides - there is no free-form CSS - so this
 * is the whole editor. The bound value is a `{ tokenName: cssValue }` map.
 */
import { computed, nextTick, ref } from 'vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import ColorPicker from '@/components/common/ui/ColorPicker.vue'
import {
  THEME_TOKEN_GROUPS,
  THEME_TOKENS,
  toHex,
  toHex8,
  toRgb,
  withDefaults as withTokenDefaults,
  type Rgba,
  type ThemeTokenMeta,
} from '@/lib/theme-tokens'
import { auditTheme } from '@/lib/wcag'

const props = defineProps<{ modelValue: Record<string, string> }>()
const emit = defineEmits<{ 'update:modelValue': [Record<string, string>] }>()

/** Preview is collapsed by default - opt-in so the editor stays compact. */
const showPreview = ref(false)

/** Current value for a token, falling back to its default. */
function valueOf(t: ThemeTokenMeta): string {
  return props.modelValue[t.name] ?? t.default
}

/** Emit an updated copy of the token map with one token changed. */
function setToken(name: string, value: string): void {
  emit('update:modelValue', { ...props.modelValue, [name]: value })
}

/** Parses any CSS colour (hex, rgb/rgba, modern slash form, named) to RGBA via
 *  the browser, so the swatch + opacity slider can read existing values in any
 *  format an admin may have saved.
 *
 *  Memoized by input string: `swatchHex` runs this per token in the template, so
 *  an unmemoized version forced ~one layout reflow per token on every re-render
 *  (i.e. every keystroke/colour-drag). Re-renders reuse the same value strings,
 *  so the cache makes them free; only a newly-typed value touches the DOM. */
const colorCache = new Map<string, Rgba>()

function parseColor(input: string): Rgba {
  const cached = colorCache.get(input)
  if (cached) return cached
  let result: Rgba = { r: 0, g: 0, b: 0, a: 1 }
  const el = document.createElement('div')
  el.style.color = input
  if (el.style.color) {
    document.body.appendChild(el)
    const m = getComputedStyle(el).color.match(/[\d.]+/g)
    el.remove()
    if (m && m.length >= 3) {
      result = { r: +m[0], g: +m[1], b: +m[2], a: m.length > 3 ? +m[3] : 1 }
    }
  }
  colorCache.set(input, result)
  return result
}

/** Opaque #rrggbb of a solid token's value, for its native colour input. */
function swatchHex(t: ThemeTokenMeta): string {
  return toHex(parseColor(valueOf(t)))
}

// Alpha tokens (modal backdrop, shadow, glow) need a colour picker with an alpha
// channel. The native <input type="color" alpha> is too new to rely on (Firefox
// / older mobile lack it), so they open the cross-browser Chrome picker instead.
const pickerToken = ref<ThemeTokenMeta | null>(null)

/** Seed value for the Chrome picker: 8-digit #rrggbbaa carries colour + alpha. */
function pickerSeed(t: ThemeTokenMeta): string {
  return toHex8(parseColor(valueOf(t)))
}

/** Apply a Chrome-picker change, storing modern rgb(r g b / a%) (the picker
 *  emits a legacy rgba() string, which we normalise). */
function onPickerChange(t: ThemeTokenMeta | null, payload: { rgba: string }): void {
  if (t) setToken(t.name, toRgb(parseColor(payload.rgba)))
}

/** Inline custom-property style so the preview reflects the edited tokens. */
const previewStyle = computed(() => {
  const s: Record<string, string> = {}
  for (const t of THEME_TOKENS) s[`--${t.name}`] = valueOf(t)
  return s
})

// -- WCAG compliance report ---------------------------------------------------
const showReport = ref(false)
/** When true, the report lists every check, not just the problems. */
const showAllChecks = ref(false)
/** Live audit of the edited theme (tokens merged over the defaults). Recomputes
 *  on every token edit, so the verdict + findings update as colours change. */
const report = computed(() => auditTheme(withTokenDefaults(props.modelValue)))
const verdictLabel = computed(() =>
  report.value.level === 'AAA' ? 'WCAG AAA' : report.value.level === 'AA' ? 'WCAG AA' : 'Below AA',
)
const fmtRatio = (r: number) => `${r.toFixed(2)}:1`
/** "text-on-accent -> board-cell-bg" style token trail for a finding. */
const tokenTrail = (fg: string, bg: string) => `${fg.startsWith('#') ? fg : `--${fg}`} -> --${bg}`

// "Find in preview": open the preview and flash the element a pairing renders as.
const stageRef = ref<HTMLElement | null>(null)
async function revealInPreview(id: string): Promise<void> {
  showPreview.value = true
  await nextTick()
  const el = stageRef.value?.querySelector<HTMLElement>(`[data-pair~="${id}"]`)
  if (!el) return
  el.scrollIntoView({ block: 'center', behavior: 'smooth' })
  el.classList.remove('theme-preview-flash')
  // Reflow so re-adding the class restarts the animation on repeat clicks.
  void el.offsetWidth
  el.classList.add('theme-preview-flash')
  window.setTimeout(() => el.classList.remove('theme-preview-flash'), 1600)
}
</script>

<template>
  <div class="token-editor">
    <!-- Collapsible live preview: representative chrome painted with the edited
         tokens. Hidden by default; expand to "feel" a theme before saving. -->
    <div class="token-preview">
      <button
        type="button"
        class="collapse-toggle"
        :aria-expanded="showPreview"
        @click="showPreview = !showPreview"
      >
        <font-awesome-icon
          :icon="['fas', showPreview ? 'chevron-up' : 'chevron-down']"
          fixed-width
        />
        <span>Live preview</span>
        <span class="token-preview-hint"
          >- hover the buttons &amp; board cells to see hover colours</span
        >
      </button>

      <div v-show="showPreview" ref="stageRef" class="token-preview-stage" :style="previewStyle">
        <div class="theme-preview-grid">
          <!-- Surfaces & text: the real panel object, real body/muted/link
               inheritance, and a real nested .subpanel. -->
          <section class="admin-panel theme-preview-panel">
            <h5 class="section-heading" data-pair="heading-panel heading-page">
              Heading &amp; highlight
            </h5>
            <p data-pair="body-panel body-page">Primary body text on a panel surface.</p>
            <p class="text-muted text-sm" data-pair="muted-panel muted-page">
              Muted secondary text.
            </p>
            <p>
              A themed
              <a href="#" data-pair="link-panel link-page" @click.prevent>hyperlink</a>
              in a sentence.
            </p>
            <div class="subpanel" data-pair="body-raised muted-raised heading-raised">
              <h5 class="section-heading">Raised surface</h5>
              <p>Rows, chips and nested panels.</p>
              <p class="text-muted text-sm">Secondary text on the raised surface.</p>
            </div>
            <input
              type="text"
              value="Input field"
              readonly
              aria-label="Sample input"
              data-pair="input-text placeholder"
            />
          </section>

          <!-- Buttons & status: the six real button intents, real badges and the
               real winner chip. Hover is live because the intents ship :hover. -->
          <section class="admin-panel theme-preview-panel">
            <h5 class="section-heading">Buttons</h5>
            <div class="form-actions">
              <button
                type="button"
                class="btn-action btn-sm"
                data-pair="primary-btn primary-btn-hover"
              >
                Primary
              </button>
              <button
                type="button"
                class="btn-view btn-sm"
                data-pair="secondary-btn secondary-btn-hover"
              >
                Secondary
              </button>
              <button type="button" class="btn-neutral btn-sm" data-pair="neutral-btn">
                Cancel
              </button>
            </div>
            <div class="form-actions">
              <button type="button" class="btn-confirm btn-sm" data-pair="success-btn">Save</button>
              <button type="button" class="btn-danger btn-sm" data-pair="danger-btn">Delete</button>
              <button type="button" class="btn-caution btn-sm" data-pair="caution-btn">Skip</button>
            </div>
            <div class="flex-row">
              <span class="badge badge--success">Paid</span>
              <span class="badge badge--danger">Error</span>
              <span class="badge badge--warning">Skip</span>
            </div>
            <div class="winner-chips">
              <span class="winner-chip" data-pair="winner-chip">ABC123</span>
            </div>
          </section>

          <!-- Modal & backdrop: the real dialog box over a contained stand-in for
               the overlay, which is position:fixed and would cover the editor. -->
          <section class="theme-preview-overlay">
            <div class="modal-box">
              <h3>Modal dialog</h3>
              <p class="confirm-msg">Floats above a dimmed backdrop.</p>
              <div class="confirm-btns">
                <button type="button" class="btn-neutral">Cancel</button>
                <button type="button" class="btn-action">Confirm</button>
              </div>
            </div>
          </section>

          <!-- Bingo board & called numbers: the real board objects at real size
               (spans the grid so the cells fit) plus the real tracker. The
               pattern-hit cell is the one carrier of --highlight-glow. -->
          <section class="admin-panel theme-preview-panel theme-preview-panel--wide">
            <h5 class="section-heading">Bingo board</h5>
            <div class="board-wrap theme-preview-board">
              <div class="board-header" data-pair="bingo-top bingo-bottom">
                <span>B</span><span>I</span><span>N</span><span>G</span><span>O</span>
              </div>
              <div class="board-grid">
                <div class="board-cell" data-pair="board-num board-num-hover">
                  <span class="cell-num">7</span>
                </div>
                <div class="board-cell"><span class="cell-num">23</span></div>
                <div class="board-cell is-free" data-pair="free-num">
                  <span class="cell-num">FREE</span>
                </div>
                <div class="board-cell pattern-hit"><span class="cell-num">52</span></div>
                <div class="board-cell"><span class="cell-num">68</span></div>
              </div>
            </div>
            <div class="numbers-cols theme-preview-tracker">
              <div class="numbers-col">
                <div class="numbers-col-header">B</div>
                <div class="num-cell">3</div>
              </div>
              <div class="numbers-col">
                <div class="numbers-col-header">I</div>
                <div class="num-cell">19</div>
              </div>
              <div class="numbers-col numbers-col--unused">
                <div class="numbers-col-header">N</div>
                <div class="num-cell">31</div>
              </div>
              <div class="numbers-col">
                <div class="numbers-col-header">G</div>
                <div class="num-cell is-called" data-pair="called-num">52</div>
              </div>
              <div class="numbers-col">
                <div class="numbers-col-header">O</div>
                <div class="num-cell">68</div>
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>

    <!-- WCAG compliance report: live audit of the edited theme's contrast. -->
    <div class="wcag" :class="`wcag--${report.level}`">
      <button
        type="button"
        class="collapse-toggle"
        :aria-expanded="showReport"
        @click="showReport = !showReport"
      >
        <font-awesome-icon
          :icon="['fas', showReport ? 'chevron-up' : 'chevron-down']"
          fixed-width
        />
        <span>Check WCAG compliance</span>
        <span class="wcag-verdict">{{ verdictLabel }}</span>
      </button>
      <div v-show="showReport" class="wcag-body">
        <p class="wcag-summary">
          <template v-if="report.level === 'AAA'"
            >Every one of the {{ report.results.length }} text pairings meets WCAG 2.1
            <strong>AAA</strong> (the strictest level).</template
          >
          <template v-else-if="report.level === 'AA'"
            >Meets WCAG 2.1 <strong>AA</strong>.
            <strong>{{ report.warnings.length }}</strong> pairing(s) are readable but fall short of
            AAA (7:1 for normal text).</template
          >
          <template v-else
            ><strong>{{ report.errors.length }}</strong> pairing(s) fail WCAG 2.1 AA - these are
            hard to read and should be fixed.</template
          >
        </p>

        <!-- Findings: errors first, then AA-only warnings, then (optionally) passes. -->
        <div v-if="report.errors.length" class="wcag-group">
          <h5 class="wcag-heading wcag-heading--error">
            <font-awesome-icon :icon="['fas', 'circle-xmark']" /> Fails AA - fix these ({{
              report.errors.length
            }})
          </h5>
          <ul class="list-stack">
            <li
              v-for="r in report.errors"
              :key="r.id"
              class="wcag-finding"
              :class="`wcag-finding--${r.status}`"
            >
              <span
                class="wcag-chip"
                :style="{ background: r.bgColor, color: r.fgColor }"
                :title="`${r.fgColor} on ${r.bgColor}`"
                >{{ r.sample }}</span
              >
              <span class="wcag-info">
                <span class="wcag-pair">{{ r.label }}</span>
                <span class="wcag-where">{{ r.where }}</span>
                <span class="wcag-tokens">{{ tokenTrail(r.fg, r.bg) }}</span>
              </span>
              <span class="wcag-metrics">
                <span class="wcag-ratio wcag-ratio--error">{{ fmtRatio(r.ratio) }}</span>
                <span class="wcag-levels">
                  <span class="wcag-level wcag-level--off">AA {{ r.aaTarget }}:1</span>
                  <span class="wcag-level wcag-level--off">AAA {{ r.aaaTarget }}:1</span>
                </span>
                <span v-if="r.large" class="wcag-large">large text</span>
              </span>
              <button type="button" class="wcag-find" @click="revealInPreview(r.id)">
                <font-awesome-icon :icon="['fas', 'magnifying-glass']" /> Find
              </button>
            </li>
          </ul>
        </div>

        <div v-if="report.warnings.length" class="wcag-group">
          <h5 class="wcag-heading wcag-heading--warn">
            <font-awesome-icon :icon="['fas', 'triangle-exclamation']" /> Passes AA, short of AAA
            ({{ report.warnings.length }})
          </h5>
          <ul class="list-stack">
            <li
              v-for="r in report.warnings"
              :key="r.id"
              class="wcag-finding"
              :class="`wcag-finding--${r.status}`"
            >
              <span
                class="wcag-chip"
                :style="{ background: r.bgColor, color: r.fgColor }"
                :title="`${r.fgColor} on ${r.bgColor}`"
                >{{ r.sample }}</span
              >
              <span class="wcag-info">
                <span class="wcag-pair">{{ r.label }}</span>
                <span class="wcag-where">{{ r.where }}</span>
                <span class="wcag-tokens">{{ tokenTrail(r.fg, r.bg) }}</span>
              </span>
              <span class="wcag-metrics">
                <span class="wcag-ratio wcag-ratio--warn">{{ fmtRatio(r.ratio) }}</span>
                <span class="wcag-levels">
                  <span class="wcag-level wcag-level--on">AA</span>
                  <span class="wcag-level wcag-level--off">AAA {{ r.aaaTarget }}:1</span>
                </span>
                <span v-if="r.large" class="wcag-large">large text</span>
              </span>
              <button type="button" class="wcag-find" @click="revealInPreview(r.id)">
                <font-awesome-icon :icon="['fas', 'magnifying-glass']" /> Find
              </button>
            </li>
          </ul>
        </div>

        <!-- Full list of every check (passes included), opt-in to avoid clutter. -->
        <button type="button" class="wcag-show-all" @click="showAllChecks = !showAllChecks">
          <font-awesome-icon
            :icon="['fas', showAllChecks ? 'chevron-up' : 'chevron-down']"
            fixed-width
          />
          {{ showAllChecks ? 'Hide' : 'Show' }} all {{ report.results.length }} checks
        </button>
        <ul v-if="showAllChecks" class="list-stack">
          <li
            v-for="r in report.results"
            :key="r.id"
            class="wcag-finding"
            :class="`wcag-finding--${r.status}`"
          >
            <span
              class="wcag-chip"
              :style="{ background: r.bgColor, color: r.fgColor }"
              :title="`${r.fgColor} on ${r.bgColor}`"
              >{{ r.sample }}</span
            >
            <span class="wcag-info">
              <span class="wcag-pair">{{ r.label }}</span>
              <span class="wcag-where">{{ r.where }}</span>
              <span class="wcag-tokens">{{ tokenTrail(r.fg, r.bg) }}</span>
            </span>
            <span class="wcag-metrics">
              <span
                class="wcag-ratio"
                :class="{
                  'wcag-ratio--error': r.status === 'fail',
                  'wcag-ratio--warn': r.status === 'aa',
                  'wcag__ratio--ok': r.status === 'aaa',
                }"
                >{{ fmtRatio(r.ratio) }}</span
              >
              <span class="wcag-levels">
                <span class="wcag-level" :class="r.aaPass ? 'wcag-level--on' : 'wcag-level--off'"
                  >AA</span
                >
                <span class="wcag-level" :class="r.aaaPass ? 'wcag-level--on' : 'wcag-level--off'"
                  >AAA</span
                >
              </span>
              <span v-if="r.large" class="wcag-large">large text</span>
            </span>
            <button type="button" class="wcag-find" @click="revealInPreview(r.id)">
              <font-awesome-icon :icon="['fas', 'magnifying-glass']" /> Find
            </button>
          </li>
        </ul>

        <p class="wcag-count">
          {{ report.passes.length }} of {{ report.results.length }} pairings meet AAA - updates live
          as you edit colours.
        </p>
      </div>
    </div>

    <div v-for="group in THEME_TOKEN_GROUPS" :key="group.title">
      <h4 class="token-group-title">{{ group.title }}</h4>
      <div class="token-rows">
        <div v-for="t in group.tokens" :key="t.name" class="token-row">
          <div class="token-row-head">
            <span class="token-row-swatch" :style="{ background: valueOf(t) }" aria-hidden="true" />
            <label :for="`tok-${t.name}`" class="token-row-label">{{ t.label }}</label>
            <!-- Solid tokens: lightweight native picker. Alpha tokens: a swatch
                 button that opens the cross-browser Chrome picker (with alpha). -->
            <input
              v-if="!t.alpha"
              type="color"
              class="token-row-color"
              :value="swatchHex(t)"
              :aria-label="`${t.label} colour`"
              @input="setToken(t.name, ($event.target as HTMLInputElement).value)"
            />
            <button
              v-else
              type="button"
              class="token-row-color token-row-color--btn"
              :style="{ background: valueOf(t) }"
              :aria-label="`Choose ${t.label} colour and opacity`"
              @click="pickerToken = t"
            />
            <input
              :id="`tok-${t.name}`"
              class="token-row-value"
              :value="valueOf(t)"
              spellcheck="false"
              :placeholder="t.default"
              :aria-describedby="`tok-${t.name}-desc`"
              @input="setToken(t.name, ($event.target as HTMLInputElement).value)"
            />
          </div>
          <p :id="`tok-${t.name}-desc`" class="token-row-desc">{{ t.desc }}</p>
        </div>
      </div>
    </div>

    <!-- Alpha-token colour picker (cross-browser alpha via the Chrome picker). -->
    <ModalOverlay
      v-if="pickerToken"
      centered
      aria-label="Theme colour picker"
      :box-style="{ maxWidth: '340px' }"
      @close="pickerToken = null"
    >
      <h3 class="mb-8"><font-awesome-icon :icon="['fad', 'palette']" /> {{ pickerToken.label }}</h3>
      <p class="text-muted text-sm mb-16">{{ pickerToken.desc }}</p>
      <ColorPicker :value="pickerSeed(pickerToken)" @change="onPickerChange(pickerToken, $event)" />
      <button class="btn-neutral mt-20 w-full" @click="pickerToken = null">Done</button>
    </ModalOverlay>
  </div>
</template>

<style scoped>
.token-editor {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* -- Collapsible preview ---------------------------------------------------- */
.token-preview {
  border: 1px solid var(--control-border);
  border-radius: var(--radius);
  overflow: hidden;
}
.token-preview-hint {
  color: var(--text-muted);
  font-weight: 400;
  font-size: 0.8rem;
}
@media (max-width: 560px) {
  .token-preview-hint {
    display: none;
  }
}

.token-preview-stage {
  background: var(--page-bg);
  color: var(--text);
  padding: 16px;
}
.theme-preview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 14px;
  align-items: start;
}

/* Containment only. Everything below sets GEOMETRY - never colour. The preview
   is built from the app's own objects (.admin-panel, .btn-*, .badge, .board-*,
   .num-cell, .modal-box) so that what an editor sees IS what the app renders;
   re-declaring a colour here would rebuild the replica this replaced.
   The stage's `color` is the one exception and is load-bearing: an inherited
   colour is an already-resolved value, so a bare <p> inheriting from the real
   <body> would paint the SAVED theme's --text. Declared here it resolves against
   the stage's own inline custom properties (previewStyle) and every descendant -
   including .subpanel and .modal-box, which declare no colour - inherits the
   EDITED token. */
.theme-preview-panel {
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
/* The board is ~396px at the real 68px cell, wider than one grid track. */
.theme-preview-panel--wide {
  grid-column: 1 / -1;
}
/* .section-heading ships an 8px bottom margin for margin-spaced panels; this
   panel is a flex column with its own gap. Child combinator on purpose - the
   .section-heading inside .subpanel keeps the real margin. */
.theme-preview-panel > .section-heading {
  margin-bottom: 0;
}

/* .board-wrap is inline-block in the app; as a flex item it would stretch and
   leave the cells adrift in oversized tracks. */
.theme-preview-board {
  align-self: flex-start;
}
/* The cell sizes itself off the VIEWPORT (player.css: min(68px, calc((100vw -
   100px) / 5))), which assumes the board sits near the page edges. Here it is
   nested admin-panel > token-preview > stage > panel, so the same formula needs a
   wider allowance; the desktop 68px is unchanged. */
.theme-preview-board .board-cell {
  --cell-size: min(68px, calc((100vw - 220px) / 5));
}
.theme-preview-tracker {
  margin-top: 12px;
}

/* Contained stand-in for .modal-overlay, which is position:fixed / inset:0 /
   z-index:500 and would cover the whole editor. Only the --modal-overlay token
   is borrowed; the dialog itself is the real .modal-box. */
.theme-preview-overlay {
  background: var(--modal-overlay);
  border-radius: var(--radius);
  padding: 18px;
  display: grid;
  place-items: center;
  min-height: 160px;
}
/* Real dialogs are >=440px wide; in a grid track this narrow the two full-size
   dialog buttons must be allowed to wrap. */
.theme-preview-overlay .confirm-btns {
  flex-wrap: wrap;
}

/* -- Token rows ------------------------------------------------------------- */
.token-group-title {
  margin: 0 0 8px;
  color: var(--highlight);
  font-size: 0.95rem;
}
.token-rows {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px 18px;
}
.token-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.token-row-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.token-row-desc {
  margin: 0;
  color: var(--text-muted);
  font-size: 0.78rem;
  line-height: 1.35;
}
.token-row-swatch {
  width: 18px;
  height: 18px;
  border-radius: 0;
  border: 1px solid var(--control-border);
  flex: 0 0 auto;
}
.token-row-label {
  flex: 1 1 auto;
  font-size: 0.85rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.token-row-color {
  width: 34px;
  height: 28px;
  padding: 2px;
  border: 1px solid var(--control-border);
  border-radius: 0;
  background: var(--panel-bg);
  cursor: pointer;
  flex: 0 0 auto;
}
/* Alpha-token swatch button: the chosen colour (incl. transparency) fills it. */
.token-row-color--btn {
  padding: 0;
}
.token-row-value {
  width: 120px;
  flex: 0 0 auto;
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 0.8rem;
  padding: 5px 7px;
}

/* "Find in preview" flash - a pulsing outline drawing the eye to the element. */
.theme-preview-flash {
  animation: theme-preview-flash 1.6s ease-out;
  border-radius: 0;
}
@keyframes theme-preview-flash {
  0%,
  100% {
    outline: 2px solid transparent;
    outline-offset: 2px;
  }
  15%,
  55% {
    outline: 2px solid var(--accent);
    outline-offset: 3px;
    box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent) 35%, transparent);
  }
}

/* -- WCAG compliance report ------------------------------------------------ */
.wcag {
  border: 1px solid var(--control-border);
  border-radius: var(--radius);
  overflow: hidden;
}
.collapse-toggle {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  text-align: left;
  background: var(--panel-raised-bg);
  color: var(--text);
  border: none;
  padding: 10px 12px;
  font-weight: 600;
  cursor: pointer;
}
.wcag-verdict {
  margin-left: auto;
  font-size: 0.78rem;
  font-weight: 700;
  padding: 2px 10px;
  border-radius: 0;
  color: var(--text-on-fill);
}
.wcag--AAA .wcag-verdict {
  background: var(--success);
}
.wcag--AA .wcag-verdict {
  background: var(--warning);
  color: #1f1a06;
}
.wcag--fail .wcag-verdict {
  background: var(--danger);
}
.wcag-body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.wcag-summary {
  margin: 0;
  font-size: 0.88rem;
  color: var(--text);
}
.wcag-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.wcag-heading {
  margin: 0;
  font-size: 0.82rem;
}
.wcag-heading--error {
  color: var(--danger);
}
.wcag-heading--warn {
  color: var(--warning);
}
/* A single finding row: live chip - description - metrics - find button. */
.wcag-finding {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 8px;
  border-radius: 0;
  background: var(--panel-raised-bg);
  border-left: 3px solid var(--control-border);
}
.wcag-finding--fail {
  border-left-color: var(--danger);
}
.wcag-finding--aa {
  border-left-color: var(--warning);
}
.wcag-finding--aaa {
  border-left-color: var(--success);
}

/* Live contrast chip: shows the actual foreground on the actual background, so
   the reviewer sees exactly how legible (or not) the pairing is. */
.wcag-chip {
  flex: 0 0 auto;
  width: 38px;
  height: 38px;
  display: grid;
  place-items: center;
  border-radius: 0;
  border: 1px solid color-mix(in srgb, var(--text) 25%, transparent);
  font-weight: 800;
  font-size: 0.95rem;
  line-height: 1;
}

.wcag-info {
  flex: 1 1 auto;
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}
.wcag-pair {
  font-size: 0.84rem;
  font-weight: 600;
  color: var(--text);
}
.wcag-where {
  font-size: 0.74rem;
  color: var(--text-muted);
}
.wcag-tokens {
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 0.7rem;
  color: var(--text-muted);
  opacity: 0.85;
}

.wcag-metrics {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
}
.wcag-ratio {
  font-family: 'Consolas', 'Monaco', monospace;
  font-weight: 700;
  font-size: 0.82rem;
  /* Default to body text so the number is always legible on the active admin
     theme; status is conveyed by the row border, the AA/AAA pills, and (for
     problems) the brighter danger/warning tints below. */
  color: var(--text);
}
.wcag-ratio--error {
  color: var(--danger);
}
.wcag-ratio--warn {
  color: var(--warning);
}
.wcag-levels {
  display: flex;
  gap: 4px;
}
.wcag-level {
  font-size: 0.62rem;
  font-weight: 700;
  padding: 1px 5px;
  border-radius: 0;
  white-space: nowrap;
}
.wcag-level--on {
  background: color-mix(in srgb, var(--success) 24%, transparent);
  color: var(--text);
}
.wcag-level--off {
  background: color-mix(in srgb, var(--danger) 18%, transparent);
  color: var(--text-muted);
}
.wcag-large {
  font-size: 0.62rem;
  color: var(--text-muted);
}

.wcag-find {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: var(--control-border);
  color: var(--text);
  border: none;
  border-radius: 0;
  padding: 5px 9px;
  font-size: 0.74rem;
  font-weight: 600;
  cursor: pointer;
}
.wcag-find:hover {
  background: color-mix(in srgb, var(--control-border) 82%, var(--text));
}

.wcag-show-all {
  align-self: flex-start;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: none;
  border: none;
  color: var(--accent);
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  padding: 2px 0;
}

.wcag-count {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

/* On narrow widths, let the finding row wrap so the chip + text + metrics stack
   instead of crushing. */
@media (max-width: 560px) {
  .wcag-finding {
    flex-wrap: wrap;
  }
  .wcag-metrics {
    align-items: flex-start;
  }
}
</style>
