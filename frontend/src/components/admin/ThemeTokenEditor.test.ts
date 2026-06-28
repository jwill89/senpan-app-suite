import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ThemeTokenEditor from './ThemeTokenEditor.vue'
import { THEME_TOKENS, defaultTokens } from '@/lib/theme-tokens'
import { auditTheme } from '@/lib/wcag'

describe('ThemeTokenEditor', () => {
  it('renders one row per token plus the live preview', () => {
    const wrapper = mount(ThemeTokenEditor, { props: { modelValue: defaultTokens() } })
    expect(wrapper.findAll('.token-row')).toHaveLength(THEME_TOKENS.length)
    expect(wrapper.find('.token-preview').exists()).toBe(true)
  })

  it('emits an updated token map when a value field changes', async () => {
    const model = defaultTokens()
    const wrapper = mount(ThemeTokenEditor, { props: { modelValue: model } })

    // The first value field corresponds to the first token (page-bg).
    const firstValue = wrapper.find('.token-row__value')
    await firstValue.setValue('#123456')

    const emits = wrapper.emitted('update:modelValue')
    expect(emits).toBeTruthy()
    const payload = emits![0][0] as Record<string, string>
    expect(payload[THEME_TOKENS[0].name]).toBe('#123456')
    // The emit is a copy, not a mutation of the original model.
    expect(model[THEME_TOKENS[0].name]).toBe(THEME_TOKENS[0].default)
  })

  it('shows a WCAG compliance verdict (AAA for the default token set)', () => {
    const wrapper = mount(ThemeTokenEditor, { props: { modelValue: defaultTokens() } })
    const verdict = wrapper.find('.wcag__verdict')
    expect(verdict.exists()).toBe(true)
    expect(verdict.text()).toBe('WCAG AAA')
  })

  it('shows help text (the usage description) under every token', () => {
    const wrapper = mount(ThemeTokenEditor, { props: { modelValue: defaultTokens() } })
    const descs = wrapper.findAll('.token-row__desc')
    expect(descs).toHaveLength(THEME_TOKENS.length)
    // The first token (page-bg) shows its description.
    expect(descs[0].text()).toBe(THEME_TOKENS[0].desc)
    expect(descs[0].text().length).toBeGreaterThan(0)
  })

  it('has a preview element for every audited pairing (so "Find" always lands)', () => {
    const wrapper = mount(ThemeTokenEditor, { props: { modelValue: defaultTokens() } })
    const ids = auditTheme(defaultTokens()).results.map((r) => r.id)
    for (const id of ids) {
      // data-pair lists space-separated ids; match the whole token (~=).
      const el = wrapper.find(`[data-pair~="${id}"]`)
      expect(el.exists(), `preview is missing an element for pairing "${id}"`).toBe(true)
    }
  })

  it('includes the neutral button, link, and winner chip in the preview', () => {
    const wrapper = mount(ThemeTokenEditor, { props: { modelValue: defaultTokens() } })
    expect(wrapper.find('.tp-btn--neutral').exists()).toBe(true)
    expect(wrapper.find('.tp-link').exists()).toBe(true)
    expect(wrapper.find('.tp-winner').exists()).toBe(true)
  })

  it('renders detailed findings with a live contrast chip for a failing theme', () => {
    // text == page-bg fails the body/page pairing.
    const broken = { ...defaultTokens(), text: '#1a1c17', 'page-bg': '#1a1c17' }
    const wrapper = mount(ThemeTokenEditor, { props: { modelValue: broken } })
    const findings = wrapper.findAll('.wcag__finding--error')
    expect(findings.length).toBeGreaterThan(0)
    // Each finding carries a chip and a Find-in-preview button.
    const first = findings[0]
    expect(first.find('.wcag__chip').exists()).toBe(true)
    expect(first.find('.wcag__find').exists()).toBe(true)
    expect(first.find('.wcag__tokens').text()).toContain('--')
  })

  it('uses a native colour input for solid tokens and a picker button for alpha tokens', () => {
    const wrapper = mount(ThemeTokenEditor, { props: { modelValue: defaultTokens() } })
    const solid = THEME_TOKENS.filter((t) => !t.alpha).length
    const alpha = THEME_TOKENS.filter((t) => t.alpha).length
    // Solid tokens get the lightweight native picker; alpha tokens get a swatch
    // button that opens the cross-browser (alpha-capable) Chrome picker.
    expect(wrapper.findAll('input[type="color"]')).toHaveLength(solid)
    expect(wrapper.findAll('.token-row__color--btn')).toHaveLength(alpha)
    expect(alpha).toBeGreaterThan(0)
  })
})
