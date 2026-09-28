import { describe, it, expect, vi } from 'vitest'
import { normalizeHardBreaks, useMarkdown } from './markdown'

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

  it('escapes raw HTML rather than rendering it (html: false - XSS guard)', async () => {
    const render = await renderer()
    const out = render('<script>alert(1)</script>')
    expect(out).not.toContain('<script>')
    expect(out).toContain('&lt;script&gt;')
  })

  it('escapes a raw <img onerror> payload instead of emitting a live tag', async () => {
    const render = await renderer()
    const out = render('<img src=x onerror=alert(1)>')
    // The XSS guard: no real <img> element is produced - the payload is rendered
    // as inert, escaped text (so the onerror handler can never run).
    expect(out).not.toMatch(/<img\b/)
    expect(out).toContain('&lt;img')
  })

  it('linkifies bare URLs', async () => {
    const render = await renderer()
    expect(render('see https://example.com now')).toContain('href="https://example.com"')
  })

  // markdown-it 15 pulled in linkify-it 6, which defaults `fuzzyLink` to false -
  // silently dropping the link on a scheme-less address an author typed. We turn it
  // back on, so this pins the behavior rather than whatever the transitive default
  // happens to be.
  it('linkifies a scheme-less www address', async () => {
    const render = await renderer()
    expect(render('visit www.example.com today')).toContain('href="http://www.example.com"')
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

describe('normalizeHardBreaks', () => {
  /**
   * Raw HTML is escaped (html: false), so a description holding a literal "<br />"
   * showed those characters to the reader instead of breaking the line. Text
   * arrives that way from more than one direction - pasted from a source that
   * emitted HTML, or serialized by a WYSIWYG editor that writes hard breaks as
   * tags - so it is normalized before parsing rather than by allowing raw HTML.
   */
  it('turns every spelling of a break tag into a newline', () => {
    expect(normalizeHardBreaks('one<br />two')).toBe('one\ntwo')
    expect(normalizeHardBreaks('one<br>two')).toBe('one\ntwo')
    expect(normalizeHardBreaks('one<br/>two')).toBe('one\ntwo')
    expect(normalizeHardBreaks('one<BR />two')).toBe('one\ntwo')
    expect(normalizeHardBreaks('one<br   />two')).toBe('one\ntwo')
  })

  it('leaves everything else exactly as written', () => {
    // Only this one inert tag is promoted; every other tag stays text for the
    // parser to escape, which is what keeps raw HTML out.
    expect(normalizeHardBreaks('a <b>bold</b> <script>x</script>')).toBe(
      'a <b>bold</b> <script>x</script>',
    )
    expect(normalizeHardBreaks('plain text')).toBe('plain text')
  })

  it('does not touch a break tag shown inside code', () => {
    // Someone documenting the tag should see it, not a blank line.
    expect(normalizeHardBreaks('use `<br />` here')).toBe('use `<br />` here')
    expect(normalizeHardBreaks('```\n<br />\n```')).toBe('```\n<br />\n```')
    expect(normalizeHardBreaks('~~~\n<br />\n~~~')).toBe('~~~\n<br />\n~~~')
    // ...while a tag OUTSIDE the code in the same string still converts.
    expect(normalizeHardBreaks('`<br />` then<br />done')).toBe('`<br />` then\ndone')
  })
})
