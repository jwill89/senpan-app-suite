import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ManagerView from './ManagerView.vue'

describe('ManagerView', () => {
  it('renders title + icon in the header', () => {
    const wrapper = mount(ManagerView, { props: { title: 'Patterns', icon: 'fa-x' } })
    const h3 = wrapper.find('.manager-header h3')
    expect(h3.text()).toBe('Patterns')
    expect(h3.find('i.fa-x').exists()).toBe(true)
  })

  it('renders the default slot (list body)', () => {
    const wrapper = mount(ManagerView, {
      props: { title: 'X' },
      slots: { default: '<div class="rows">items</div>' },
    })
    expect(wrapper.find('.rows').exists()).toBe(true)
  })

  it('renders actions, toolbar, and pagination slots when provided', () => {
    const wrapper = mount(ManagerView, {
      props: { title: 'X' },
      slots: {
        actions: '<button class="act">New</button>',
        toolbar: '<div class="tb">search</div>',
        pagination: '<nav class="pg">pages</nav>',
      },
    })
    expect(wrapper.find('.manager-actions .act').exists()).toBe(true)
    expect(wrapper.find('.manager-toolbar .tb').exists()).toBe(true)
    expect(wrapper.find('.pg').exists()).toBe(true)
  })

  it('omits the actions/toolbar wrappers when those slots are absent', () => {
    const wrapper = mount(ManagerView, { props: { title: 'X' } })
    expect(wrapper.find('.manager-actions').exists()).toBe(false)
    expect(wrapper.find('.manager-toolbar').exists()).toBe(false)
  })
})
