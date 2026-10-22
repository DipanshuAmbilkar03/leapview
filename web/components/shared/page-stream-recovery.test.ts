import { describe, expect, test } from 'bun:test'
import { PageStreamMonitor, pageStreamFailure, pageStreamRecovery, type PageStreamFailure, type PageStreamPresenter } from './page-stream-recovery'

const stream = { id: 'stream-root' }
const other = { id: 'command-owner' }

function fetchEvent(type: string, el: object = stream, status?: string | number): Event {
  return new CustomEvent('datastar-fetch', { detail: { type, el, argsRaw: status === undefined ? {} : { status } } })
}

function monitor(hidden = false): PageStreamMonitor {
  return new PageStreamMonitor({ isStream: (el) => el === stream, hidden: () => hidden })
}

describe('page stream failure copy', () => {
  test.each([
    [403, 'forbidden'],
    [429, 'rate-limited'],
    [500, 'unavailable'],
    [503, 'unavailable'],
    [null, 'network'],
    [0, 'network'],
    [404, 'unexpected'],
    [400, 'unexpected'],
  ] as const)('classifies status %s as %s', (status, kind) => {
    const failure = pageStreamFailure(status)
    expect(failure.kind).toBe(kind)
    expect(failure.status).toBe(status)
    expect(failure.title).not.toBe('')
    expect(failure.message).not.toBe('')
  })

  test('each kind has its own heading and tells the reader what to do without apologising', () => {
    const copy = [403, 429, 503, null, 404].map((status) => pageStreamFailure(status))
    expect(new Set(copy.map((failure) => failure.title)).size).toBe(copy.length)
    for (const failure of copy) expect(`${failure.title} ${failure.message}`).not.toMatch(/sorry|oops|unfortunately/i)
    expect(pageStreamFailure(429).message).toContain('retry')
    expect(pageStreamFailure(403).message).toContain('Ask an admin')
  })
})

describe('page stream monitor', () => {
  test('ignores events from elements that do not own the page stream', () => {
    const m = monitor()
    expect(m.observe(fetchEvent('error', other, 503))).toEqual({ action: 'none' })
    expect(m.observe(fetchEvent('retries-failed', other))).toEqual({ action: 'none' })
  })

  test.each([403, 429, 502, 503])('shows the recovery card for HTTP %s on the stream', (status) => {
    const m = monitor()
    m.observe(fetchEvent('started'))
    expect(m.observe(fetchEvent('error', stream, String(status)))).toEqual({ action: 'fail', failure: pageStreamFailure(status) })
  })

  test('sends an expired session on the stream to sign in', () => {
    const m = monitor()
    m.observe(fetchEvent('started'))
    expect(m.observe(fetchEvent('error', stream, '401'))).toEqual({ action: 'sign-in' })
  })

  test('shows the network card when retries are exhausted', () => {
    const m = monitor()
    m.observe(fetchEvent('started'))
    expect(m.observe(fetchEvent('retries-failed'))).toEqual({ action: 'fail', failure: pageStreamFailure(null) })
  })

  test('keeps the error copy when the stream finishes after the error', () => {
    const m = monitor()
    m.observe(fetchEvent('started'))
    m.observe(fetchEvent('error', stream, '503'))
    expect(m.observe(fetchEvent('finished'))).toEqual({ action: 'none' })
  })

  test('shows the unexpected-response card when the stream ends before delivering anything', () => {
    const m = monitor()
    m.observe(fetchEvent('started'))
    const transition = m.observe(fetchEvent('finished'))
    expect(transition.action).toBe('fail')
    expect((transition as { failure: PageStreamFailure }).failure.kind).toBe('unexpected')
  })

  test('does not flag a stream that delivered content before it ended', () => {
    const m = monitor()
    m.observe(fetchEvent('started'))
    expect(m.observe(fetchEvent('datastar-patch-elements'))).toEqual({ action: 'recover' })
    expect(m.observe(fetchEvent('finished'))).toEqual({ action: 'none' })
  })

  test('treats the first event as belonging to a stream that opened before it was listening', () => {
    const m = monitor()
    const transition = m.observe(fetchEvent('finished'))
    expect(transition.action).toBe('fail')
    expect((transition as { failure: PageStreamFailure }).failure.kind).toBe('unexpected')
    const loaded = monitor()
    expect(loaded.observe(fetchEvent('datastar-patch-elements'))).toEqual({ action: 'recover' })
    expect(loaded.observe(fetchEvent('finished'))).toEqual({ action: 'none' })
  })

  test('does not flag a stream released while the tab is hidden', () => {
    const m = monitor(true)
    m.observe(fetchEvent('started'))
    expect(m.observe(fetchEvent('finished'))).toEqual({ action: 'none' })
  })

  test('waits for the replacement stream when a resumed tab aborts the previous one', () => {
    const m = monitor()
    m.observe(fetchEvent('started'))
    m.observe(fetchEvent('started'))
    expect(m.observe(fetchEvent('finished'))).toEqual({ action: 'none' })
    expect(m.observe(fetchEvent('datastar-patch-signals'))).toEqual({ action: 'recover' })
    expect(m.observe(fetchEvent('finished'))).toEqual({ action: 'none' })
  })

  test('covers a page that never loaded once reconnection keeps failing', () => {
    const m = monitor()
    m.observe(fetchEvent('started'))
    expect(m.observe(fetchEvent('retrying'))).toEqual({ action: 'none' })
    expect(m.observe(fetchEvent('retrying'))).toEqual({ action: 'none' })
    expect(m.observe(fetchEvent('retrying'))).toEqual({ action: 'fail', failure: pageStreamFailure(null) })
    expect(m.observe(fetchEvent('datastar-patch-elements'))).toEqual({ action: 'recover' })
  })

  test('never covers a loaded page while the stream reconnects', () => {
    const m = monitor()
    m.observe(fetchEvent('started'))
    m.observe(fetchEvent('datastar-patch-elements'))
    for (let attempt = 0; attempt < 6; attempt++) expect(m.observe(fetchEvent('retrying'))).toEqual({ action: 'none' })
  })

  test('hides the card when the stream starts again', () => {
    const m = monitor()
    m.observe(fetchEvent('started'))
    m.observe(fetchEvent('error', stream, '503'))
    expect(m.observe(fetchEvent('started'))).toEqual({ action: 'recover' })
  })
})

describe('page stream recovery signal', () => {
  function recordingPresenter() {
    const shown: string[] = []
    const calls = { signIn: 0 }
    const presenter: PageStreamPresenter = {
      show: (failure) => { shown.push(failure.kind) },
      signIn: () => { calls.signIn++ },
    }
    return { presenter, shown, calls }
  }

  test('returns the next signal value and presents the failure', () => {
    const m = monitor()
    const { presenter, shown } = recordingPresenter()
    expect(pageStreamRecovery(fetchEvent('error', stream, '503'), false, m, presenter)).toBe(true)
    expect(shown).toEqual(['unavailable'])
    expect(pageStreamRecovery(fetchEvent('started'), true, m, presenter)).toBe(false)
  })

  test('keeps the current value for events that do not change it', () => {
    const m = monitor()
    const { presenter } = recordingPresenter()
    m.observe(fetchEvent('started'))
    expect(pageStreamRecovery(fetchEvent('retrying'), false, m, presenter)).toBe(false)
    expect(pageStreamRecovery(fetchEvent('retrying'), true, m, presenter)).toBe(true)
  })

  test('redirects an expired session to sign in without opening the card', () => {
    const m = monitor()
    const { presenter, shown, calls } = recordingPresenter()
    expect(pageStreamRecovery(fetchEvent('error', stream, '401'), false, m, presenter)).toBe(false)
    expect(calls.signIn).toBe(1)
    expect(shown).toEqual([])
  })
})
