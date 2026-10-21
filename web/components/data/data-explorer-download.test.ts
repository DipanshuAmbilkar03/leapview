import { expect, test } from 'bun:test'
import { downloadExploration } from './data-explorer-saved'

for (const format of ['csv', 'parquet']) test(`oversized ${format} exports stay in Explorer with actionable recovery`, async () => {
  const original = globalThis.fetch
  const statuses: string[] = []
  globalThis.fetch = Object.assign(async () => new Response('too large', { status: 413 }), { preconnect: () => {} })
  try {
    await downloadExploration(`/explore/export?format=${format}`, { onShareStatus: message => statuses.push(message) })
    expect(statuses.at(-1)).toContain('Open Filters')
    expect(statuses.at(-1)).toContain('10,000 rows or 32 MiB')
  } finally { globalThis.fetch = original }
})

test('HTML sign-in responses are not saved as exploration data', async () => {
  const original = globalThis.fetch
  const statuses: string[] = []
  globalThis.fetch = Object.assign(async () => new Response('<html>Sign in</html>', { headers: { 'Content-Type': 'text/html' } }), { preconnect: () => {} })
  try {
    await downloadExploration('/explore/export?format=csv', { onShareStatus: message => statuses.push(message) })
    expect(statuses.at(-1)).toContain('Sign in again')
  } finally { globalThis.fetch = original }
})
