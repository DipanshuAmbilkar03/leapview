import { expect, test } from 'bun:test'
import { startAdminPageTestFixture, stopAdminPageTestFixture } from './admin-page.test-fixture'

// The fixture lives inside the test, not in file-level hooks. This file runs in
// one bun process with suites that register their own file-level hooks, and in
// CI the shared hook-managed server was intermittently gone (ERR_CONNECTION_REFUSED)
// by the time the only test here navigated to it.
test('archived chat menu enters on the first action from its trigger', async () => {
  const fixture = await startAdminPageTestFixture()
  const page = await fixture.browser.newPage({ viewport: { width: 1100, height: 760 } })
  try {
    await page.goto(fixture.baseURL)
    await page.waitForFunction(() => customElements.get('lv-admin-page') && customElements.get('lv-archived-chats'))
    const state = await page.evaluate(async () => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({
        page: {
          kind: 'admin', title: 'Archived chats', active: 'archived-chats', headerTitle: 'Archived chats', headerDetail: 'Manage archived conversations.',
        },
        chatManagement: {
          action: '', conversationId: '', completedRequestId: '',
          archivedConversations: [{ id: 'archived-1', title: 'Last quarter', status: 'archived', updatedAt: '2026-09-17T00:45:00Z' }],
        },
      })
      const admin = document.querySelector('lv-admin-page') as any
      await admin.updateComplete
      const archived = (admin.shadowRoot as ShadowRoot).querySelector('lv-archived-chats') as any
      await archived.updateComplete
      const root = archived.shadowRoot as ShadowRoot
      const historyActionButtons = Array.from(root.querySelectorAll<HTMLElement>('.history-actions button'))
      const historyActions = historyActionButtons.map((item) => item.textContent?.trim())
      const actionRects = historyActionButtons.map((item) => item.getBoundingClientRect())
      const searchTop = (root.querySelector('.toolbar') as HTMLElement).getBoundingClientRect().top
      const trigger = root.querySelector('summary') as HTMLElement
      trigger.focus()
      trigger.click()
      await archived.updateComplete
      trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, composed: true }))
      return {
        historyActions,
        actionsShareRow: actionRects.length === 2 && Math.abs(actionRects[0].top - actionRects[1].top) < 1,
        searchFollowsActions: actionRects.every((rect) => rect.bottom < searchTop),
        open: (root.querySelector('details') as HTMLDetailsElement).open,
        focusedAction: (root.activeElement as HTMLElement | null)?.textContent?.trim(),
      }
    })
    expect(state).toEqual({
      historyActions: ['Archive all', 'Delete all'],
      actionsShareRow: true,
      searchFollowsActions: true,
      open: true,
      focusedAction: 'Select',
    })
  } finally {
    await page.close()
    await stopAdminPageTestFixture(fixture)
  }
}, 30_000)
