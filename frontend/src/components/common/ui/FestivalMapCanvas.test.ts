import { describe, it, expect, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import FestivalMapCanvas from './FestivalMapCanvas.vue'
import type { PublicFestivalStall, PublicStallOccupant } from '@/types/api'

/**
 * The map has to OPEN fitted, not merely be fittable.
 *
 * jsdom lays nothing out, so the geometry every calculation in the component reads
 * is stubbed below - which is the point: it lets the one ordering that broke this
 * be reproduced exactly, rather than depending on how fast an image happens to load
 * on the machine running the suite.
 */

const PLAN_W = 1200
const PLAN_H = 800 // a 3:2 plan
const FRAME_W = 1152 // the column the map is drawn in
/** .map-viewport's shape before the plan has told it what to be (see the CSS). */
const FALLBACK_ASPECT = 1.6

function rect(width: number, height: number): DOMRect {
  return { width, height, top: 0, left: 0, right: width, bottom: height, x: 0, y: 0 } as DOMRect
}

/**
 * Gives the two elements the component measures a believable size.
 *
 * The frame takes its shape from `--map-aspect`, exactly as the stylesheet does,
 * so the stub reproduces the real dependency: the frame is the WRONG shape until
 * the component has set that variable. The canvas is always sized - which is the
 * cached-image case, where the plan has its full size at mount, before the load
 * event fires - and reports the SCALED box a real one would, since the component
 * divides the zoom back out of it.
 */
function stubLayout(): () => void {
  const original = Element.prototype.getBoundingClientRect
  Element.prototype.getBoundingClientRect = function (this: Element): DOMRect {
    const el = this as HTMLElement
    if (el.classList.contains('map-viewport')) {
      const aspect = Number(el.style.getPropertyValue('--map-aspect')) || FALLBACK_ASPECT
      return rect(FRAME_W, FRAME_W / aspect)
    }
    if (el.classList.contains('map-canvas')) {
      const pan = el.closest<HTMLElement>('.map-pan')
      const scale = Number(/scale\(([\d.]+)\)/.exec(pan?.style.transform ?? '')?.[1] ?? 1)
      return rect(FRAME_W * scale, (FRAME_W / (PLAN_W / PLAN_H)) * scale)
    }
    return original.call(this)
  }
  return () => {
    Element.prototype.getBoundingClientRect = original
  }
}

let restore: (() => void) | null = null
afterEach(() => {
  restore?.()
  restore = null
})

async function openMap() {
  restore = stubLayout()
  const wrapper = mount(FestivalMapCanvas, {
    props: { mapImage: '/images/plan.png', stalls: [] },
  })
  const img = wrapper.find('.map-canvas-bg')
  Object.defineProperty(img.element, 'naturalWidth', { value: PLAN_W })
  Object.defineProperty(img.element, 'naturalHeight', { value: PLAN_H })
  await img.trigger('load')
  await nextTick()
  await nextTick()
  return wrapper
}

describe('festival map canvas: the opening fit', () => {
  /**
   * The bug this pins. The frame's shape comes from the plan, so it is the
   * fallback shape until the image has loaded - and the canvas already has its
   * full size at mount whenever the image is in the browser's cache, which is the
   * ordinary case for a second visit. Fitting against that fallback lands at
   * exactly FALLBACK_ASPECT / planAspect: a 3:2 plan opened at 0.9375, a few
   * percent small and inset inside its own frame, and only pressing "Fit" - which
   * measures again, by then against the right frame - put it right.
   */
  it('opens filling its frame, not at the shape the frame started as', async () => {
    const wrapper = await openMap()
    expect(wrapper.vm.zoom).toBeCloseTo(1, 3)
    // Stated the other way round, so a regression cannot pass by landing anywhere
    // near 1: this is the exact wrong answer the ordering used to give.
    expect(wrapper.vm.zoom).not.toBeCloseTo(FALLBACK_ASPECT / (PLAN_W / PLAN_H), 3)
  })

  it('gives the frame the plan\'s shape, so "Fit" has nothing left to change', async () => {
    const wrapper = await openMap()
    const opened = wrapper.vm.zoom
    wrapper.vm.reset()
    await nextTick()
    // Pressing Fit on a map that opened fitted must be a no-op. It was the tell:
    // the map visibly grew, which is only possible if it had not been fitted.
    expect(wrapper.vm.zoom).toBeCloseTo(opened, 5)
  })

  it('tells the widget around it what shape to become', async () => {
    const wrapper = await openMap()
    // The widget caps its own width to this, so the toolbar cannot end up wider
    // than the plan underneath it.
    expect(wrapper.emitted('frame-aspect')?.[0]).toEqual([PLAN_W / PLAN_H])
  })
})

/**
 * The rally mark on the plan.
 *
 * It used to be the stamp's own artwork, and a rally stamp is very often a picture
 * of food - printed on a stall it read as a menu, so a visitor took it for what the
 * stall serves rather than for something to collect. The plan now carries one
 * consistent mark and the artwork stays where it is explained, in the panel a
 * tapped stall opens (asserted in FestivalMapWidget.test.ts).
 */
function stall(over: Partial<PublicStallOccupant> = {}, id = 1): PublicFestivalStall {
  return {
    id,
    shape: 'rect',
    color: '#e0a480',
    text_color: '#2d1b12',
    selection_color: '',
    placement: { x: 10, y: 10, width: 20, height: 15, rotation: 0 },
    occupants: [
      {
        id: id * 10,
        title: 'Tea Bar',
        description: '',
        stall_type: 'food',
        type_label: '',
        times: [],
        is_open: true,
        in_stamp_rally: false,
        ...over,
      },
    ],
  }
}

describe('festival map canvas: the stamp rally mark', () => {
  it('draws an icon on the plan and never the stamp artwork', () => {
    const wrapper = mount(FestivalMapCanvas, {
      props: {
        mapImage: '/images/plan.png',
        stalls: [stall({ in_stamp_rally: true, stamp_image: '/images/onigiri.png' })],
      },
    })
    const badge = wrapper.find('.map-stall-stamp')
    expect(badge.exists()).toBe(true)
    expect(badge.find('[data-icon="stamp"]').exists()).toBe(true)
    // The artwork must not reach the plan in any form.
    expect(wrapper.find('.map-canvas img.map-stall-stamp').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('onigiri')
  })

  /**
   * The mark says "this stall is on the rally", so it follows THAT and not whether
   * anyone has uploaded art yet - which is what it used to key on, leaving a rally
   * stall unmarked until someone did.
   */
  it('marks a rally stall whose stamp has no artwork yet', () => {
    const wrapper = mount(FestivalMapCanvas, {
      props: { mapImage: '/images/plan.png', stalls: [stall({ in_stamp_rally: true })] },
    })
    expect(wrapper.find('.map-stall-stamp').exists()).toBe(true)
  })

  it('leaves a stall that is not on the rally unmarked', () => {
    const wrapper = mount(FestivalMapCanvas, {
      props: { mapImage: '/images/plan.png', stalls: [stall()] },
    })
    expect(wrapper.find('.map-stall-stamp').exists()).toBe(false)
  })

  /**
   * The mark is one of the stall's own markings, so it takes the stall's label
   * colour. The stylesheet reads it as `color: var(--stall-text)`; what is asserted
   * here is that the variable the badge reads carries this stall's setting.
   */
  it("carries the stall's own label colour for the mark to take", () => {
    const wrapper = mount(FestivalMapCanvas, {
      props: { mapImage: '/images/plan.png', stalls: [stall({ in_stamp_rally: true })] },
    })
    expect(wrapper.find('.map-stall').attributes('style')).toContain('--stall-text: #2d1b12')
  })
})
