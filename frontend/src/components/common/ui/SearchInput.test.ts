import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SearchInput from './SearchInput.vue'

describe('SearchInput', () => {
  it('binds the model value to the input', () => {
    const wrapper = mount(SearchInput, { props: { modelValue: 'hello' } })
    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('hello')
  })

  it('emits update:modelValue on input', async () => {
    const wrapper = mount(SearchInput, { props: { modelValue: '' } })
    const input = wrapper.find('input')
    ;(input.element as HTMLInputElement).value = 'abc'
    await input.trigger('input')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['abc'])
  })

  it('uses the placeholder and aria-label props', () => {
    const wrapper = mount(SearchInput, {
      props: { modelValue: '', placeholder: 'Find patterns…', ariaLabel: 'Search patterns' },
    })
    const input = wrapper.find('input')
    expect(input.attributes('placeholder')).toBe('Find patterns…')
    expect(input.attributes('aria-label')).toBe('Search patterns')
  })
})
