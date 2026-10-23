export type CopyOptions = {
  /** Visible element holding the value; its text is selected when every copy path fails so Ctrl+C still works. */
  selectOnFailure?: Element | null
}

/**
 * Copies text without ever throwing. The async Clipboard API is tried first, then the legacy
 * execCommand path (plain-http origins, denied permissions); when both fail the visible source
 * is selected so the user can copy it by hand. Resolves true only when the clipboard was written.
 */
export async function copyText(value: string, options: CopyOptions = {}): Promise<boolean> {
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
      return true
    }
  } catch {
    // Permission denied or insecure context: fall through to the legacy path.
  }
  if (legacyCopy(value)) return true
  selectContents(options.selectOnFailure)
  return false
}

function legacyCopy(value: string): boolean {
  if (typeof document === 'undefined' || typeof document.execCommand !== 'function') return false
  const active = document.activeElement as HTMLElement | null
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.setAttribute('readonly', '')
  textarea.setAttribute('aria-hidden', 'true')
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  try {
    document.body.append(textarea)
    textarea.select()
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    textarea.remove()
    active?.focus?.({ preventScroll: true })
  }
}

function selectContents(element: Element | null | undefined): void {
  if (!element || typeof window === 'undefined') return
  try {
    window.getSelection()?.selectAllChildren(element)
  } catch {
    // Selection is best effort; the failure toast still tells the user what happened.
  }
}
