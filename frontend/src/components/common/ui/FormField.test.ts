import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FormField from './FormField.vue'

describe('FormField', () => {
  it('renders the label and the control slot', () => {
    const wrapper = mount(FormField, {
      props: { label: 'Title' },
      slots: { default: '<input class="probe" />' },
    })
    expect(wrapper.find('.field-label').text()).toBe('Title')
    expect(wrapper.find('input.probe').exists()).toBe(true)
  })

  it('appends a required marker when required', () => {
    const wrapper = mount(FormField, { props: { label: 'Title', required: true } })
    expect(wrapper.find('.field-label .req').exists()).toBe(true)
  })

  it('omits the required marker by default', () => {
    const wrapper = mount(FormField, { props: { label: 'Title' } })
    expect(wrapper.find('.req').exists()).toBe(false)
  })

  it('renders help text from the help prop', () => {
    const wrapper = mount(FormField, { props: { label: 'X', help: 'Be careful' } })
    expect(wrapper.find('.field-help').text()).toBe('Be careful')
  })

  it('renders the #help slot over the help prop when both are given', () => {
    const wrapper = mount(FormField, {
      props: { label: 'X', help: 'plain' },
      slots: { help: '<a href="#">link</a>' },
    })
    expect(wrapper.find('.field-help a').exists()).toBe(true)
  })

  it('renders no label element when neither label prop nor slot is given', () => {
    const wrapper = mount(FormField, { slots: { default: '<input />' } })
    expect(wrapper.find('.field-label').exists()).toBe(false)
  })

  it('wires htmlFor to the label for attribute', () => {
    const wrapper = mount(FormField, { props: { label: 'X', htmlFor: 'my-input' } })
    expect(wrapper.find('.field-label').attributes('for')).toBe('my-input')
  })
})
