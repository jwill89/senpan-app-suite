/**
 * Exports a bingo board to a downloadable PNG "card" image.
 *
 * The board itself is captured directly from the live, already-themed DOM
 * element (`.board-wrap`) so it follows the active custom theme — gradients,
 * colors, header font, and the player's stamps (emoji + custom data-URL images)
 * are preserved exactly as rendered.
 *
 * The captured board is then composited into a larger, themed frame that adds:
 *   • a header  — site title, "Bingo Card", the card id, and the site logo
 *                 (top-right, on its own row so it is never obstructed);
 *   • a footer  — a link to the site and a short excerpt of the game details.
 *
 * Frame colors/fonts are read from the active theme's CSS custom properties so
 * the surround matches whatever theme is in use.
 *
 * `html-to-image` is dynamically imported so its ~30 KB only loads when a
 * player actually exports a card (keeps the player route payload small).
 */

export interface ExportCardOptions {
  /** The `.board-wrap` element to capture (BingoBoard's root). */
  element: HTMLElement
  /** Download file name (without extension). */
  fileName: string
  /** Site / app title shown in the header. */
  title: string
  /** Card id shown in the header. */
  cardId: string
  /** Site link shown in the footer (e.g. `window.location.host`). */
  link: string
  /** Optional game details (markdown) — excerpted into the footer. */
  gameDetails?: string
  /** Logo URL to draw top-right (defaults to `/images/logo.png`). */
  logoUrl?: string
}

/** Device-pixel scale — matches the board capture pixelRatio for crispness. */
const SCALE = 2

/** Reads a CSS custom property off :root, with a fallback. */
function cssVar(name: string, fallback: string): string {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

/** Loads an image for compositing; resolves null on failure (logo is optional). */
function loadImage(url: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    img.onload = () => resolve(img)
    img.onerror = () => resolve(null)
    img.src = url
  })
}

/** Triggers a browser download for the given blob + file name. */
function triggerDownload(blob: Blob, fileName: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = fileName
  document.body.appendChild(a)
  a.click()
  a.remove()
  // Revoke on the next tick so the download has started.
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/** Font family used for all non-header (details/card-id/link) text. */
const DETAIL_FONT = "'Segoe UI', system-ui, sans-serif"

/** A single character carrying its bold/italic style (for rich text drawing). */
interface StyledChar {
  ch: string
  bold: boolean
  italic: boolean
}
/** A "word" = a run of non-space styled characters. */
type Word = StyledChar[]

/** Builds the canvas font string for a styled char at the given size. */
function detailFont(size: number, bold: boolean, italic: boolean): string {
  return `${italic ? 'italic ' : ''}${bold ? '700 ' : ''}${size}px ${DETAIL_FONT}`
}

/** Parses inline markdown emphasis into styled runs (bold / italic). */
export function parseInlineRuns(text: string): { text: string; bold: boolean; italic: boolean }[] {
  const runs: { text: string; bold: boolean; italic: boolean }[] = []
  const re =
    /(\*\*\*|___)([\s\S]+?)\1|(\*\*|__)([\s\S]+?)\3|(\*|_)([\s\S]+?)\5|`([\s\S]+?)`|~~([\s\S]+?)~~/g
  let last = 0
  let m: RegExpExecArray | null
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) runs.push({ text: text.slice(last, m.index), bold: false, italic: false })
    if (m[2] !== undefined) runs.push({ text: m[2], bold: true, italic: true })
    else if (m[4] !== undefined) runs.push({ text: m[4], bold: true, italic: false })
    else if (m[6] !== undefined) runs.push({ text: m[6], bold: false, italic: true })
    else if (m[7] !== undefined) runs.push({ text: m[7], bold: false, italic: false }) // inline code
    else if (m[8] !== undefined) runs.push({ text: m[8], bold: false, italic: false }) // strikethrough
    last = re.lastIndex
  }
  if (last < text.length) runs.push({ text: text.slice(last), bold: false, italic: false })
  return runs
}

/**
 * Parses game-details markdown into paragraphs of style-tagged "words", so the
 * footer can render bold/italic emphasis. Block syntax (headings, bullets,
 * blockquotes, links/images, code fences) is normalized to plain lines first;
 * headings render bold.
 */
export function parseDetailParagraphs(md: string): Word[][] {
  const cleaned = md
    .replace(/```[\s\S]*?```/g, ' ') // fenced code
    .replace(/!\[[^\]]*\]\([^)]*\)/g, ' ') // images
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1') // links → text
    .replace(/`([^`]*)`/g, '$1') // inline-code backticks (keep text)
    .replace(/\r/g, '')

  const paragraphs: Word[][] = []
  for (const rawLine of cleaned.split('\n')) {
    let line = rawLine
    const heading = line.match(/^\s{0,3}(#{1,6})\s+(.*)$/)
    const forceBold = !!heading
    if (heading) line = heading[2]
    line = line
      .replace(/^\s{0,3}>\s?/, '') // blockquote
      .replace(/^\s*[-*+]\s+/, '• ') // list bullet
      .replace(/[ \t]{2,}/g, ' ')
      .trim()
    if (!line) continue

    // Flatten styled runs → words (split on spaces, preserving style).
    const words: Word[] = []
    let current: Word = []
    for (const run of parseInlineRuns(line)) {
      for (const ch of run.text) {
        if (ch === ' ') {
          if (current.length) {
            words.push(current)
            current = []
          }
        } else {
          current.push({ ch, bold: run.bold || forceBold, italic: run.italic })
        }
      }
    }
    if (current.length) words.push(current)
    if (words.length) paragraphs.push(words)
  }
  return paragraphs
}

/** Measures the rendered width of a styled word at `size`. */
function measureWord(ctx: CanvasRenderingContext2D, word: Word, size: number): number {
  let w = 0
  for (const c of word) {
    ctx.font = detailFont(size, c.bold, c.italic)
    w += ctx.measureText(c.ch).width
  }
  return w
}

/**
 * Greedily wraps paragraphs of styled words into lines within `maxWidth`,
 * capping at `maxLines`. Appends an ellipsis word when content is truncated.
 */
function wrapWords(
  ctx: CanvasRenderingContext2D,
  paragraphs: Word[][],
  size: number,
  maxWidth: number,
  maxLines: number,
): Word[][] {
  ctx.font = detailFont(size, false, false)
  const spaceW = ctx.measureText(' ').width
  const all: Word[][] = []
  for (const words of paragraphs) {
    let line: Word[] = []
    let lineW = 0
    for (const word of words) {
      const ww = measureWord(ctx, word, size)
      const needed = (line.length ? spaceW : 0) + ww
      if (lineW + needed > maxWidth && line.length) {
        all.push(line)
        line = [word]
        lineW = ww
      } else {
        line.push(word)
        lineW += needed
      }
    }
    if (line.length) all.push(line)
  }
  if (all.length <= maxLines) return all
  const kept = all.slice(0, maxLines)
  kept[kept.length - 1] = [...kept[kept.length - 1], [{ ch: '…', bold: false, italic: false }]]
  return kept
}

/** Draws one wrapped line of styled words at (x, baselineY). */
function drawWordLine(
  ctx: CanvasRenderingContext2D,
  line: Word[],
  x: number,
  baselineY: number,
  size: number,
): void {
  ctx.font = detailFont(size, false, false)
  const spaceW = ctx.measureText(' ').width
  let cx = x
  line.forEach((word, i) => {
    if (i > 0) cx += spaceW
    for (const c of word) {
      ctx.font = detailFont(size, c.bold, c.italic)
      ctx.fillText(c.ch, cx, baselineY)
      cx += ctx.measureText(c.ch).width
    }
  })
}

/** Shrinks the current ctx font until `text` fits within `maxWidth` (min 60%). */
function fitText(ctx: CanvasRenderingContext2D, text: string, maxWidth: number): void {
  const match = ctx.font.match(/(\d+(?:\.\d+)?)px/)
  if (!match) return
  let size = parseFloat(match[1])
  const min = size * 0.6
  while (size > min && ctx.measureText(text).width > maxWidth) {
    size -= 1
    ctx.font = ctx.font.replace(/\d+(?:\.\d+)?px/, `${size}px`)
  }
}

/** Builds a rounded-rectangle path (clamped radius) without filling/stroking. */
function roundRectPath(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
): void {
  const radius = Math.min(r, w / 2, h / 2)
  ctx.beginPath()
  ctx.moveTo(x + radius, y)
  ctx.arcTo(x + w, y, x + w, y + h, radius)
  ctx.arcTo(x + w, y + h, x, y + h, radius)
  ctx.arcTo(x, y + h, x, y, radius)
  ctx.arcTo(x, y, x + w, y, radius)
  ctx.closePath()
}

/**
 * Captures the themed board element and composites it into a framed PNG card
 * (header + board + footer), then downloads it. Renders at 2× for a crisp image.
 */
export async function exportCardImage(opts: ExportCardOptions): Promise<void> {
  const { toCanvas } = await import('html-to-image')

  // Make sure the (possibly Google-hosted) header font is ready so canvas text
  // metrics + rendering use it.
  if (document.fonts?.ready) {
    try {
      await document.fonts.ready
    } catch {
      /* non-fatal */
    }
  }

  // 1. Capture the live styled board. Try with web-font embedding first (so the
  //    header font matches the theme); fall back to skipping fonts if that
  //    throws (cross-origin font CSS can occasionally fail) so the export still
  //    succeeds.
  let boardCanvas: HTMLCanvasElement
  try {
    boardCanvas = await toCanvas(opts.element, { pixelRatio: SCALE, cacheBust: true })
  } catch {
    boardCanvas = await toCanvas(opts.element, {
      pixelRatio: SCALE,
      cacheBust: true,
      skipFonts: true,
    })
  }

  // 2. Theme colors/fonts for the frame.
  const headerFont = cssVar('--header-font', "'Arapey', serif")
  const gold = cssVar('--gold', '#d6bdae')
  const text = cssVar('--text', '#f0ebe3')
  const textDim = cssVar('--text-dim', '#a5a58c')
  const bgStart = cssVar('--board-gradient-start', '#2f3328')
  const bgEnd = cssVar('--bg', '#1a1c17')

  // 3. Layout metrics (device px).
  const S = SCALE
  const pad = 36 * S
  const gap = 26 * S
  const boardW = boardCanvas.width
  const boardH = boardCanvas.height
  const totalW = boardW + pad * 2

  const titleSize = 40 * S
  const subSize = 23 * S
  const cardIdSize = 24 * S
  const logoH = 86 * S
  const linkSize = 14 * S // small, tucked into the bottom-right corner
  const detailSize = 19 * S
  const detailLineH = 27 * S
  const maxDetailLines = 4

  // Parse + wrap footer details (with bold/italic styling) before sizing canvas.
  const measure = document.createElement('canvas').getContext('2d')!
  const detailParas = opts.gameDetails ? parseDetailParagraphs(opts.gameDetails) : []
  const detailLines = detailParas.length
    ? wrapWords(measure, detailParas, detailSize, totalW - pad * 2, maxDetailLines)
    : []

  // Header is tall enough for the logo row, with the card id beneath it.
  const headerH = logoH + 12 * S + cardIdSize
  const footerH =
    (detailLines.length ? detailLines.length * detailLineH + 16 * S : 0) + linkSize + 8 * S
  const totalH = pad + headerH + gap + boardH + gap + footerH + pad

  // 4. Compose.
  const out = document.createElement('canvas')
  out.width = totalW
  out.height = totalH
  const ctx = out.getContext('2d')
  if (!ctx) throw new Error('Canvas not supported')

  // Background gradient (matches the board wrapper).
  const bg = ctx.createLinearGradient(0, 0, 0, totalH)
  bg.addColorStop(0, bgStart)
  bg.addColorStop(1, bgEnd)
  ctx.fillStyle = bg
  ctx.fillRect(0, 0, totalW, totalH)

  // Subtle gold inset border.
  ctx.strokeStyle = gold
  ctx.globalAlpha = 0.35
  ctx.lineWidth = 2 * S
  ctx.strokeRect(pad / 2, pad / 2, totalW - pad, totalH - pad)
  ctx.globalAlpha = 1

  // ── Header ────────────────────────────────────────────────────────────────
  const topY = pad
  const logo = await loadImage(opts.logoUrl || '/images/logo.png')
  let logoW = 0
  if (logo && logo.width > 0) {
    logoW = (logo.width / logo.height) * logoH
    ctx.drawImage(logo, totalW - pad - logoW, topY, logoW, logoH)
  }

  // Title + "Bingo Card" on the left.
  ctx.textBaseline = 'alphabetic'
  ctx.textAlign = 'left'
  const titleMaxW = totalW - pad * 2 - (logoW ? logoW + 24 * S : 0)
  ctx.fillStyle = gold
  ctx.font = `700 ${titleSize}px ${headerFont}`
  fitText(ctx, opts.title || 'Bingo', titleMaxW)
  ctx.fillText(opts.title || 'Bingo', pad, topY + titleSize)
  ctx.fillStyle = text
  ctx.font = `${subSize}px 'Segoe UI', system-ui, sans-serif`
  ctx.fillText('Bingo Card', pad, topY + titleSize + 10 * S + subSize)

  // Card id under the logo, right-aligned.
  ctx.textAlign = 'right'
  ctx.fillStyle = gold
  ctx.font = `600 ${cardIdSize}px 'Segoe UI', system-ui, sans-serif`
  ctx.fillText(`Card #${opts.cardId}`, totalW - pad, topY + logoH + 12 * S + cardIdSize)

  // ── Board (centered) ───────────────────────────────────────────────────────
  // The captured board has rounded corners (board-wrap border-radius). Clip to a
  // matching rounded rect so the corner triangles reveal the frame gradient
  // behind (no mismatched solid fill), then stroke a border so the card stands
  // out from the background instead of blending into it.
  const boardX = Math.round((totalW - boardW) / 2)
  const boardY = topY + headerH + gap
  const boardRadius = 16 * S
  ctx.save()
  roundRectPath(ctx, boardX, boardY, boardW, boardH, boardRadius)
  ctx.clip()
  ctx.drawImage(boardCanvas, boardX, boardY)
  ctx.restore()
  roundRectPath(ctx, boardX, boardY, boardW, boardH, boardRadius)
  ctx.strokeStyle = gold
  ctx.lineWidth = 3 * S
  ctx.stroke()

  // ── Footer ─────────────────────────────────────────────────────────────────
  // Game-details excerpt (left), with markdown bold/italic preserved.
  if (detailLines.length) {
    ctx.textAlign = 'left'
    ctx.textBaseline = 'alphabetic'
    ctx.fillStyle = textDim
    let footY = boardY + boardH + gap
    for (const lineWords of detailLines) {
      footY += detailLineH
      drawWordLine(ctx, lineWords, pad, footY - (detailLineH - detailSize), detailSize)
    }
  }

  // Site link — small and dim, tucked into the bottom-right corner, out of the way.
  ctx.textAlign = 'right'
  ctx.textBaseline = 'bottom'
  ctx.fillStyle = textDim
  ctx.globalAlpha = 0.8
  ctx.font = `600 ${linkSize}px ${DETAIL_FONT}`
  ctx.fillText(opts.link, totalW - pad, totalH - pad)
  ctx.globalAlpha = 1

  // 5. Download.
  const blob = await new Promise<Blob | null>((resolve) => out.toBlob(resolve, 'image/png'))
  if (!blob) throw new Error('Failed to render image')
  triggerDownload(blob, opts.fileName + '.png')
}

