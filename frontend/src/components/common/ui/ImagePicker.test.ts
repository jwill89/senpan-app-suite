import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ImageCategory, ImageEntry } from '@/types/api'

// Stub the endpoint layer the images store talks to.
const { categories, list, upload } = vi.hoisted(() => ({
  categories: vi.fn(async () => ({ categories: [] as ImageCategory[] })),
  list: vi.fn(async (dir: string) => ({ dir, images: [] as ImageEntry[] })),
  upload: vi.fn(async () => ({ uploaded: ['new.png'], skipped: [] })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: { images: { categories, list, upload } },
}))

import ImagePicker from './ImagePicker.vue'
import { useAuthStore } from '@/stores/auth'
import type { User } from '@/types/api'

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

  it('lists images alphabetically by file name, digit-aware', async () => {
    list.mockImplementation(async (dir: string) => ({
      dir,
      images: ['img10.png', 'Zebra.png', 'img2.png', 'apple.png'].map((n) => entry(dir, n)),
    }))
    const wrapper = mountPicker('images/raffles/apple.png')
    await flushPromises()
    expect(wrapper.findAll('.img-thumb').map((t) => t.attributes('title'))).toEqual([
      'apple.png',
      'img2.png',
      'img10.png',
      'Zebra.png',
    ])
  })

  describe('upload control', () => {
    /** Signs in a user holding exactly the given permission keys. */
    function signIn(...permissions: string[]): void {
      useAuthStore().user = { is_admin: false, permissions } as unknown as User
    }

    it('is hidden for users without the system-images permission', async () => {
      signIn('teahouse-affiliates')
      const wrapper = mountPicker()
      await flushPromises()
      expect(wrapper.find('.image-picker-upload').exists()).toBe(false)
      expect(wrapper.find('input[type="file"]').exists()).toBe(false)
    })

    it('is shown to system-images holders', async () => {
      signIn('system-images')
      const wrapper = mountPicker()
      await flushPromises()
      expect(wrapper.find('.image-picker-upload').exists()).toBe(true)
    })

    it('uploads dropped files into the category being browsed', async () => {
      signIn('system-images')
      const wrapper = mountPicker('images/raffles/prize.png')
      await flushPromises()
      const file = new File(['x'], 'new.png', { type: 'image/png' })
      await wrapper.find('.image-picker-drop').trigger('drop', { dataTransfer: { files: [file] } })
      await flushPromises()

      expect(upload).toHaveBeenCalledTimes(1)
      const [form] = upload.mock.calls[0] as unknown as [FormData]
      expect(form.get('dir')).toBe('raffles')
      expect(form.getAll('files')).toHaveLength(1)
      // The category is re-listed so the new image appears in the grid.
      expect(list).toHaveBeenLastCalledWith('raffles')
    })

    it('ignores drops when the user cannot upload', async () => {
      const wrapper = mountPicker('images/raffles/prize.png')
      await flushPromises()
      const file = new File(['x'], 'new.png', { type: 'image/png' })
      await wrapper.find('.image-picker-drop').trigger('drop', { dataTransfer: { files: [file] } })
      await flushPromises()
      expect(upload).not.toHaveBeenCalled()
    })
  })
})
