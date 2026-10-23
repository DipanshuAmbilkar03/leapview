import { expect, test } from 'bun:test'
import * as echarts from 'echarts'

import { defaultRendererContext } from '../host-controller'
import { echartsOption } from './echarts'
import { cartesianFixture } from './echarts-test-fixtures'

const ranked = [['Technology', 90], ['Furniture', 60], ['Office Supplies', 30]]

function axisPolicy(id: string, inversion: 'automatic' | 'normal' | 'inverted' = 'automatic') {
  return {
    id, type: 'automatic', inversion, ticks: 'automatic', grid: 'automatic', labelRotation: 'automatic',
    dateUnit: 'automatic', scale: 'automatic', zero: 'automatic', tickDensity: 'automatic',
  }
}

function rankedEnvelope(mark: 'bar' | 'column' | 'line' | 'area' | 'combo', orientation: 'horizontal' | 'vertical', rows: unknown[][] = ranked) {
  const envelope = cartesianFixture(mark) as any
  Object.assign(envelope.spec.presentation, { orientation, stacked: false, dataZoom: false, legend: 'hidden', step: false })
  envelope.dataState.datasets[0].rows = rows
  return envelope
}

function withChart<T>(option: Record<string, any>, inspect: (chart: any) => T, size = { width: 640, height: 360 }): T {
  const chart = echarts.init(null, null, { renderer: 'svg', ssr: true, ...size })
  try {
    chart.setOption({ ...option, animation: false }, { notMerge: true, lazyUpdate: false })
    chart.renderToSVGString()
    return inspect(chart)
  } finally {
    chart.dispose()
  }
}

function categoryCoordinates(chart: any, axis: 'xAxis' | 'yAxis', count: number): number[] {
  // Pixel positions on the rendered chart, so the assertion covers ECharts'
  // own inverse handling rather than only the option we generated.
  const index = { [axis === 'yAxis' ? 'yAxisIndex' : 'xAxisIndex']: 0 }
  return Array.from({ length: count }, (_, category) => chart.convertToPixel(index, category) as number)
}

test('horizontal bars read top-down: rank 1 is drawn at the top of the category axis', () => {
  const option = echartsOption(rankedEnvelope('bar', 'horizontal'), defaultRendererContext) as any
  expect(option.yAxis).toMatchObject({ type: 'category', inverse: true })
  expect(option.xAxis.inverse).toBeUndefined()

  const [first, second, third] = withChart(option, (chart) => categoryCoordinates(chart, 'yAxis', 3)) as [number, number, number]
  expect(first).toBeLessThan(second)
  expect(second).toBeLessThan(third)
})

test('horizontal column and line orientations share the top-down category order', () => {
  for (const mark of ['column', 'line', 'area'] as const) {
    const option = echartsOption(rankedEnvelope(mark, 'horizontal'), defaultRendererContext) as any
    expect(option.yAxis, mark).toMatchObject({ type: 'category', inverse: true })
  }
})

test('an authored inverted category axis flips the new top-down default back to bottom-up', () => {
  for (const inversion of ['automatic', 'normal'] as const) {
    const envelope = rankedEnvelope('bar', 'horizontal')
    envelope.spec.axes = [axisPolicy('x', inversion)]
    expect((echartsOption(envelope, defaultRendererContext) as any).yAxis.inverse).toBe(true)
  }
  const envelope = rankedEnvelope('bar', 'horizontal')
  envelope.spec.axes = [axisPolicy('x', 'inverted')]
  const option = echartsOption(envelope, defaultRendererContext) as any
  expect(option.yAxis.inverse).toBeFalsy()
  const [first, , third] = withChart(option, (chart) => categoryCoordinates(chart, 'yAxis', 3)) as [number, number, number]
  expect(first).toBeGreaterThan(third)
})

test('the value axis inversion policy is independent of the category order default', () => {
  const envelope = rankedEnvelope('bar', 'horizontal')
  envelope.spec.axes = [axisPolicy('primary_y', 'inverted')]
  const option = echartsOption(envelope, defaultRendererContext) as any
  expect(option.yAxis.inverse).toBe(true)
  expect(option.xAxis.inverse).toBe(true)
})

test('vertical column charts keep left-to-right category order', () => {
  const option = echartsOption(rankedEnvelope('column', 'vertical'), defaultRendererContext) as any
  expect(option.xAxis).toMatchObject({ type: 'category' })
  expect(option.xAxis.inverse).toBeUndefined()
  expect(option.yAxis.inverse).toBeUndefined()
  const [first, , third] = withChart(option, (chart) => categoryCoordinates(chart, 'xAxis', 3)) as [number, number, number]
  expect(first).toBeLessThan(third)
})

test('horizontal stacked multi-measure bars keep series, legend and stack order while ranks read top-down', () => {
  const envelope = cartesianFixture('bar', ['label', 'cost', 'revenue']) as any
  Object.assign(envelope.spec.presentation, { orientation: 'horizontal', stacked: true, stacking: 'normal', dataZoom: false, legend: 'bottom', step: false })
  envelope.dataState.datasets[0].rows = [['Technology', 40, 50], ['Furniture', 30, 30], ['Office Supplies', 10, 20]]
  const option = echartsOption(envelope, defaultRendererContext) as any

  expect(option.yAxis).toMatchObject({ type: 'category', inverse: true })
  expect(option.series.filter((series: any) => series.stack).map((series: any) => series.name)).toEqual(['cost', 'revenue'])
  expect(option.legend.data?.map((item: any) => item.name ?? item) ?? ['cost', 'revenue']).toEqual(['cost', 'revenue'])
  const [first, , third] = withChart(option, (chart) => categoryCoordinates(chart, 'yAxis', 3)) as [number, number, number]
  expect(first).toBeLessThan(third)
})

test('horizontal category-series bars (split series) use the same top-down order', () => {
  const envelope = cartesianFixture('bar', ['label', 'value', 'region']) as any
  envelope.spec.series = { dataset: 'primary', field: 'region' }
  envelope.spec.y = [{ dataset: 'primary', field: 'value' }]
  Object.assign(envelope.spec.presentation, { orientation: 'horizontal', stacked: true, stacking: 'normal', dataZoom: false, legend: 'bottom', step: false })
  envelope.dataState.datasets[0].rows = [['Technology', 40, 'East'], ['Technology', 50, 'West'], ['Furniture', 30, 'East'], ['Furniture', 30, 'West']]
  const option = echartsOption(envelope, defaultRendererContext) as any
  expect(option.yAxis).toMatchObject({ type: 'category', inverse: true })
  expect(option.series.filter((series: any) => series.stack).map((series: any) => series.name)).toEqual(['East', 'West'])
})

test('horizontal combo charts put the first category at the top', () => {
  const envelope = cartesianFixture('combo', ['label', 'cost', 'revenue']) as any
  Object.assign(envelope.spec.presentation, { orientation: 'horizontal', stacked: false, dataZoom: false, legend: 'hidden', step: false })
  envelope.dataState.datasets[0].rows = [['Technology', 40, 50], ['Furniture', 30, 30]]
  const option = echartsOption(envelope, defaultRendererContext) as any
  const categoryAxis = Array.isArray(option.yAxis) ? option.yAxis.find((axis: any) => axis.type === 'category') : option.yAxis
  expect(categoryAxis).toMatchObject({ type: 'category', inverse: true })
})

function annotatedHorizontalLine(category: string, inversion: 'automatic' | 'inverted' = 'automatic') {
  const envelope = rankedEnvelope('line', 'horizontal', [['A', 10], ['B', 20], ['C', 30]])
  envelope.spec.axes = [axisPolicy('x', inversion)]
  envelope.spec.eventAnnotations = [{ id: 'launch', axis: 'x', value: { kind: 'text', value: category }, label: 'Launch', tone: 'ink' }]
  return (echartsOption(envelope, defaultRendererContext) as any).series[0].markLine.data[0]
}

test('a category annotation on the top row of a horizontal chart is labelled below its line', () => {
  // Top-down by default: the first category is the top row.
  expect(annotatedHorizontalLine('A').label).toEqual({ position: 'insideEndBottom' })
  expect(annotatedHorizontalLine('B').label).toBeUndefined()
  expect(annotatedHorizontalLine('C').label).toBeUndefined()
  // Authored inversion puts the last category on the top row.
  expect(annotatedHorizontalLine('C', 'inverted').label).toEqual({ position: 'insideEndBottom' })
  expect(annotatedHorizontalLine('A', 'inverted').label).toBeUndefined()
})
