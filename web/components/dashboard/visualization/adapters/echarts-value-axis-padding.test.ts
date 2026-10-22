import { expect, test } from 'bun:test'
import * as echarts from 'echarts'

import { defaultRendererContext } from '../host-controller'
import { echartsOption } from './echarts'
import { cartesianFixture } from './echarts-test-fixtures'

function lineEnvelope(mark: 'line' | 'area', orientation: 'horizontal' | 'vertical', rows: unknown[][]) {
  const envelope = cartesianFixture(mark) as any
  Object.assign(envelope.spec.presentation, { orientation, stacked: false, dataZoom: false, legend: 'hidden', step: false })
  envelope.dataState.datasets[0].rows = rows
  return envelope
}

function withChart<T>(option: Record<string, any>, inspect: (chart: any) => T): T {
  const chart = echarts.init(null, null, { renderer: 'svg', ssr: true, width: 640, height: 360 })
  try {
    chart.setOption({ ...option, animation: false }, { notMerge: true, lazyUpdate: false })
    chart.renderToSVGString()
    return inspect(chart)
  } finally {
    chart.dispose()
  }
}

test('non-negative line and area value axes never extend below zero', () => {
  for (const mark of ['line', 'area'] as const) {
    const rows = [['Jan', 1_200_000], ['Feb', 2_900_000], ['Mar', 2_100_000], ['Apr', 3_400_000]]
    const option = echartsOption(lineEnvelope(mark, 'vertical', rows), defaultRendererContext) as any
    expect(option.yAxis.boundaryGap).toEqual(['0%', '5%'])
    withChart(option, (chart) => {
      const scale = chart.getModel().getComponent('yAxis').axis.scale
      expect(scale.getExtent()[0], mark).toBeGreaterThanOrEqual(0)
      for (const tick of scale.getTicks()) expect(tick.value, `${mark} tick`).toBeGreaterThanOrEqual(0)
      expect(scale.getExtent()[1]).toBeGreaterThan(3_400_000)
    })
  }
})

test('non-negative horizontal line value axes pad only the max side', () => {
  const rows = [['Jan', 1_200_000], ['Feb', 3_400_000]]
  const option = echartsOption(lineEnvelope('line', 'horizontal', rows), defaultRendererContext) as any
  expect(option.xAxis.boundaryGap).toEqual(['0%', '5%'])
})

test('a series that touches zero keeps its baseline without a negative tick', () => {
  const option = echartsOption(lineEnvelope('line', 'vertical', [['A', 0], ['B', 10]]), defaultRendererContext) as any
  withChart(option, (chart) => {
    expect(chart.getModel().getComponent('yAxis').axis.scale.getExtent()[0]).toBe(0)
  })
})

test('value axes with negative data keep padding on both sides', () => {
  const option = echartsOption(lineEnvelope('line', 'vertical', [['A', -5], ['B', 10]]), defaultRendererContext) as any
  expect(option.yAxis.boundaryGap).toEqual(['5%', '5%'])
  withChart(option, (chart) => {
    expect(chart.getModel().getComponent('yAxis').axis.scale.getExtent()[0]).toBeLessThan(-5)
  })
})

test('non-numeric and null measures do not disable the non-negative padding rule', () => {
  const option = echartsOption(lineEnvelope('line', 'vertical', [['A', null], ['B', 4], ['C', 9]]), defaultRendererContext) as any
  expect(option.yAxis.boundaryGap).toEqual(['0%', '5%'])
})

test('stacked and percent non-negative areas pad only the max side, percent axes stay unpadded', () => {
  const stacked = lineEnvelope('area', 'vertical', [['A', 2], ['B', 4]])
  stacked.spec.presentation.stacked = true
  expect((echartsOption(stacked, defaultRendererContext) as any).yAxis.boundaryGap).toEqual(['0%', '5%'])
  const percent = lineEnvelope('area', 'vertical', [['A', 2], ['B', 4]])
  percent.spec.presentation.stacking = 'percent'
  expect((echartsOption(percent, defaultRendererContext) as any).yAxis.boundaryGap).toBeUndefined()
})

