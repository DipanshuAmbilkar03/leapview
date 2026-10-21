import { expect, test } from 'bun:test'
import { networkConnections } from './network-overview'
import { networkFixture } from './adapters/echarts-test-fixtures'

test('dense network overview retains every connection and searches both node labels', () => {
  const envelope = networkFixture('sankey') as any
  envelope.dataState.datasets[0].rows = Array.from({ length: 120 }, (_, index) => [`Category ${index}`, 'Delivered', index])
  expect(networkConnections(envelope)).toHaveLength(120)
  expect(networkConnections(envelope, 'category 119')).toEqual([{ source: 'Category 119', target: 'Delivered', value: '119' }])
  expect(networkConnections(envelope, 'DELIVERED')).toHaveLength(120)
})
