import type { ChatTranscriptItemSignal } from '../../generated/signals'
import { readAttachedMessage } from './attachments'
import { dashboardSourceValidationError } from './dashboard-source-error'

export type DashboardGenerationStage = 'discover' | 'assemble' | 'query' | 'preview'

/** An immediate presentation hint; the agent remains responsible for deciding
 * whether and how to create a dashboard. Attachments are never intent signals. */
export function isDashboardBuildPrompt(input: string): boolean {
  const text = readAttachedMessage(input).text
  if (/\b(?:do\s+not|don['’]t|never|without)\s+(?:\w+\s+){0,3}(?:build|create|make|generate|design)\b/i.test(text)) return false
  const action = '(?:build|create|make|generate|design)'
  const request = '(?:please\\s+)?(?:(?:can|could|would|will)\\s+you\\s+(?:please\\s+)?(?:help\\s+me\\s+)?|help\\s+me\\s+|I\\s+(?:want|need|would\\s+like)\\s+(?:you\\s+)?to\\s+|I[’\']d\\s+like\\s+(?:you\\s+)?to\\s+)?'
  // Start with an instruction or direct request. A dashboard action mentioned
  // inside a question or explanation does not authorize opening authoring.
  return new RegExp(`^\\s*${request}${action}\\b[^.!?\\n]{0,110}\\b(?:dashboard|report)\\b`, 'i').test(text)

}

export function dashboardGenerationState(transcript: ChatTranscriptItemSignal[], runId: string, running: boolean, error = '') {
  const tools = transcript.filter(item => item.kind === 'tool' && Boolean(runId) && item.runId === runId)
  const failed = [...tools].reverse().find(item => item.status === 'error' || item.error)
  const validation = failed && (!error || error === failed.error) ? dashboardSourceValidationError(failed) : undefined
  if (!running) return {
    stage: 'assemble' as DashboardGenerationStage,
    label: error || failed?.error ? 'Dashboard generation interrupted' : 'Dashboard generation paused',
    detail: validation?.summary || error || failed?.error || 'Continue in chat to finish your dashboard.',
    technicalDetails: validation?.details,
  }
  const latest = [...tools].reverse().find(item => item.status !== 'error' && !item.error)
  const name = latest?.name ?? ''
  if (name === 'preview_dashboard_draft') return { stage: 'preview' as const, label: 'Preparing your dashboard preview', detail: 'Checking the layout and loading its visuals.' }
  if (['query_visual', 'query_dashboard_visual', 'query_semantic_model'].includes(name)) return { stage: 'query' as const, label: 'Bringing your data into view', detail: 'Running queries for your dashboard visuals.' }
  if (['create_dashboard_draft', 'fork_dashboard', 'edit_dashboard_source', 'add_dashboard_visual', 'add_dashboard_page', 'assign_dashboard_field', 'execute_dashboard_command'].includes(name) || failed) return { stage: 'assemble' as const, label: 'Building your dashboard', detail: 'Arranging charts, metrics, and filters on the canvas.' }
  return { stage: 'discover' as const, label: 'Building your dashboard', detail: 'Finding the right data and planning your dashboard.' }
}

// Only the accepted run may choose this presentation; historical prompts and
// previews must never reopen authoring for a different request.
export function dashboardBuildPromptForRun(transcript: ChatTranscriptItemSignal[], runId: string): string {
  const latestUser = [...transcript].reverse().find(item => item.kind === 'user' && (!runId || item.runId === runId))
  const authoring = Boolean(runId && transcript.some(item => item.kind === 'tool' && item.runId === runId
    && ['create_dashboard_draft', 'fork_dashboard', 'edit_dashboard_source'].includes(item.name ?? '') && item.status !== 'error' && !item.error))
  return authoring || (latestUser && isDashboardBuildPrompt(latestUser.text ?? '')) ? latestUser?.text || 'Build a dashboard' : ''
}
