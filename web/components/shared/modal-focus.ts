const focusableSelector = [
  'a[href]:not([tabindex="-1"])',
  'button:not([disabled]):not([tabindex="-1"])',
  'input:not([disabled]):not([tabindex="-1"])',
  'select:not([disabled]):not([tabindex="-1"])',
  'textarea:not([disabled]):not([tabindex="-1"])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

export function composedFocusableElements(root: ParentNode): HTMLElement[] {
  const focusable: HTMLElement[] = []
  const visit = (element: Element): void => {
    const style = getComputedStyle(element)
    if (element.matches('[hidden], [inert]') || style.display === 'none') return
    if (element instanceof HTMLElement && element.matches(focusableSelector)
      && element.tabIndex >= 0 && !element.matches(':disabled')
      && style.visibility === 'visible' && element.getClientRects().length > 0) {
      focusable.push(element)
    }
    // Traverse rendered slots and open shadow roots in place, rather than
    // grouping light-DOM controls separately from their nested controls.
    const children = element instanceof HTMLSlotElement
      ? element.assignedElements({ flatten: true })
      : Array.from((element.shadowRoot ?? element).children)
    children.forEach(visit)
  }
  Array.from(root.children).forEach(visit)
  return focusable
}

/** Wrap only at rendered composed-tree boundaries; nested controls keep their keys. */
export function wrapModalTab(event: KeyboardEvent, root: ParentNode): void {
  if (event.key !== 'Tab' || event.defaultPrevented) return
  const focusable = composedFocusableElements(root)
  if (focusable.length === 0) return
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  const active = event.composedPath()[0]
  if (event.shiftKey && active === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && active === last) {
    event.preventDefault()
    first.focus()
  }
}

/** The focused element, descending through open shadow roots. */
export function deepActiveElement(doc: Document = document): HTMLElement | null {
  let active: Element | null = doc.activeElement
  while (active?.shadowRoot?.activeElement) active = active.shadowRoot.activeElement
  return active instanceof HTMLElement && active !== doc.body ? active : null
}

/**
 * Initial focus for a modal surface: the first control in rendered order, with
 * the dismiss control only as a fallback so Enter never closes the surface by
 * accident and a password field or first input is ready to type into.
 */
export function focusInitialElement(root: ParentNode, dismiss?: Element | null): boolean {
  const focusable = composedFocusableElements(root)
  const initial = focusable.find(element => element !== dismiss) ?? focusable[0]
  initial?.focus()
  return Boolean(initial)
}

/**
 * Return focus to the control that opened a modal surface. Skipped when the
 * user has already moved focus somewhere deliberate, outside the surface.
 */
export function restoreFocus(target: HTMLElement | null | undefined, surface?: Node | null, doc: Document = document): void {
  if (!target?.isConnected) return
  const active = deepActiveElement(doc)
  if (active && active !== target && !(surface && composedContains(surface, active))) return
  target.focus({ preventScroll: true })
}

function composedContains(ancestor: Node, node: Node): boolean {
  for (let current: Node | null = node; current; current = current.parentNode ?? (current as ShadowRoot).host ?? null) {
    if (current === ancestor) return true
  }
  return false
}
