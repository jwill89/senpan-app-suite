import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { Image, ImageCategory } from '@/api/client'

// The store calls the API client and the upload helper; only those are faked (unwrap
// is the real one). Each client fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({ GET: vi.fn() }))
const apiUpload = vi.hoisted(() => vi.fn())
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  apiUpload,
}))

/** The directories the category listing was asked for, in order. */
function listedDirs(): string[] {
  return api.GET.mock.calls
    .filter(([path]) => path === '/image-categories/{dir}/images')
    .map(([, init]) => (init as { params: { path: { dir: string } } }).params.path.dir)
}

import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import { useAuthStore } from '@/stores/auth'
import type { Account } from '@/api/client'

function cat(name: string, dir: string): ImageCategory {
  return { name, dir, fileCount: 0, totalSize: 0 }
}
function entry(dir: string, name: string): Image {
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
  api.GET.mockReset()
  apiUpload.mockReset()
  apiUpload.mockResolvedValue({ uploaded: [entry('raffles', 'new.png')], skipped: [] })
  api.GET.mockImplementation(async (path: string, init?: { params: { path: { dir: string } } }) => {
    if (path === '/image-categories') {
      return ok({ categories: [cat('Raffle', 'raffles'), cat('Flourishes', 'flourishes')] })
    }
    const dir = init!.params.path.dir
    return ok({
      dir,
      images:
        dir === 'raffles'
          ? [entry('raffles', 'prize.png')]
          : [entry('flourishes', 'swirl.svg'), entry('flourishes', 'photo.png')],
    })
  })
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
    expect(api.GET.mock.calls.filter(([p]) => p === '/image-categories')).toHaveLength(1)
  })

  it("starts in the current value's category and highlights it", async () => {
    const wrapper = mountPicker('images/raffles/prize.png')
    await flushPromises()
    const select = wrapper.find('select').element
    expect(select.value).toBe('raffles')
    expect(wrapper.find('.img-thumb.is-active').exists()).toBe(true)
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
    expect(listedDirs()).toContain('flourishes')
    expect(wrapper.findAll('.img-thumb')).toHaveLength(2)
  })

  it('filters the grid by the extensions prop', async () => {
    const wrapper = mountPicker('images/flourishes/swirl.svg', { extensions: ['.svg'] })
    await flushPromises()
    const thumbs = wrapper.findAll('.img-thumb')
    expect(thumbs).toHaveLength(1)
    expect(thumbs[0]!.attributes('title')).toBe('swirl.svg')
  })

  it('clears the value via the Remove button', async () => {
    const wrapper = mountPicker('images/raffles/prize.png')
    await flushPromises()
    await wrapper.find('button.btn-neutral').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([''])
  })

  it('lists images alphabetically by file name, digit-aware', async () => {
    api.GET.mockImplementation(
      async (path: string, init?: { params: { path: { dir: string } } }) =>
        path === '/image-categories'
          ? ok({ categories: [cat('Raffle', 'raffles')] })
          : ok({
              dir: init!.params.path.dir,
              images: ['img10.png', 'Zebra.png', 'img2.png', 'apple.png'].map((n) =>
                entry(init!.params.path.dir, n),
              ),
            }),
    )
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
      useAuthStore().user = { id: 1, username: 'u', isAdmin: false, permissions } satisfies Account
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

      expect(apiUpload).toHaveBeenCalledTimes(1)
      const [endpoint, form] = apiUpload.mock.calls[0] as unknown as [string, FormData]
      expect(endpoint).toBe('image-categories/raffles/images')
      expect(form.getAll('files')).toHaveLength(1)
      // The category is re-listed so the new image appears in the grid.
      expect(listedDirs().at(-1)).toBe('raffles')
    })

    it('ignores drops when the user cannot upload', async () => {
      const wrapper = mountPicker('images/raffles/prize.png')
      await flushPromises()
      const file = new File(['x'], 'new.png', { type: 'image/png' })
      await wrapper.find('.image-picker-drop').trigger('drop', { dataTransfer: { files: [file] } })
      await flushPromises()
      expect(apiUpload).not.toHaveBeenCalled()
    })
  })
})
