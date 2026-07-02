/**
 * WCAG 2.1 contrast auditing for themes.
 *
 * Scores the real text/background pairings a theme produces (body text, muted
 * text, links, headings, board numbers, every button intent, …) against the
 * WCAG 2.1 contrast minimums and returns a report the theme editor surfaces via
 * the "Check WCAG compliance" button. Mirrors the Go checker in
 * `src/cmd/themetool` — keep the pair list in sync.
 *
 * Thresholds: normal text AA 4.5:1 / AAA 7:1; large text AA 3:1 / AAA 4.5:1.
 */

/** Fixed dark ink the caution button paints on the (always light) warning fill
 *  — decoupled from --text-on-accent. Keep in sync with .btn-caution (base.css). */
export const CAUTION_INK = '#1f1a06'

/** A checked pairing. `fg` is a token name or a literal "#hex"; `bg` is a token name. */
interface Pair {
  /** Stable id; matches a `data-pair` element in the editor's live preview. */
  id: string
  fg: string
  bg: string
  /** What the user sees this pairing as. */
  label: string
  /** Where in the UI this pairing appears — shown in the report for context. */
  where: string
  /** Large/display text (board numbers, called number) — relaxed thresholds. */
  large?: boolean
  /** Short glyph shown in the report's live contrast chip. */
  sample?: string
}

const PAIRS: Pair[] = [
  {
    id: 'body-page',
    fg: 'text',
    bg: 'page-bg',
    label: 'Body text on the page',
    where: 'Page background, outside any panel',
  },
  {
    id: 'body-panel',
    fg: 'text',
    bg: 'panel-bg',
    label: 'Body text on a panel',
    where: 'Paragraphs and labels inside panels',
  },
  {
    id: 'body-raised',
    fg: 'text',
    bg: 'panel-raised-bg',
    label: 'Body text on a raised surface',
    where: 'Table rows, chips, nested panels',
  },
  {
    id: 'input-text',
    fg: 'text',
    bg: 'input-bg',
    label: 'Text typed into a field',
    where: 'What you type into inputs/textareas',
  },
  {
    id: 'neutral-btn',
    fg: 'text',
    bg: 'control-border',
    label: 'Neutral button label',
    where: 'Cancel / Close / secondary actions',
  },
  {
    id: 'muted-page',
    fg: 'text-muted',
    bg: 'page-bg',
    label: 'Muted text on the page',
    where: 'Hints/captions outside panels',
  },
  {
    id: 'muted-panel',
    fg: 'text-muted',
    bg: 'panel-bg',
    label: 'Muted text on a panel',
    where: 'Secondary/help text inside panels',
  },
  {
    id: 'muted-raised',
    fg: 'text-muted',
    bg: 'panel-raised-bg',
    label: 'Muted text on a raised surface',
    where: 'Secondary text in rows/chips',
  },
  {
    id: 'placeholder',
    fg: 'text-muted',
    bg: 'input-bg',
    label: 'Placeholder text in a field',
    where: 'The greyed prompt inside empty inputs',
  },
  {
    id: 'link-panel',
    fg: 'accent',
    bg: 'panel-bg',
    label: 'Link on a panel',
    where: 'Hyperlinks inside panels',
  },
  {
    id: 'link-page',
    fg: 'accent',
    bg: 'page-bg',
    label: 'Link on the page',
    where: 'Hyperlinks on the page background',
  },
  {
    id: 'heading-panel',
    fg: 'highlight',
    bg: 'panel-bg',
    label: 'Heading on a panel',
    where: 'Section headings inside panels',
  },
  {
    id: 'heading-page',
    fg: 'highlight',
    bg: 'page-bg',
    label: 'Heading on the page',
    where: 'Headings on the page background',
  },
  {
    id: 'heading-raised',
    fg: 'highlight',
    bg: 'panel-raised-bg',
    label: 'Heading on a raised surface',
    where: 'Headings inside rows/nested panels',
  },
  // The B-I-N-G-O column letters paint with --highlight over the board wrapper's
  // gradient (--board-gradient-start → --board-gradient-end); check both stops.
  {
    id: 'bingo-top',
    fg: 'highlight',
    bg: 'board-gradient-start',
    label: 'B-I-N-G-O header letters (board top)',
    where: 'The B I N G O column letters',
    large: true,
    sample: 'B',
  },
  {
    id: 'bingo-bottom',
    fg: 'highlight',
    bg: 'board-gradient-end',
    label: 'B-I-N-G-O header letters (board bottom)',
    where: 'The B I N G O column letters',
    large: true,
    sample: 'O',
  },
  {
    id: 'primary-btn',
    fg: 'text-on-accent',
    bg: 'accent',
    label: 'Primary button label',
    where: 'Main call-to-action buttons',
  },
  {
    id: 'primary-btn-hover',
    fg: 'text-on-accent',
    bg: 'accent-hover',
    label: 'Primary button label (hover)',
    where: 'A primary button while hovered',
  },
  {
    id: 'called-num',
    fg: 'text-on-accent',
    bg: 'highlight',
    label: 'Called number on its badge',
    where: 'The big "last called" number',
    large: true,
    sample: '52',
  },
  {
    id: 'winner-chip',
    fg: 'text-on-accent',
    bg: 'text-muted',
    label: 'Winner chip label',
    where: 'The winner announcement chip',
  },
  {
    id: 'board-num',
    fg: 'text-on-accent',
    bg: 'board-cell-bg',
    label: 'Bingo board number',
    where: 'Numbers in the board cells',
    large: true,
    sample: '12',
  },
  {
    id: 'board-num-hover',
    fg: 'text-on-accent',
    bg: 'board-cell-hover-bg',
    label: 'Bingo board number (hover)',
    where: 'A board number while hovered',
    large: true,
    sample: '12',
  },
  {
    id: 'free-num',
    fg: 'text-on-accent',
    bg: 'board-free-bg',
    label: 'FREE cell number',
    where: 'The centre FREE cell',
    large: true,
    sample: '★',
  },
  {
    id: 'secondary-btn',
    fg: 'text-on-fill',
    bg: 'accent-2',
    label: 'Secondary button label',
    where: 'View / secondary-fill buttons',
  },
  {
    id: 'secondary-btn-hover',
    fg: 'text-on-fill',
    bg: 'accent-2-hover',
    label: 'Secondary button label (hover)',
    where: 'A secondary button while hovered',
  },
  {
    id: 'success-btn',
    fg: 'text-on-fill',
    bg: 'success',
    label: 'Confirm/success button label',
    where: 'Confirm/save and success badges',
  },
  {
    id: 'danger-btn',
    fg: 'text-on-fill',
    bg: 'danger',
    label: 'Danger button label',
    where: 'Delete/destructive buttons & badges',
  },
  {
    id: 'caution-btn',
    fg: CAUTION_INK,
    bg: 'warning',
    label: 'Caution button label',
    where: 'Skip / reset / halftime buttons',
  },
]

/** Parsed RGB (0–255). Alpha is ignored — no checked pair uses an alpha token. */
function parseRgb(input: string): [number, number, number] | null {
  const s = (input || '').trim()
  const hex = s.match(/^#([0-9a-f]{3,8})$/i)
  if (hex) {
    let h = hex[1]
    if (h.length === 3 || h.length === 4)
      h = h
        .split('')
        .map((c) => c + c)
        .join('')
    return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)]
  }
  const fn = s.match(/^rgba?\(([^)]+)\)$/i)
  if (fn) {
    const parts = fn[1]
      .replace(/\//g, ' ')
      .split(/[\s,]+/)
      .filter(Boolean)
    if (parts.length >= 3) {
      const ch = (x: string) => (x.endsWith('%') ? (parseFloat(x) / 100) * 255 : parseFloat(x))
      return [ch(parts[0]), ch(parts[1]), ch(parts[2])]
    }
  }
  return null
}

function relLuminance([r, g, b]: [number, number, number]): number {
  const lin = (c: number) => {
    const v = c / 255
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)
  }
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
}

/** WCAG contrast ratio between two CSS colours, or null if either won't parse. */
export function contrastRatio(fg: string, bg: string): number | null {
  const a = parseRgb(fg)
  const b = parseRgb(bg)
  if (!a || !b) return null
  let [l1, l2] = [relLuminance(a), relLuminance(b)]
  if (l1 < l2) [l1, l2] = [l2, l1]
  return (l1 + 0.05) / (l2 + 0.05)
}

export type PairStatus = 'aaa' | 'aa' | 'fail'

export interface PairResult {
  id: string
  label: string
  where: string
  /** Foreground source: a token name, or a literal "#hex" (the caution ink). */
  fg: string
  /** Background token name. */
  bg: string
  /** Resolved CSS colour the foreground actually renders as. */
  fgColor: string
  /** Resolved CSS colour the background actually renders as. */
  bgColor: string
  /** Glyph for the live contrast chip. */
  sample: string
  ratio: number
  large: boolean
  aaTarget: number
  aaaTarget: number
  aaPass: boolean
  aaaPass: boolean
  status: PairStatus
}

export interface WcagReport {
  /** Best level the whole theme satisfies. */
  level: 'AAA' | 'AA' | 'fail'
  results: PairResult[]
  errors: PairResult[] // below AA
  warnings: PairResult[] // AA but not AAA
  passes: PairResult[] // AAA
}

/**
 * Audits a fully-populated token map (token name → CSS value) and returns a
 * per-pairing report plus the overall level. Unparseable pairs are reported as
 * failures so they're never silently passed.
 */
export function auditTheme(tokens: Record<string, string>): WcagReport {
  const results: PairResult[] = []
  for (const p of PAIRS) {
    const fgColor = p.fg.startsWith('#') ? p.fg : (tokens[p.fg] ?? '')
    const bgColor = tokens[p.bg] ?? ''
    const r = contrastRatio(fgColor, bgColor) ?? 0
    const large = !!p.large
    const aaTarget = large ? 3 : 4.5
    const aaaTarget = large ? 4.5 : 7
    const aaPass = r >= aaTarget
    const aaaPass = r >= aaaTarget
    const status: PairStatus = aaaPass ? 'aaa' : aaPass ? 'aa' : 'fail'
    results.push({
      id: p.id,
      label: p.label,
      where: p.where,
      fg: p.fg,
      bg: p.bg,
      fgColor,
      bgColor,
      sample: p.sample ?? 'Aa',
      ratio: r,
      large,
      aaTarget,
      aaaTarget,
      aaPass,
      aaaPass,
      status,
    })
  }
  const errors = results.filter((r) => r.status === 'fail')
  const warnings = results.filter((r) => r.status === 'aa')
  const passes = results.filter((r) => r.status === 'aaa')
  const level: WcagReport['level'] = errors.length ? 'fail' : warnings.length ? 'AA' : 'AAA'
  return { level, results, errors, warnings, passes }
}
