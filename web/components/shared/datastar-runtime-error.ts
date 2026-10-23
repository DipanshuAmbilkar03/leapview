const hostID = 'lv-datastar-runtime-error'

const styles = `
  :host {
    position: fixed;
    inset: auto var(--base-size-16, 16px) var(--base-size-16, 16px) var(--base-size-16, 16px);
    z-index: var(--z-index-popover, 1000);
    display: flex;
    justify-content: center;
    pointer-events: none;
  }
  .notice {
    box-sizing: border-box;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--base-size-8, 8px) var(--base-size-16, 16px);
    max-width: min(34rem, 100%);
    border: var(--lv-border-width, 1px) solid var(--lv-border-default, #d0d7de);
    border-inline-start: 4px solid var(--lv-fg-danger, #cf222e);
    border-radius: var(--lv-radius-default, 6px);
    background: var(--lv-bg-panel, #fff);
    box-shadow: var(--lv-shadow-floating-lg, 0 8px 24px rgb(31 35 40 / 0.2));
    padding: var(--base-size-12, 12px) var(--base-size-16, 16px);
    color: var(--lv-fg-default, #1f2328);
    font: var(--lv-type-body, 14px/1.5 system-ui, sans-serif);
    pointer-events: auto;
  }
  .copy { flex: 1 1 14rem; min-width: 0; margin: 0; text-wrap: pretty; }
  .copy strong { display: block; font-weight: var(--base-text-weight-semibold, 600); }
  .copy span { color: var(--lv-fg-muted, #59636e); }
  button {
    box-sizing: border-box;
    min-width: 44px;
    min-height: 44px;
    border: var(--lv-border-width, 1px) solid var(--lv-border-default, #d0d7de);
    border-radius: var(--lv-radius-default, 6px);
    background: var(--lv-bg-control, #f6f8fa);
    padding: 0 var(--base-size-16, 16px);
    color: var(--lv-fg-default, #1f2328);
    font: inherit;
    font-weight: var(--base-text-weight-medium, 500);
    cursor: pointer;
  }
  button:hover { background: var(--lv-bg-control-hover, #eaeef2); }
  button:focus-visible { outline: 2px solid var(--lv-line-accent, #0969da); outline-offset: 2px; }
`

/**
 * Tells the person that the page's interactive runtime did not load.
 *
 * Lit routes render their own content, so there is no per-route place that is
 * guaranteed to be on screen when the shared runtime fails. This document-level
 * notice is idempotent, announced as an alert, and offers the one recovery that
 * works for a lost script: reloading the page.
 */
export function showDatastarRuntimeError(doc: Document | undefined = globalThis.document): void {
  if (!doc?.body || doc.getElementById(hostID)) return
  const host = doc.createElement('div')
  host.id = hostID
  host.dataset.state = 'error'
  const root = host.attachShadow({ mode: 'open' })

  const style = doc.createElement('style')
  style.textContent = styles

  const notice = doc.createElement('div')
  notice.className = 'notice'
  notice.setAttribute('role', 'alert')

  const copy = doc.createElement('p')
  copy.className = 'copy'
  const title = doc.createElement('strong')
  title.textContent = 'Part of this page did not load'
  const detail = doc.createElement('span')
  detail.textContent = 'Controls and live data may not respond. Reload to try again.'
  copy.append(title, detail)

  const reload = doc.createElement('button')
  reload.type = 'button'
  reload.textContent = 'Reload page'
  reload.addEventListener('click', () => doc.defaultView?.location.reload())

  notice.append(copy, reload)
  root.append(style, notice)
  doc.body.append(host)
}

export function hideDatastarRuntimeError(doc: Document | undefined = globalThis.document): void {
  doc?.getElementById(hostID)?.remove()
}
