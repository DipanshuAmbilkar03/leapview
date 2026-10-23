import { afterAll, beforeAll, describe, expect, test } from 'bun:test'
import { createServer, type Server, type ServerResponse } from 'node:http'
import { readFile } from 'node:fs/promises'
import { extname, join, normalize } from 'node:path'
import { chromium, type Browser, type Page } from '@playwright/test'

// Real Datastar, the real command.js bundle, and the exact inline listener the
// server renders on <main>, exercised against /updates responses that used to
// leave the page blank.
let server: Server
let browser: Browser
let baseURL = ''
const projectRoot = process.cwd()
const bundleRoot = join(projectRoot, '.tmp/page-stream-recovery-test')
const openStreams = new Set<ServerResponse>()

const fetchHandler = (async () => {
  const source = await readFile(join(projectRoot, 'internal/platform/web/page/page.go'), 'utf8')
  const match = source.match(/const pageStreamFetchHandler = "([^\n]*)"\n/)
  if (!match?.[1]) throw new Error('page.go no longer defines pageStreamFetchHandler')
  return match[1]
})()

beforeAll(async () => {
  const build = await Bun.build({
    entrypoints: [join(projectRoot, 'web/components/shared/command.ts')],
    target: 'browser',
    format: 'esm',
    splitting: true,
    outdir: bundleRoot,
    naming: { entry: '[name].[ext]', chunk: 'chunks/[name]-[hash].[ext]' },
  })
  if (!build.success) throw new Error(build.logs.map(String).join('\n'))
  const document = await page()
  server = createServer(async (request, response) => {
    const url = new URL(request.url ?? '/', 'http://127.0.0.1')
    if (url.pathname === '/') {
      response.setHeader('content-type', 'text/html')
      response.end(document)
      return
    }
    if (url.pathname === '/login') {
      response.setHeader('content-type', 'text/html')
      response.end('<!doctype html><title>Sign in</title><h1>Sign in</h1>')
      return
    }
    if (url.pathname === '/updates') {
      const kind = url.searchParams.get('case') ?? ''
      // A real server takes a moment to answer; the early-* cases skip that to
      // race the response against command.js.
      if (kind === 'reset') {
        // Dropping the socket before any response bytes makes the browser
        // fail the fetch, which Datastar retries with backoff.
        serveUpdates(kind, response)
        return
      }
      setTimeout(() => serveUpdates(kind.replace(/^early-/, 'status-'), response), kind.startsWith('early-') ? 0 : 300)
      return
    }
    if (url.pathname === '/cmd') {
      response.writeHead(Number(url.searchParams.get('status') ?? '500'), { 'content-type': 'text/plain' }).end('failed')
      return
    }
    if (url.pathname.startsWith('/static/')) {
      const relative = url.pathname === '/static/vendor/datastar-1.0.2.js'
        ? join(projectRoot, 'static/vendor/datastar-1.0.2.js')
        : join(bundleRoot, normalize(url.pathname.slice('/static/'.length)))
      try {
        response.setHeader('content-type', extname(relative) === '.js' ? 'text/javascript' : 'application/octet-stream')
        response.end(await readFile(relative))
      } catch {
        response.writeHead(404).end('not found')
      }
      return
    }
    response.writeHead(404).end('not found')
  })
  await new Promise<void>((resolve) => server.listen(0, resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('test server did not bind to a port')
  baseURL = `http://127.0.0.1:${address.port}`
  browser = await chromium.launch()
})

afterAll(async () => {
  for (const response of openStreams) response.destroy()
  await browser?.close()
  server?.closeAllConnections()
  if (server?.listening) {
    await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
  }
}, 15_000)

function serveUpdates(kind: string, response: ServerResponse): void {
  switch (kind) {
    case 'status-503':
      response.writeHead(503, { 'content-type': 'text/plain' }).end('unavailable')
      return
    case 'status-403':
      response.writeHead(403, { 'content-type': 'text/plain' }).end('forbidden')
      return
    case 'status-429':
      response.writeHead(429, { 'content-type': 'text/plain' }).end('slow down')
      return
    case 'status-401':
      response.writeHead(401, { 'content-type': 'text/plain' }).end('unauthorized')
      return
    case 'text-200':
      // Datastar only understands event streams, HTML and JSON; anything else
      // ends the request without an error event.
      response.writeHead(200, { 'content-type': 'text/plain' }).end('maintenance')
      return
    case 'reset':
      response.destroy()
      return
    case 'live':
      response.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-cache' })
      response.write('event: datastar-patch-signals\ndata: signals {"loaded": true}\n\n')
      openStreams.add(response)
      response.on('close', () => openStreams.delete(response))
      return
    default:
      response.writeHead(404).end('not found')
  }
}

async function page(): Promise<string> {
  const handler = (await fetchHandler).replaceAll('&', '&amp;').replaceAll('"', '&quot;')
  return `<!doctype html><html><head>
    <script type="module" src="/static/vendor/datastar-1.0.2.js"></script>
    <script type="module" src="/static/command.js"></script>
  </head><body>
    <main data-page-stream-recovery-root data-on:datastar-fetch="${handler}"
      data-init="@get('/updates?case=' + new URLSearchParams(location.search).get('case'), {openWhenHidden: true})">
      <section data-page-stream-recovery data-show="$pageStreamRecovery" style="display:none" role="alert" aria-live="assertive" aria-labelledby="recovery-title">
        <div>
          <h1 id="recovery-title" data-page-stream-recovery-title>Unable to load this page</h1>
          <p data-page-stream-recovery-message>We couldn't connect to LeapView.</p>
          <button type="button" data-on:click="window.location.reload()">Retry</button>
        </div>
      </section>
      <button id="unavailable" data-on:click="@post('/cmd?status=503', {retry: 'never', retryMaxCount: 0, openWhenHidden: true})">Apply</button>
      <button id="expired" data-on:click="@post('/cmd?status=401', {retry: 'never', retryMaxCount: 0, openWhenHidden: true})">Apply expired</button>
    </main>
  </body></html>`
}

async function open(kind: string, commandScriptDelayMs = 0): Promise<Page> {
  const tab = await browser.newPage()
  if (commandScriptDelayMs > 0) {
    await tab.route('**/static/command.js', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, commandScriptDelayMs))
      await route.continue()
    })
  }
  await tab.goto(`${baseURL}/?case=${kind}`)
  return tab
}

describe('page stream recovery in a real browser', () => {
  test.each([
    ['status-503', 'LeapView is unavailable'],
    ['status-403', "You can't open this page"],
    ['status-429', 'Too many requests'],
    ['text-200', "This page didn't load"],
  ])('%s opens the recovery card with matching copy and focuses Retry', async (kind, title) => {
    const tab = await open(kind)
    try {
      const card = tab.locator('[data-page-stream-recovery]')
      await card.waitFor({ state: 'visible', timeout: 10_000 })
      expect(await tab.locator('[data-page-stream-recovery-title]').textContent()).toBe(title)
      expect((await tab.locator('[data-page-stream-recovery-message]').textContent())?.trim()).not.toBe("We couldn't connect to LeapView.")
      await tab.waitForFunction(() => document.activeElement?.textContent === 'Retry')
      expect(await tab.getByRole('alert').count()).toBe(1)
    } finally {
      await tab.close()
    }
  }, 20_000)

  test('connection resets cover a page that never loaded once reconnecting keeps failing', async () => {
    const tab = await open('reset')
    try {
      await tab.locator('[data-page-stream-recovery]').waitFor({ state: 'visible', timeout: 15_000 })
      expect(await tab.locator('[data-page-stream-recovery-title]').textContent()).toBe("Can't reach LeapView")
    } finally {
      await tab.close()
    }
  }, 30_000)

  test('an expired session on the stream goes to sign in', async () => {
    const tab = await open('status-401')
    try {
      await tab.waitForURL('**/login?error=session_expired', { timeout: 10_000 })
    } finally {
      await tab.close()
    }
  }, 20_000)

  test('a healthy stream never opens the card', async () => {
    const tab = await open('live')
    try {
      await tab.waitForFunction(() => (window as unknown as { LeapViewCommand?: unknown }).LeapViewCommand !== undefined)
      await new Promise((resolve) => setTimeout(resolve, 800))
      expect(await tab.locator('[data-page-stream-recovery]').isVisible()).toBe(false)
    } finally {
      await tab.close()
    }
  }, 20_000)
})

describe('stream failures that beat command.js', () => {
  test('an immediate 401 still redirects to sign in', async () => {
    const tab = await open('early-401', 1_500)
    try {
      await tab.waitForURL('**/login?error=session_expired', { timeout: 10_000 })
    } finally {
      await tab.close()
    }
  }, 20_000)

  test('an immediate 503 still opens the recovery card, and command.js keeps it open', async () => {
    const tab = await open('early-503', 1_500)
    try {
      await tab.locator('[data-page-stream-recovery]').waitFor({ state: 'visible', timeout: 10_000 })
      await tab.waitForFunction(() => (window as unknown as { LeapViewCommand?: unknown }).LeapViewCommand !== undefined)
      expect(await tab.locator('[data-page-stream-recovery]').isVisible()).toBe(true)
    } finally {
      await tab.close()
    }
  }, 20_000)
})

describe('command failures in a real browser', () => {
  test('an unclaimed failure is announced instead of failing silently', async () => {
    const tab = await open('live')
    try {
      await tab.locator('#unavailable').click()
      const toast = tab.getByRole('alert').filter({ hasText: "LeapView is unavailable, so your change wasn't applied" })
      await toast.waitFor({ state: 'visible', timeout: 10_000 })
      expect(await tab.locator('[data-page-stream-recovery]').isVisible()).toBe(false)
    } finally {
      await tab.close()
    }
  }, 20_000)

  test('an expired session on a command goes to sign in', async () => {
    const tab = await open('live')
    try {
      await tab.locator('#expired').click()
      await tab.waitForURL('**/login?error=session_expired', { timeout: 10_000 })
    } finally {
      await tab.close()
    }
  }, 20_000)
})
