import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SubPageHeader from './SubPageHeader.vue'

describe('SubPageHeader', () => {
  it('renders the title and a Back button', () => {
    const wrapper = mount(SubPageHeader, { props: { title: 'New Pattern' } })
    expect(wrapper.find('h3').text()).toBe('New Pattern')
    expect(wrapper.find('button').text()).toContain('Back')
  })

  it('emits back when the Back button is clicked', async () => {
    const wrapper = mount(SubPageHeader, { props: { title: 'X' } })
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('back')).toHaveLength(1)
  })

  it('renders a custom title via the default slot', () => {
    const wrapper = mount(SubPageHeader, { slots: { default: '<span class="c">Edit</span>' } })
    expect(wrapper.find('h3 .c').text()).toBe('Edit')
  })
})
