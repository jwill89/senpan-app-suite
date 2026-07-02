<script setup lang="ts">
/**
 * Structured theme editor: one row per design token (a colour swatch + value
 * field), grouped by role, with a collapsible live preview that exercises every
 * token (surfaces, text, buttons, status, the bingo board, and a modal/shadow).
 * A theme is just a set of token overrides — there is no free-form CSS — so this
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

/** Preview is collapsed by default — opt-in so the editor stays compact. */
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

// ── WCAG compliance report ───────────────────────────────────────────────────
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
/** "text-on-accent → board-cell-bg" style token trail for a finding. */
const tokenTrail = (fg: string, bg: string) => `${fg.startsWith('#') ? fg : `--${fg}`} → --${bg}`

// "Find in preview": open the preview and flash the element a pairing renders as.
const stageRef = ref<HTMLElement | null>(null)
async function revealInPreview(id: string): Promise<void> {
  showPreview.value = true
  await nextTick()
  const el = stageRef.value?.querySelector<HTMLElement>(`[data-pair~="${id}"]`)
  if (!el) return
  el.scrollIntoView({ block: 'center', behavior: 'smooth' })
  el.classList.remove('tp-flash')
  // Reflow so re-adding the class restarts the animation on repeat clicks.
  void el.offsetWidth
  el.classList.add('tp-flash')
  window.setTimeout(() => el.classList.remove('tp-flash'), 1600)
}
</script>

<template>
  <div class="token-editor">
    <!-- Collapsible live preview: representative chrome painted with the edited
         tokens. Hidden by default; expand to "feel" a theme before saving. -->
    <div class="token-preview">
      <button
        type="button"
        class="token-preview__toggle"
        :aria-expanded="showPreview"
        @click="showPreview = !showPreview"
      >
        <font-awesome-icon
          :icon="['fas', showPreview ? 'chevron-up' : 'chevron-down']"
          fixed-width
        />
        <span>Live preview</span>
        <span class="token-preview__hint"
          >— hover the buttons &amp; board cells to see hover colours</span
        >
      </button>

      <div v-show="showPreview" ref="stageRef" class="token-preview__stage" :style="previewStyle">
        <div class="tp-grid">
          <!-- Surfaces & text -->
          <section class="tp-panel">
            <h5 class="tp-h" data-pair="heading-panel heading-page">Heading &amp; highlight</h5>
            <p class="tp-text" data-pair="body-panel body-page">
              Primary body text on a panel surface.
            </p>
            <p class="tp-muted" data-pair="muted-panel muted-page">Muted secondary text.</p>
            <p class="tp-text">
              A themed
              <a href="#" class="tp-link" data-pair="link-panel link-page" @click.prevent
                >hyperlink</a
              >
              in a sentence.
            </p>
            <div class="tp-raised" data-pair="body-raised muted-raised heading-raised">
              Raised surface — rows, chips, nested panels
            </div>
            <input
              class="tp-input"
              type="text"
              value="Input field"
              readonly
              aria-label="Sample input"
              data-pair="input-text placeholder"
            />
          </section>

          <!-- Buttons & status -->
          <section class="tp-panel">
            <h5 class="tp-h">Buttons</h5>
            <div class="tp-row">
              <button
                type="button"
                class="tp-btn tp-btn--primary"
                data-pair="primary-btn primary-btn-hover"
              >
                Primary
              </button>
              <button
                type="button"
                class="tp-btn tp-btn--secondary"
                data-pair="secondary-btn secondary-btn-hover"
              >
                Secondary
              </button>
              <button type="button" class="tp-btn tp-btn--neutral" data-pair="neutral-btn">
                Cancel
              </button>
            </div>
            <div class="tp-row">
              <button type="button" class="tp-btn tp-btn--success" data-pair="success-btn">
                Success
              </button>
              <button type="button" class="tp-btn tp-btn--danger" data-pair="danger-btn">
                Danger
              </button>
              <button type="button" class="tp-btn tp-btn--warning" data-pair="caution-btn">
                Warning
              </button>
            </div>
            <div class="tp-row">
              <span class="tp-badge tp-badge--success">Paid</span>
              <span class="tp-badge tp-badge--danger">Error</span>
              <span class="tp-badge tp-badge--warning">Skip</span>
            </div>
            <span class="tp-winner" data-pair="winner-chip">🏆 Winner: Player 12</span>
          </section>

          <!-- Bingo board -->
          <section class="tp-panel">
            <h5 class="tp-h">Bingo board</h5>
            <div class="tp-board">
              <div class="tp-board__row tp-board__head" data-pair="bingo-top bingo-bottom">
                <span>B</span><span>I</span><span>N</span><span>G</span><span>O</span>
              </div>
              <div class="tp-board__row">
                <span class="tp-cell" data-pair="board-num board-num-hover">7</span>
                <span class="tp-cell">23</span>
                <span class="tp-cell tp-cell--free" data-pair="free-num">★</span>
                <span class="tp-cell">52</span>
                <span class="tp-cell">68</span>
              </div>
            </div>
            <div class="tp-called" data-pair="called-num">
              <span class="tp-called__label">G</span><span class="tp-called__num">52</span>
            </div>
          </section>

          <!-- Modal & shadow -->
          <section class="tp-overlay">
            <div class="tp-modal">
              <h5 class="tp-h">Modal dialog</h5>
              <p class="tp-muted">Floats above a dimmed backdrop with a drop shadow.</p>
              <div class="tp-row tp-row--end">
                <button type="button" class="tp-btn tp-btn--secondary">Cancel</button>
                <button type="button" class="tp-btn tp-btn--primary">Confirm</button>
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
        class="wcag__toggle"
        :aria-expanded="showReport"
        @click="showReport = !showReport"
      >
        <font-awesome-icon
          :icon="['fas', showReport ? 'chevron-up' : 'chevron-down']"
          fixed-width
        />
        <span>Check WCAG compliance</span>
        <span class="wcag__verdict">{{ verdictLabel }}</span>
      </button>
      <div v-show="showReport" class="wcag__body">
        <p class="wcag__summary">
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
            ><strong>{{ report.errors.length }}</strong> pairing(s) fail WCAG 2.1 AA — these are
            hard to read and should be fixed.</template
          >
        </p>

        <!-- Findings: errors first, then AA-only warnings, then (optionally) passes. -->
        <div v-if="report.errors.length" class="wcag__group">
          <h5 class="wcag__h wcag__h--error">
            <font-awesome-icon :icon="['fas', 'circle-xmark']" /> Fails AA — fix these ({{
              report.errors.length
            }})
          </h5>
          <ul class="wcag__list">
            <li v-for="r in report.errors" :key="r.id" class="wcag__finding wcag__finding--error">
              <span
                class="wcag__chip"
                :style="{ background: r.bgColor, color: r.fgColor }"
                :title="`${r.fgColor} on ${r.bgColor}`"
                >{{ r.sample }}</span
              >
              <span class="wcag__info">
                <span class="wcag__pair">{{ r.label }}</span>
                <span class="wcag__where">{{ r.where }}</span>
                <span class="wcag__tokens">{{ tokenTrail(r.fg, r.bg) }}</span>
              </span>
              <span class="wcag__metrics">
                <span class="wcag__ratio wcag__ratio--error">{{ fmtRatio(r.ratio) }}</span>
                <span class="wcag__levels">
                  <span class="wcag__lvl wcag__lvl--off">AA {{ r.aaTarget }}:1</span>
                  <span class="wcag__lvl wcag__lvl--off">AAA {{ r.aaaTarget }}:1</span>
                </span>
                <span v-if="r.large" class="wcag__lg">large text</span>
              </span>
              <button type="button" class="wcag__find" @click="revealInPreview(r.id)">
                <font-awesome-icon :icon="['fas', 'magnifying-glass']" /> Find
              </button>
            </li>
          </ul>
        </div>

        <div v-if="report.warnings.length" class="wcag__group">
          <h5 class="wcag__h wcag__h--warn">
            <font-awesome-icon :icon="['fas', 'triangle-exclamation']" /> Passes AA, short of AAA
            ({{ report.warnings.length }})
          </h5>
          <ul class="wcag__list">
            <li v-for="r in report.warnings" :key="r.id" class="wcag__finding wcag__finding--warn">
              <span
                class="wcag__chip"
                :style="{ background: r.bgColor, color: r.fgColor }"
                :title="`${r.fgColor} on ${r.bgColor}`"
                >{{ r.sample }}</span
              >
              <span class="wcag__info">
                <span class="wcag__pair">{{ r.label }}</span>
                <span class="wcag__where">{{ r.where }}</span>
                <span class="wcag__tokens">{{ tokenTrail(r.fg, r.bg) }}</span>
              </span>
              <span class="wcag__metrics">
                <span class="wcag__ratio wcag__ratio--warn">{{ fmtRatio(r.ratio) }}</span>
                <span class="wcag__levels">
                  <span class="wcag__lvl wcag__lvl--on">AA</span>
                  <span class="wcag__lvl wcag__lvl--off">AAA {{ r.aaaTarget }}:1</span>
                </span>
                <span v-if="r.large" class="wcag__lg">large text</span>
              </span>
              <button type="button" class="wcag__find" @click="revealInPreview(r.id)">
                <font-awesome-icon :icon="['fas', 'magnifying-glass']" /> Find
              </button>
            </li>
          </ul>
        </div>

        <!-- Full list of every check (passes included), opt-in to avoid clutter. -->
        <button type="button" class="wcag__showall" @click="showAllChecks = !showAllChecks">
          <font-awesome-icon
            :icon="['fas', showAllChecks ? 'chevron-up' : 'chevron-down']"
            fixed-width
          />
          {{ showAllChecks ? 'Hide' : 'Show' }} all {{ report.results.length }} checks
        </button>
        <ul v-if="showAllChecks" class="wcag__list">
          <li
            v-for="r in report.results"
            :key="r.id"
            class="wcag__finding"
            :class="`wcag__finding--${r.status}`"
          >
            <span
              class="wcag__chip"
              :style="{ background: r.bgColor, color: r.fgColor }"
              :title="`${r.fgColor} on ${r.bgColor}`"
              >{{ r.sample }}</span
            >
            <span class="wcag__info">
              <span class="wcag__pair">{{ r.label }}</span>
              <span class="wcag__where">{{ r.where }}</span>
              <span class="wcag__tokens">{{ tokenTrail(r.fg, r.bg) }}</span>
            </span>
            <span class="wcag__metrics">
              <span
                class="wcag__ratio"
                :class="{
                  'wcag__ratio--error': r.status === 'fail',
                  'wcag__ratio--warn': r.status === 'aa',
                  'wcag__ratio--ok': r.status === 'aaa',
                }"
                >{{ fmtRatio(r.ratio) }}</span
              >
              <span class="wcag__levels">
                <span class="wcag__lvl" :class="r.aaPass ? 'wcag__lvl--on' : 'wcag__lvl--off'"
                  >AA</span
                >
                <span class="wcag__lvl" :class="r.aaaPass ? 'wcag__lvl--on' : 'wcag__lvl--off'"
                  >AAA</span
                >
              </span>
              <span v-if="r.large" class="wcag__lg">large text</span>
            </span>
            <button type="button" class="wcag__find" @click="revealInPreview(r.id)">
              <font-awesome-icon :icon="['fas', 'magnifying-glass']" /> Find
            </button>
          </li>
        </ul>

        <p class="wcag__count">
          {{ report.passes.length }} of {{ report.results.length }} pairings meet AAA · updates live
          as you edit colours.
        </p>
      </div>
    </div>

    <div v-for="group in THEME_TOKEN_GROUPS" :key="group.title" class="token-group">
      <h4 class="token-group__title">{{ group.title }}</h4>
      <div class="token-rows">
        <div v-for="t in group.tokens" :key="t.name" class="token-row">
          <div class="token-row__head">
            <span
              class="token-row__swatch"
              :style="{ background: valueOf(t) }"
              aria-hidden="true"
            />
            <label :for="`tok-${t.name}`" class="token-row__label">{{ t.label }}</label>
            <!-- Solid tokens: lightweight native picker. Alpha tokens: a swatch
                 button that opens the cross-browser Chrome picker (with alpha). -->
            <input
              v-if="!t.alpha"
              type="color"
              class="token-row__color"
              :value="swatchHex(t)"
              :aria-label="`${t.label} colour`"
              @input="setToken(t.name, ($event.target as HTMLInputElement).value)"
            />
            <button
              v-else
              type="button"
              class="token-row__color token-row__color--btn"
              :style="{ background: valueOf(t) }"
              :aria-label="`Choose ${t.label} colour and opacity`"
              @click="pickerToken = t"
            />
            <input
              :id="`tok-${t.name}`"
              class="token-row__value"
              :value="valueOf(t)"
              spellcheck="false"
              :placeholder="t.default"
              :aria-describedby="`tok-${t.name}-desc`"
              @input="setToken(t.name, ($event.target as HTMLInputElement).value)"
            />
          </div>
          <p :id="`tok-${t.name}-desc`" class="token-row__desc">{{ t.desc }}</p>
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
      <p class="text-dim text-sm mb-16">{{ pickerToken.desc }}</p>
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

/* ── Collapsible preview ──────────────────────────────────────────────────── */
.token-preview {
  border: 1px solid var(--control-border);
  border-radius: var(--radius);
  overflow: hidden;
}
.token-preview__toggle {
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
.token-preview__hint {
  color: var(--text-muted);
  font-weight: 400;
  font-size: 0.8rem;
}
@media (max-width: 560px) {
  .token-preview__hint {
    display: none;
  }
}

.token-preview__stage {
  background: var(--page-bg);
  padding: 16px;
}
.tp-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 14px;
  align-items: start;
}

/* Panels demonstrate panel-bg, border, shadow, and contained text/controls. */
.tp-panel {
  background: var(--panel-bg);
  border: 1px solid var(--control-border);
  border-radius: var(--radius);
  padding: 14px;
  box-shadow: 0 2px 10px var(--shadow);
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.tp-h {
  margin: 0;
  color: var(--highlight);
  font-size: 1rem;
}
.tp-text {
  margin: 0;
  color: var(--text);
  font-size: 0.9rem;
}
.tp-muted {
  margin: 0;
  color: var(--text-muted);
  font-size: 0.82rem;
}
.tp-raised {
  background: var(--panel-raised-bg);
  color: var(--text);
  border-radius: 6px;
  padding: 8px 10px;
  font-size: 0.82rem;
}
.tp-input {
  background: var(--input-bg);
  color: var(--text);
  border: 1px solid var(--control-border);
  border-radius: 6px;
  padding: 6px 8px;
  font-size: 0.85rem;
}

/* Buttons — incl. real :hover so accent-hover / accent-2-hover are felt. */
.tp-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.tp-row--end {
  justify-content: flex-end;
}
.tp-btn {
  border: none;
  border-radius: 6px;
  padding: 6px 12px;
  font-weight: 600;
  font-size: 0.85rem;
  cursor: pointer;
  transition: background 0.12s;
}
.tp-btn--primary {
  background: var(--accent);
  color: var(--text-on-accent);
}
.tp-btn--primary:hover {
  background: var(--accent-hover);
}
.tp-btn--secondary {
  background: var(--accent-2);
  color: var(--text-on-fill);
}
.tp-btn--secondary:hover {
  background: var(--accent-2-hover);
}
.tp-btn--success {
  background: var(--success);
  color: var(--text-on-fill);
}
.tp-btn--danger {
  background: var(--danger);
  color: var(--text-on-fill);
}
/* Caution button paints a fixed dark ink (matches .btn-caution), NOT
   --text-on-fill, so --warning can stay a bright amber in every theme. */
.tp-btn--warning {
  background: var(--warning);
  color: #1f1a06;
}
.tp-btn--neutral {
  background: var(--control-border);
  color: var(--text);
}

/* Themed inline link (matches `a { color: var(--accent) }`). */
.tp-link {
  color: var(--accent);
  font-weight: 600;
}

/* Winner chip — gradient from highlight to text-muted with text-on-accent ink,
   mirroring the real winner announcement chip. */
.tp-winner {
  align-self: flex-start;
  background: linear-gradient(135deg, var(--highlight), var(--text-muted));
  color: var(--text-on-accent);
  border-radius: 999px;
  padding: 4px 12px;
  font-size: 0.78rem;
  font-weight: 700;
}

/* Status badges */
.tp-badge {
  border-radius: 999px;
  padding: 2px 10px;
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--text-on-fill);
}
.tp-badge--success {
  background: var(--success);
}
.tp-badge--danger {
  background: var(--danger);
}
.tp-badge--warning {
  background: var(--warning);
}

/* Bingo board — gradient wrapper, light cells (dark numbers), FREE cell, and a
   "last called" badge showing highlight + its glow. */
.tp-board {
  background: linear-gradient(var(--board-gradient-start), var(--board-gradient-end));
  border-radius: 8px;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.tp-board__row {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 4px;
}
.tp-board__head span {
  text-align: center;
  color: var(--highlight);
  font-weight: 800;
  font-size: 0.8rem;
}
.tp-cell {
  background: var(--board-cell-bg);
  color: var(--text-on-accent);
  aspect-ratio: 1;
  display: grid;
  place-items: center;
  border-radius: 4px;
  font-weight: 700;
  font-size: 0.8rem;
  transition: background 0.12s;
}
.tp-cell:hover {
  background: var(--board-cell-hover-bg);
}
.tp-cell--free {
  background: var(--board-free-bg);
}
.tp-called {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  align-self: flex-start;
  background: var(--highlight);
  color: var(--text-on-accent);
  padding: 4px 12px;
  border-radius: 6px;
  font-weight: 800;
  box-shadow: 0 0 14px var(--highlight-glow);
}
.tp-called__label {
  font-size: 0.75rem;
  opacity: 0.85;
}
.tp-called__num {
  font-size: 1.05rem;
}

/* Modal & shadow — the dimmed backdrop uses modal-overlay; the dialog uses
   panel-bg with a heavier shadow. */
.tp-overlay {
  background: var(--modal-overlay);
  border-radius: var(--radius);
  padding: 18px;
  display: grid;
  place-items: center;
  min-height: 160px;
}
.tp-modal {
  width: 100%;
  background: var(--panel-bg);
  border: 1px solid var(--control-border);
  border-radius: var(--radius);
  padding: 14px;
  box-shadow: 0 12px 32px var(--shadow);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/* ── Token rows ───────────────────────────────────────────────────────────── */
.token-group__title {
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
.token-row__head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.token-row__desc {
  margin: 0;
  color: var(--text-muted);
  font-size: 0.78rem;
  line-height: 1.35;
}
.token-row__swatch {
  width: 18px;
  height: 18px;
  border-radius: 4px;
  border: 1px solid var(--control-border);
  flex: 0 0 auto;
}
.token-row__label {
  flex: 1 1 auto;
  font-size: 0.85rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.token-row__color {
  width: 34px;
  height: 28px;
  padding: 2px;
  border: 1px solid var(--control-border);
  border-radius: 6px;
  background: var(--panel-bg);
  cursor: pointer;
  flex: 0 0 auto;
}
/* Alpha-token swatch button: the chosen colour (incl. transparency) fills it. */
.token-row__color--btn {
  padding: 0;
}
.token-row__value {
  width: 120px;
  flex: 0 0 auto;
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 0.8rem;
  padding: 5px 7px;
}

/* "Find in preview" flash — a pulsing outline drawing the eye to the element. */
.tp-flash {
  animation: tp-flash 1.6s ease-out;
  border-radius: 6px;
}
@keyframes tp-flash {
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

/* ── WCAG compliance report ──────────────────────────────────────────────── */
.wcag {
  border: 1px solid var(--control-border);
  border-radius: var(--radius);
  overflow: hidden;
}
.wcag__toggle {
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
.wcag__verdict {
  margin-left: auto;
  font-size: 0.78rem;
  font-weight: 700;
  padding: 2px 10px;
  border-radius: 999px;
  color: var(--text-on-fill);
}
.wcag--AAA .wcag__verdict {
  background: var(--success);
}
.wcag--AA .wcag__verdict {
  background: var(--warning);
  color: #1f1a06;
}
.wcag--fail .wcag__verdict {
  background: var(--danger);
}
.wcag__body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.wcag__summary {
  margin: 0;
  font-size: 0.88rem;
  color: var(--text);
}
.wcag__group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.wcag__h {
  margin: 0;
  font-size: 0.82rem;
}
.wcag__h--error {
  color: var(--danger);
}
.wcag__h--warn {
  color: var(--warning);
}
.wcag__list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* A single finding row: live chip · description · metrics · find button. */
.wcag__finding {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 8px;
  border-radius: 8px;
  background: var(--panel-raised-bg);
  border-left: 3px solid var(--control-border);
}
.wcag__finding--fail {
  border-left-color: var(--danger);
}
.wcag__finding--aa {
  border-left-color: var(--warning);
}
.wcag__finding--aaa {
  border-left-color: var(--success);
}

/* Live contrast chip: shows the actual foreground on the actual background, so
   the reviewer sees exactly how legible (or not) the pairing is. */
.wcag__chip {
  flex: 0 0 auto;
  width: 38px;
  height: 38px;
  display: grid;
  place-items: center;
  border-radius: 6px;
  border: 1px solid color-mix(in srgb, var(--text) 25%, transparent);
  font-weight: 800;
  font-size: 0.95rem;
  line-height: 1;
}

.wcag__info {
  flex: 1 1 auto;
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}
.wcag__pair {
  font-size: 0.84rem;
  font-weight: 600;
  color: var(--text);
}
.wcag__where {
  font-size: 0.74rem;
  color: var(--text-muted);
}
.wcag__tokens {
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 0.7rem;
  color: var(--text-muted);
  opacity: 0.85;
}

.wcag__metrics {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
}
.wcag__ratio {
  font-family: 'Consolas', 'Monaco', monospace;
  font-weight: 700;
  font-size: 0.82rem;
  /* Default to body text so the number is always legible on the active admin
     theme; status is conveyed by the row border, the AA/AAA pills, and (for
     problems) the brighter danger/warning tints below. */
  color: var(--text);
}
.wcag__ratio--error {
  color: var(--danger);
}
.wcag__ratio--warn {
  color: var(--warning);
}
.wcag__levels {
  display: flex;
  gap: 4px;
}
.wcag__lvl {
  font-size: 0.62rem;
  font-weight: 700;
  padding: 1px 5px;
  border-radius: 4px;
  white-space: nowrap;
}
.wcag__lvl--on {
  background: color-mix(in srgb, var(--success) 24%, transparent);
  color: var(--text);
}
.wcag__lvl--off {
  background: color-mix(in srgb, var(--danger) 18%, transparent);
  color: var(--text-muted);
}
.wcag__lg {
  font-size: 0.62rem;
  color: var(--text-muted);
}

.wcag__find {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: var(--control-border);
  color: var(--text);
  border: none;
  border-radius: 6px;
  padding: 5px 9px;
  font-size: 0.74rem;
  font-weight: 600;
  cursor: pointer;
}
.wcag__find:hover {
  background: color-mix(in srgb, var(--control-border) 82%, var(--text));
}

.wcag__showall {
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

.wcag__count {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

/* On narrow widths, let the finding row wrap so the chip + text + metrics stack
   instead of crushing. */
@media (max-width: 560px) {
  .wcag__finding {
    flex-wrap: wrap;
  }
  .wcag__metrics {
    align-items: flex-start;
  }
}
</style>
