import { fetchEventStatus, redirectToSignIn } from './command-failure'

export type PageStreamFailureKind = 'forbidden' | 'rate-limited' | 'unavailable' | 'network' | 'unexpected'

export type PageStreamFailure = {
  kind: PageStreamFailureKind
  status: number | null
  title: string
  message: string
}

export type PageStreamTransition =
  | { action: 'none' }
  | { action: 'recover' }
  | { action: 'fail', failure: PageStreamFailure }
  | { action: 'sign-in' }

export type PageStreamPresenter = {
  show(failure: PageStreamFailure): void
  signIn(): void
}

export type PageStreamEnvironment = {
  isStream(element: unknown): boolean
  hidden(): boolean
}

type FetchDetail = { type?: string, el?: unknown }

type StreamState = {
  inFlight: number
  sawContent: boolean
  failed: boolean
  retries: number
}

// A reconnect that fails this many times in a row before the page ever loaded
// has outlasted a routine blip (about three seconds of backoff).
const retriesBeforeRecovery = 3

const copy: Record<PageStreamFailureKind, { title: string, message: string }> = {
  forbidden: {
    title: "You can't open this page",
    message: "Your account doesn't have access. Ask an admin for access, or sign in with a different account.",
  },
  'rate-limited': {
    title: 'Too many requests',
    message: 'LeapView is limiting requests from this browser. Wait a moment, then retry.',
  },
  unavailable: {
    title: 'LeapView is unavailable',
    message: "The service can't respond right now. Retry in a moment.",
  },
  network: {
    title: "Can't reach LeapView",
    message: 'Check your connection, then retry.',
  },
  unexpected: {
    title: "This page didn't load",
    message: "LeapView sent a response this page can't use. Retry, and contact your admin if it keeps happening.",
  },
}

/** Status-specific recovery copy for a page stream that failed to open. */
export function pageStreamFailure(status: number | null): PageStreamFailure {
  const kind = status === 403 ? 'forbidden'
    : status === 429 ? 'rate-limited'
      : status !== null && status >= 500 ? 'unavailable'
        : status === null || status === 0 ? 'network'
          : 'unexpected'
  return { kind, status, ...copy[kind] }
}

/**
 * Tracks the lifecycle of each page stream element (the one carrying
 * data-page-stream-recovery-root) and decides when the document-level recovery
 * card must open. Datastar reports a non-2xx response as an error event but
 * reports a 200 that is not an event stream only as a finished request, so a
 * stream that ends before delivering any content counts as a failure too.
 */
export class PageStreamMonitor {
  private readonly streams = new WeakMap<object, StreamState>()

  constructor(private readonly environment: PageStreamEnvironment = browserEnvironment) {}

  observe(event: Event): PageStreamTransition {
    const detail = (event as CustomEvent<FetchDetail>).detail
    const element = detail?.el
    if (typeof element !== 'object' || element === null || !this.environment.isStream(element)) return { action: 'none' }
    let state = this.streams.get(element)
    if (!state) {
      // Datastar opens the page stream while its own module runs, before
      // command.js (listed after it) can listen, so the first event seen here
      // is normally not 'started': the request that produced it is in flight.
      state = { inFlight: detail.type === 'started' ? 0 : 1, sawContent: false, failed: false, retries: 0 }
      this.streams.set(element, state)
    }
    switch (detail.type) {
      case 'started':
        state.inFlight++
        state.sawContent = false
        state.failed = false
        state.retries = 0
        return { action: 'recover' }
      case 'datastar-patch-elements':
      case 'datastar-patch-signals':
        state.sawContent = true
        state.retries = 0
        return { action: 'recover' }
      case 'retrying':
        state.retries++
        if (state.sawContent || state.retries < retriesBeforeRecovery) return { action: 'none' }
        state.failed = true
        return { action: 'fail', failure: pageStreamFailure(null) }
      case 'error': {
        const status = fetchEventStatus(event)
        if (status === 401) return { action: 'sign-in' }
        state.failed = true
        return { action: 'fail', failure: pageStreamFailure(status) }
      }
      case 'retries-failed':
        state.failed = true
        return { action: 'fail', failure: pageStreamFailure(null) }
      case 'finished':
        state.inFlight = Math.max(0, state.inFlight - 1)
        if (state.inFlight > 0 || state.sawContent || state.failed || this.environment.hidden()) return { action: 'none' }
        state.failed = true
        return { action: 'fail', failure: { ...pageStreamFailure(200), status: null } }
      default:
        return { action: 'none' }
    }
  }
}

const browserEnvironment: PageStreamEnvironment = {
  isStream: (element) => typeof (element as Element).hasAttribute === 'function'
    && (element as Element).hasAttribute('data-page-stream-recovery-root'),
  hidden: () => typeof document !== 'undefined' && document.hidden,
}

/** True when the element owns the document's initial /updates stream. */
export function isPageStreamElement(element: unknown): boolean {
  return browserEnvironment.isStream(element)
}

const domPresenter: PageStreamPresenter = {
  show(failure) {
    const card = document.querySelector<HTMLElement>('[data-page-stream-recovery]')
    if (!card) return
    const wasVisible = card.checkVisibility()
    card.dataset.failure = failure.kind
    const title = card.querySelector('[data-page-stream-recovery-title]')
    if (title) title.textContent = failure.title
    const message = card.querySelector('[data-page-stream-recovery-message]')
    if (message) message.textContent = failure.message
    if (wasVisible) return
    // Datastar reveals the card as soon as the signal changes; move focus to
    // the action once it is displayed so keyboard users land on it.
    setTimeout(() => {
      if (card.checkVisibility()) card.querySelector<HTMLElement>('button')?.focus()
    }, 0)
  },
  signIn: redirectToSignIn,
}

const defaultMonitor = new PageStreamMonitor()

/**
 * Evaluates one Datastar fetch event for the page stream and returns the next
 * value of the $pageStreamRecovery signal, so the inline listener on the page
 * root stays a single assignment.
 */
export function pageStreamRecovery(
  event: Event,
  current: boolean,
  monitor: PageStreamMonitor = defaultMonitor,
  presenter: PageStreamPresenter = domPresenter,
): boolean {
  const transition = monitor.observe(event)
  switch (transition.action) {
    case 'recover':
      return false
    case 'fail':
      presenter.show(transition.failure)
      return true
    case 'sign-in':
      presenter.signIn()
      return current === true
    default:
      return current === true
  }
}
