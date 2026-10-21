import { expect, test } from 'bun:test'
import { cellLabel } from './windowed-table-presentation'

test('date-only values omit time while timestamp values preserve it', () => {
  expect(cellLabel('2018-07-17T00:00:00Z', { key: 'date', label: 'Date', type: 'date' })).toBe('2018-07-17')
  expect(cellLabel('2018-07-17T12:34:56Z', { key: 'time', label: 'Time', type: 'timestamp' })).toBe('2018-07-17 12:34:56Z')
})
