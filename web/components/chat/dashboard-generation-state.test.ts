import { expect, test } from 'bun:test'
import type { ChatTranscriptItemSignal } from '../../generated/signals'
import { dashboardGenerationState, isDashboardBuildPrompt } from './dashboard-generation-state'
import { dashboardSourceValidationError } from './dashboard-source-error'

const tool = (name: string, status = 'running', runId = 'new'): ChatTranscriptItemSignal => ({ id: name, kind: 'tool', name, status, runId })

test('dashboard build intent opens only for a requested dashboard action', () => {
  for (const text of ['Build a dashboard of sales', 'Please create me a dashboard', 'Can you make a sales dashboard?', 'Design a dashboard for finance', 'Generate a dashboard', 'Create a new report with metrics', 'Can you build me a dashboard?', 'Could you help me make a sales dashboard?', 'I would like you to create a dashboard', 'Help me build a dashboard']) expect(isDashboardBuildPrompt(text)).toBe(true)
  for (const text of ['What is a dashboard?', 'Explain the dashboard filter', 'List dashboards', 'Do not create a dashboard', 'Don’t build a dashboard', 'Create a chart of sales', 'Show the dashboard', 'Explain how to create a dashboard', 'How do I build a dashboard?', 'Could you tell me how to build a dashboard?', 'Please explain how to make a dashboard', 'I want to understand how to build a dashboard']) expect(isDashboardBuildPrompt(text)).toBe(false)
})

test('assembly stage follows the active run, never stale tools or elapsed time', () => {
  const transcript = [tool('preview_dashboard_draft', 'complete', 'old'), tool('catalog_search')]
  expect(dashboardGenerationState(transcript, 'new', true).stage).toBe('discover')
  expect(dashboardGenerationState(transcript, '', true).stage).toBe('discover')
  expect(dashboardGenerationState([...transcript, tool('create_dashboard_draft')], 'new', true).stage).toBe('assemble')
  expect(dashboardGenerationState([...transcript, tool('query_visual')], 'new', true).stage).toBe('query')
  expect(dashboardGenerationState([...transcript, tool('preview_dashboard_draft')], 'new', true).stage).toBe('preview')
})

test('stopped and failed runs do not claim progress or completion', () => {
  expect(dashboardGenerationState([], 'new', false).label).toBe('Dashboard generation paused')
  expect(dashboardGenerationState([], 'new', false, 'Request failed').label).toBe('Dashboard generation interrupted')
  const failed = { ...tool('preview_dashboard_draft', 'error'), error: 'Invalid metric' }
  expect(dashboardGenerationState([failed], 'new', true).stage).toBe('assemble')
  expect(dashboardGenerationState([failed], 'new', false).detail).toBe('Invalid metric')
})

test('source validation failures use a recoverable summary and retain technical details', () => {
  const diagnostic = 'invalid_arguments: compiler.contract: spec.visuals.revenue.query is invalid'
  const failed = { ...tool('edit_dashboard_source', 'error'), error: diagnostic }
  const state = dashboardGenerationState([failed], 'new', false)
  expect(state.label).toBe('Dashboard generation interrupted')
  expect(state.detail).toBe('The dashboard changes need correction. Ask the agent to fix the draft and try again.')
  expect(state.technicalDetails).toBe(diagnostic)
  expect(dashboardGenerationState([{ ...failed, name: 'query_semantic_model' }], 'new', false).detail).toBe(diagnostic)
  expect(dashboardGenerationState([failed], 'new', false, 'Provider connection failed').detail).toBe('Provider connection failed')
  expect(dashboardSourceValidationError({ ...failed, error: 'You cannot edit this dashboard.' })).toBeUndefined()
  const resultJson = JSON.stringify({ error: { code: 'invalid_arguments', message: 'Invalid YAML', diagnostics: [{ path: 'spec.visuals.revenue', message: 'Required field missing' }] } })
  expect(dashboardSourceValidationError({ ...failed, error: 'Invalid YAML', resultJson })?.details).toBe(`Invalid YAML\n\n${resultJson}`)
})

test('fallback compiler diagnostics do not turn permission failures into source validation errors', () => {
  const transported = 'error:\n  code: authoring_failed\n  diagnostics[1]{code,message,path}:\n    compiler.contract,access forbidden,""\n  message: access forbidden'
  const failed = { ...tool('edit_dashboard_source', 'error'), error: transported, resultJson: transported }
  expect(dashboardSourceValidationError(failed)).toBeUndefined()
  expect(dashboardGenerationState([failed], 'new', false).detail).toBe(transported)
  expect(dashboardSourceValidationError({ ...failed, error: 'invalid_arguments: source edit 0 oldText was not found', resultJson: JSON.stringify({ error: { code: 'authoring_failed', message: 'access forbidden' } }) })).toBeUndefined()
  expect(dashboardSourceValidationError({ ...failed, error: 'compiler.contract: invalid dashboard', resultJson: '' })).toBeUndefined()
})
