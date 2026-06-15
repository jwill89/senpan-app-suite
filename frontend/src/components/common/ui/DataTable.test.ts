import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DataTable, { type DataColumn } from './DataTable.vue'

interface Row {
  id: number
  name: string
  count: number
}

const columns: DataColumn[] = [
  { key: 'name', label: 'Name', sortable: true },
  { key: 'count', label: 'Count', align: 'center' },
]

const rows: Row[] = [
  { id: 1, name: 'Alpha', count: 3 },
  { id: 2, name: 'Beta', count: 7 },
]

describe('DataTable', () => {
  it('renders a header per column and a row per item', () => {
    const wrapper = mount(DataTable, { props: { columns, rows, rowKey: 'id' } })
    expect(wrapper.findAll('thead th')).toHaveLength(2)
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
  })

  it('renders default cell values from the row keys', () => {
    const wrapper = mount(DataTable, { props: { columns, rows, rowKey: 'id' } })
    const firstRow = wrapper.findAll('tbody tr')[0].findAll('td')
    expect(firstRow[0].text()).toBe('Alpha')
    expect(firstRow[1].text()).toBe('3')
  })

  it('marks sortable headers and emits sort with the column key on click', async () => {
    const wrapper = mount(DataTable, { props: { columns, rows, rowKey: 'id' } })
    const headers = wrapper.findAll('thead th')
    expect(headers[0].classes()).toContain('is-sortable')
    expect(headers[1].classes()).not.toContain('is-sortable')
    await headers[0].trigger('click')
    expect(wrapper.emitted('sort')?.[0]).toEqual(['name'])
  })

  it('does not emit sort for a non-sortable column', async () => {
    const wrapper = mount(DataTable, { props: { columns, rows, rowKey: 'id' } })
    await wrapper.findAll('thead th')[1].trigger('click')
    expect(wrapper.emitted('sort')).toBeUndefined()
  })

  it('shows a sort arrow only on the active sorted column', () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows, rowKey: 'id', sortKey: 'name', sortDir: 'asc' as const },
    })
    expect(wrapper.findAll('thead th')[0].text()).toContain('▲')
  })

  it('applies alignment classes from the column align option', () => {
    const wrapper = mount(DataTable, { props: { columns, rows, rowKey: 'id' } })
    expect(wrapper.findAll('thead th')[1].classes()).toContain('ta-center')
  })

  it('renders the empty slot when there are no rows', () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows: [] as Row[], rowKey: 'id' },
      slots: { empty: '<p class="none">Nothing</p>' },
    })
    expect(wrapper.find('.none').exists()).toBe(true)
    expect(wrapper.findAll('tbody tr')).toHaveLength(0)
  })
})
