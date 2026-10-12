import type { ChatTranscriptItemSignal } from '../../generated/signals'

/** Keep authoring diagnostics available without requiring readers to interpret
 * compiler messages before they can decide how to continue. */
export function dashboardSourceValidationError(item: ChatTranscriptItemSignal): { summary: string; details: string } | undefined {
  if (item.name !== 'edit_dashboard_source') return undefined
  const diagnostic = item.error?.trim() ?? ''
  let code = ''
  try {
    const parsedCode = JSON.parse(item.resultJson || '{}')?.error?.code
    if (typeof parsedCode === 'string') code = parsedCode
  } catch { /* Results may use TOON. */ }
  if (!code) code = (item.resultJson || diagnostic).match(/^error:\s*\n\s+code:\s*([a-z_]+)\s*$/m)?.[1] ?? ''
  // compiler.contract is also the fallback diagnostic for permission and
  // infrastructure failures. An explicit error code takes precedence.
  if (code && code !== 'invalid_arguments') return undefined
  if (code !== 'invalid_arguments' && !/\binvalid_arguments\b|\bsource edit \d+ oldText\b|\bdecode edited dashboard source\b/.test(diagnostic)) return undefined
  return {
    summary: 'The dashboard changes need correction. Ask the agent to fix the draft and try again.',
    details: [diagnostic, item.resultJson?.trim()].filter((value, index, values) => value && values.indexOf(value) === index).join('\n\n'),
  }
}
