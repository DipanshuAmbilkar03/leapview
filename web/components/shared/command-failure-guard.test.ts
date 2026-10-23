import { describe, expect, test } from 'bun:test'
import { browserCommandFailure } from './command-failure'
import { CommandFailureGuard, commandFailureNotice, type CommandFailureEffects } from './command-failure-guard'

const stream = { id: 'stream-root' }
const owner = { id: 'dashboard-page' }

function fetchEvent(type: string, el: object = owner, status?: string | number): Event {
  return new CustomEvent('datastar-fetch', { detail: { type, el, argsRaw: status === undefined ? {} : { status } } })
}

function harness(now = () => 0) {
  const notices: Array<{ message: string, reload: boolean }> = []
  const calls = { signIn: 0 }
  const effects: CommandFailureEffects = {
    isPageStream: (el) => el === stream,
    signIn: () => { calls.signIn++ },
    notify: (notice) => { notices.push({ message: notice.message, reload: notice.reload }) },
    defer: (run) => run(),
    now,
  }
  return { guard: new CommandFailureGuard(effects), notices, calls }
}

describe('command failure guard', () => {
  test('sends an expired session to sign in instead of leaving the page silently stale', () => {
    const { guard, notices, calls } = harness()
    guard.handle(fetchEvent('error', owner, '401'))
    expect(calls.signIn).toBe(1)
    expect(notices).toEqual([])
  })

  test.each([403, 404, 409, 429, 400, 422, 500, 503])('tells the user when a command fails with HTTP %s', (status) => {
    const { guard, notices, calls } = harness()
    guard.handle(fetchEvent('error', owner, String(status)))
    expect(calls.signIn).toBe(0)
    expect(notices).toHaveLength(1)
    expect(notices[0]?.message).not.toBe('')
  })

  test('tells the user when a command cannot reach the server', () => {
    const { guard, notices } = harness()
    guard.handle(fetchEvent('retries-failed'))
    expect(notices).toHaveLength(1)
    expect(notices[0]?.message).toContain('connection')
  })

  test('ignores progress events', () => {
    const { guard, notices, calls } = harness()
    for (const type of ['started', 'finished', 'retrying', 'datastar-patch-elements']) guard.handle(fetchEvent(type))
    expect(notices).toEqual([])
    expect(calls.signIn).toBe(0)
  })

  test('leaves the page stream to the recovery card', () => {
    const { guard, notices, calls } = harness()
    guard.handle(fetchEvent('error', stream, '503'))
    guard.handle(fetchEvent('error', stream, '401'))
    expect(notices).toEqual([])
    expect(calls.signIn).toBe(0)
  })

  test('stays quiet when a component already presented the failure', () => {
    const { guard, notices } = harness()
    const event = fetchEvent('error', owner, '503')
    browserCommandFailure(event, 'Saving settings')
    guard.handle(event)
    expect(notices).toEqual([])
  })

  test('still redirects an expired session that a component presented', () => {
    const { guard, calls } = harness()
    const event = fetchEvent('error', owner, '401')
    browserCommandFailure(event, 'Saving settings')
    guard.handle(event)
    expect(calls.signIn).toBe(1)
  })

  test('waits for components to claim the failure before notifying', () => {
    const notices: string[] = []
    let deferred: (() => void) | undefined
    const guard = new CommandFailureGuard({
      isPageStream: () => false,
      signIn: () => {},
      notify: (notice) => { notices.push(notice.message) },
      defer: (run) => { deferred = run },
      now: () => 0,
    })
    const event = fetchEvent('error', owner, '503')
    guard.handle(event)
    expect(notices).toEqual([])
    browserCommandFailure(event, 'Saving settings')
    deferred?.()
    expect(notices).toEqual([])
  })

  test('collapses a burst of identical failures into one notice', () => {
    let clock = 0
    const { guard, notices } = harness(() => clock)
    guard.handle(fetchEvent('error', owner, '503'))
    clock = 500
    guard.handle(fetchEvent('error', owner, '503'))
    expect(notices).toHaveLength(1)
    clock = 5_000
    guard.handle(fetchEvent('error', owner, '503'))
    expect(notices).toHaveLength(2)
  })

  test('does not collapse different failures', () => {
    const { guard, notices } = harness()
    guard.handle(fetchEvent('error', owner, '503'))
    guard.handle(fetchEvent('error', owner, '403'))
    expect(notices).toHaveLength(2)
  })
})

describe('command failure notices', () => {
  test('offers a reload when the page state is stale', () => {
    expect(commandFailureNotice({ kind: 'conflict', status: 409, retryable: false, message: '' }).reload).toBe(true)
    expect(commandFailureNotice({ kind: 'not-found', status: 404, retryable: false, message: '' }).reload).toBe(true)
    expect(commandFailureNotice({ kind: 'unavailable', status: 503, retryable: true, message: '' }).reload).toBe(false)
  })

  test('says what happened and what to do, without apologising or blaming', () => {
    const kinds = ['forbidden', 'not-found', 'conflict', 'rate-limited', 'unavailable', 'invalid-draft', 'validation', 'network', 'unknown'] as const
    const messages = kinds.map((kind) => commandFailureNotice({ kind, status: null, retryable: false, message: '' }).message)
    expect(new Set(messages).size).toBe(kinds.length)
    for (const message of messages) {
      expect(message).not.toMatch(/sorry|oops|unfortunately|error occurred/i)
      expect(message.length).toBeLessThan(140)
    }
  })
})
