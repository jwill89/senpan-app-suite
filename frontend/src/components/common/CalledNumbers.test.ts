import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import CalledNumbers from './CalledNumbers.vue'

const noneCalled = () => false

describe('CalledNumbers', () => {
  it('renders five columns headed B-I-N-G-O', () => {
    const w = mount(CalledNumbers, { props: { count: 0, isCalled: noneCalled } })
    expect(w.findAll('.numbers-col-header').map((h) => h.text())).toEqual(['B', 'I', 'N', 'G', 'O'])
  })

  it('dims only the columns marked inactive by activeColumns', () => {
    const w = mount(CalledNumbers, {
      props: { count: 0, isCalled: noneCalled, activeColumns: [true, false, false, false, true] },
    })
    const dimmed = w.findAll('.numbers-col').map((c) => c.classes().includes('col-unused'))
    expect(dimmed).toEqual([false, true, true, true, false])
  })

  it('dims nothing when activeColumns is omitted', () => {
    const w = mount(CalledNumbers, { props: { count: 0, isCalled: noneCalled } })
    expect(w.findAll('.numbers-col.col-unused')).toHaveLength(0)
  })

  it('highlights a called number', () => {
    const w = mount(CalledNumbers, { props: { count: 1, isCalled: (n: number) => n === 5 } })
    const called = w.findAll('.num-cell.called')
    expect(called).toHaveLength(1)
    expect(called[0].text()).toBe('5')
  })
})
