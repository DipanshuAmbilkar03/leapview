import { afterAll, beforeAll, expect, test } from 'bun:test'
import AxeBuilder from '@axe-core/playwright'
import { expect as browserExpect } from '@playwright/test'
import { startAdminPageTestFixture, stopAdminPageTestFixture, type AdminPageTestFixture } from './admin-page.test-fixture'

let fixture: AdminPageTestFixture

beforeAll(async () => {
  fixture = await startAdminPageTestFixture()
})

afterAll(async () => {
  if (fixture) await stopAdminPageTestFixture(fixture)
}, 15_000)

test('appearance theme choices remain within the viewport near the bottom on narrow pages', async () => {
  const context = await fixture.browser.newContext()
  const page = await context.newPage()
  try {
    await page.goto(fixture.baseURL)
    await page.waitForFunction(() => customElements.get('lv-personal-settings'))
    await page.evaluate(async () => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ page: { kind: 'admin', title: 'Appearance', active: 'appearance', headerTitle: 'Appearance', headerDetail: '' }, personalSettings: {
        active: 'appearance',
        profile: { id: 'principal-1', email: 'jacob@example.com', displayName: 'Jacob Nielsen', theme: 'system', identitySource: 'local', canEditDisplayName: true, hasLocalPassword: true },
        security: { localPasswordEnabled: true, sessions: [], authoringSessions: [] },
        tokens: { items: [], permissionOptionsReady: true, capabilities: [] },
      } })
      await (document.querySelector('lv-admin-page') as any).updateComplete
    })
    const trigger = page.locator('lv-personal-settings .theme-trigger')
    for (const width of [1440, 1280, 768, 390, 375]) {
      const height = width === 1280 ? 320 : 900
      await page.setViewportSize({ width, height })
      await page.evaluate(width => { document.body.style.paddingLeft = width >= 640 ? '248px' : '44px' }, width)
      await trigger.scrollIntoViewIfNeeded()
      // Exercise the same low trigger position as the mobile Appearance page.
      await trigger.evaluate((element: HTMLElement) => {
        const current = element.getBoundingClientRect().top
        document.body.style.paddingTop = `${Math.max(0, 760 - current)}px`
      })
      await trigger.click()
      const menu = page.locator('lv-personal-settings .theme-menu')
      await menu.waitFor({ state: 'visible' })
      const bounds = await menu.boundingBox()
      expect(bounds!.x).toBeGreaterThanOrEqual(0)
      expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width)
      expect(bounds!.y).toBeGreaterThanOrEqual(0)
      expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(height)
      const accessibility = await new AxeBuilder({ page }).withRules(['scrollable-region-focusable']).analyze()
      expect(accessibility.violations).toEqual([])
      if (height === 320) {
        expect(await menu.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(true)
        await menu.focus()
        await page.keyboard.press('PageDown')
        await browserExpect.poll(() => menu.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
      }
      await page.keyboard.press('End')
      const focused = await page.locator('lv-personal-settings').evaluate((personal: any) => {
        const option = personal.shadowRoot.activeElement as HTMLElement
        const menu = personal.shadowRoot.querySelector('.theme-menu').getBoundingClientRect()
        const bounds = option.getBoundingClientRect()
        return { theme: option.dataset.theme, visible: bounds.top >= menu.top && bounds.bottom <= menu.bottom }
      })
      expect(focused.theme).toBe('dark_tritanopia')
      expect(focused.visible).toBe(true)
      if (height === 320) {
        await page.locator('lv-personal-settings').evaluate((personal: HTMLElement) => personal.addEventListener('lv-personal-theme-command', (event) => { personal.dataset.chosenTheme = (event as CustomEvent).detail.theme }, { once: true }))
        await page.keyboard.press('Enter')
        await browserExpect(page.locator('lv-personal-settings')).toHaveAttribute('data-chosen-theme', 'dark_tritanopia')
        await page.locator('lv-personal-settings').evaluate(async (personal: any) => {
          const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
          const response = { personalSettings: { profile: { theme: 'dark_tritanopia' } } }
          mergePatch(response)
          document.dispatchEvent(new CustomEvent('datastar-fetch', { detail: { el: personal, type: 'datastar-patch-signals', argsRaw: { signals: JSON.stringify(response) } } }))
          document.dispatchEvent(new CustomEvent('datastar-fetch', { detail: { el: personal, type: 'finished' } }))
          await personal.updateComplete
        })
        await browserExpect(trigger).toHaveAttribute('aria-expanded', 'false')
        await browserExpect(trigger).toBeEnabled()
        await trigger.press('ArrowDown')
        await menu.waitFor({ state: 'visible' })
      }
      await page.keyboard.press('Escape')
      expect(await trigger.getAttribute('aria-expanded')).toBe('false')
      expect(await trigger.evaluate((element: HTMLElement) => element.getRootNode() instanceof ShadowRoot && (element.getRootNode() as ShadowRoot).activeElement === element)).toBe(true)
      await trigger.click()
      await page.mouse.click(5, 5)
      expect(await trigger.getAttribute('aria-expanded')).toBe('false')
      await page.evaluate(() => { document.body.style.paddingTop = '0px' })
    }
  } finally { await context.close() }
}, 20_000)
