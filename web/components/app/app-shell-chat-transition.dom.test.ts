import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'
import { testDocument } from './app-shell.dom.fixture'

let server: Server
let baseURL = ''
let browser: Browser
const root = process.cwd()
const assetRoot = join(root, '.tmp/app-shell-test')

beforeAll(async () => {
  server = createServer(async (request, response) => {
    const url = new URL(request.url ?? '/', 'http://127.0.0.1')
    if (url.pathname === '/chats/c3') {
      response.setHeader('content-type', 'text/html')
      response.end(testDocument(true).replace('<lv-route-page slot="page"></lv-route-page>', '<lv-chat-page slot="page"></lv-chat-page>'))
      return
    }
    const fileRoot = url.pathname.startsWith('/tmp/') ? assetRoot : root
    const pathname = url.pathname.startsWith('/tmp/') ? url.pathname.replace('/tmp/', '/') : url.pathname
    const file = normalize(join(fileRoot, pathname))
    if (!file.startsWith(fileRoot)) { response.writeHead(404); response.end(); return }
    try {
      response.setHeader('content-type', file.endsWith('.css') ? 'text/css' : 'text/javascript')
      response.end(await readFile(file))
    } catch { response.writeHead(404); response.end() }
  })
  await new Promise<void>(resolve => server.listen(0, resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('test server did not bind to a port')
  baseURL = `http://127.0.0.1:${address.port}`
  browser = await chromium.launch()
})

afterAll(async () => {
  await browser?.close()
  server.closeAllConnections()
  await new Promise<void>((resolve, reject) => server.close((error: NodeJS.ErrnoException | undefined) => error && error.code !== 'ERR_SERVER_NOT_RUNNING' ? reject(error) : resolve()))
}, 15_000)

test('cold chat builder handoff suppresses the sidebar before chat signals and upgrade', async () => {
  const page = await browser.newPage({ viewport: { width: 1320, height: 900 } })
  try {
    await page.goto(`${baseURL}/chats/c3?preview=builder`)
    await page.waitForFunction(() => Boolean(document.querySelector('lv-app-shell')?.shadowRoot?.querySelector('main')))
    const frames = await page.locator('lv-app-shell').evaluate(async (element: HTMLElement) => {
      const frames: Array<{ sidebar: boolean; mainLeft: number; mainWidth: number }> = []
      for (let i = 0; i < 4; i++) {
        await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
        const main = element.shadowRoot!.querySelector('main')!
        const rect = main.getBoundingClientRect()
        frames.push({ sidebar: Boolean(element.shadowRoot!.querySelector('lv-sidebar')), mainLeft: Math.round(rect.left), mainWidth: Math.round(rect.width) })
      }
      return frames
    })
    expect(frames).toEqual(Array(4).fill({ sidebar: false, mainLeft: 0, mainWidth: 1320 }))
    expect(await page.evaluate(() => customElements.get('lv-chat-page'))).toBeUndefined()
  } finally { await page.close() }
})
