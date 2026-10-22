import { expect, test } from 'bun:test'
import { isNumericType } from './field-types'

test('numeric types cover the union of the previous preview, filter, and icon checks', () => {
  for (const type of ['INTEGER', 'int', 'BIGINT', 'smallint', 'tinyint', 'HUGEINT', 'decimal(18,2)', 'DECIMAL', 'DOUBLE', 'FLOAT', 'float64', 'NUMERIC', 'number', 'REAL', ' Number ']) {
    expect(isNumericType(type)).toBe(true)
  }
})

test('non-numeric and missing types are not numeric', () => {
  for (const type of ['VARCHAR', 'string', 'DATE', 'TIMESTAMP', 'BOOLEAN', 'JSON', 'BLOB', 'uuid', '', undefined]) {
    expect(isNumericType(type)).toBe(false)
  }
})

test('REAL is numeric for filters and previews, which the filter-only check used to miss', () => {
  expect(isNumericType('real')).toBe(true)
})
