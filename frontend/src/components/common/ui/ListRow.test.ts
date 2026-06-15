import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ListRow from './ListRow.vue'

describe('ListRow', () => {
  it('renders the body (default slot)', () => {
    const wrapper = mount(ListRow, { slots: { default: '<h4>Title</h4>' } })
    expect(wrapper.find('.list-row-body h4').text()).toBe('Title')
  })

  it('renders media and actions slots, with actions pinned in their own region', () => {
    const wrapper = mount(ListRow, {
      slots: {
        media: '<img class="cover" />',
        default: 'body',
        actions: '<button class="del">x</button>',
      },
    })
    expect(wrapper.find('.list-row-media .cover').exists()).toBe(true)
    expect(wrapper.find('.list-row-actions .del').exists()).toBe(true)
  })

  it('omits media/actions wrappers when those slots are absent', () => {
    const wrapper = mount(ListRow, { slots: { default: 'body' } })
    expect(wrapper.find('.list-row-media').exists()).toBe(false)
    expect(wrapper.find('.list-row-actions').exists()).toBe(false)
  })
})
