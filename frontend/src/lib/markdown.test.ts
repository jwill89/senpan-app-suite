import { describe, it, expect, vi } from 'vitest'
import { useMarkdown } from './markdown'

// markdown-it is lazy-loaded on first use; wait for the reactive `ready` flag
// to flip before asserting on rendered output.
async function renderer() {
  const { render, ready } = useMarkdown()
  await vi.waitFor(() => expect(ready.value).toBe(true))
  return render
}

describe('useMarkdown', () => {
  it('renders basic inline markdown', async () => {
    const render = await renderer()
    expect(render('**bold** and _em_')).toContain('<strong>bold</strong>')
    expect(render('**bold** and _em_')).toContain('<em>em</em>')
  })

  it('escapes raw HTML rather than rendering it (html: false — XSS guard)', async () => {
    const render = await renderer()
    const out = render('<script>alert(1)</script>')
    expect(out).not.toContain('<script>')
    expect(out).toContain('&lt;script&gt;')
  })

  it('escapes a raw <img onerror> payload instead of emitting a live tag', async () => {
    const render = await renderer()
    const out = render('<img src=x onerror=alert(1)>')
    // The XSS guard: no real <img> element is produced — the payload is rendered
    // as inert, escaped text (so the onerror handler can never run).
    expect(out).not.toMatch(/<img\b/)
    expect(out).toContain('&lt;img')
  })

  it('linkifies bare URLs', async () => {
    const render = await renderer()
    expect(render('see https://example.com now')).toContain('href="https://example.com"')
  })

  it('converts single newlines to <br> (breaks: true)', async () => {
    const render = await renderer()
    expect(render('line one\nline two')).toContain('<br>')
  })

  it('returns empty string for null/undefined/empty input', async () => {
    const render = await renderer()
    expect(render('')).toBe('')
    expect(render(null)).toBe('')
    expect(render(undefined)).toBe('')
  })
})
