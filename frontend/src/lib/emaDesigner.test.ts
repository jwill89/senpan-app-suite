import { describe, expect, it, vi } from 'vitest'
import {
  type Affine,
  type EmaImageLayer,
  type EmaLayers,
  type EmaTextLayer,
  applyAffine,
  createHistory,
  deserializeLayers,
  downscaleSize,
  exportSize,
  filterFonts,
  fitScale,
  fontChoices,
  fontFaceSpec,
  historyPush,
  historyRedo,
  historyUndo,
  invertAffine,
  isAtEnd,
  konvaFontStyle,
  layerLabel,
  localBrushWidth,
  moveLayer,
  placementScale,
  reachableShift,
  removeLayer,
  roundCoord,
  serializeLayers,
  textAnchor,
  toLocalPoint,
  topFirst,
  updateLayer,
  waitForFontFace,
  WEB_SAFE_FONTS,
} from './emaDesigner'

function image(id: string, name = `${id}.png`): EmaImageLayer {
  return {
    id,
    kind: 'image',
    name,
    assetId: `asset-${id}`,
    width: 100,
    height: 50,
    visible: true,
    opacity: 1,
    x: 0,
    y: 0,
    rotation: 0,
    scaleX: 1,
    scaleY: 1,
    strokes: [],
  }
}

function text(id: string, words: string): EmaTextLayer {
  return {
    id,
    kind: 'text',
    text: words,
    fontFamily: 'Georgia',
    fontSize: 40,
    fill: '#000000',
    stroke: '#ffffff',
    strokeWidth: 0,
    bold: false,
    italic: false,
    align: 'center',
    visible: true,
    opacity: 1,
    x: 0,
    y: 0,
    rotation: 0,
    scaleX: 1,
    scaleY: 1,
    strokes: [],
  }
}

const ids = (layers: EmaLayers): string[] => layers.map((l) => l.id)

describe('exportSize', () => {
  it('doubles the plaque template', () => {
    expect(exportSize(842, 595)).toEqual({ width: 1684, height: 1190 })
  })

  it('caps the width at 2048 and keeps the proportions', () => {
    expect(exportSize(1600, 1000)).toEqual({ width: 2048, height: 1280 })
  })
})

describe('downscaleSize', () => {
  it('leaves a picture that already fits alone', () => {
    expect(downscaleSize(1200, 800)).toEqual({ width: 1200, height: 800, scaled: false })
    expect(downscaleSize(2048, 2048)).toEqual({ width: 2048, height: 2048, scaled: false })
  })

  it('brings the longest side of a phone photo down to the cap', () => {
    expect(downscaleSize(4032, 3024)).toEqual({ width: 2048, height: 1536, scaled: true })
    expect(downscaleSize(3024, 4032)).toEqual({ width: 1536, height: 2048, scaled: true })
  })

  it('never rounds a sliver down to nothing', () => {
    expect(downscaleSize(10000, 1, 2048)).toEqual({ width: 2048, height: 1, scaled: true })
  })
})

describe('fitScale and placementScale', () => {
  it('fits by whichever side is tighter', () => {
    expect(fitScale({ width: 842, height: 595 }, { width: 421, height: 1000 })).toBe(0.5)
    expect(fitScale({ width: 842, height: 595 }, { width: 2000, height: 297.5 })).toBe(0.5)
  })

  it('is 0 for an empty box or content', () => {
    expect(fitScale({ width: 0, height: 10 }, { width: 10, height: 10 })).toBe(0)
    expect(fitScale({ width: 10, height: 10 }, { width: 10, height: 0 })).toBe(0)
  })

  it('places a big picture inside 60% of the plaque', () => {
    const scale = placementScale({ width: 2048, height: 1536 }, { width: 842, height: 595 })
    expect(1536 * scale).toBeCloseTo(595 * 0.6)
  })

  it('never enlarges a small picture', () => {
    expect(placementScale({ width: 64, height: 64 }, { width: 842, height: 595 })).toBe(1)
  })
})

describe('affine helpers', () => {
  // Scaled by 2, turned 90 degrees, then moved to (100, 50).
  const m: Affine = [0, 2, -2, 0, 100, 50]

  it('applies a matrix', () => {
    expect(applyAffine(m, { x: 10, y: 0 })).toEqual({ x: 100, y: 70 })
  })

  it('round-trips a point through the inverse', () => {
    const p = { x: 12.5, y: -7 }
    const back = toLocalPoint(m, applyAffine(m, p))
    expect(back?.x).toBeCloseTo(p.x)
    expect(back?.y).toBeCloseTo(p.y)
  })

  it('has no inverse for a collapsed transform', () => {
    expect(invertAffine([0, 0, 0, 0, 5, 5])).toBeNull()
    expect(toLocalPoint([0, 0, 0, 0, 5, 5], { x: 1, y: 1 })).toBeNull()
  })

  it('sizes the brush in the layer, undoing the stage and layer scales', () => {
    // The stage is shown at half size. A layer at the plaque's own scale is then
    // drawn at 0.5 on screen, and a 20 px brush is 20 of its units.
    expect(localBrushWidth(20, 0.5, [0.5, 0, 0, 0.5, 40, 40])).toBe(20)
    // A layer blown up to 2x is drawn at 1, so the same brush covers 10 of its units.
    expect(localBrushWidth(20, 0.5, [1, 0, 0, 1, 0, 0])).toBe(10)
    // Turning the layer does not change how wide the brush is.
    expect(localBrushWidth(20, 0.5, [0, 1, -1, 0, 30, 30])).toBe(10)
  })

  it('rounds coordinates to a tenth', () => {
    expect(roundCoord(12.3456)).toBe(12.3)
    expect(roundCoord(-0.04)).toBe(-0)
  })
})

describe('reachableShift', () => {
  const plaque = { width: 842, height: 595 }

  it('leaves a layer that is still on the plaque alone', () => {
    expect(reachableShift({ x: -50, y: 500, width: 100, height: 100 }, plaque, 24)).toEqual({
      x: 0,
      y: 0,
    })
  })

  it('pulls a layer dragged off the left and top back into reach', () => {
    expect(reachableShift({ x: -300, y: -200, width: 100, height: 50 }, plaque, 24)).toEqual({
      x: 224,
      y: 174,
    })
  })

  it('pulls a layer dragged off the right and bottom back into reach', () => {
    expect(reachableShift({ x: 900, y: 650, width: 100, height: 50 }, plaque, 24)).toEqual({
      x: -82,
      y: -79,
    })
  })
})

describe('textAnchor', () => {
  const box = { width: 200, height: 60 }

  it('anchors on the side the text is aligned to, halfway down', () => {
    expect(textAnchor('left', box)).toEqual({ x: 0, y: 30 })
    expect(textAnchor('center', box)).toEqual({ x: 100, y: 30 })
    expect(textAnchor('right', box)).toEqual({ x: 200, y: 30 })
  })
})

describe('layer edits', () => {
  const layers: EmaLayers = [image('a'), image('b'), text('c', 'hi')]

  it('moves a layer up and down a step', () => {
    expect(ids(moveLayer(layers, 'a', 'up'))).toEqual(['b', 'a', 'c'])
    expect(ids(moveLayer(layers, 'c', 'down'))).toEqual(['a', 'c', 'b'])
  })

  it('leaves a layer already at the end where it is', () => {
    expect(moveLayer(layers, 'c', 'up')).toBe(layers)
    expect(moveLayer(layers, 'a', 'down')).toBe(layers)
    expect(moveLayer(layers, 'nope', 'up')).toBe(layers)
  })

  it('knows which layers are at an end', () => {
    expect(isAtEnd(layers, 'c', 'up')).toBe(true)
    expect(isAtEnd(layers, 'a', 'down')).toBe(true)
    expect(isAtEnd(layers, 'b', 'up')).toBe(false)
  })

  it('removes a layer', () => {
    expect(ids(removeLayer(layers, 'b'))).toEqual(['a', 'c'])
    expect(removeLayer(layers, 'nope')).toBe(layers)
  })

  it('updates one layer without touching the others', () => {
    const next = updateLayer(layers, 'b', { opacity: 0.5 })
    expect(next[1]?.opacity).toBe(0.5)
    expect(next[0]).toBe(layers[0])
    expect(layers[1]?.opacity).toBe(1)
    expect(updateLayer(layers, 'nope', { opacity: 0 })).toBe(layers)
  })

  it('lists the top layer first without reordering the source', () => {
    expect(ids(topFirst(layers))).toEqual(['c', 'b', 'a'])
    expect(ids(layers)).toEqual(['a', 'b', 'c'])
  })

  it('round-trips through a snapshot', () => {
    const stroked = updateLayer(layers, 'a', { strokes: [{ size: 4, points: [1, 2, 3, 4] }] })
    expect(deserializeLayers(serializeLayers(stroked))).toEqual(stroked)
  })
})

describe('layerLabel', () => {
  it('names text by its first non-blank line', () => {
    expect(layerLabel(text('t', '\n  Good luck  \nsecond line'))).toBe('Good luck')
  })

  it('falls back when there is nothing to show', () => {
    expect(layerLabel(text('t', '   '))).toBe('Text')
    expect(layerLabel(image('i', ''))).toBe('Picture')
  })

  it('shortens a long name', () => {
    const label = layerLabel(image('i', 'a-very-long-photo-file-name-from-a-phone.jpeg'), 12)
    expect(label).toHaveLength(12)
    expect(label.endsWith('…')).toBe(true)
  })
})

describe('history', () => {
  it('undoes and redoes in order', () => {
    let h = createHistory('a')
    h = historyPush(h, 'b')
    h = historyPush(h, 'c')
    h = historyUndo(h)
    expect(h.present).toBe('b')
    h = historyUndo(h)
    expect(h.present).toBe('a')
    h = historyRedo(h)
    expect(h.present).toBe('b')
    expect(h.future).toEqual(['c'])
  })

  it('drops the redo states when something new happens', () => {
    let h = historyUndo(historyPush(historyPush(createHistory('a'), 'b'), 'c'))
    h = historyPush(h, 'x')
    expect(h.future).toEqual([])
    expect(h.past).toEqual(['a', 'b'])
  })

  it('does not record a state equal to the present', () => {
    const h = historyPush(createHistory('a'), 'b')
    expect(historyPush(h, 'b')).toBe(h)
  })

  it('returns the same history when there is nothing to undo or redo', () => {
    const h = createHistory('a')
    expect(historyUndo(h)).toBe(h)
    expect(historyRedo(h)).toBe(h)
  })

  it('forgets the oldest steps past the limit', () => {
    let h = createHistory(0)
    for (let i = 1; i <= 10; i++) h = historyPush(h, i, 3)
    expect(h.past).toEqual([7, 8, 9])
    expect(h.present).toBe(10)
  })
})

describe('fonts', () => {
  it('offers the Google fonts it was given after the web-safe ones', () => {
    const choices = fontChoices([' Caveat ', 'Caveat', '', 'Georgia', 'Bangers'], ['Fallback'])
    expect(choices.basic).toBe(WEB_SAFE_FONTS)
    expect(choices.google).toEqual(['Caveat', 'Bangers'])
  })

  it('falls back when the server sent none', () => {
    expect(fontChoices([], ['Arapey', 'Abel']).google).toEqual(['Arapey', 'Abel'])
    expect(fontChoices(['  '], ['Arapey']).google).toEqual(['Arapey'])
  })

  it('filters by name, ignoring case', () => {
    expect(filterFonts(['Bebas Neue', 'Barlow', 'Caveat'], ' ba ')).toEqual([
      'Bebas Neue',
      'Barlow',
    ])
    expect(filterFonts(['Caveat'], '')).toEqual(['Caveat'])
  })

  it("builds Konva's font style", () => {
    expect(konvaFontStyle(false, false)).toBe('normal')
    expect(konvaFontStyle(true, false)).toBe('bold')
    expect(konvaFontStyle(false, true)).toBe('italic')
    expect(konvaFontStyle(true, true)).toBe('italic bold')
  })

  it('names one face for document.fonts', () => {
    expect(fontFaceSpec('Bebas Neue', true, true)).toBe('italic 700 40px "Bebas Neue"')
    expect(fontFaceSpec('Odd"Name', false, false)).toBe('normal 400 40px "OddName"')
  })
})

describe('waitForFontFace', () => {
  it('asks again until the stylesheet has brought a face in', async () => {
    vi.useFakeTimers()
    try {
      const load = vi
        .fn<(spec: string, text?: string) => Promise<FontFace[]>>()
        .mockResolvedValueOnce([])
        .mockResolvedValueOnce([])
        .mockResolvedValue([{} as FontFace])
      const done = waitForFontFace({ load }, 'normal 400 40px "Caveat"', 'wish', 6000, 100)
      await vi.advanceTimersByTimeAsync(250)
      await expect(done).resolves.toBe(true)
      expect(load).toHaveBeenCalledTimes(3)
      expect(load).toHaveBeenLastCalledWith('normal 400 40px "Caveat"', 'wish')
    } finally {
      vi.useRealTimers()
    }
  })

  it('gives up at the deadline', async () => {
    vi.useFakeTimers()
    try {
      const load = vi
        .fn<(spec: string, text?: string) => Promise<FontFace[]>>()
        .mockResolvedValue([])
      const done = waitForFontFace({ load }, 'x', '', 500, 100)
      await vi.advanceTimersByTimeAsync(1000)
      await expect(done).resolves.toBe(false)
      expect(load).toHaveBeenCalledWith('x', ' ')
    } finally {
      vi.useRealTimers()
    }
  })

  it('keeps asking after a failed load', async () => {
    vi.useFakeTimers()
    try {
      const load = vi
        .fn<(spec: string, text?: string) => Promise<FontFace[]>>()
        .mockRejectedValueOnce(new Error('network'))
        .mockResolvedValue([{} as FontFace])
      const done = waitForFontFace({ load }, 'x', 'a', 1000, 100)
      await vi.advanceTimersByTimeAsync(150)
      await expect(done).resolves.toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })
})
