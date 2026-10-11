import { expect, test } from 'bun:test'
import * as echarts from 'echarts'
import { createChartFixture, defaultChartOptions } from '../../../../../playground/chart-fixtures'
import { EChartsHandle, echartsOption, responsiveEChartsPatch } from './echarts'
import { responsiveEChartsLayoutKey } from './echarts/view-state'
import { defaultRendererContext } from '../host-controller'
import { CategoryColorRegistry } from './echarts/category-colors'

function fixture(mark: string, longLabels = true) {
  const envelope = createChartFixture(mark, { ...defaultChartOptions, scenario: longLabels ? 'long-labels' : 'standard' }).envelope
  if (envelope.spec.kind === 'point') {
    envelope.spec.color = { dataset: 'primary', field: 'category' }
    envelope.spec.colorScale = { kind: 'categorical' }
    envelope.spec.presentation.brush = []
  }
  if (longLabels && envelope.spec.kind === 'cartesian') {
    for (const field of envelope.spec.datasets[0]!.fields) {
      if (field.role === 'metric') field.label = `${field.label} — international enterprise accounts and strategic revenue forecast`
    }
  }
  return envelope
}

function globalRect(element: any) {
  const rect = element.getBoundingRect().clone()
  const transform = element.getComputedTransform()
  if (transform) rect.applyTransform(transform)
  return rect
}

function assertPagerSeparation(view: any) {
    const previous = globalRect(view._controllerGroup.childOfName('pagePrev'))
    const pageText = globalRect(view._controllerGroup.childOfName('pageText'))
    const nextButton = globalRect(view._controllerGroup.childOfName('pageNext'))
    expect(previous.x + previous.width).toBeLessThanOrEqual(pageText.x + 0.001)
    expect(pageText.x + pageText.width).toBeLessThanOrEqual(nextButton.x + 0.001)
}

function assertWholeLegendPages(chart: any, width: number, height: number) {
  const model = chart.getModel().getComponent('legend')
  const view = chart.getViewOfComponentModel(model)
  const expected = model.getData().map((item: any) => item.get('name'))
  const visited = new Set<string>()
  chart.dispatchAction({ type: 'legendScroll', legendId: model.id, scrollDataIndex: 0 })
  for (let page = 0; page <= expected.length; page++) {
    chart.renderToSVGString()
    const content = view.getContentGroup()
    assertPagerSeparation(view)
    const clipWidth = view._containerGroup.__rectSize
    const bounds = globalRect(view._backgroundEl)
    expect(bounds.x).toBeGreaterThanOrEqual(-0.001)
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(width + 0.001)
    expect(bounds.y + bounds.height).toBeLessThanOrEqual(height + 0.001)
    expect(bounds.x + bounds.width / 2).toBeCloseTo(width / 2, 1)
    for (const item of content.children()) {
      const rect = item.getBoundingRect()
      const start = content.x + item.x + rect.x
      const end = start + rect.width
      if (end <= 0 || start >= clipWidth) continue
      const name = model.getData()[item.__legendDataIndex].get('name')
      expect(start, name).toBeGreaterThanOrEqual(-0.001)
      expect(end, name).toBeLessThanOrEqual(clipWidth + 0.001)
      visited.add(name)
    }
    const next = view._getPageInfo(model).pageNextDataIndex
    if (next == null) break
    chart.dispatchAction({ type: 'legendScroll', legendId: model.id, scrollDataIndex: next })
  }
  expect([...visited].sort()).toEqual([...expected].sort())
}

for (const mark of ['line', 'bar', 'combo', 'pie', 'donut', 'funnel', 'scatter', 'radar']) {
  for (const width of [220, 375, 768, 1440]) {
    test(`${mark} bottom legend stays centered with whole pages at ${width}px`, () => {
      const source = echartsOption(fixture(mark)) as any
      const chart = echarts.init(null, null, { renderer: 'svg', ssr: true, width, height: 400 })
      try {
        chart.setOption({ ...source, ...responsiveEChartsPatch(source, width, 400), animation: false })
        assertWholeLegendPages(chart, width, 400)
      } finally { chart.dispose() }
    })
  }
}

test('roomy scatter axis labels leave a clear gap above the bottom legend', () => {
  const source = echartsOption(fixture('scatter', false)) as any
  const chart: any = echarts.init(null, null, { renderer: 'svg', ssr: true, width: 768, height: 400 })
  try {
    chart.setOption({ ...source, ...responsiveEChartsPatch(source, 768, 400), animation: false })
    chart.renderToSVGString()
    const axis = globalRect(chart.getViewOfComponentModel(chart.getModel().getComponent('xAxis')).group)
    const legend = globalRect(chart.getViewOfComponentModel(chart.getModel().getComponent('legend'))._backgroundEl)
    expect(axis.y + axis.height + 4).toBeLessThanOrEqual(legend.y)
  } finally { chart.dispose() }
})

test('bottom legend geometry re-evaluates during roomy width changes and preserves selection', () => {
  const envelope = fixture('line')
  const source = echartsOption(envelope) as any
  expect(responsiveEChartsLayoutKey(envelope, 768, 400, false, source)).not.toBe(responsiveEChartsLayoutKey(envelope, 1100, 400, false, source))
  const chart: any = echarts.init(null, null, { renderer: 'svg', ssr: true, width: 375, height: 400 })
  try {
    chart.setOption({ ...source, ...responsiveEChartsPatch(source, 375, 400), animation: false })
    const name = chart.getModel().getComponent('legend').getData()[0].get('name')
    chart.dispatchAction({ type: 'legendUnSelect', name })
    for (const width of [768, 1100, 220, 1440, 375]) {
      chart.resize({ width, height: 400 })
      chart.setOption(responsiveEChartsPatch(source, width, 400))
      assertWholeLegendPages(chart, width, 400)
      expect(chart.getModel().getComponent('legend').isSelected(name)).toBe(false)
    }
  } finally { chart.dispose() }
})

test('legend layout preserves explicitly hidden legends and authored top positions', () => {
  const envelope = fixture('line') as any
  envelope.spec.presentation.legend = 'hidden'
  const hidden = echartsOption(envelope) as any
  expect(hidden.legend).toBeUndefined()
  expect(responsiveEChartsPatch(hidden, 220, 400).legend).toBeUndefined()
  envelope.spec.presentation.legend = 'top'
  const top = echartsOption(envelope) as any
  expect(responsiveEChartsPatch(top, 220, 400).legend.top).toBe(0)
  expect(responsiveEChartsPatch(top, 220, 400).legend.bottom).toBeUndefined()
})

test('large bottom legends keep every item and three-digit pager text visible', () => {
  const envelope = createChartFixture('donut', defaultChartOptions).envelope as any
  envelope.dataState.datasets[0].rows = Array.from({ length: 120 }, (_, index) => [`Series ${index + 1}`, index + 1])
  const source = echartsOption(envelope) as any
  const chart = echarts.init(null, null, { renderer: 'svg', ssr: true, width: 220, height: 400 })
  try {
    chart.setOption({ ...source, ...responsiveEChartsPatch(source, 220, 400), animation: false })
    const model = (chart as any).getModel().getComponent('legend')
    const view = (chart as any).getViewOfComponentModel(model)
    expect(model.getData()).toHaveLength(120)
    expect(view._getPageInfo(model).pageCount).toBeGreaterThanOrEqual(100)
    for (const scrollDataIndex of [0, 119]) {
      chart.dispatchAction({ type: 'legendScroll', legendId: model.id, scrollDataIndex })
      chart.renderToSVGString()
      assertPagerSeparation(view)
      const bounds = globalRect(view._backgroundEl)
      expect(bounds.x).toBeGreaterThanOrEqual(0)
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(220)
    }
  } finally { chart.dispose() }
})

test('scatter reserves a series legend band only for visible categorical color groups', () => {
  const envelope = fixture('scatter', false) as any
  envelope.spec.presentation.legend = 'hidden'
  expect((echartsOption(envelope) as any).grid.bottom).toBe(16)
  envelope.spec.presentation.legend = 'bottom'
  envelope.spec.color = undefined
  envelope.spec.colorScale = undefined
  expect((echartsOption(envelope) as any).grid.bottom).toBe(16)
  envelope.spec.color = { dataset: 'primary', field: 'value' }
  envelope.spec.colorScale = { kind: 'quantitative' }
  const quantitative = echartsOption(envelope) as any
  expect(quantitative.grid.bottom).toBe(16)
  expect(quantitative.grid.right).toBe(54)
  expect(quantitative.visualMap).toMatchObject({ right: 0, top: 'middle' })
})

test('roomy overflowing legends keep pager arrows and text readable in the current theme', () => {
  const context = { ...defaultRendererContext, theme: 'dark' as const, colors: { ...defaultRendererContext.colors, foreground: '#c9d1d9', muted: '#8b949e', grid: '#30363d' } }
  const source = echartsOption(fixture('line'), context) as any
  const chart: any = echarts.init(null, null, { renderer: 'svg', ssr: true, width: 768, height: 400 })
  try {
    chart.setOption({ ...source, ...responsiveEChartsPatch(source, 768, 400), animation: false })
    const model = chart.getModel().getComponent('legend')
    expect(model.get('pageIconColor')).toBe(context.colors.foreground)
    expect(model.getModel('pageTextStyle').getTextColor()).toBe(context.colors.muted)
  } finally { chart.dispose() }
})

for (const side of ['left', 'right'] as const) {
  test(`roomy resized funnel keeps moved ${side} legend centered and within the card`, () => {
    const envelope = fixture('funnel') as any
    envelope.spec.presentation.legend = side
    const chart: any = echarts.init(null, null, { renderer: 'svg', ssr: true, width: 900, height: 400 })
    const handle = new EChartsHandle({} as HTMLElement, {} as HTMLElement, chart, new CategoryColorRegistry())
    try {
      handle.mount(envelope, defaultRendererContext)
      void handle.whenReady().catch(() => {})
      handle.resize(900, 400)
      handle.resize(700, 400)
      // A browser frame flushes lazy options; explicit SSR resize does the same.
      chart.resize({ width: 700, height: 400 })
      assertWholeLegendPages(chart, 700, 400)
      expect(envelope.spec.presentation.legend).toBe(side)
    } finally { handle.dispose() }
  })
}
