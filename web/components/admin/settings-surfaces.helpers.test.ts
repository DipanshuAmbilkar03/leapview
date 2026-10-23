import { expect, test } from 'bun:test'
import { groupNameProblem } from './settings-surfaces.helpers'

const groups = [{ id: 'g1', name: 'Analytics Team' }, { id: 'g2', name: 'Finance' }]

test('group names must be non-empty', () => {
  expect(groupNameProblem(groups, '')).toBe('Enter a group name.')
  expect(groupNameProblem(groups, '   ')).toBe('Enter a group name.')
})

test('duplicate group names are reported case-insensitively and ignore surrounding space', () => {
  expect(groupNameProblem(groups, 'analytics team')).toBe('A group named "Analytics Team" already exists. Choose a different name.')
  expect(groupNameProblem(groups, '  FINANCE ')).toBe('A group named "Finance" already exists. Choose a different name.')
})

test('a group may keep its own name when renamed, but not take another group\'s', () => {
  expect(groupNameProblem(groups, 'Finance', 'g2')).toBe('')
  expect(groupNameProblem(groups, 'Finance', 'g1')).toContain('already exists')
})

test('unique names are accepted', () => {
  expect(groupNameProblem(groups, 'Support')).toBe('')
})
