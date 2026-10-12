import type { DashboardChatComponent } from './dashboard-preview-contract'

// Owns same-document preview navigation. Chat renders the selected surface;
// this controller retains the conversation URL and history entry independently.
export function setChatPreviewLocation(mode: 'builder' | 'dashboard' | null, replace = false): void {
  const url = new URL(window.location.href)
  if (mode) url.searchParams.set('preview', mode)
  else url.searchParams.delete('preview')
  if (replace) window.history.replaceState(window.history.state, '', url)
  else window.history.pushState(window.history.state, '', url)
}

export function clearChatDashboardLocation(): void {
  const url = new URL(window.location.href)
  url.searchParams.delete('dashboard')
  window.history.replaceState(window.history.state, '', url)
}

export function rememberChatDashboardLocation(href: string, links: Record<string, DashboardChatComponent[]>, signature: string): void {
  const url = new URL(window.location.href)
  url.searchParams.set('dashboard', href)
  // Only identities belong in history; the server owns the current draft.
  window.history.replaceState({ ...window.history.state, chatDashboard: { href, links, signature } }, '', url)
}

// A handoff URL carries presentation only; the conversation stream still owns
// intent, run state, and authorization. Invalid dashboard destinations cannot
// open an embedded builder.
export function chatBuilderLocation(url = new URL(window.location.href)): boolean {
  return url.searchParams.get('preview') === 'builder'
    && (!url.searchParams.has('dashboard') || Boolean(validChatBuilderHref(url.searchParams.get('dashboard'), url)))
}

export function validChatBuilderHref(value: string | null, base = new URL(window.location.href)): string {
  if (!value) return ''
  try {
    const href = new URL(value, base)
    if (href.origin !== base.origin || !/^\/dashboards\/[^/]+\/edit$/.test(href.pathname)) return ''
    const result = new URL(href.pathname, base.origin)
    result.searchParams.set('embed', 'chat')
    if (href.searchParams.has('draft')) result.searchParams.set('draft', href.searchParams.get('draft')!)
    if (href.searchParams.has('page')) result.searchParams.set('page', href.searchParams.get('page')!)
    return result.pathname + result.search
  } catch { return '' }
}
