import { expect, test } from 'bun:test'

import { formatCell } from './format'

test('table cells use the visualization formatting contract when supplied by the IR', () => {
  expect(formatCell(1234.5, {
    key: 'revenue', label: 'Revenue', align: 'right', role: 'metric',
    visualizationFormat: { kind: 'currency', currency: 'USD' },
  })).toBe('$1,234.50')
  expect(formatCell(null, { key: 'revenue', label: 'Revenue', visualizationFormat: { kind: 'number' } })).toBe('—')
  expect(formatCell('', { key: 'pivot_revenue', label: 'Revenue', role: 'metric', visualizationFormat: { kind: 'currency', currency: 'BRL' } })).toBe('—')
  expect(formatCell('-', { key: 'pivot_orders', label: 'Orders', role: 'metric', visualizationFormat: { kind: 'number' } })).toBe('—')
  expect(formatCell('—', { key: 'pivot_orders', label: 'Orders', role: 'metric', visualizationFormat: { kind: 'number' } })).toBe('—')
  expect(formatCell('—', { key: 'pivot_orders', label: 'Orders', role: 'metric', visualizationFormat: { kind: 'number' } }, true)).toBe('0')
  expect(formatCell(undefined, { key: 'pivot_revenue', label: 'Revenue', role: 'metric', visualizationFormat: { kind: 'currency', currency: 'BRL' } }, true)).toBe('R$0.00')
  expect(formatCell(null, { key: 'pivot_revenue', label: 'Revenue', role: 'metric', visualizationFormat: { kind: 'currency', currency: 'BRL' } }, true)).toBe('—')
  expect(formatCell(null, { key: 'pivot_revenue', label: 'Revenue', role: 'metric', format: 'currency' }, true)).toBe('-')
  expect(formatCell('12', { key: 'orders', label: 'Orders', role: 'metric', visualizationFormat: { kind: 'number' } })).toBe('12')
  expect(formatCell('2026-06-01', { key: 'purchase_date', label: 'Purchased', visualizationFormat: { kind: 'temporal' } })).toBe('2026-06-01')
  expect(() => formatCell('1e3', { key: 'orders', label: 'Orders', role: 'metric', visualizationFormat: { kind: 'number' } })).toThrow(/canonical decimal string/)
})

test('temporal table display follows the field type, never midnight or the field name', () => {
  for (const value of ['2024-01-02', '2024-01-02T00:00:00Z', '2024-01-02T00:00:00+14:00']) {
    expect(formatCell(value, { key: 'arbitrary', label: 'Value', dataType: 'date' })).toBe('2024-01-02')
    expect(formatCell(value, { key: 'arbitrary', label: 'Value', dataType: 'date', visualizationFormat: { kind: 'temporal' } })).toBe('2024-01-02')
  }
  for (const dataType of ['temporal', 'timestamp', 'datetime', 'DateTimeTz', 'string', undefined]) {
    for (const value of ['2024-01-02T00:00:00Z', '2024-01-02T13:45:12.123Z', '2024-01-02T00:00:00+14:00']) {
      expect(formatCell(value, { key: 'purchase_date', label: 'Date', dataType })).toBe(value)
    }
  }
  expect(formatCell(null, { key: 'day', label: 'Day', dataType: 'date' })).toBe('-')
})

test('typed temporal columns preserve timestamp precision with the default temporal format', () => {
  for (const value of ['2024-01-02T00:00:00Z', '2024-01-02T13:45:12.123456Z']) {
    expect(formatCell(value, { key: 'event', label: 'Event', dataType: 'temporal', visualizationFormat: { kind: 'temporal' } })).toBe(value)
  }
  expect(formatCell('2024-01-02T13:45:12Z', { key: 'event', label: 'Event', dataType: 'temporal', visualizationFormat: { kind: 'temporal', dateStyle: 'short' } })).toBe('2024-01-02')
  expect(formatCell('2024-01-02T13:45:12Z', { key: 'event', label: 'Event', dataType: 'temporal', visualizationFormat: { kind: 'temporal', timeStyle: 'medium' } })).toBe('13:45:12')
})
