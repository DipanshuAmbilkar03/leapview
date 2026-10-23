import { afterEach, expect, test } from 'bun:test'
import { copyText } from './clipboard'

const globals = globalThis as Record<string, unknown>
const original = { navigator: globals.navigator, document: globals.document, window: globals.window }

afterEach(() => {
  for (const [key, value] of Object.entries(original)) {
    Object.defineProperty(globalThis, key, { value, configurable: true, writable: true })
  }
})

function stub(key: 'navigator' | 'document' | 'window', value: unknown): void {
  Object.defineProperty(globalThis, key, { value, configurable: true, writable: true })
}

function fakeDocument(execResult: boolean | Error) {
  const appended: unknown[] = []
  const textarea = { value: '', style: {} as Record<string, string>, setAttribute() {}, select() {}, remove() { appended.pop() } }
  return {
    appended,
    document: {
      activeElement: null,
      body: { append: (node: unknown) => appended.push(node) },
      createElement: () => textarea,
      execCommand: () => { if (execResult instanceof Error) throw execResult; return execResult },
    },
  }
}

test('copyText resolves true when the async clipboard accepts the value', async () => {
  const written: string[] = []
  stub('navigator', { clipboard: { writeText: async (value: string) => { written.push(value) } } })
  expect(await copyText('secret')).toBe(true)
  expect(written).toEqual(['secret'])
})

test('copyText never throws when the clipboard permission is denied and falls back to execCommand', async () => {
  stub('navigator', { clipboard: { writeText: async () => { throw new DOMException('denied', 'NotAllowedError') } } })
  const { document, appended } = fakeDocument(true)
  stub('document', document)
  expect(await copyText('secret')).toBe(true)
  expect(appended).toHaveLength(0)
})

test('copyText selects the visible source and resolves false when every path fails', async () => {
  stub('navigator', { clipboard: undefined })
  stub('document', fakeDocument(false).document)
  const selected: unknown[] = []
  stub('window', { getSelection: () => ({ selectAllChildren: (node: unknown) => selected.push(node) }) })
  const source = {} as Element
  expect(await copyText('secret', { selectOnFailure: source })).toBe(false)
  expect(selected).toEqual([source])
})

test('copyText survives a throwing execCommand and a throwing selection API', async () => {
  stub('navigator', {})
  stub('document', fakeDocument(new Error('blocked')).document)
  stub('window', { getSelection: () => { throw new Error('no selection') } })
  expect(await copyText('secret', { selectOnFailure: {} as Element })).toBe(false)
})
