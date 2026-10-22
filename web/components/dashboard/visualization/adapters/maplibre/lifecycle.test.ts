import { expect, test } from 'bun:test'
import { waitForMapLoad } from './lifecycle'

type Listener = (...args: any[]) => void

class FakeLoadMap {
  readonly listeners = new Map<string, Set<Listener>>()
  styleLoaded = false
  on(event: string, listener: Listener): this {
    this.listeners.set(event, (this.listeners.get(event) ?? new Set()).add(listener))
    return this
  }
  once(event: string, listener: Listener): this {
    const wrapped: Listener = (...args) => { this.off(event, wrapped); listener(...args) }
    return this.on(event, wrapped)
  }
  off(event: string, listener: Listener): this {
    this.listeners.get(event)?.delete(listener)
    return this
  }
  isStyleLoaded(): boolean { return this.styleLoaded }
  fire(event: string, ...args: any[]): void { for (const listener of [...(this.listeners.get(event) ?? [])]) listener(...args) }
  listenerCount(): number { return [...this.listeners.values()].reduce((total, set) => total + set.size, 0) }
}

test('waitForMapLoad resolves on load and detaches its listeners', async () => {
  const map = new FakeLoadMap()
  const pending = waitForMapLoad(map as never, 1_000)
  map.fire('load')
  await pending
  expect(map.listenerCount()).toBe(0)
})

test('waitForMapLoad rejects when the style fails and load never follows', async () => {
  const map = new FakeLoadMap()
  const pending = waitForMapLoad(map as never, 1_000, 5)
  map.fire('error', { error: new Error('style request failed') })
  await expect(pending).rejects.toThrow('style request failed')
  expect(map.listenerCount()).toBe(0)
})

test('waitForMapLoad ignores tile, source, and post-style errors', async () => {
  const map = new FakeLoadMap()
  const pending = waitForMapLoad(map as never, 1_000)
  map.fire('error', { sourceId: 'basemap', error: new Error('tile 404') })
  map.fire('error', { tile: {}, error: new Error('tile 500') })
  map.styleLoaded = true
  map.fire('error', { error: new Error('glyph range missing') })
  map.fire('load')
  await pending
})

test('waitForMapLoad rejects with a timeout when load never arrives', async () => {
  const map = new FakeLoadMap()
  await expect(waitForMapLoad(map as never, 5)).rejects.toThrow('Timed out')
  expect(map.listenerCount()).toBe(0)
})

test('waitForMapLoad still resolves when a sprite or glyph error precedes load', async () => {
  // A missing sprite sheet or glyph range errors before the style is ready,
  // but MapLibre still fires load and renders the data layers.
  const map = new FakeLoadMap()
  const pending = waitForMapLoad(map as never, 1_000, 50)
  map.fire('error', { error: new Error('sprite 404') })
  map.fire('load')
  await pending
  expect(map.listenerCount()).toBe(0)
})
