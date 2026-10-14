import type { ReactiveController, ReactiveControllerHost } from 'lit'
import { domainEvents, emitDomainEvent } from '../shared/events'
import { normalizedReferenceQuery, type ChatContextReference, type ChatReferenceSearchDetail } from './reference'

type SearchHost = HTMLElement & ReactiveControllerHost
type SearchResponse = { suggestions: ChatContextReference[]; query: string; requestId: number }
let requestSequence = 0

/** Owns transient lookup state separately from the server's response identity. */
export class ChatReferenceSearchController implements ReactiveController {
  pending = false
  error = ''
  private lastQuery: string | null = null
  private requestId = 0
  private accepted: SearchResponse = { suggestions: [], query: '', requestId: 0 }

  constructor(
    private readonly host: SearchHost,
    private readonly currentQuery: () => string | null,
    private readonly response: () => SearchResponse,
    private readonly limitReached: () => boolean,
  ) { host.addController(this) }

  hostConnected(): void { this.host.addEventListener('lv-chat-reference-search-failed', this.failed) }
  hostDisconnected(): void { this.host.removeEventListener('lv-chat-reference-search-failed', this.failed) }

  hostUpdated(): void {
    if (!this.pending || !this.matches(this.response())) return
    const response = this.response()
    this.accepted = { ...response, suggestions: [...response.suggestions] }
    this.pending = false
    this.error = ''
    this.host.requestUpdate()
  }

  reset(): void {
    this.lastQuery = null
    this.pending = false
    this.error = ''
    this.host.requestUpdate()
  }

  search(query: string | null): void {
    if (query === null || this.limitReached()) { this.reset(); return }
    if (query === this.lastQuery) return
    this.lastQuery = query
    this.pending = true
    this.error = ''
    this.requestId = ++requestSequence
    this.accepted = { suggestions: [], query: '', requestId: 0 }
    this.host.requestUpdate()
    emitDomainEvent<ChatReferenceSearchDetail>(this.host, domainEvents.chatReferenceSearch, {
      query, requestId: this.requestId, owner: this.host,
    })
  }

  readonly retry = (): void => { this.lastQuery = null; this.search(this.currentQuery()) }

  candidates(): ChatContextReference[] {
    const response = this.response()
    if (this.requestId === 0 && response.requestId === 0 && response.query === '') return response.suggestions
    if (this.matches(response)) return response.suggestions
    return this.matches(this.accepted) ? this.accepted.suggestions : []
  }

  private matches(response: SearchResponse): boolean {
    const query = this.currentQuery()
    return query !== null && this.requestId > 0 && response.requestId === this.requestId
      && normalizedReferenceQuery(response.query) === normalizedReferenceQuery(query)
  }

  private readonly failed = (event: Event): void => {
    const detail = (event as CustomEvent<Pick<ChatReferenceSearchDetail, 'query' | 'requestId'>>).detail
    if (!detail || !this.pending || !this.matches({ ...detail, suggestions: [] })) return
    this.pending = false
    this.error = 'Context search failed. Retry or press Escape to dismiss.'
    this.host.requestUpdate()
  }
}
