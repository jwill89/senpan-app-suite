import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BingoCardEditor from './BingoCardEditor.vue'
import { emptyNumberBoard } from '@/lib/constants'

describe('BingoCardEditor', () => {
  it('renders 24 number inputs and a FREE centre', () => {
    const w = mount(BingoCardEditor, { props: { modelValue: emptyNumberBoard() } })
    expect(w.findAll('.card-editor-grid input')).toHaveLength(24)
    expect(w.find('.ce-free').text()).toBe('FREE')
  })

  it('emits an updated board when a cell changes', async () => {
    const w = mount(BingoCardEditor, { props: { modelValue: emptyNumberBoard() } })
    await w.find('.card-editor-grid input').setValue('7')
    const emitted = w.emitted('update:modelValue')
    expect(emitted).toBeTruthy()
    const board = emitted!.at(-1)![0] as number[][]
    expect(board[0][0]).toBe(7)
  })

  it('Generate Random emits a fully-filled board with a FREE centre', async () => {
    const w = mount(BingoCardEditor, { props: { modelValue: emptyNumberBoard() } })
    await w.find('button').trigger('click')
    const board = w.emitted('update:modelValue')!.at(-1)![0] as number[][]
    expect(board[2][2]).toBe(0)
    expect(board.flat().filter((n) => n !== 0)).toHaveLength(24)
  })

  it('flags an out-of-range cell as invalid', () => {
    const board = emptyNumberBoard()
    board[0][0] = 99 // out of the B (1–15) range
    const w = mount(BingoCardEditor, { props: { modelValue: board } })
    expect(w.find('.ce-invalid').exists()).toBe(true)
  })
})
