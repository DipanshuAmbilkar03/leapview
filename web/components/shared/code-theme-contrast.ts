/**
 * Contrast corrections for the GitHub Shiki themes.
 *
 * The stock themes use a handful of token colors that fall under the WCAG 1.4.3
 * minimum of 4.5:1 once a highlighted or focused line tints the background with
 * the accent wash: comments, tag names, keywords and variables in the light
 * theme (4.0-4.2:1 on highlighted lines) and comments in the dark theme (3.05:1).
 * Each replacement keeps the original hue and only deepens it (light theme) or
 * lifts it (dark theme), using the nearest GitHub Primer foreground tones.
 *
 * Keys are the lowercase colors emitted by the theme; Shiki applies them per
 * theme name through the `colorReplacements` option.
 */
export const accessibleCodeColorReplacements = {
  'github-light': {
    '#6a737d': '#57606a', // comments
    '#22863a': '#116329', // tag names
    '#d73a49': '#cf222e', // keywords
    '#e36209': '#a0420b', // variables
  },
  'github-dark': {
    '#6a737d': '#9ba3ae', // comments
  },
} as const satisfies Record<'github-light' | 'github-dark', Record<string, string>>
