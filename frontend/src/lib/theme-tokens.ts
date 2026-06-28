/**
 * Canonical theme design tokens — the single source of truth for the theme
 * editor. A theme is just a set of overrides for these tokens; the applied
 * stylesheet is a generated `:root{…}` block (see tokensToCss), so themes can
 * only retint the design tokens and never carry arbitrary CSS.
 *
 * Keep this list in sync with the `:root` block in `assets/app.css` and with
 * `themeTokenOrder` in the Go store (the server validates against the same
 * allowlist and generates the same CSS).
 */

export interface ThemeTokenMeta {
  /** Token name without the leading `--` (e.g. `page-bg`). */
  name: string
  /** Human-readable label shown in the editor. */
  label: string
  /** Where the colour is used — shown as help text under the editor row (sourced
   *  from the app.css :root comments). */
  desc: string
  /** Default value, mirroring app.css :root. */
  default: string
  /** True for tokens whose values use alpha (rgba) — edited as text, not a swatch. */
  alpha?: boolean
}

export interface ThemeTokenGroup {
  title: string
  tokens: ThemeTokenMeta[]
}

/** The token groups, in the order they should appear in the editor + CSS. */
export const THEME_TOKEN_GROUPS: ThemeTokenGroup[] = [
  {
    title: 'Backgrounds',
    tokens: [
      {
        name: 'page-bg',
        label: 'Page background',
        desc: 'The app/page background, behind all panels.',
        default: '#1a1c17',
      },
      {
        name: 'panel-bg',
        label: 'Panel surface',
        desc: 'Card and panel surfaces — the main raised areas content sits on.',
        default: '#272a22',
      },
      {
        name: 'panel-raised-bg',
        label: 'Raised surface',
        desc: 'Nested surfaces: panels within panels, table rows, and chips.',
        default: '#2f3228',
      },
      {
        name: 'control-border',
        label: 'Control / divider border',
        desc: 'Outline for controls and dividers; readable on both panel surfaces.',
        default: '#4a4d3f',
      },
      {
        name: 'input-bg',
        label: 'Input fill',
        desc: 'Fill for form controls (inputs, selects, textareas) on any surface.',
        default: '#272a22',
      },
    ],
  },
  {
    title: 'Accents & actions',
    tokens: [
      {
        name: 'accent',
        label: 'Primary accent',
        desc: 'Primary actions — primary buttons, links, and focus rings.',
        default: '#d6bdae',
      },
      {
        name: 'accent-hover',
        label: 'Primary accent (hover)',
        desc: 'Hover colour for primary buttons and links.',
        default: '#c4a999',
      },
      {
        name: 'accent-2',
        label: 'Secondary accent',
        desc: 'Secondary actions — secondary buttons.',
        default: '#474b3c',
      },
      {
        name: 'accent-2-hover',
        label: 'Secondary accent (hover)',
        desc: 'Hover colour for secondary buttons.',
        default: '#3a3d30',
      },
      {
        name: 'highlight',
        label: 'Highlight',
        desc: 'Highlight accent — called numbers, section headings, and gold trim.',
        default: '#d6bdae',
      },
    ],
  },
  {
    title: 'Text',
    tokens: [
      {
        name: 'text',
        label: 'Primary text',
        desc: 'Default body and heading text.',
        default: '#f0ebe3',
      },
      {
        name: 'text-muted',
        label: 'Muted text',
        desc: 'Secondary / muted text — hints, captions, help text.',
        default: '#d3d3bf',
      },
      {
        name: 'text-on-accent',
        label: 'Text on accent',
        desc: 'Text shown on accent and highlight fills (e.g. primary buttons, called numbers).',
        default: '#1a1c17',
      },
      {
        name: 'text-on-fill',
        label: 'Text on fills',
        desc: 'Text on toast, badge, and status fills (success/danger/warning).',
        default: '#f5f1ea',
      },
    ],
  },
  {
    title: 'Status',
    tokens: [
      {
        name: 'success',
        label: 'Success',
        desc: 'Success state — confirmations and “paid” badges.',
        default: '#175020',
      },
      {
        name: 'danger',
        label: 'Danger',
        desc: 'Danger / error state — destructive actions and error messages.',
        default: '#9a2018',
      },
      {
        name: 'warning',
        label: 'Warning',
        desc: 'Warning / caution state — skip badges and alerts.',
        default: '#e0a82e',
      },
    ],
  },
  {
    title: 'Bingo board',
    tokens: [
      {
        name: 'board-cell-bg',
        label: 'Cell background',
        desc: 'Background of each bingo board number cell.',
        default: '#f0ebe3',
      },
      {
        name: 'board-cell-hover-bg',
        label: 'Cell hover',
        desc: 'Board cell background on hover.',
        default: '#e5ded4',
      },
      {
        name: 'board-free-bg',
        label: 'FREE cell',
        desc: 'Background of the centre FREE space.',
        default: '#d6bdae',
      },
      {
        name: 'board-gradient-start',
        label: 'Board gradient (top)',
        desc: 'Top colour of the board wrapper’s background gradient.',
        default: '#2f3328',
      },
      {
        name: 'board-gradient-end',
        label: 'Board gradient (bottom)',
        desc: 'Bottom colour of the board wrapper’s background gradient.',
        default: '#272a22',
      },
    ],
  },
  {
    title: 'Effects',
    tokens: [
      {
        name: 'modal-overlay',
        label: 'Modal backdrop',
        desc: 'Full-screen dimmed backdrop behind modal dialogs. Set its opacity with the slider.',
        default: 'rgb(0 0 0 / 70%)',
        alpha: true,
      },
      {
        name: 'shadow',
        label: 'Shadow',
        desc: 'Drop-shadow colour for panels and modals. Set its opacity with the slider.',
        default: 'rgb(0 0 0 / 50%)',
        alpha: true,
      },
      {
        name: 'highlight-glow',
        label: 'Highlight glow',
        desc: 'Glow around highlighted elements, e.g. the last-called number. Set its opacity with the slider.',
        default: 'rgb(214 189 174 / 50%)',
        alpha: true,
      },
    ],
  },
]

/** Flat list of every token, in canonical order. */
export const THEME_TOKENS: ThemeTokenMeta[] = THEME_TOKEN_GROUPS.flatMap((g) => g.tokens)

/** Set of valid token names, for filtering unknown keys. */
const TOKEN_NAMES = new Set(THEME_TOKENS.map((t) => t.name))

/** A fresh token map seeded with every token's default value. */
export function defaultTokens(): Record<string, string> {
  const out: Record<string, string> = {}
  for (const t of THEME_TOKENS) out[t.name] = t.default
  return out
}

/**
 * Merges a (possibly partial) saved token map over the defaults so the editor
 * always has a value for every token. Unknown keys are dropped.
 */
export function withDefaults(tokens: Record<string, string> | undefined): Record<string, string> {
  const out = defaultTokens()
  if (tokens) {
    for (const [k, v] of Object.entries(tokens)) {
      if (TOKEN_NAMES.has(k) && v) out[k] = v
    }
  }
  return out
}

/** An RGBA colour with 0–255 channels and a 0–1 alpha. */
export interface Rgba {
  r: number
  g: number
  b: number
  a: number
}

/** Clamps + rounds a channel to an integer in 0–255. */
function channel(n: number): number {
  return Math.max(0, Math.min(255, Math.round(n)))
}

/** Opaque `#rrggbb` for a colour (drops alpha) — for the native colour input. */
export function toHex(c: Rgba): string {
  const p = (n: number) => channel(n).toString(16).padStart(2, '0')
  return '#' + p(c.r) + p(c.g) + p(c.b)
}

/** 8-digit `#rrggbbaa` for a colour — the value an alpha-enabled `<input
 *  type="color" alpha>` accepts/serializes. */
export function toHex8(c: Rgba): string {
  const p = (n: number) => channel(n).toString(16).padStart(2, '0')
  const a = Math.round(Math.max(0, Math.min(1, c.a)) * 255)
  return '#' + p(c.r) + p(c.g) + p(c.b) + p(a)
}

/**
 * Renders a colour as a modern CSS `rgb()` string — `rgb(r g b / a%)` when it has
 * alpha, or `rgb(r g b)` when fully opaque. (CSS Color 4 makes `rgba()` a legacy
 * alias; the slash-separated form is the current standard.)
 */
export function toRgb(c: Rgba): string {
  const pct = Math.max(0, Math.min(100, Math.round(c.a * 100)))
  const base = `rgb(${channel(c.r)} ${channel(c.g)} ${channel(c.b)}`
  return pct >= 100 ? `${base})` : `${base} / ${pct}%)`
}

/**
 * Renders a token map as a `:root{…}` stylesheet, emitting only known tokens in
 * canonical order. Mirrors the server's TokensToCSS so live preview matches what
 * the backend will serve. Returns '' when nothing usable is present.
 */
export function tokensToCss(tokens: Record<string, string>): string {
  const decls: string[] = []
  for (const t of THEME_TOKENS) {
    const v = (tokens[t.name] || '').trim()
    if (v) decls.push(`--${t.name}:${v};`)
  }
  return decls.length ? `:root{${decls.join('')}}` : ''
}
