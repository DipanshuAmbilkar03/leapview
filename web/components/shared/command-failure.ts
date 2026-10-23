export type BrowserCommandFailureKind =
  | 'session-expired'
  | 'forbidden'
  | 'not-found'
  | 'conflict'
  | 'rate-limited'
  | 'unavailable'
  | 'invalid-draft'
  | 'validation'
  | 'network'
  | 'unknown'

export type BrowserCommandFailure = {
  kind: BrowserCommandFailureKind
  status: number | null
  retryable: boolean
  message: string
}

type DatastarFetchDetail = {
  type?: string
  argsRaw?: { status?: string | number }
  el?: Element
}

/**
 * Datastar's fetch lifecycle is document-global. A command component may only
 * consume events from the element whose data-on attribute initiated its
 * request. Nested components use their shadow host as that command owner.
 */
export function ownsBrowserCommandFetch(element: Element, event: Event): boolean {
  const owner = (event as CustomEvent<DatastarFetchDetail>).detail?.el
  if (!(owner instanceof Element)) return false
  let candidate: Element | null = element
  while (candidate) {
    if (owner === candidate) return true
    const root = candidate.getRootNode()
    candidate = root instanceof ShadowRoot ? root.host : null
  }
  return false
}

/** Where every surface sends a browser whose session ended mid-page. */
export const sessionExpiredLocation = '/login?error=session_expired'

/** Leaves the stale page for sign-in; a page already on the login route stays put. */
export function redirectToSignIn(): void {
  if (!window.location.pathname.startsWith('/login')) window.location.replace(sessionExpiredLocation)
}

const claimedFailures = new WeakSet<Event>()

/** HTTP status carried by a Datastar fetch event, or null when it has none. */
export function fetchEventStatus(event: Event): number | null {
  return normalizeStatus((event as CustomEvent<DatastarFetchDetail>).detail?.argsRaw?.status)
}

/**
 * True once a component has surfaced this failure itself. The global command
 * failure guard only notifies for failures that no component claimed.
 */
export function isBrowserCommandFailureClaimed(event: Event): boolean {
  return claimedFailures.has(event)
}

/**
 * Normalizes Datastar's terminal fetch events into LeapView's browser failure
 * contract. Components still own the domain-specific action text, but they do
 * not need to reinterpret transport statuses or retry exhaustion independently.
 * Returning a failure claims the event: the caller is presenting it.
 */
export function browserCommandFailure(event: Event, action = 'This action'): BrowserCommandFailure | null {
  const result = classifyBrowserCommandFailure(event, action)
  if (result) claimedFailures.add(event)
  return result
}

/** Classifies a terminal fetch event without claiming it. */
export function classifyBrowserCommandFailure(event: Event, action = 'This action'): BrowserCommandFailure | null {
  const detail = (event as CustomEvent<DatastarFetchDetail>).detail
  if (detail?.type !== 'error' && detail?.type !== 'retries-failed') return null

  const status = normalizeStatus(detail.argsRaw?.status)
  const prefix = action.trim() || 'This action'
  if (status === 401) return failure('session-expired', status, false, `${prefix} could not continue because your session expired. Sign in again, then retry.`)
  if (status === 403) return failure('forbidden', status, false, `${prefix} is not permitted for your account.`)
  if (status === 404) return failure('not-found', status, false, `${prefix} could not continue because the resource changed or is no longer available. Reload the page.`)
  if (status === 409 || status === 412) return failure('conflict', status, false, `${prefix} conflicted with a newer change. Reload the latest state before retrying.`)
  if (status === 429) return failure('rate-limited', status, true, `${prefix} was temporarily rate limited. Wait a moment, then retry.`)
  if (status === 400) return failure('invalid-draft', status, false, `${prefix} could not be completed because the draft is invalid or incomplete. Review the draft inputs and try again.`)
  if (status === 422) return failure('validation', status, false, `${prefix} was rejected by validation. Fix the highlighted draft issues, then retry.`)
  if (status !== null && status >= 500) return failure('unavailable', status, true, `${prefix} could not be completed because the service is temporarily unavailable. Your previous state was kept; retry when ready.`)
  if (detail.type === 'retries-failed' || status === null || status === 0) return failure('network', status, true, `${prefix} could not reach the server. Your previous state was kept; check the connection and retry.`)
  return failure('unknown', status, false, `${prefix} could not be completed. Your previous state was kept; review the page and retry.`)
}

function normalizeStatus(value: string | number | undefined): number | null {
  if (typeof value === 'number' && Number.isInteger(value) && value >= 0) return value
  if (typeof value !== 'string' || value.trim() === '') return null
  const parsed = Number(value)
  return Number.isInteger(parsed) && parsed >= 0 ? parsed : null
}

function failure(kind: BrowserCommandFailureKind, status: number | null, retryable: boolean, message: string): BrowserCommandFailure {
  return { kind, status, retryable, message }
}
