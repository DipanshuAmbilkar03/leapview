import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test'
import { JSDOM } from 'jsdom'

// Lit reads DOM globals at import time, so install jsdom's for this file. jsdom
// has no layout and reports no client rects, which would hide every control from
// the composed focusable walk; report one rect per element instead.
const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true })
const globals = ['window', 'document', 'customElements', 'getComputedStyle', ...Object.getOwnPropertyNames(dom.window).filter(name => /^(HTML|SVG|CSS|Document|Shadow|Node|Element|Event|CustomEvent|KeyboardEvent|Text|Comment|Range|Tree|MutationObserver|Mutation|ElementInternals)/.test(name))]
const previous = new Map<string, PropertyDescriptor | undefined>()

type DrawerElement = HTMLElement & { open: boolean; modal: boolean; updateComplete: Promise<boolean> }

beforeAll(async () => {
  const window = dom.window as unknown as Record<string, unknown>
  for (const name of globals) {
    previous.set(name, Object.getOwnPropertyDescriptor(globalThis, name))
    Object.defineProperty(globalThis, name, { configurable: true, writable: true, value: window[name] })
  }
  dom.window.HTMLElement.prototype.getClientRects = function () { return [{}] as unknown as DOMRectList }
  await import('./drawer')
})

afterEach(() => { document.body.replaceChildren() })

afterAll(() => {
  for (const [name, descriptor] of previous) {
    if (descriptor) Object.defineProperty(globalThis, name, descriptor)
    else delete (globalThis as Record<string, unknown>)[name]
  }
  dom.window.close()
})

const tick = (ms = 5) => new Promise(resolve => setTimeout(resolve, ms))

async function openDrawer(options: { modal?: boolean } = {}) {
  const opener = document.createElement('button')
  opener.textContent = 'Credentials'
  const elsewhere = document.createElement('button')
  document.body.append(opener, elsewhere)
  opener.focus()
  const drawer = document.createElement('lv-drawer') as DrawerElement
  drawer.modal = options.modal ?? true
  drawer.innerHTML = '<input type="password" aria-label="Password">'
  drawer.open = true
  document.body.append(drawer)
  await drawer.updateComplete
  await tick()
  return { opener, elsewhere, drawer, field: drawer.querySelector('input')! }
}

test('a modal drawer moves focus to its first field rather than the close button', async () => {
  const { drawer, field } = await openDrawer()
  expect(document.activeElement).toBe(field)
  expect(drawer.shadowRoot!.activeElement).toBeNull()
})

test('closing a modal drawer returns focus to the control that opened it', async () => {
  const { opener, drawer, field } = await openDrawer()
  expect(document.activeElement).toBe(field)
  drawer.open = false
  await drawer.updateComplete
  expect(document.activeElement).toBe(opener)
})

test('removing an open modal drawer returns focus to the control that opened it', async () => {
  const { opener, drawer, field } = await openDrawer()
  expect(document.activeElement).toBe(field)
  drawer.remove()
  await tick()
  expect(document.activeElement).toBe(opener)
})

test('closing does not steal focus the user already moved outside the drawer', async () => {
  const { elsewhere, drawer, field } = await openDrawer()
  expect(document.activeElement).toBe(field)
  elsewhere.focus()
  drawer.open = false
  await drawer.updateComplete
  expect(document.activeElement).toBe(elsewhere)
})

test('a non-modal drawer leaves focus where it was', async () => {
  const { opener, drawer } = await openDrawer({ modal: false })
  expect(document.activeElement).toBe(opener)
  drawer.open = false
  await drawer.updateComplete
  expect(document.activeElement).toBe(opener)
})

test('Escape in a modal drawer asks the host to close it', async () => {
  const { drawer, field } = await openDrawer()
  let closed = 0
  drawer.addEventListener('lv-drawer-close', () => { closed += 1 })
  field.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true, composed: true, cancelable: true }))
  expect(closed).toBeGreaterThan(0)
})
