import { expect, test } from 'bun:test'
import { JSDOM } from 'jsdom'
import { hideDatastarRuntimeError, showDatastarRuntimeError } from './datastar-runtime-error'

function page(): Document {
  return new JSDOM('<!doctype html><body><main>Report</main></body>', { url: 'https://leapview.test/' }).window.document
}

const notice = (doc: Document) => doc.getElementById('lv-datastar-runtime-error')

test('the runtime failure notice is an alert with a native reload button', () => {
  const doc = page()
  showDatastarRuntimeError(doc)

  const host = notice(doc)!
  expect(host).not.toBeNull()
  const alert = host.shadowRoot!.querySelector('[role="alert"]')!
  expect(alert.textContent).toContain('Part of this page did not load')
  expect(alert.textContent).toContain('Reload to try again')
  const button = alert.querySelector('button')!
  expect(button.type).toBe('button')
  expect(button.textContent).toBe('Reload page')
})

test('the runtime failure notice is idempotent and removable', () => {
  const doc = page()
  showDatastarRuntimeError(doc)
  showDatastarRuntimeError(doc)
  expect(doc.querySelectorAll('#lv-datastar-runtime-error')).toHaveLength(1)
  hideDatastarRuntimeError(doc)
  expect(notice(doc)).toBeNull()
  hideDatastarRuntimeError(doc)
})

test('the runtime failure notice keeps a visible focus style and a 44px target', () => {
  const doc = page()
  showDatastarRuntimeError(doc)
  const css = notice(doc)!.shadowRoot!.querySelector('style')!.textContent!
  expect(css).toContain('button:focus-visible')
  expect(css).toContain('min-height: 44px')
})
