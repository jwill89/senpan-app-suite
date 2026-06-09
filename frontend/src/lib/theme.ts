/**
 * DOM head manipulation helpers for fonts and custom themes.
 *
 * Mirrors the original app.js methods:
 *   _applyHeaderFont / _loadGoogleFont — load a Google Font and set the
 *     --header-font CSS variable used by headings and the bingo board.
 *   _applyCustomCSS — inject/update the active custom theme <style> element.
 *
 * Custom themes are global, class-based CSS that override the design tokens on
 * :root — so they must continue to be injected into <head> verbatim.
 *
 * In addition to Google Fonts, the header/board font can be one of the fonts
 * uploaded via System → Font Upload. `applyUploadedFonts()` registers an
 * @font-face for each uploaded file (served from FONT_BASE_URL) so those
 * families are usable anywhere `--header-font` is — for players too, not just
 * the admin preview. `applyHeaderFont()` then skips the Google Fonts request
 * for a family it knows is an uploaded one.
 */
import { FONT_BASE_URL } from '@/stores/fonts'

/**
 * Injects a Google Fonts <link> stylesheet for the given font family.
 * Tracks loaded fonts by element id to avoid duplicates.
 */
export function loadGoogleFont(fontFamily: string): void {
  if (!fontFamily) return
  const id = 'gfont-' + fontFamily.replace(/\s+/g, '-').toLowerCase()
  if (document.getElementById(id)) return
  const link = document.createElement('link')
  link.id = id
  link.rel = 'stylesheet'
  link.href = `https://fonts.googleapis.com/css2?family=${encodeURIComponent(
    fontFamily,
  )}:ital,wght@0,400;0,700;0,800;1,400&display=swap`
  document.head.appendChild(link)
}

// ── Uploaded fonts (System → Font Upload) ───────────────────────────────────

/** Maps a font file extension to its CSS @font-face format() hint. */
const FONT_FORMAT_HINTS: Record<string, string> = {
  ttf: 'truetype',
  otf: 'opentype',
  woff: 'woff',
  woff2: 'woff2',
  eot: 'embedded-opentype',
}

/** Families of the currently-registered uploaded fonts (no Google load needed). */
let uploadedFamilies = new Set<string>()

/** Derives the CSS font-family name from a font filename (strips the extension). */
export function fontFamilyFromFile(filename: string): string {
  return filename.replace(/\.[^.]+$/, '').trim()
}

/** True when `family` is one of the registered uploaded fonts. */
export function isUploadedFamily(family: string): boolean {
  return uploadedFamilies.has(family.trim())
}

// ── Font-metric clamping ────────────────────────────────────────────────────
//
// Some fonts bake oversized vertical metrics (ascent / descent / line-gap) into
// the file, so the browser reserves a huge empty box above and below the
// glyphs. On the bingo board and in the settings preview this shows up as
// fonts that look tiny / mis-centred or get clipped by their container.
//
// We detect the offenders by measuring each loaded font's bounding box with the
// Canvas TextMetrics API, then normalise them via the CSS @font-face metric
// override descriptors (ascent-override / descent-override / line-gap-override),
// which change the metrics used for layout WITHOUT scaling the glyphs. Fonts
// with sane metrics are left untouched.

/** A clamped font's ascent/descent as fractions of the em (e.g. 0.8 = 80%). */
export interface FontMetricOverride {
  ascent: number
  descent: number
}

/** Only clamp fonts whose total box (ascent+descent) exceeds this many ems. */
const METRIC_CLAMP_THRESHOLD = 1.5
/** Target total box for a clamped font — comfortable, even breathing room. */
const METRIC_CLAMP_TARGET = 1.25

/**
 * Given a font's measured ascent and descent (as fractions of the em), returns
 * clamped override metrics when the font's box is oversized, or null when the
 * font is already well-proportioned and should be left as-is. The clamp scales
 * ascent and descent proportionally so the glyphs stay vertically centred.
 */
export function clampFontMetrics(
  ascentRatio: number,
  descentRatio: number,
): FontMetricOverride | null {
  if (!Number.isFinite(ascentRatio) || !Number.isFinite(descentRatio)) return null
  if (ascentRatio < 0 || descentRatio < 0) return null
  const total = ascentRatio + descentRatio
  if (!(total > METRIC_CLAMP_THRESHOLD)) return null
  const scale = METRIC_CLAMP_TARGET / total
  return { ascent: ascentRatio * scale, descent: descentRatio * scale }
}

/** Override metrics for clamped families, plus families already evaluated. */
const metricOverrides = new Map<string, FontMetricOverride>()
const measuredFamilies = new Set<string>()
/** The most recent filename list, so a clamp pass can rewrite the same set. */
let lastFontFilenames: string[] = []

/** Builds a single @font-face rule, including override descriptors if clamped. */
function fontFaceRule(file: string): string {
  const family = fontFamilyFromFile(file)
  if (!family) return ''
  const ext = (file.split('.').pop() || '').toLowerCase()
  const hint = FONT_FORMAT_HINTS[ext]
  const url = `${FONT_BASE_URL}/${encodeURIComponent(file)}`
  const formatPart = hint ? ` format('${hint}')` : ''
  const o = metricOverrides.get(family)
  const overridePart = o
    ? `ascent-override:${(o.ascent * 100).toFixed(1)}%;` +
      `descent-override:${(o.descent * 100).toFixed(1)}%;line-gap-override:0%;`
    : ''
  return `@font-face{font-family:'${family}';src:url('${url}')${formatPart};${overridePart}font-display:swap;}`
}

/** Writes the <style id="uploaded-fonts"> element with the current rules. */
function writeUploadedFontStyle(filenames: string[]): void {
  const rules = filenames.map(fontFaceRule).filter(Boolean).join('\n')
  let el = document.getElementById('uploaded-fonts') as HTMLStyleElement | null
  if (!el) {
    el = document.createElement('style')
    el.id = 'uploaded-fonts'
    document.head.appendChild(el)
  }
  el.textContent = rules
}

/**
 * Measures a loaded font's bounding box via canvas and returns clamp overrides
 * for it (or null when it needn't be clamped / can't be measured). The font
 * must already be loaded, otherwise the canvas falls back to a system font and
 * the measurement is meaningless.
 */
function measureFontOverride(family: string): FontMetricOverride | null {
  try {
    const ctx = document.createElement('canvas').getContext('2d')
    if (!ctx) return null
    const px = 100
    ctx.font = `${px}px '${family}'`
    const m = ctx.measureText('Hg')
    const asc = m.fontBoundingBoxAscent
    const desc = m.fontBoundingBoxDescent
    if (asc === undefined || desc === undefined) return null
    return clampFontMetrics(asc / px, desc / px)
  } catch {
    return null
  }
}

/**
 * Loads each uploaded font, measures its metrics, and re-registers any oversized
 * ones with clamp overrides. Runs once per family (results are cached). Silently
 * no-ops where the Font Loading API is unavailable (e.g. SSR / older browsers).
 */
async function clampUploadedFontMetrics(filenames: string[]): Promise<void> {
  if (typeof document === 'undefined' || !document.fonts) return
  let changed = false
  await Promise.all(
    filenames.map(async (file) => {
      const family = fontFamilyFromFile(file)
      if (!family || measuredFamilies.has(family)) return
      try {
        await document.fonts.load(`32px '${family}'`)
      } catch {
        return // unloaded font would measure as the fallback — skip, retry later
      }
      measuredFamilies.add(family)
      const o = measureFontOverride(family)
      if (o) {
        metricOverrides.set(family, o)
        changed = true
      }
    }),
  )
  if (changed) writeUploadedFontStyle(lastFontFilenames)
}

/**
 * Registers @font-face rules for the given uploaded font filenames, replacing
 * any previously-registered set. Each family is the filename without its
 * extension; the source is the file served from FONT_BASE_URL. Safe to call
 * repeatedly (it rewrites a single <style> element).
 *
 * After registering, it asynchronously measures the fonts and re-registers any
 * with oversized vertical metrics using clamp overrides (see above).
 */
export function applyUploadedFonts(filenames: string[]): void {
  uploadedFamilies = new Set(filenames.map(fontFamilyFromFile).filter(Boolean))
  lastFontFilenames = filenames.slice()

  // Forget cached results for families that are no longer present.
  for (const fam of [...measuredFamilies]) {
    if (!uploadedFamilies.has(fam)) {
      measuredFamilies.delete(fam)
      metricOverrides.delete(fam)
    }
  }

  writeUploadedFontStyle(filenames)
  void clampUploadedFontMetrics(filenames)
}

/**
 * Sets the --header-font CSS variable and loads the font. Uploaded fonts are
 * already registered via applyUploadedFonts(), so only non-uploaded families
 * are fetched from Google Fonts. Defaults to 'Arapey' when no font is given.
 */
export function applyHeaderFont(fontFamily: string | null | undefined): void {
  const family = fontFamily || 'Arapey'
  document.documentElement.style.setProperty('--header-font', `'${family}', serif`)
  if (!isUploadedFamily(family)) loadGoogleFont(family)
}

/** Injects or updates the custom CSS <style> element in the document head. */
export function applyCustomCSS(css: string): void {
  let el = document.getElementById('bingo-custom-theme') as HTMLStyleElement | null
  if (!el) {
    el = document.createElement('style')
    el.id = 'bingo-custom-theme'
    document.head.appendChild(el)
  }
  el.textContent = css
}
