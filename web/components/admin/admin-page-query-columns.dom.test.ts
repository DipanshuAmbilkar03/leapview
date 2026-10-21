import { afterAll, beforeAll, expect, test } from 'bun:test'
import { expect as browserExpect } from '@playwright/test'
import { startAdminPageTestFixture, stopAdminPageTestFixture, type AdminPageTestFixture } from './admin-page.test-fixture'
import { queryAuditFixturePage } from './admin-page-query.test-fixture'

let fixture: AdminPageTestFixture

beforeAll(async () => {
  fixture = await startAdminPageTestFixture()
})

afterAll(async () => {
  if (fixture) await stopAdminPageTestFixture(fixture)
}, 15_000)

for (const theme of ['light', 'dark'] as const) {
  test(`query history Columns menu dismisses outside its shadow root and closes on disconnection (${theme})`, async () => {
    const page = await fixture.browser.newPage({ viewport: { width: 1440, height: 900 }, colorScheme: theme })
    try {
      await page.goto(fixture.baseURL)
      await page.waitForFunction(() => customElements.get('lv-admin-page') && customElements.get('lv-record-table'))
      await page.evaluate(async ({ fixture, theme }) => {
        // Use the shipped theme tokens rather than this fixture's light-only overrides.
        document.head.querySelectorAll('style').forEach(style => style.remove())
        document.documentElement.dataset.colorMode = theme
        document.documentElement.dataset.lightTheme = 'light'
        document.documentElement.dataset.darkTheme = 'dark'
        const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
        mergePatch({ page: fixture, adminQueryHistory: fixture.queryHistory, adminQueryDetail: { eventId: '', loading: false, error: '' } })
        const element = document.querySelector('lv-admin-page') as any
        await element.updateComplete
      }, { fixture: queryAuditFixturePage(), theme })
      await page.addStyleTag({ path: 'static/app.css' })
      const table = page.locator('lv-admin-page lv-record-table')
      const selector = table.locator('.record-table-column-selector')
      const trigger = selector.getByRole('button', { name: 'Columns', exact: true })
      await trigger.click()
      await browserExpect(trigger).toHaveAttribute('aria-expanded', 'true')
      // With only two rows, the menu extends below the panel's footer.
      // Its last option remains reachable by scrolling within the bounded menu.
      await selector.getByRole('checkbox', { name: 'Error', exact: true }).scrollIntoViewIfNeeded()
      await browserExpect(selector.getByRole('checkbox', { name: 'Error', exact: true })).toBeInViewport({ timeout: 1_000 })
      await selector.getByRole('checkbox', { name: 'Error', exact: true }).check()
      await browserExpect(table.getByRole('button', { name: 'Sort by Error', exact: true })).toHaveCount(1)

      // Inside controls are retargeted to lv-admin-page at document level.
      // They must keep the menu open and still update column visibility.
      await selector.getByRole('checkbox', { name: 'Runtime', exact: true }).uncheck()
      await browserExpect(trigger).toHaveAttribute('aria-expanded', 'true')
      await browserExpect(table.getByRole('button', { name: 'Sort by Runtime', exact: true })).toHaveCount(0)
      await page.locator('lv-admin-page #query-filter-search').click()
      await browserExpect(trigger).toHaveAttribute('aria-expanded', 'false')
      await browserExpect(page.locator('lv-admin-page #query-filter-search')).toBeFocused()

      await trigger.click()
      await browserExpect(trigger).toHaveAttribute('aria-expanded', 'true')
      await selector.getByRole('checkbox', { name: 'Runtime', exact: true }).focus()
      await page.keyboard.press('Escape')
      await browserExpect(trigger).toHaveAttribute('aria-expanded', 'false')
      await browserExpect(trigger).toBeFocused()
      await trigger.click()
      await browserExpect(trigger).toHaveAttribute('aria-expanded', 'true')
      await trigger.click()
      await browserExpect(trigger).toHaveAttribute('aria-expanded', 'false')

      await trigger.click()
      const detachedStillOpen = await table.evaluate(async (element: any) => {
        const parent = element.parentElement!
        element.remove()
        document.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, composed: true }))
        document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, composed: true }))
        await element.updateComplete
        const stillOpen = element.querySelector('.record-table-column-menu').matches(':popover-open')
        parent.append(element)
        await element.updateComplete
        return stillOpen
      })
      expect(detachedStillOpen).toBe(false)
      // Reconnection can open a fresh menu without retaining the old overlay.
      await trigger.click()
      await page.locator('lv-admin-page #query-filter-search').click()
      await browserExpect(trigger).toHaveAttribute('aria-expanded', 'false')
    } finally {
      await page.close()
    }
  })
}

for (const width of [390, 768, 1440]) {
  test(`query history keeps controls clear of data and selected columns reachable at ${width}px`, async () => {
    const page = await fixture.browser.newPage({ viewport: { width, height: 600 } })
    try {
      await page.goto(fixture.baseURL)
      await page.waitForFunction(() => customElements.get('lv-admin-page') && customElements.get('lv-record-table'))
      await page.evaluate(async (data) => {
        const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
        data.queryHistory.table.rows[0].principal_id = '00000000-0000-7000-8000-000000000001'
        mergePatch({ page: data, adminQueryHistory: data.queryHistory, adminQueryDetail: { eventId: '', loading: false, error: '' } })
        await (document.querySelector('lv-admin-page') as any).updateComplete
      }, queryAuditFixturePage())
      await page.addStyleTag({ path: 'static/app.css' })
      const table = page.locator('lv-admin-page lv-record-table')
      const trigger = table.getByRole('button', { name: 'Columns', exact: true })
      await trigger.scrollIntoViewIfNeeded()
      const triggerBox = (await trigger.boundingBox())!
      const headerBox = (await table.locator('thead').boundingBox())!
      expect(triggerBox.y + triggerBox.height).toBeLessThanOrEqual(headerBox.y)
      await browserExpect(table.getByRole('button', { name: 'Sort by Runtime', exact: true })).toBeVisible()
      const code = table.locator('.record-code', { hasText: '00000000-0000-7000-8000-000000000001' })
      const idFitsCell = await code.evaluate(element => element.getBoundingClientRect().right <= element.closest('td')!.getBoundingClientRect().right)
      expect(idFitsCell).toBe(true)
      await browserExpect(code).toHaveAttribute('title', '00000000-0000-7000-8000-000000000001')
      await trigger.click()
      const menu = table.locator('.record-table-column-menu')
      await menu.getByRole('checkbox', { name: 'Error', exact: true }).scrollIntoViewIfNeeded()
      const menuBox = (await menu.boundingBox())!
      expect(menuBox.x).toBeGreaterThanOrEqual(0)
      expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(width)
      expect(menuBox.y).toBeGreaterThanOrEqual(0)
      expect(menuBox.y + menuBox.height).toBeLessThanOrEqual(600)
      await menu.getByRole('checkbox', { name: 'Error', exact: true }).check()
      await page.keyboard.press('Escape')
      await browserExpect(trigger).toBeFocused()
      await table.locator('.record-table-wrap').evaluate(e => { e.scrollLeft = e.scrollWidth })
      await browserExpect(table.getByRole('button', { name: 'Sort by Error', exact: true })).toBeVisible()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    } finally { await page.close() }
  }, 20_000)
}
