import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import BingoBoard from './BingoBoard.vue'

// BingoBoard renders CornerFlourish (player mode), which reads the app store, so
// a Pinia instance must be active for those mounts.
beforeEach(() => setActivePinia(createPinia()))

/** A standard 5×5 board with the conventional FREE (0) center square. */
function sampleBoard(): number[][] {
  return [
    [1, 16, 31, 46, 61],
    [2, 17, 32, 47, 62],
    [3, 18, 0, 48, 63],
    [4, 19, 33, 49, 64],
    [5, 20, 34, 50, 65],
  ]
}

describe('BingoBoard', () => {
  it('renders the B-I-N-G-O header', () => {
    const wrapper = mount(BingoBoard, { props: { board: sampleBoard(), mode: 'preview' } })
    const letters = wrapper.findAll('.board-header span').map((s) => s.text())
    expect(letters).toEqual(['B', 'I', 'N', 'G', 'O'])
  })

  it('renders all 25 cells with FREE in the center', () => {
    const wrapper = mount(BingoBoard, { props: { board: sampleBoard(), mode: 'preview' } })
    const cells = wrapper.findAll('.board-cell')
    expect(cells).toHaveLength(25)
    expect(wrapper.find('.board-cell.free .cell-num').text()).toBe('FREE')
  })

  it('applies the board-preview modifier when preview is set', () => {
    const wrapper = mount(BingoBoard, { props: { board: sampleBoard(), preview: true } })
    expect(wrapper.find('.board-wrap').classes()).toContain('board-preview')
  })

  it('emits cellClick with coordinates in player mode', async () => {
    const wrapper = mount(BingoBoard, {
      props: { board: sampleBoard(), mode: 'player', cellClass: () => ['board-cell'] },
    })
    await wrapper.findAll('.board-cell')[0].trigger('click')
    expect(wrapper.emitted('cellClick')?.[0]).toEqual([0, 0, 1])
  })

  it('shows the stamp emoji on stamped cells', () => {
    const wrapper = mount(BingoBoard, {
      props: {
        board: sampleBoard(),
        mode: 'player',
        isStamped: (ri: number, ci: number) => ri === 0 && ci === 0,
        cellClass: (ri: number, ci: number) =>
          ri === 0 && ci === 0 ? ['board-cell', 'stamped'] : ['board-cell'],
        stampEmoji: '⭐',
        stampShape: 'star',
      },
    })
    const first = wrapper.findAll('.board-cell')[0]
    expect(first.classes()).toContain('stamped')
    expect(first.find('.stamp-mark').text()).toBe('⭐')
  })

  it('renders a custom stamp image when the shape is custom', () => {
    const dataUrl = 'data:image/png;base64,AAAA'
    const wrapper = mount(BingoBoard, {
      props: {
        board: sampleBoard(),
        mode: 'player',
        isStamped: () => true,
        cellClass: () => ['board-cell', 'stamped'],
        stampShape: 'custom',
        customStampImage: dataUrl,
      },
    })
    const img = wrapper.find('.stamp-custom-img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe(dataUrl)
  })

  it('routes the secondary stamp to non-pattern cells and keeps the primary on pattern cells', () => {
    const wrapper = mount(BingoBoard, {
      props: {
        board: sampleBoard(),
        mode: 'player',
        isStamped: () => true, // every cell stamped
        cellClass: () => ['board-cell', 'stamped'],
        stampEmoji: '⭐',
        stampShape: 'star',
        stampMarkStyle: { background: 'pink' },
        secondaryStampStyle: { background: 'blue' },
        // Only the top-left cell is part of a winning pattern.
        isWinningPatternCell: (ri: number, ci: number) => ri === 0 && ci === 0,
      },
    })
    const marks = wrapper.findAll('.stamp-mark')
    // Pattern cell (0,0): primary stamp → shows the emoji.
    expect(marks[0].text()).toBe('⭐')
    expect(marks[0].attributes('style')).toContain('background: pink')
    // Non-pattern cell (0,1): secondary stamp → plain circle, no emoji.
    expect(marks[1].text()).toBe('')
    expect(marks[1].attributes('style')).toContain('background: blue')
  })

  it('uses the primary stamp everywhere when no secondary style is provided', () => {
    const wrapper = mount(BingoBoard, {
      props: {
        board: sampleBoard(),
        mode: 'player',
        isStamped: () => true,
        cellClass: () => ['board-cell', 'stamped'],
        stampEmoji: '⭐',
        isWinningPatternCell: () => false, // would be "non-pattern" everywhere
      },
    })
    // Without secondaryStampStyle, every stamped cell keeps the primary emoji.
    expect(wrapper.findAll('.stamp-mark')[1].text()).toBe('⭐')
  })
})
