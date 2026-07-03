import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ImageCategory, ImageEntry } from '@/types/api'

// Stub the endpoint layer the images store talks to.
const { categories, list } = vi.hoisted(() => ({
  categories: vi.fn(async () => ({ categories: [] as ImageCategory[] })),
  list: vi.fn(async (dir: string) => ({ dir, images: [] as ImageEntry[] })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: { images: { categories, list } },
}))

import ImagePicker from './ImagePicker.vue'

function cat(name: string, dir: string): ImageCategory {
  return { name, dir, file_count: 0, total_size: 0 }
}
function entry(dir: string, name: string): ImageEntry {
  return {
    name,
    url: `https://h/images/${dir}/${name}`,
    path: `images/${dir}/${name}`,
    size: 1,
    modified: '',
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  categories.mockResolvedValue({
    categories: [cat('Raffle', 'raffles'), cat('Flourishes', 'flourishes')],
  })
  list.mockImplementation(async (dir: string) => ({
    dir,
    images:
      dir === 'raffles'
        ? [entry('raffles', 'prize.png')]
        : [entry('flourishes', 'swirl.svg'), entry('flourishes', 'photo.png')],
  }))
})

function mountPicker(modelValue = '', props: Record<string, unknown> = {}) {
  return mount(ImagePicker, { props: { modelValue, ...props } })
}

describe('ImagePicker', () => {
  it('loads categories on mount and offers them sorted by name', async () => {
    const wrapper = mountPicker()
    await flushPromises()
    const options = wrapper.findAll('option')
    expect(options.map((o) => o.text())).toEqual(['Flourishes', 'Raffle'])
    expect(categories).toHaveBeenCalledTimes(1)
  })

  it("starts in the current value's category and highlights it", async () => {
    const wrapper = mountPicker('images/raffles/prize.png')
    await flushPromises()
    const select = wrapper.find('select').element
    expect(select.value).toBe('raffles')
    expect(wrapper.find('.img-thumb.active').exists()).toBe(true)
  })

  it('emits the path by default when an image is clicked', async () => {
    const wrapper = mountPicker('images/raffles/other.png')
    await flushPromises()
    await wrapper.find('.img-thumb').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['images/raffles/prize.png'])
  })

  it('emits the absolute URL with value-key="url"', async () => {
    const wrapper = mountPicker('https://h/images/raffles/other.png', { valueKey: 'url' })
    await flushPromises()
    await wrapper.find('.img-thumb').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([
      'https://h/images/raffles/prize.png',
    ])
  })

  it('switches categories and loads their images on select change', async () => {
    const wrapper = mountPicker('images/raffles/prize.png')
    await flushPromises()
    await wrapper.find('select').setValue('flourishes')
    await flushPromises()
    expect(list).toHaveBeenCalledWith('flourishes')
    expect(wrapper.findAll('.img-thumb')).toHaveLength(2)
  })

  it('filters the grid by the extensions prop', async () => {
    const wrapper = mountPicker('images/flourishes/swirl.svg', { extensions: ['.svg'] })
    await flushPromises()
    const thumbs = wrapper.findAll('.img-thumb')
    expect(thumbs).toHaveLength(1)
    expect(thumbs[0].attributes('title')).toBe('swirl.svg')
  })

  it('clears the value via the Remove button', async () => {
    const wrapper = mountPicker('images/raffles/prize.png')
    await flushPromises()
    await wrapper.find('button.btn-neutral').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([''])
  })
})
