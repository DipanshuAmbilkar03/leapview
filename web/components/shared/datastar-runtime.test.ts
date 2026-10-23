import { expect, test } from 'bun:test'
import { createDatastarRuntimeLoader, datastarRuntimeURL } from './datastar-runtime'

test('the Datastar runtime import is cached once it succeeds', async () => {
  const urls: string[] = []
  const runtime = { root: {} }
  const load = createDatastarRuntimeLoader(async (url) => { urls.push(url); return runtime })
  expect(await load()).toBe(runtime as never)
  expect(await load()).toBe(runtime as never)
  expect(urls).toEqual([datastarRuntimeURL])
})

test('a failed Datastar runtime import is not cached forever', async () => {
  let calls = 0
  const runtime = { root: {} }
  const load = createDatastarRuntimeLoader(async () => {
    if (++calls === 1) throw new TypeError('Failed to fetch dynamically imported module')
    return runtime
  })
  await expect(load()).rejects.toThrow('Failed to fetch')
  expect(await load()).toBe(runtime as never)
  expect(calls).toBe(2)
})
