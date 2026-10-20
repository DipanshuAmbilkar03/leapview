import { expect, test } from 'bun:test'
import type { Page } from '@playwright/test'
import { chatPageBrowserFixture, postReadyDashboardProjection, testDocument } from './chat-page-browser.test-fixture'

const fixture = chatPageBrowserFixture()

async function paintedBuilderStates(page: Page) {
  return page.evaluate(async () => {
    const frames: Array<{ builder: boolean; generation: boolean; titlebar: boolean }> = []
    for (let i = 0; i < 4; i++) {
      await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
      const root = document.querySelector('lv-chat-page')!.shadowRoot!
      const visible = (selector: string) => {
        const element = root.querySelector(selector)
        return Boolean(element && element.getBoundingClientRect().width && element.getBoundingClientRect().height
          && getComputedStyle(element).visibility !== 'hidden')
      }
      frames.push({ builder: visible('.builder-stage'), generation: visible('lv-dashboard-generation'), titlebar: visible('.conversation-titlebar') })
    }
    return frames
  })
}

async function screenshot(page: Page, name: string) {
  if (process.env.LEAPVIEW_CHAT_SCREENSHOT_DIR) {
    await page.screenshot({ path: `${process.env.LEAPVIEW_CHAT_SCREENSHOT_DIR}/${name}.png`, fullPage: true })
  }
}

for (const viewport of [{ width: 1440, height: 900 }, { width: 390, height: 820 }]) test(`cold builder handoff keeps its layout before bootstrap and on the first accepted run (${viewport.width}px)`, async () => {
  const page = await fixture.browser.newPage({ viewport })
  try {
    await page.goto(`${fixture.baseURL}/unhydrated?preview=builder`)
    await page.waitForFunction(() => Boolean(document.querySelector('lv-chat-page')?.shadowRoot?.querySelector('.main')))
    await screenshot(page, `cold-builder-before-bootstrap-${viewport.width}`)
    expect(await paintedBuilderStates(page)).toEqual(Array(4).fill({ builder: true, generation: true, titlebar: false }))
    expect(await page.getByText('Loading conversation…', { exact: true }).isVisible()).toBe(true)
    expect(await page.getByText('Agent unavailable', { exact: true }).count()).toBe(0)
    await page.evaluate(async () => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev' as string)
      mergePatch({
        page: { kind: 'chat', view: 'conversation', title: 'Chats' },
        agent: { activeConversationId: 'c3', transcript: [{ id: 'user', kind: 'user', text: 'Build a sales dashboard', runId: 'build-run' }],
          status: { enabled: true, running: true, runId: 'build-run' }, composer: { value: '', disabled: false } },
      })
    })
    expect(await paintedBuilderStates(page)).toEqual(Array(4).fill({ builder: true, generation: true, titlebar: false }))
    expect(await page.locator('lv-chat-page').evaluate((element: any) => element.generationPrompt)).toBe('Build a sales dashboard')
    await screenshot(page, `cold-builder-after-bootstrap-${viewport.width}`)
  } finally { await page.close() }
})

test('new dashboard submit carries the builder layout into its cold conversation destination', async () => {
  const page = await fixture.browser.newPage({ viewport: { width: 1440, height: 900 } })
  let releaseAcceptance = () => {}
  const acceptance = new Promise<void>(resolve => { releaseAcceptance = resolve })
  try {
    await page.route('**/chats/turns', async route => {
      await acceptance
      await route.fulfill({ contentType: 'text/event-stream', body: 'event: datastar-patch-signals\ndata: signals {"agent":{"activeConversationId":"c3"}}\n\n' })
    })
    await page.route('**/chats/c3**', route => route.fulfill({ contentType: 'text/html', body: testDocument('conversation', 'active', true, false) }))
    await page.goto(`${fixture.baseURL}/new`)
    const textarea = page.locator('lv-chat-composer textarea')
    await textarea.fill('Build a sales dashboard')
    await textarea.press('Enter')
    expect(await paintedBuilderStates(page)).toEqual(Array(4).fill({ builder: true, generation: true, titlebar: false }))
    releaseAcceptance()
    await page.waitForURL('**/chats/c3**')
    expect(page.url()).toBe(`${fixture.baseURL}/chats/c3?preview=builder`)
    await page.waitForFunction(() => Boolean(document.querySelector('lv-chat-page')?.shadowRoot?.querySelector('.main')))
    expect(await paintedBuilderStates(page)).toEqual(Array(4).fill({ builder: true, generation: true, titlebar: false }))
    await screenshot(page, 'new-dashboard-cold-destination')
  } finally {
    releaseAcceptance()
    await page.close()
  }
})

test('generated dashboard stays covered until its selected page reports ready', async () => {
  const page = await fixture.browser.newPage({ viewport: { width: 1440, height: 900 } })
  let releaseBuilder = () => {}
  const builderResponse = new Promise<void>(resolve => { releaseBuilder = resolve })
  try {
    await page.route('**/dashboards/generated/edit**', async route => {
      await builderResponse
      await route.fulfill({ contentType: 'text/html', body: '<lv-dashboard-builder>Ready empty dashboard</lv-dashboard-builder>' })
    })
    await page.goto(fixture.baseURL)
    const chat = page.locator('lv-chat-page')
    await chat.locator('lv-chat-composer').waitFor()
    await chat.evaluate(async (element: any) => {
      element.showOptimisticTurn(new CustomEvent('lv-chat-submit', { detail: { input: 'Build a sales dashboard', references: [] } }))
      await element.updateComplete
      await element.openGeneratedDashboard('/dashboards/generated/edit?embed=chat&page=overview')
    })
    const frame = chat.locator('.builder-frame')
    const overlay = chat.locator('.builder-loading')
    await screenshot(page, 'generated-dashboard-slow-response')
    expect(await overlay.isVisible()).toBe(true)
    expect(await frame.evaluate(element => ({ visibility: getComputedStyle(element).visibility, display: getComputedStyle(element).display }))).toEqual({ visibility: 'hidden', display: 'block' })
    expect(await paintedBuilderStates(page)).toEqual(Array(4).fill({ builder: true, generation: true, titlebar: false }))
    const before = await chat.locator('.builder-stage').boundingBox()
    releaseBuilder()
    const builder = page.frameLocator('.builder-frame').locator('lv-dashboard-builder')
    await builder.waitFor({ state: 'attached' })
    const projection = {
      type: 'lv-builder-saved', ready: false, updating: false, canEdit: true, revisionId: 'rev-1', pageId: 'overview', pageTitle: 'Overview',
      href: '/dashboards/generated/edit?embed=chat&page=overview', pages: [{ id: 'overview', title: 'Overview' }], components: [], artifacts: [], visuals: {},
      reference: { reference: { kind: 'dashboard', id: 'generated' }, name: 'Generated', hierarchy: [], locations: [], context: [] },
    }
    // Even a valid early projection and a loaded iframe are insufficient.
    await builder.evaluate((_, message) => window.parent.postMessage(message, window.location.origin), projection)
    await chat.evaluate(async (element: any) => { await element.updateComplete })
    expect(await overlay.isVisible()).toBe(true)
    expect(await frame.evaluate(element => getComputedStyle(element).visibility)).toBe('hidden')
    // A same-origin message from another window must not reveal this iframe.
    await page.evaluate(message => window.postMessage({ ...message, ready: true }, window.location.origin), projection)
    expect(await paintedBuilderStates(page)).toEqual(Array(4).fill({ builder: true, generation: true, titlebar: false }))
    expect(await overlay.isVisible()).toBe(true)
    await builder.evaluate((_, message) => window.parent.postMessage({ ...message, ready: true }, window.location.origin), projection)
    await overlay.waitFor({ state: 'hidden' })
    expect(await frame.isVisible()).toBe(true)
    expect(await frame.evaluate(element => getComputedStyle(element).visibility)).toBe('visible')
    expect(await chat.locator('.builder-stage').boundingBox()).toEqual(before)
    await screenshot(page, 'generated-dashboard-ready')
  } finally {
    releaseBuilder()
    await page.close()
  }
})

test('reused iframe cannot report its old dashboard ready during a new navigation', async () => {
  const page = await fixture.browser.newPage({ viewport: { width: 1440, height: 900 } })
  let releaseNewDashboard = () => {}
  const newDashboard = new Promise<void>(resolve => { releaseNewDashboard = resolve })
  try {
    await page.route('**/dashboards/old/edit**', route => route.fulfill({ contentType: 'text/html', body: '<lv-dashboard-builder>Old dashboard</lv-dashboard-builder>' }))
    await page.route('**/dashboards/new-dashboard/edit**', async route => {
      await newDashboard
      await route.fulfill({ contentType: 'text/html', body: '<lv-dashboard-builder>New dashboard</lv-dashboard-builder>' })
    })
    await page.goto(fixture.baseURL)
    const chat = page.locator('lv-chat-page')
    await chat.locator('lv-chat-composer').waitFor()
    await chat.evaluate(async (element: any) => {
      element.restoredBuilderHref = '/dashboards/old/edit?embed=chat&page=overview'
      element.enterBuilder()
      await element.updateComplete
    })
    await postReadyDashboardProjection(page, 'old')
    await chat.evaluate(async (element: any) => {
      await element.openGeneratedDashboard('/dashboards/new-dashboard/edit?embed=chat&page=overview')
    })
    await page.frameLocator('.builder-frame').locator('body').evaluate(() => {
      window.parent.postMessage({
        type: 'lv-builder-saved', ready: true, revisionId: 'stale-revision', pageId: 'overview',
        href: '/dashboards/old/edit?embed=chat&page=overview',
        reference: { reference: { kind: 'dashboard', id: 'old' }, name: 'Old', hierarchy: [], locations: [], context: [] },
        components: [], artifacts: [], visuals: {},
      }, window.location.origin)
    })
    expect(await paintedBuilderStates(page)).toEqual(Array(4).fill({ builder: true, generation: true, titlebar: false }))
    expect(await chat.evaluate((element: any) => ({ href: element.savedBuilderHref, pageId: element.dashboardPageId }))).toEqual({ href: '', pageId: '' })
    expect(await chat.locator('.builder-frame').evaluate(element => getComputedStyle(element).visibility)).toBe('hidden')
    releaseNewDashboard()
    await page.frameLocator('.builder-frame').getByText('New dashboard').waitFor({ state: 'attached' })
    await postReadyDashboardProjection(page, 'new-dashboard')
    await page.frameLocator('.builder-frame').getByText('New dashboard').waitFor()
    expect(await chat.locator('.builder-loading').isVisible()).toBe(false)
  } finally {
    releaseNewDashboard()
    await page.close()
  }
})

test('a completed run in the first handoff bootstrap opens its generated dashboard once', async () => {
  const page = await fixture.browser.newPage()
  let opens = 0
  try {
    await page.route('**/chats/c3/actions/preview/open?*', route => {
      opens++
      return route.fulfill({ contentType: 'text/html', body: '<lv-dashboard-builder>Fast completed dashboard</lv-dashboard-builder>' })
    })
    await page.goto(`${fixture.baseURL}/unhydrated?preview=builder`)
    await page.waitForFunction(() => Boolean(document.querySelector('lv-chat-page')?.shadowRoot))
    await page.evaluate(async () => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev' as string)
      mergePatch({ page: { kind: 'chat', view: 'conversation', title: 'Chats' }, agent: {
        activeConversationId: 'c3', status: { enabled: true, running: false, runId: 'fast-build' }, composer: { value: '', disabled: false },
        transcript: [
          { id: 'user', kind: 'user', text: 'Build a sales dashboard', runId: 'fast-build' },
          { id: 'create', kind: 'tool', name: 'create_dashboard_draft', toolCallId: 'create', runId: 'fast-build', status: 'complete' },
          { id: 'preview', kind: 'tool', name: 'preview_dashboard_draft', toolCallId: 'preview', runId: 'fast-build', status: 'complete' },
        ],
      } })
    })
    await postReadyDashboardProjection(page)
    await page.frameLocator('.builder-frame').getByText('Fast completed dashboard').waitFor()
    await page.getByRole('button', { name: 'Expand chat', exact: true }).click()
    await page.locator('lv-chat-page').evaluate(async (element: any) => {
      for (let i = 0; i < 3; i++) { element.requestUpdate(); await element.updateComplete }
    })
    expect(opens).toBe(1)
    expect(await page.locator('lv-chat-page').evaluate((element: any) => element.builderOpen)).toBe(false)
  } finally { await page.close() }
})

test('an ordinary accepted prompt clears an unconfirmed builder handoff hint', async () => {
  const page = await fixture.browser.newPage()
  try {
    await page.goto(`${fixture.baseURL}/unhydrated?preview=builder`)
    await page.waitForFunction(() => Boolean(document.querySelector('lv-chat-page')?.shadowRoot))
    await page.locator('lv-chat-page').evaluate(async (element: any) => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev' as string)
      mergePatch({ page: { kind: 'chat', view: 'conversation', title: 'Chats' }, agent: {
        activeConversationId: 'c3', status: { enabled: true, running: true, runId: 'ordinary-run' }, composer: { value: '', disabled: false },
        transcript: [{ id: 'user', kind: 'user', text: 'Explain revenue', runId: 'ordinary-run' }],
      } })
      await element.updateComplete
    })
    expect(await paintedBuilderStates(page)).toEqual(Array(4).fill({ builder: false, generation: false, titlebar: true }))
    expect(new URL(page.url()).searchParams.get('preview')).toBeNull()
  } finally { await page.close() }
})

test('a generated builder bootstrap error replaces loading and dismisses its timeout', async () => {
  const page = await fixture.browser.newPage()
  try {
    await page.route('**/dashboards/broken/edit**', route => route.fulfill({ contentType: 'text/html', body: '<lv-dashboard-builder>Broken builder</lv-dashboard-builder>' }))
    await page.goto(fixture.baseURL)
    const chat = page.locator('lv-chat-page')
    await chat.locator('lv-chat-composer').waitFor()
    await page.clock.install()
    await chat.evaluate(async (element: any) => {
      await element.openGeneratedDashboard('/dashboards/broken/edit?embed=chat&page=overview')
    })
    const builder = page.frameLocator('.builder-frame').locator('lv-dashboard-builder')
    await builder.waitFor({ state: 'attached' })
    await builder.evaluate(() => window.parent.postMessage({ type: 'lv-builder-operation-error', message: 'Failed to bootstrap dashboard.' }, window.location.origin))
    await chat.getByRole('alert').filter({ hasText: 'Failed to bootstrap dashboard.' }).waitFor()
    expect(await chat.locator('.builder-loading').isVisible()).toBe(false)
    expect(await chat.evaluate((element: any) => element.savingDashboard)).toBe(false)
    await page.clock.fastForward(46_000)
    expect(await chat.getByRole('alert').filter({ hasText: 'Failed to bootstrap dashboard.' }).isVisible()).toBe(true)
    expect(await chat.getByText('The dashboard is taking longer to open. Reload the chat to retry.').count()).toBe(0)
  } finally { await page.close() }
})


test('generated dashboard navigation keeps Back and Forward on the parent layout across reused frames', async () => {
  const page = await fixture.browser.newPage()
  try {
    await page.route('**/dashboards/*/edit**', route => route.fulfill({ contentType: 'text/html', body: '<lv-dashboard-builder>Dashboard</lv-dashboard-builder>' }))
    await page.goto(`${fixture.baseURL}/chats/c1`)
    const chat = page.locator('lv-chat-page')
    await chat.locator('lv-chat-thread').waitFor()
    const initial = page.url()
    const open = async (id: string) => {
      await chat.evaluate(async (element: any, id) => { await element.openGeneratedDashboard(`/dashboards/${id}/edit?embed=chat&page=overview`) }, id)
      await page.waitForFunction(id => (document.querySelector('lv-chat-page') as any).builderFrame.contentWindow.location.pathname === `/dashboards/${id}/edit`, id)
      await postReadyDashboardProjection(page, id)
    }
    await open('first')
    const first = page.url()
    await page.evaluate(() => history.back())
    await page.waitForURL(initial)
    expect(await chat.evaluate((element: any) => element.builderOpen)).toBe(false)
    // A readiness message queued by the abandoned document must not rewrite Back.
    await chat.evaluate((element: any) => window.dispatchEvent(new MessageEvent('message', {
      origin: window.location.origin, source: element.builderFrame.contentWindow, data: {
        type: 'lv-builder-saved', ready: true, href: '/dashboards/first/edit?embed=chat&page=overview',
        pageId: 'overview', revisionId: 'late-first', components: [], artifacts: [], visuals: {},
        reference: { reference: { kind: 'dashboard', id: 'first' }, name: 'first', hierarchy: [], locations: [], context: [] },
      },
    })))
    expect(page.url()).toBe(initial)
    await page.evaluate(() => history.forward())
    await page.waitForURL(first)
    expect(await chat.evaluate((element: any) => element.builderOpen)).toBe(true)
    await chat.evaluate(async (element: any) => { await element.closeDashboardPreview() })
    const priorLayout = page.url()
    await open('second')
    const second = page.url()
    await page.evaluate(() => history.back())
    await page.waitForURL(priorLayout)
    expect(await chat.evaluate((element: any) => element.builderOpen)).toBe(false)
    await page.waitForFunction(() => (document.querySelector('lv-chat-page') as any).builderFrame.contentWindow.location.pathname === '/dashboards/first/edit')
    await postReadyDashboardProjection(page, 'first')
    expect(page.url()).toBe(priorLayout)
    await page.evaluate(() => history.forward())
    await page.waitForURL(second)
    expect(await chat.evaluate((element: any) => element.builderOpen)).toBe(true)
    await page.waitForFunction(() => (document.querySelector('lv-chat-page') as any).builderFrame.contentWindow.location.pathname === '/dashboards/second/edit')
    await postReadyDashboardProjection(page, 'second')
    expect(page.url()).toBe(second)
  } finally { await page.close() }
})


test('restored dashboard navigation waits for the iframe to mount after chat bootstrap', async () => {
  const page = await fixture.browser.newPage()
  try {
    await page.route('**/dashboards/restored/edit**', route => route.fulfill({ contentType: 'text/html', body: '<lv-dashboard-builder>Restored dashboard</lv-dashboard-builder>' }))
    const href = '/dashboards/restored/edit?embed=chat&page=overview'
    await page.goto(`${fixture.baseURL}/unhydrated?preview=dashboard&dashboard=${encodeURIComponent(href)}`)
    await page.getByText('Loading chat…', { exact: true }).waitFor()
    expect(await page.locator('.builder-frame').count()).toBe(0)
    await page.evaluate(async () => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev' as string)
      setTimeout(() => mergePatch({ page: { kind: 'chat', view: 'conversation', title: 'Chats' }, agent: {
        activeConversationId: 'c3', status: { enabled: true, running: false }, composer: { value: '', disabled: false }, transcript: [],
      } }), 0)
    })
    await page.frameLocator('.builder-frame').getByText('Restored dashboard').waitFor({ state: 'attached' })
    await postReadyDashboardProjection(page, 'restored')
    expect(new URL(page.url()).searchParams.get('preview')).toBe('dashboard')
    expect(await page.locator('lv-chat-page').evaluate((element: any) => element.builderOpen)).toBe(false)
  } finally { await page.close() }
})


test('Back from a completed generated transcript keeps its dashboard available for Preview', async () => {
  const page = await fixture.browser.newPage()
  try {
    await page.route('**/chats/*/actions/*/open?*', route => route.fulfill({ contentType: 'text/html', body: '<lv-dashboard-builder>Completed dashboard</lv-dashboard-builder>' }))
    await page.goto(fixture.baseURL)
    const chat = page.locator('lv-chat-page')
    await chat.locator('lv-chat-composer').waitFor()
    const initial = page.url()
    await chat.evaluate(async (element: any) => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev' as string)
      mergePatch({ agent: { status: { enabled: true, running: true, runId: 'done' } } })
      await element.updateComplete
    })
    await chat.evaluate(async (element: any) => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev' as string)
      mergePatch({ agent: {
        status: { enabled: true, running: false, runId: 'done' },
        transcript: ['create_dashboard_draft', 'edit_dashboard_source', 'preview_dashboard_draft'].map(name => ({ id: name, kind: 'tool', name, toolCallId: name, runId: 'done', status: 'complete' })),
      } })
      await element.updateComplete
    })
    await page.waitForFunction(() => (document.querySelector('lv-chat-page') as any).builderOpen)
    await postReadyDashboardProjection(page)
    await page.evaluate(() => history.back())
    await page.waitForURL(initial)
    await postReadyDashboardProjection(page)
    expect(page.url()).toBe(initial)
    expect(await chat.evaluate((element: any) => element.builderOpen)).toBe(false)
    await page.getByRole('button', { name: 'Preview dashboard', exact: true }).click()
    expect(new URL(page.url()).searchParams.get('dashboard')).toBe('/dashboards/demo/edit?embed=chat&page=overview')
    await postReadyDashboardProjection(page)
    expect(await chat.evaluate((element: any) => ({ open: element.builderOpen, canEdit: element.builderCanEdit, loading: element.builderLoading }))).toEqual({ open: true, canEdit: true, loading: false })
  } finally { await page.close() }
})
