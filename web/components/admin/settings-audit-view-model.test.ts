import { expect, test } from 'bun:test'
import type { AuditEventSignal } from '../../generated/signals'
import { auditActorHref, auditResourceHref, auditTable } from './settings-audit-view-model'

function event(overrides: Partial<AuditEventSignal>): AuditEventSignal {
  return { id: 'evt-1', action: 'group.delete', resourceKind: 'group', resourceId: 'group-1', createdAt: '2026-10-01T00:00:00Z', ...overrides }
}

test('audit rows link to a group or user that still exists', () => {
  expect(auditResourceHref(event({ resourceLabel: 'Analysts' }))).toBe('/admin/groups/group-1')
  expect(auditResourceHref(event({ resourceKind: 'principal', resourceId: 'p 1', resourceLabel: 'Ada Lovelace' }))).toBe('/admin/principals/p%201')
})

test('audit rows do not link to resources that were deleted', () => {
  // The server falls back to the humanized kind when the id no longer resolves.
  expect(auditResourceHref(event({ resourceLabel: 'Group' }))).toBe('')
  expect(auditResourceHref(event({ resourceLabel: '' }))).toBe('')
  expect(auditResourceHref(event({}))).toBe('')
  expect(auditResourceHref(event({ resourceKind: 'principal', resourceLabel: 'Principal' }))).toBe('')
})

test('audit rows never link kinds without a detail page', () => {
  expect(auditResourceHref(event({ resourceKind: 'role_binding', resourceId: 'b1', resourceLabel: 'Binding' }))).toBe('')
})

test('actors link only while the principal is still known', () => {
  expect(auditActorHref(event({ principalId: 'p1', principalName: 'Ada' }))).toBe('/admin/principals/p1')
  expect(auditActorHref(event({ principalId: 'p1', principalEmail: 'ada@example.com' }))).toBe('/admin/principals/p1')
  expect(auditActorHref(event({ principalId: 'p1' }))).toBe('')
  expect(auditActorHref(event({}))).toBe('')
})

test('the audit table carries the same link decisions', () => {
  const table = auditTable({ items: [event({ resourceLabel: 'Group' }), event({ id: 'evt-2', resourceLabel: 'Analysts' })], loading: false } as never)
  expect(table.rows.map((row) => row.resource.href)).toEqual(['', '/admin/groups/group-1'])
})
