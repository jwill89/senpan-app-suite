import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PlacementEditor, { type PlaceItem } from './PlacementEditor.vue'

/**
 * The placement editors (stamp rally cards and, in their twin MapStallEditor,
 * festival pitches) were pointer-only: each item was a bare <div> carrying nothing
 * but @pointerdown, with no role, no tabindex and no key handler, so the whole
 * editor was unreachable without a mouse.
 *
 * Dragging is inherently a pointer gesture and is not what these assert. SELECTING
 * is the part that must be reachable, because once an item is selected the form's
 * numeric position, size and rotation fields edit exactly the values a drag would -
 * so a keyboard user can place things precisely, just not by dragging.
 */
function items(): PlaceItem[] {
  return [
    {
      key: 's1',
      label: 'Flora Teahouse',
      image: '',
      placement: { x: 10, y: 10, width: 20, height: 20, rotation: 0 },
      kind: 'stamp',
    },
    {
      key: 'p1',
      label: 'Grand Prize',
      image: '',
      placement: { x: 50, y: 50, width: 20, height: 20, rotation: 0 },
      kind: 'prize',
    },
  ]
}

describe('PlacementEditor accessibility', () => {
  it('exposes each item as a focusable button with a distinguishing name', () => {
    const wrapper = mount(PlacementEditor, {
      props: { cardImage: '', items: items(), selectedKey: null },
    })
    const els = wrapper.findAll('.placement-item')
    expect(els).toHaveLength(2)

    for (const el of els) {
      expect(el.attributes('role')).toBe('button')
      expect(el.attributes('tabindex')).toBe('0')
      expect(el.attributes('aria-label')).toBeTruthy()
    }
    // Named per item, so one row's control is distinguishable from the next when
    // tabbing or listing controls - not a repeated generic label.
    const labels = els.map((e) => e.attributes('aria-label'))
    expect(new Set(labels).size).toBe(labels.length)
    expect(labels[0]).toContain('Flora Teahouse')
    expect(labels[1]).toContain('Grand Prize')
  })

  it('selects an item with Enter or Space', async () => {
    const wrapper = mount(PlacementEditor, {
      props: { cardImage: '', items: items(), selectedKey: null },
    })
    const els = wrapper.findAll('.placement-item')

    await els[0].trigger('keydown.enter')
    await els[1].trigger('keydown.space')

    const selected = wrapper.emitted('select')
    expect(selected).toBeTruthy()
    expect(selected?.map((e) => e[0])).toEqual(['s1', 'p1'])
  })

  it('reports which item is selected', () => {
    const wrapper = mount(PlacementEditor, {
      props: { cardImage: '', items: items(), selectedKey: 'p1' },
    })
    const els = wrapper.findAll('.placement-item')
    expect(els[0].attributes('aria-pressed')).toBe('false')
    expect(els[1].attributes('aria-pressed')).toBe('true')
  })
})
