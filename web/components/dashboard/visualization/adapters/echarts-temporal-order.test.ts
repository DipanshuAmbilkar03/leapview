import { expect, test } from 'bun:test'
import { echartsOption } from './echarts'
import { cartesianFixture } from './echarts-test-fixtures'

test('time series connect points chronologically without changing query rows or identities', () => {
  const envelope = cartesianFixture('line') as any
  envelope.spec.datasets[0].fields[0].dataType = 'date'
  envelope.dataState.datasets[0].rows = [['2024-03-01', 100], ['2024-01-01', 90], ['2024-02-01', 80]]
  const option = echartsOption(envelope) as any
  const dataset = Array.isArray(option.dataset) ? option.dataset.find((dataset: any) => dataset.id === option.series[0].datasetId) : option.dataset
  expect(dataset.source.slice(1).map((row: any[]) => row[0])).toEqual(['2024-01-01', '2024-02-01', '2024-03-01'])
  expect(option.series[0].__lv_source_row_indices).toEqual([1, 2, 0])
  expect(envelope.dataState.datasets[0].rows[0][0]).toBe('2024-03-01')
})

test('chronological rendering keeps cross-highlight attached to the original governed date', () => {
  const envelope = cartesianFixture('line') as any
  envelope.spec.datasets[0].fields[0].dataType = 'date'
  envelope.spec.datasets[0].fields[0].sourceRef = 'orders.date'
  envelope.dataState.datasets[0].rows = [['2024-03-01', 100], ['2024-01-01', 90], ['2024-02-01', 80]]
  envelope.highlights = [{ sourceVisualID: 'source', interactionID: 'point_selection', entries: [{ mappings: [{ targetFieldID: 'orders.date', value: '2024-03-01' }] }] }]
  const option = echartsOption(envelope) as any
  const series = option.series[0]
  expect(series.itemStyle.opacity({ dataIndex: 0 })).toBe(0.2)
  expect(series.itemStyle.opacity({ dataIndex: 2 })).toBe(1)
  const dataset = option.dataset.find((dataset: any) => dataset.id === series.datasetId)
  expect(dataset.source[3]).toEqual(envelope.dataState.datasets[0].rows[0])
})

test('each temporal category series sorts independently and retains its query row mapping', () => {
  const envelope = cartesianFixture('line', ['label', 'value', 'category']) as any
  envelope.spec.datasets[0].fields[0].dataType = 'date'
  envelope.spec.datasets[0].fields[2].dataType = 'string'
  envelope.spec.datasets[0].fields[2].role = 'dimension'
  envelope.spec.y = [{ dataset:'primary', field:'value' }]
  envelope.spec.series = { dataset:'primary', field:'category' }
  envelope.dataState.datasets[0].rows = [['2024-03-01', 100, 'A'], ['2024-01-01', 90, 'B'], ['2024-01-01', 80, 'A'], ['2024-03-01', 70, 'B']]
  const option = echartsOption(envelope) as any
  const a = option.series.find((series: any) => series.name === 'A')
  const b = option.series.find((series: any) => series.name === 'B')
  expect(a.__lv_source_row_indices).toEqual([2, 0])
  expect(b.__lv_source_row_indices).toEqual([1, 3])
  expect(option.dataset.find((dataset: any) => dataset.id === a.datasetId).source.slice(1).map((row: any[]) => row[0])).toEqual(['2024-01-01', '2024-03-01'])
})
