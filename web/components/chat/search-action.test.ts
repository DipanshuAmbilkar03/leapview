import { expect, test } from 'bun:test'
import type { ChatTranscriptItemSignal } from '../../generated/signals'
import { searchActionHref } from './search-action'

test('search handoff retains exact kinds, domain, cursor and limit', () => {
  const item = { kind: 'tool', name: 'catalog_search', status: 'complete', argumentsJson: JSON.stringify({ query: 'sales', kinds: ['model', 'dashboard'], domain: 'finance', cursor: 'page/2', limit: 7 }) } as ChatTranscriptItemSignal
  const url = new URL(searchActionHref(item)!, 'https://example.test')
  expect(url.searchParams.getAll('kind')).toEqual(['model', 'dashboard'])
  expect(Object.fromEntries(['q', 'domain', 'cursor', 'limit'].map(key => [key, url.searchParams.get(key)]))).toEqual({ q: 'sales', domain: 'finance', cursor: 'page/2', limit: '7' })
})
