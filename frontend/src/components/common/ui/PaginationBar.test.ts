import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PaginationBar from './PaginationBar.vue'

describe('PaginationBar', () => {
  it('renders nothing when there is a single page', () => {
    const wrapper = mount(PaginationBar, { props: { page: 1, totalPages: 1 } })
    expect(wrapper.find('.pagination-bar').exists()).toBe(false)
  })

  it('disables Prev on the first page', () => {
    const wrapper = mount(PaginationBar, { props: { page: 1, totalPages: 3 } })
    const [prev, next] = wrapper.findAll('button')
    expect((prev.element as HTMLButtonElement).disabled).toBe(true)
    expect((next.element as HTMLButtonElement).disabled).toBe(false)
  })

  it('disables Next on the last page', () => {
    const wrapper = mount(PaginationBar, { props: { page: 3, totalPages: 3 } })
    const [prev, next] = wrapper.findAll('button')
    expect((prev.element as HTMLButtonElement).disabled).toBe(false)
    expect((next.element as HTMLButtonElement).disabled).toBe(true)
  })

  it('emits go with the previous / next page number', async () => {
    const wrapper = mount(PaginationBar, { props: { page: 2, totalPages: 4 } })
    const [prev, next] = wrapper.findAll('button')
    await prev.trigger('click')
    await next.trigger('click')
    expect(wrapper.emitted('go')?.[0]).toEqual([1])
    expect(wrapper.emitted('go')?.[1]).toEqual([3])
  })

  it('shows the current page indicator', () => {
    const wrapper = mount(PaginationBar, { props: { page: 2, totalPages: 4 } })
    expect(wrapper.text()).toContain('Page 2 / 4')
  })
})
