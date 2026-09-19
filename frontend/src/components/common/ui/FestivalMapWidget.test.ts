import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import FestivalMapWidget from './FestivalMapWidget.vue'
import type { PublicFestivalMap } from '@/types/api'

/**
 * The map widget is the one piece of this app that runs somewhere we do not
 * control: the same component renders on our own page and, with `embedded`, inside
 * an <iframe> on a Carrd. Both facts below are things a person would only notice
 * once it was already pasted onto somebody's live site.
 */

/** A minimal router, so <RouterLink> resolves real hrefs rather than being stubbed. */
function router() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { template: '<div />' } },
      { path: '/raffles/:id', name: 'raffle-detail', component: { template: '<div />' } },
      {
        path: '/stamp-rallies/:id',
        name: 'stamp-rally-signup',
        component: { template: '<div />' },
      },
    ],
  })
}

function festivalMap(): PublicFestivalMap {
  return {
    id: 1,
    title: 'Summer Night Festival',
    slug: 'summer',
    description: '',
    times: [],
    map_image: '/images/plan.png',
    is_active: true,
    stamp_rally_id: 9,
    stamp_rally_title: 'Lantern Rally',
    stamp_rally_signup: true,
    stalls: [
      {
        id: 1,
        shape: 'rect',
        color: '#e0a480',
        text_color: '',
        selection_color: '',
        placement: { x: 10, y: 10, width: 20, height: 15, rotation: 0 },
        occupants: [
          {
            id: 11,
            title: '',
            description: '',
            stall_type: 'food',
            type_label: '',
            times: [],
            is_open: true,
            in_stamp_rally: true,
            rally_title: 'Lantern Rally',
            stamp_image: '/images/onigiri.png',
            affiliate: {
              name: 'Lunaria',
              subtitle: '',
              owners: [],
              logo: '',
              discord_link: '',
              carrd_link: '',
            },
            raffle: { id: 7, title: 'Prize Draw', prize_image: '' },
          },
        ],
      },
    ],
  }
}

async function widget(embedded: boolean) {
  const r = router()
  await r.push('/')
  await r.isReady()
  const wrapper = mount(FestivalMapWidget, {
    props: { map: festivalMap(), embedded },
    global: { plugins: [r] },
  })
  // Open the pitch, which is where the two in-app links live.
  await wrapper.find('.map-stall').trigger('pointerup')
  return wrapper
}

afterEach(() => vi.useRealTimers())

describe('festival map widget: embedded links', () => {
  /**
   * Embedded, our pages are reached from inside somebody else's frame. Following a
   * raffle link in place would load our site INTO their layout - the visitor loses
   * the page they were reading and lands on a version of ours squeezed into a box
   * the host sized for a map.
   */
  it('opens its in-app links in a new tab when embedded', async () => {
    const wrapper = await widget(true)
    const links = wrapper.findAll('.map-stall-rally a')
    expect(links.length).toBe(2) // the raffle, and the stamp card sign-up

    for (const link of links) {
      expect(link.attributes('target')).toBe('_blank')
      // A real href, not a click handler: vue-router deliberately leaves a _blank
      // link to the browser, so the href is what actually navigates.
      expect(link.attributes('href')).toMatch(/^\/(raffles|stamp-rallies)\/\d+$/)
    }
  })

  /**
   * ...and on our own page it must NOT, or every stall panel would scatter tabs
   * behind someone browsing a festival.
   */
  it('navigates in place on our own page', async () => {
    const wrapper = await widget(false)
    const links = wrapper.findAll('.map-stall-rally a')
    expect(links.length).toBe(2)
    for (const link of links) expect(link.attributes('target')).toBeUndefined()
  })

  it('fills its frame only when embedded', async () => {
    expect((await widget(true)).find('.map-embed').classes()).toContain('is-framed')
    expect((await widget(false)).find('.map-embed').classes()).not.toContain('is-framed')
  })
})

/**
 * The hint covers the whole plan, so unlike the corner note it replaced it CANNOT
 * be left up: it would be dimming the map a visitor is trying to read. Both ways
 * out of it are asserted here.
 */
describe('festival map widget: the usage hint', () => {
  it('clears itself after seven seconds when nobody touches the map', async () => {
    vi.useFakeTimers()
    const wrapper = mount(FestivalMapWidget, {
      props: { map: festivalMap() },
      global: { plugins: [router()] },
    })
    expect(wrapper.find('.map-hint').classes()).not.toContain('is-dismissed')

    vi.advanceTimersByTime(6999)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.map-hint').classes()).not.toContain('is-dismissed')

    vi.advanceTimersByTime(1)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.map-hint').classes()).toContain('is-dismissed')
  })

  it.each(['pointerdown', 'wheel', 'keydown'])(
    'clears on the first %s, which is the visitor showing they have got it',
    async (event) => {
      const wrapper = mount(FestivalMapWidget, {
        props: { map: festivalMap() },
        global: { plugins: [router()] },
      })
      await wrapper.find('.map-embed-stage').trigger(event)
      expect(wrapper.find('.map-hint').classes()).toContain('is-dismissed')
    },
  )

  /**
   * Driving the map from the toolbar is using the map. Zooming from the bar and
   * having the overlay stay put would read as the hint being stuck.
   */
  it('clears on a toolbar control too', async () => {
    const wrapper = mount(FestivalMapWidget, {
      props: { map: festivalMap() },
      global: { plugins: [router()] },
    })
    await wrapper.find('.map-toolbar').trigger('pointerdown')
    expect(wrapper.find('.map-hint').classes()).toContain('is-dismissed')
  })

  /** It is told about, never touched - so it must not be read out as a control. */
  it('is hidden from assistive technology', async () => {
    const wrapper = mount(FestivalMapWidget, {
      props: { map: festivalMap() },
      global: { plugins: [router()] },
    })
    expect(wrapper.find('.map-hint').attributes('aria-hidden')).toBe('true')
  })
})

/**
 * The other half of moving the rally mark off the plan: the artwork has to land
 * somewhere, and the panel is where it is actually explained - beside the rally's
 * name, under a line saying what it is for. On the stall it was a picture of food
 * with nothing to say it was a stamp.
 */
describe('festival map widget: the stamp artwork', () => {
  it('shows the stamp artwork in the panel, where it is explained', async () => {
    const wrapper = await widget(false)
    // Two blocks share this class - the raffle running here, and the rally. Pick
    // the rally's by what it says rather than by its position.
    const rally = wrapper
      .findAll('.map-stall-rally')
      .find((b) => b.text().includes('Lantern Rally'))
    expect(rally).toBeDefined()
    expect(rally?.find('img').attributes('src')).toContain('onigiri')
  })
})
