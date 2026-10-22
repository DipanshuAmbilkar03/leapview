import { expect, test } from 'bun:test'
import { agentUnavailable } from './agent-availability'

test('agent surfaces are hidden only when the server reports the agent disabled', () => {
  expect(agentUnavailable({ status: { enabled: false } })).toBe(true)
  expect(agentUnavailable({ status: { enabled: true } })).toBe(false)
  expect(agentUnavailable({ status: {} })).toBe(false)
  expect(agentUnavailable({})).toBe(false)
  expect(agentUnavailable(null)).toBe(false)
  expect(agentUnavailable(undefined)).toBe(false)
})
