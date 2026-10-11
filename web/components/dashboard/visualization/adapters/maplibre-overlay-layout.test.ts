import { expect, test } from 'bun:test'
import { mapLegendPlacement, mapOverlaysNeedStacking } from './maplibre/overlays'

test('bottom map legends stay centered and clear earlier side positioning', () => {
  expect(mapLegendPlacement('bottom', '46px')).toEqual({ left: '50%', right: '', top: '', bottom: '46px', transform: 'translateX(-50%)', width: 'max-content' })
  expect(mapLegendPlacement('right', '46px')).toEqual({ left: '', right: '10px', top: '', bottom: '46px', transform: '', width: '' })
  expect(mapLegendPlacement('top', '46px')).toEqual({ left: '10px', right: '', top: '10px', bottom: '', transform: '', width: '' })
})

test('centered map legends reserve space for the data disclosure even on roomy maps', () => {
  // Legend spans 190–410; the right-hand disclosure spans 390–590.
  expect(mapOverlaysNeedStacking(600, 220, 200, 'bottom')).toBe(true)
  expect(mapOverlaysNeedStacking(600, 220, 170, 'bottom')).toBe(false)
  // Eight pixels separate the legend from the disclosure at the boundary.
  expect(mapOverlaysNeedStacking(600, 220, 172, 'bottom')).toBe(false)
  expect(mapOverlaysNeedStacking(600, 220, 173, 'bottom')).toBe(true)
  // Authored side legends still use opposing edges.
  expect(mapOverlaysNeedStacking(600, 220, 200, 'left')).toBe(false)
  expect(mapOverlaysNeedStacking(680, 150, 520)).toBe(true)
})
