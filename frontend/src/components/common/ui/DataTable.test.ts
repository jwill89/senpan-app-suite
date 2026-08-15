import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DataTable, { type DataColumn, type DataTableView } from './DataTable.vue'

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

  const names = (w: ReturnType<typeof mount>) =>
    w.findAll('tbody tr').map((tr) => tr.findAll('td')[0].text())

  it('marks sortable headers and sorts the rows itself on click', async () => {
    const wrapper = mount(DataTable, { props: { columns, rows, rowKey: 'id' } })
    const headers = wrapper.findAll('thead th')
    expect(headers[0].classes()).toContain('is-sortable')
    expect(headers[1].classes()).not.toContain('is-sortable')
    await headers[0].trigger('click')
    expect(names(wrapper)).toEqual(['Alpha', 'Beta'])
    await headers[0].trigger('click')
    expect(names(wrapper)).toEqual(['Beta', 'Alpha'])
  })

  it('does not sort on a non-sortable column', async () => {
    const wrapper = mount(DataTable, { props: { columns, rows, rowKey: 'id' } })
    await wrapper.findAll('thead th')[1].trigger('click')
    expect(wrapper.find('.dt-sort-icon').exists()).toBe(false)
    expect(names(wrapper)).toEqual(['Alpha', 'Beta'])
  })

  it('opens on the column given by defaultSort, in the given direction', () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows, rowKey: 'id', defaultSort: { key: 'name', dir: 'desc' as const } },
    })
    expect(names(wrapper)).toEqual(['Beta', 'Alpha'])
    const headers = wrapper.findAll('thead th')
    expect(headers[0].find('.dt-sort-icon').exists()).toBe(true)
    expect(headers[1].find('.dt-sort-icon').exists()).toBe(false)
  })

  it('points the sort icon in the sorted direction', () => {
    const asc = mount(DataTable, {
      props: { columns, rows, rowKey: 'id', defaultSort: { key: 'name', dir: 'asc' as const } },
    })
    const desc = mount(DataTable, {
      props: { columns, rows, rowKey: 'id', defaultSort: { key: 'name', dir: 'desc' as const } },
    })
    // The global stub exposes the rendered icon name as `data-icon`.
    expect(asc.find('.dt-sort-icon').attributes('data-icon')).toBe('chevron-up')
    expect(desc.find('.dt-sort-icon').attributes('data-icon')).toBe('chevron-down')
  })

  it('sorts numerically, not lexically, on a numeric column', async () => {
    const many = [
      { id: 1, name: 'a', count: 10 },
      { id: 2, name: 'b', count: 9 },
      { id: 3, name: 'c', count: 100 },
    ]
    const cols: DataColumn[] = [{ key: 'count', label: 'Count', sortable: true }]
    const wrapper = mount(DataTable, { props: { columns: cols, rows: many, rowKey: 'id' } })
    await wrapper.findAll('thead th')[0].trigger('click')
    expect(wrapper.findAll('tbody tr').map((tr) => tr.text())).toEqual(['9', '10', '100'])
  })

  it('filters rows, and reports the filtered total rather than the page size', async () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows, rowKey: 'id', filter: 'alp' },
    })
    expect(names(wrapper)).toEqual(['Alpha'])
    const view = wrapper.emitted('update:view')?.at(-1)?.[0] as { total: number }
    expect(view.total).toBe(1)
  })

  it('honours a custom filterFn over the default all-column match', () => {
    const wrapper = mount(DataTable, {
      props: {
        columns,
        rows,
        rowKey: 'id',
        filter: '7',
        // Only match on name, so the row whose *count* is 7 must not appear.
        filterFn: (r: unknown, q: string) => (r as Row).name.toLowerCase().includes(q),
      },
    })
    expect(names(wrapper)).toEqual([])
  })

  it('sorts BEFORE paginating, so page 1 holds the true first rows', async () => {
    // The bug this guards: sorting a page instead of the list. Descending by
    // name, page 1 of 1-per-page must be Gamma - not Alpha reordered in place.
    const three = [
      { id: 1, name: 'Alpha', count: 1 },
      { id: 2, name: 'Beta', count: 2 },
      { id: 3, name: 'Gamma', count: 3 },
    ]
    const wrapper = mount(DataTable, {
      props: {
        columns,
        rows: three,
        rowKey: 'id',
        pageSize: 1,
        defaultSort: { key: 'name', dir: 'desc' as const },
      },
    })
    expect(names(wrapper)).toEqual(['Gamma'])
    const view = wrapper.emitted('update:view')?.at(-1)?.[0] as { totalPages: number }
    expect(view.totalPages).toBe(3)
  })

  it('clamps the page back into range when the filter shrinks the result set', async () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows, rowKey: 'id', pageSize: 1, page: 2 },
    })
    expect(names(wrapper)).toEqual(['Beta'])
    await wrapper.setProps({ filter: 'alp' })
    expect(wrapper.emitted('update:page')?.at(-1)).toEqual([1])
  })

  it('applies alignment classes from the column align option', () => {
    const wrapper = mount(DataTable, { props: { columns, rows, rowKey: 'id' } })
    expect(wrapper.findAll('thead th')[1].classes()).toContain('ta-center')
  })

  it('applies a per-row class from the rowClass function', () => {
    const wrapper = mount(DataTable, {
      props: {
        columns,
        rows,
        rowKey: 'id',
        rowClass: (r: unknown) => ({ 'row-selected': (r as Row).id === 2 }),
      },
    })
    const trs = wrapper.findAll('tbody tr')
    expect(trs[0].classes()).not.toContain('row-selected')
    expect(trs[1].classes()).toContain('row-selected')
  })

  it('applies a static rowClass string to every row', () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows, rowKey: 'id', rowClass: 'zebra' },
    })
    expect(wrapper.findAll('tbody tr').every((tr) => tr.classes().includes('zebra'))).toBe(true)
  })

  it('renders the empty slot when there are no rows', () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows: [] as Row[], rowKey: 'id' },
      slots: { empty: '<p class="none">Nothing</p>' },
    })
    expect(wrapper.find('.none').exists()).toBe(true)
    expect(wrapper.findAll('tbody tr')).toHaveLength(0)
  })

  it('is not expandable and rows are inert without a #detail slot', async () => {
    const wrapper = mount(DataTable, { props: { columns, rows, rowKey: 'id' } })
    const firstRow = wrapper.findAll('tbody tr')[0]
    expect(firstRow.classes()).not.toContain('dt-expandable')
    await firstRow.trigger('click')
    expect(wrapper.findAll('tbody tr')).toHaveLength(2) // no detail row inserted
  })

  it('toggles an expandable detail row on click when a #detail slot is provided', async () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows, rowKey: 'id' },
      slots: { detail: '<span class="detail">D</span>' },
    })
    expect(wrapper.findAll('tbody tr')[0].classes()).toContain('dt-expandable')
    expect(wrapper.findAll('.detail')).toHaveLength(0)
    await wrapper.findAll('tbody tr')[0].trigger('click')
    expect(wrapper.findAll('.detail')).toHaveLength(1)
    await wrapper.findAll('tbody tr')[0].trigger('click') // collapse
    expect(wrapper.findAll('.detail')).toHaveLength(0)
  })

  it('applies a fixed column width from the column width option', () => {
    const wide: DataColumn[] = [{ key: 'name', label: 'Name', width: '120px' }]
    const wrapper = mount(DataTable, { props: { columns: wide, rows, rowKey: 'id' } })
    expect(wrapper.find('thead th').attributes('style')).toContain('width: 120px')
  })

  it('exposes expandable rows as keyboard-operable buttons with aria-expanded', async () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows, rowKey: 'id' },
      slots: { detail: '<span class="detail">D</span>' },
    })
    const firstRow = wrapper.findAll('tbody tr')[0]
    expect(firstRow.attributes('role')).toBe('button')
    expect(firstRow.attributes('tabindex')).toBe('0')
    expect(firstRow.attributes('aria-expanded')).toBe('false')
    await firstRow.trigger('keydown', { key: 'Enter' })
    expect(wrapper.findAll('.detail')).toHaveLength(1)
    expect(wrapper.findAll('tbody tr')[0].attributes('aria-expanded')).toBe('true')
  })

  it('toggles an expandable row on Space', async () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows, rowKey: 'id' },
      slots: { detail: '<span class="detail">D</span>' },
    })
    await wrapper.findAll('tbody tr')[0].trigger('keydown', { key: ' ' })
    expect(wrapper.findAll('.detail')).toHaveLength(1)
  })

  it('does not add button semantics to rows without a #detail slot', () => {
    const wrapper = mount(DataTable, { props: { columns, rows, rowKey: 'id' } })
    const tr = wrapper.findAll('tbody tr')[0]
    expect(tr.attributes('role')).toBeUndefined()
    expect(tr.attributes('tabindex')).toBeUndefined()
    expect(tr.attributes('aria-expanded')).toBeUndefined()
  })

  it('lets clicks on inner cell controls through without toggling the row', async () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows, rowKey: 'id' },
      slots: {
        'cell-name': '<button class="inner">Act</button>',
        detail: '<span class="detail">D</span>',
      },
    })
    await wrapper.find('.inner').trigger('click')
    expect(wrapper.findAll('.detail')).toHaveLength(0) // row stayed collapsed
  })

  it('does not toggle when a keydown originates from an inner control', async () => {
    const wrapper = mount(DataTable, {
      props: { columns, rows, rowKey: 'id' },
      slots: {
        'cell-name': '<button class="inner">Act</button>',
        detail: '<span class="detail">D</span>',
      },
    })
    await wrapper.find('.inner').trigger('keydown', { key: 'Enter' })
    expect(wrapper.findAll('.detail')).toHaveLength(0)
  })
})

describe('DataTable opt-in features', () => {
  const many: Row[] = [
    { id: 1, name: 'Ann', count: 1 },
    { id: 2, name: 'Bob', count: 2 },
    { id: 3, name: 'Ann', count: 3 },
    { id: 4, name: 'Cid', count: 4 },
  ]
  const cols: DataColumn[] = [
    { key: 'name', label: 'Name', sortable: true, facetable: true },
    { key: 'count', label: 'Count', sortable: true },
  ]
  const lastView = (w: ReturnType<typeof mount>) =>
    w.emitted('update:view')?.at(-1)?.[0] as DataTableView

  /** Capture the CSV text a call to exportCsv would download. */
  function captureCsv(run: () => void): string {
    let csv = ''
    const OrigBlob = globalThis.Blob
    const origCreate = URL.createObjectURL
    const origRevoke = URL.revokeObjectURL
    globalThis.Blob = class extends OrigBlob {
      constructor(parts: BlobPart[], opts?: BlobPropertyBag) {
        super(parts, opts)
        // exportCsv passes a single string part; capture it without stringifying
        // BlobPart, which could legitimately be a Blob/ArrayBuffer.
        csv = parts.filter((x): x is string => typeof x === 'string').join('')
      }
    } as unknown as typeof Blob
    URL.createObjectURL = () => 'blob:stub'
    URL.revokeObjectURL = () => undefined
    try {
      run()
    } finally {
      globalThis.Blob = OrigBlob
      URL.createObjectURL = origCreate
      URL.revokeObjectURL = origRevoke
    }
    return csv
  }
  const exportFrom = (w: ReturnType<typeof mount>, name?: string): string =>
    captureCsv(() => (w.vm as unknown as { exportCsv: (n?: string) => void }).exportCsv(name))

  // -- grouping ---------------------------------------------------------------
  it('group-by keeps rows sharing a value contiguous while sorting within them', async () => {
    const wrapper = mount(DataTable, {
      props: { columns: cols, rows: many, rowKey: 'id', groupBy: 'name' },
    })
    const names = () => wrapper.findAll('tbody tr').map((tr) => tr.findAll('td')[0].text())
    expect(names()).toEqual(['Ann', 'Ann', 'Bob', 'Cid'])
    // Sorting by count must not scatter the two Anns.
    await wrapper.findAll('thead th')[1].trigger('click')
    expect(names()).toEqual(['Ann', 'Ann', 'Bob', 'Cid'])
    const counts = wrapper.findAll('tbody tr').map((tr) => tr.findAll('td')[1].text())
    expect(counts.slice(0, 2)).toEqual(['1', '3'])
  })

  // -- selection --------------------------------------------------------------
  it('selectable renders a checkbox column and toggles a row key', async () => {
    const wrapper = mount(DataTable, {
      props: { columns: cols, rows: many, rowKey: 'id', selectable: true, selected: [] },
    })
    expect(wrapper.findAll('thead th')).toHaveLength(3)
    const boxes = wrapper.findAll('tbody input[type="checkbox"]')
    expect(boxes).toHaveLength(4)
    await boxes[1].trigger('change')
    expect(wrapper.emitted('update:selected')?.at(-1)?.[0]).toEqual([2])
  })

  it('the header checkbox selects every FILTERED row, not just the page', async () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: cols,
        rows: many,
        rowKey: 'id',
        selectable: true,
        selected: [],
        filter: 'ann',
        pageSize: 1,
      },
    })
    // One row on screen, but two match the filter.
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    await wrapper.find('thead input[type="checkbox"]').trigger('change')
    expect(wrapper.emitted('update:selected')?.at(-1)?.[0]).toEqual([1, 3])
  })

  // -- faceted filters --------------------------------------------------------
  it('reports facet values with counts for a facetable column', () => {
    const wrapper = mount(DataTable, { props: { columns: cols, rows: many, rowKey: 'id' } })
    expect(lastView(wrapper).facets.name).toEqual([
      { value: 'Ann', count: 2 },
      { value: 'Bob', count: 1 },
      { value: 'Cid', count: 1 },
    ])
  })

  it('column-filters narrow the rows by exact value', () => {
    const wrapper = mount(DataTable, {
      props: { columns: cols, rows: many, rowKey: 'id', columnFilters: { name: 'Ann' } },
    })
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(lastView(wrapper).total).toBe(2)
  })

  // -- resizing ---------------------------------------------------------------
  it('resizable renders a grip per column, and only then', () => {
    const plain = mount(DataTable, { props: { columns: cols, rows: many, rowKey: 'id' } })
    expect(plain.findAll('.dt-resizer')).toHaveLength(0)

    const wrapper = mount(DataTable, {
      props: { columns: cols, rows: many, rowKey: 'id', resizable: true },
    })
    expect(wrapper.findAll('.dt-resizer')).toHaveLength(2)
  })

  it('resizable does not re-lay-out columns until one is actually dragged', () => {
    // Guards the trap in making a table resizable: reading TanStack's size
    // eagerly pins every unsized column to its 150px default, silently
    // reflowing tables nobody touched. Declared widths must survive, and
    // undeclared ones must stay auto.
    const sized: DataColumn[] = [
      { key: 'name', label: 'Name', width: '80px' },
      { key: 'count', label: 'Count' },
    ]
    const wrapper = mount(DataTable, {
      props: { columns: sized, rows: many, rowKey: 'id', resizable: true },
    })
    const th = wrapper.findAll('thead th')
    expect(th[0].attributes('style')).toContain('80px')
    expect(th[1].attributes('style')).toBeUndefined()
  })

  it('a click on the resize grip does not sort the column', async () => {
    const wrapper = mount(DataTable, {
      props: { columns: cols, rows: many, rowKey: 'id', resizable: true },
    })
    await wrapper.findAll('.dt-resizer')[0].trigger('click')
    expect(wrapper.find('.dt-sort-icon').exists()).toBe(false)
  })

  // -- CSV --------------------------------------------------------------------
  it('exportCsv writes every filtered row in sort order, not just the page', () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: cols,
        rows: many,
        rowKey: 'id',
        filter: 'ann',
        pageSize: 1,
        defaultSort: { key: 'count', dir: 'desc' as const },
      },
    })
    const lines = exportFrom(wrapper, 'rows').replace(/^﻿/, '').trim().split('\r\n')
    expect(lines[0]).toBe('Name,Count')
    // Both matching rows, descending by count - despite pageSize 1.
    expect(lines.slice(1)).toEqual(['Ann,3', 'Ann,1'])
  })

  it('exportCsv quotes fields containing a comma or quote', () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }] as DataColumn[],
        rows: [{ id: 1, name: 'Last, First "Nick"', count: 0 }],
        rowKey: 'id',
      },
    })
    expect(exportFrom(wrapper)).toContain('"Last, First ""Nick"""')
  })

  it('exportCsv leaves out columns marked noExport', () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [
          { key: 'name', label: 'Name' },
          { key: 'count', label: 'Count', noExport: true },
        ] as DataColumn[],
        rows: [{ id: 1, name: 'Ann', count: 7 }],
        rowKey: 'id',
      },
    })
    const csv = exportFrom(wrapper)
    expect(csv).toContain('Name')
    expect(csv).not.toContain('Count')
    expect(csv).not.toContain('7')
  })
})
