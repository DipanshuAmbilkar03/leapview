import {
  classifyBrowserCommandFailure,
  isBrowserCommandFailureClaimed,
  redirectToSignIn,
  type BrowserCommandFailure,
  type BrowserCommandFailureKind,
} from './command-failure'
import { isPageStreamElement } from './page-stream-recovery'

export type CommandFailureNotice = {
  message: string
  /** The page no longer matches the server; offer a reload with the message. */
  reload: boolean
}

export type CommandFailureEffects = {
  isPageStream(element: unknown): boolean
  signIn(): void
  notify(notice: CommandFailureNotice): void
  defer(run: () => void): void
  now(): number
}

const notices: Record<BrowserCommandFailureKind, CommandFailureNotice> = {
  'session-expired': { message: 'Your session expired. Sign in again to continue.', reload: false },
  forbidden: { message: "You don't have permission to do that. Ask an admin if you need access.", reload: false },
  'not-found': { message: 'That item changed or was removed. Reload the page to see the latest.', reload: true },
  conflict: { message: 'Someone changed this first. Reload to see the latest, then try again.', reload: true },
  'rate-limited': { message: 'Too many requests. Wait a moment, then try again.', reload: false },
  unavailable: { message: "LeapView is unavailable, so your change wasn't applied. Try again in a moment.", reload: false },
  'invalid-draft': { message: "That change wasn't accepted. Check your input and try again.", reload: false },
  validation: { message: "That change didn't pass validation. Review it and try again.", reload: false },
  network: { message: "Can't reach LeapView, so your change wasn't applied. Check your connection and try again.", reload: false },
  unknown: { message: "Something went wrong and your change wasn't applied. Try again.", reload: false },
}

const duplicateWindowMs = 3_000

export function commandFailureNotice(failure: BrowserCommandFailure): CommandFailureNotice {
  return notices[failure.kind]
}

/**
 * Document-level safety net for Datastar commands. Components that present
 * their own failures claim the event through browserCommandFailure; anything
 * left unclaimed would otherwise fail silently, so it is announced here. An
 * expired session always ends at the sign-in page, whoever owns the command.
 * The page stream has its own recovery card and is left alone.
 */
export class CommandFailureGuard {
  private lastMessage = ''
  private lastShownAt = Number.NEGATIVE_INFINITY

  constructor(private readonly effects: CommandFailureEffects) {}

  handle(event: Event): void {
    const element = (event as CustomEvent<{ el?: unknown }>).detail?.el
    if (this.effects.isPageStream(element)) return
    const failure = classifyBrowserCommandFailure(event)
    if (!failure) return
    if (failure.kind === 'session-expired') {
      this.effects.signIn()
      return
    }
    // Component listeners run after this one in the same dispatch; give them
    // the chance to claim the event before announcing it.
    this.effects.defer(() => {
      if (isBrowserCommandFailureClaimed(event)) return
      const notice = commandFailureNotice(failure)
      const now = this.effects.now()
      if (notice.message === this.lastMessage && now - this.lastShownAt < duplicateWindowMs) return
      this.lastMessage = notice.message
      this.lastShownAt = now
      this.effects.notify(notice)
    })
  }
}

async function showNotice(notice: CommandFailureNotice): Promise<void> {
  const { showToast } = await import('./toast')
  showToast({
    message: notice.message,
    tone: 'error',
    // An error stays long enough to read and pauses under pointer and focus.
    durationMs: 10_000,
    ...(notice.reload ? { action: { label: 'Reload', onClick: () => window.location.reload() } } : {}),
  })
}

let installed = false

/** Installs the document-level guard once per page. */
export function installCommandFailureGuard(): void {
  if (installed || typeof document === 'undefined') return
  installed = true
  const guard = new CommandFailureGuard({
    isPageStream: isPageStreamElement,
    signIn: redirectToSignIn,
    notify: (notice) => { void showNotice(notice).catch((error: unknown) => console.error('command failure notice', error)) },
    defer: (run) => { setTimeout(run, 0) },
    now: () => Date.now(),
  })
  document.addEventListener('datastar-fetch', (event) => guard.handle(event))
}
