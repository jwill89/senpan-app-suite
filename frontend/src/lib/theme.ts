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
 */

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

/**
 * Sets the --header-font CSS variable and loads the font from Google Fonts.
 * Defaults to 'Arapey' when no font is given.
 */
export function applyHeaderFont(fontFamily: string | null | undefined): void {
  const family = fontFamily || 'Arapey'
  document.documentElement.style.setProperty('--header-font', `'${family}', serif`)
  loadGoogleFont(family)
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
