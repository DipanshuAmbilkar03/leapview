import { expect, test } from 'bun:test'
import { retryableLazy } from './lazy-import'

test('retryableLazy runs the loader once for concurrent and later callers', async () => {
  let calls = 0
  const load = retryableLazy(async () => ({ id: ++calls }))
  const [first, second] = await Promise.all([load(), load()])
  expect(first).toBe(second)
  expect(await load()).toBe(first)
  expect(calls).toBe(1)
})

test('retryableLazy forgets a rejected load so the next call retries', async () => {
  let calls = 0
  const load = retryableLazy(async () => {
    calls++
    if (calls === 1) throw new Error('chunk failed')
    return 'ready'
  })
  await expect(load()).rejects.toThrow('chunk failed')
  expect(await load()).toBe('ready')
  expect(await load()).toBe('ready')
  expect(calls).toBe(2)
})

test('retryableLazy rejects every caller waiting on a failed attempt with the same error', async () => {
  const failure = new Error('offline')
  const load = retryableLazy<string>(() => Promise.reject(failure))
  const results = await Promise.allSettled([load(), load()])
  expect(results.map((result) => (result.status === 'rejected' ? result.reason : null))).toEqual([failure, failure])
})

test('retryableLazy turns a synchronously throwing loader into a retryable rejection', async () => {
  let calls = 0
  const load = retryableLazy<string>(() => {
    if (++calls === 1) throw new Error('sync failure')
    return Promise.resolve('ok')
  })
  await expect(load()).rejects.toThrow('sync failure')
  expect(await load()).toBe('ok')
})

test('a late failure of an old attempt does not evict a newer successful load', async () => {
  let rejectFirst: (error: Error) => void = () => {}
  let calls = 0
  const load = retryableLazy<string>(() => {
    calls++
    return calls === 1 ? new Promise((_, reject) => { rejectFirst = reject }) : Promise.resolve('second')
  })
  const first = load()
  await Promise.resolve()
  rejectFirst(new Error('first failed'))
  await expect(first).rejects.toThrow('first failed')
  expect(await load()).toBe('second')
  expect(await load()).toBe('second')
  expect(calls).toBe(2)
})
