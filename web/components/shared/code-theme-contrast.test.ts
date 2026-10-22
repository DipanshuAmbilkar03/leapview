import { expect, test } from 'bun:test'
import githubDark from '@shikijs/themes/github-dark'
import githubLight from '@shikijs/themes/github-light'
import { accessibleCodeColorReplacements } from './code-theme-contrast'

type Rgb = [number, number, number]

function parse(hex: string): Rgb {
  return [1, 3, 5].map((index) => parseInt(hex.slice(index, index + 2), 16)) as Rgb
}

function luminance([red, green, blue]: Rgb): number {
  const [r, g, b] = [red, green, blue].map((channel) => {
    const value = channel / 255
    return value <= 0.03928 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
  }) as Rgb
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrast(foreground: string, background: Rgb): number {
  const [lighter, darker] = [luminance(parse(foreground)), luminance(background)].sort((a, b) => b - a)
  return (lighter! + 0.05) / (darker! + 0.05)
}

function blend(overlay: Rgb, alpha: number, base: Rgb): Rgb {
  return overlay.map((channel, index) => Math.round(channel * alpha + base[index]! * (1 - alpha))) as Rgb
}

// --lv-bg-accent-muted is Primer's accent wash: #ddf4ff in light and
// rgba(56, 139, 253, 0.1) in dark, painted behind highlighted and focused lines.
const accentWash: Rgb = [56, 139, 253]
const themes = {
  'github-light': { theme: githubLight, surfaces: [parse('#ffffff'), parse('#ddf4ff')] },
  'github-dark': { theme: githubDark, surfaces: [parse('#24292e'), blend(accentWash, 0.1, parse('#24292e'))] },
} as const

for (const [name, { theme, surfaces }] of Object.entries(themes)) {
  test(`${name} syntax colors meet 4.5:1 on plain and highlighted lines`, () => {
    const replacements: Record<string, string> = accessibleCodeColorReplacements[name as keyof typeof accessibleCodeColorReplacements]
    const failures: string[] = []
    for (const rule of theme.tokenColors ?? []) {
      const foreground = rule.settings?.foreground?.toLowerCase()
      // Only text-bearing token colors matter; the theme's "ignored" and
      // carriage-return rules never paint visible glyphs.
      const scopes = [rule.scope ?? []].flat().join(',')
      if (!foreground || /carriage-return|markup\.ignored|markup\.untracked/.test(scopes)) continue
      const effective = replacements[foreground] ?? foreground
      for (const surface of surfaces) {
        const ratio = contrast(effective, surface)
        if (ratio < 4.5) failures.push(`${foreground}->${effective} on rgb(${surface}) = ${ratio.toFixed(2)}:1 (${scopes.slice(0, 40)})`)
      }
    }
    expect(failures).toEqual([])
  })

  test(`${name} replacements only target colors the theme actually emits`, () => {
    const emitted = new Set((theme.tokenColors ?? []).map((rule) => rule.settings?.foreground?.toLowerCase()))
    const replacements: Record<string, string> = accessibleCodeColorReplacements[name as keyof typeof accessibleCodeColorReplacements]
    for (const original of Object.keys(replacements)) expect(emitted.has(original)).toBe(true)
  })
}
