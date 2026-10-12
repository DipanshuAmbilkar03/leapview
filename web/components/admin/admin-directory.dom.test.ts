import { afterAll, beforeAll, test } from 'bun:test'
import { expect as browserExpect } from '@playwright/test'
import { startAdminPageTestFixture, stopAdminPageTestFixture, type AdminPageTestFixture } from './admin-page.test-fixture'

let fixture: AdminPageTestFixture
beforeAll(async () => { fixture = await startAdminPageTestFixture() })
afterAll(async () => { if (fixture) await stopAdminPageTestFixture(fixture) }, 15_000)

test('blocked users keep their blocked status in the directory', async () => {
  const page = await fixture.browser.newPage()
  try {
    await page.goto(fixture.baseURL)
    await page.waitForFunction(() => customElements.get('lv-admin-page'))
    await page.evaluate(async () => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ page: { kind: 'admin', title: 'Users', active: 'principals', headerTitle: 'Users', directoryList: {
        searchPlaceholder: 'Search users', filterLabel: 'Filter users', items: [
          { id: 'blocked', name: 'Blocked user', username: 'blocked', email: 'blocked@example.com', href: '/admin/principals/blocked', status: 'blocked', groupCount: 0, joinedAt: '', lastSeenAt: '2026-10-01T00:00:00Z' },
        ],
      } } })
    })
    await browserExpect(page.getByRole('cell', { name: 'Blocked', exact: true })).toBeVisible()
    await browserExpect(page.getByRole('cell', { name: 'Active', exact: true })).toHaveCount(0)
  } finally { await page.close() }
})
