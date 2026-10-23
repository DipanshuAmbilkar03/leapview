import { expect, test } from 'bun:test'

import type { InlineVisualizationDataState, VisualizationEnvelope } from '../../../../generated/visualization'
import { defaultRendererContext } from '../host-controller'
import {
  accessibleLabel,
  kpiConditionalPresentation,
  kpiLayoutFeatures,
  kpiText,
  applyKpiProgressAccessibility,
  resolveKPIWidgetLayout,
} from './html'

test('HTML KPI accessible labels normalize sentence boundaries', () => {
  expect(accessibleLabel(['Revenue', 'Revenue against target.', 'Current $10.00. Target $12.00.', undefined]))
    .toBe('Revenue. Revenue against target. Current $10.00. Target $12.00.')
})

test('HTML KPI values compose governed display units with the field formatting contract', () => {
  const envelope = {
    schemaVersion: 14, visualID: 'revenue', rendererID: 'html', specRevision: 'sha256:test', dataRevision: 1,
    spec: {
      kind: 'kpi', title: 'Revenue', datasets: [{ id: 'primary', fields: [{ id: 'value', role: 'metric', dataType: 'decimal', nullable: false, label: 'Revenue', format: { kind: 'currency', currency: 'BRL' } }] }],
      dataBudget: { maxRows: 1, requiredCompleteness: 'complete' }, accessibility: { title: 'Revenue', description: 'Revenue' }, interactions: [],
      value: { dataset: 'primary', field: 'value' },
      presentation: { mode: 'compact', delta: 'absolute', favorableDirection: 'neutral', missingComparison: 'show_unavailable', ranges: [], tone: 'success' },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 1, generation: 1, datasets: [{ id: 'primary', specRevision: 'sha256:test', dataRevision: 1, generation: 1, columns: ['value'], rows: [[1234.5]], completeness: 'complete' }] },
    selection: [], highlights: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope

  expect(kpiText(envelope)).toBe('R$1.23K')
  expect(kpiText(envelope, { ...defaultRendererContext, locale: 'pt-BR' })).toBe('R$\u00a01,23K')
})

test('HTML KPI formatting resolves semantic backgrounds, readable text, and redundant status cues', () => {
  const envelope = {
    schemaVersion: 14, visualID: 'health', rendererID: 'html', specRevision: 'sha256:health', dataRevision: 1,
    spec: {
      kind: 'kpi', title: 'Health', datasets: [{ id: 'primary', fields: [{ id: 'value', role: 'metric', dataType: 'decimal', nullable: false, label: 'Health' }] }],
      dataBudget: { maxRows: 1, requiredCompleteness: 'complete' }, accessibility: { title: 'Health', description: 'Health' }, interactions: [],
      conditionalFormatting: [
        {
          id: 'background', target: 'visual_background', field: { dataset: 'primary', field: 'value' },
          rule: { kind: 'rules', rules: [{ operator: 'less_than', value: 50, style: { color: 'danger', icon: 'warning' } }], nullStyle: { icon: 'warning' }, defaultStyle: { color: 'success', icon: 'arrow_up' } },
        },
        {
          id: 'value', target: 'kpi_value', field: { dataset: 'primary', field: 'value' },
          rule: { kind: 'rules', rules: [{ operator: 'less_than', value: 50, style: { color: 'warning', icon: 'arrow_down' } }], nullStyle: { icon: 'warning' }, defaultStyle: { color: 'ink', icon: 'arrow_up' } },
        },
      ],
      value: { dataset: 'primary', field: 'value' },
      presentation: { mode: 'compact', delta: 'absolute', favorableDirection: 'neutral', missingComparison: 'show_unavailable', ranges: [], tone: 'danger' },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:health', dataRevision: 1, generation: 1, datasets: [{ id: 'primary', specRevision: 'sha256:health', dataRevision: 1, generation: 1, columns: ['value'], rows: [[35]], completeness: 'complete' }] },
    selection: [], highlights: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope

  expect(kpiConditionalPresentation(envelope, defaultRendererContext)).toEqual({
    background: defaultRendererContext.colors.danger,
    foreground: defaultRendererContext.colors.surface,
    valueColor: defaultRendererContext.colors.surface,
    iconLabel: 'decreasing',
  })
})

test('HTML KPI conditional value formatting preserves null, first-match, default, and theme-safe cues', () => {
  const envelope = {
    schemaVersion: 14, visualID: 'health', rendererID: 'html', specRevision: 'sha256:health-cues', dataRevision: 1,
    spec: {
      kind: 'kpi', title: 'Health', datasets: [{ id: 'primary', fields: [{ id: 'value', role: 'metric', dataType: 'decimal', nullable: true, label: 'Health' }] }],
      dataBudget: { maxRows: 1, requiredCompleteness: 'complete' }, accessibility: { title: 'Health', description: 'Health' }, interactions: [],
      conditionalFormatting: [{
        id: 'value', target: 'kpi_value', field: { dataset: 'primary', field: 'value' },
        rule: {
          kind: 'rules',
          rules: [
            { operator: 'greater_or_equal', value: 0, style: { color: 'warning', icon: 'circle' } },
            { operator: 'greater_or_equal', value: 80, style: { color: 'success', icon: 'arrow_up' } },
          ],
          nullStyle: { icon: 'warning' }, defaultStyle: { color: 'danger', icon: 'arrow_down' },
        },
      }],
      value: { dataset: 'primary', field: 'value' },
      presentation: { mode: 'compact', delta: 'absolute', favorableDirection: 'neutral', missingComparison: 'show_unavailable', ranges: [] },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:health-cues', dataRevision: 1, generation: 1, datasets: [{ id: 'primary', specRevision: 'sha256:health-cues', dataRevision: 1, generation: 1, columns: ['value'], rows: [[null]], completeness: 'complete' }] },
    selection: [], highlights: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope
  const dark = { ...defaultRendererContext, theme: 'dark' as const, colors: { ...defaultRendererContext.colors, danger: '#ff7b72', attention: '#d29922', success: '#56d364' } }

  expect(kpiConditionalPresentation(envelope, dark)).toMatchObject({ iconLabel: 'warning' })
  ;(envelope.dataState as InlineVisualizationDataState).datasets[0]!.rows = [[90]]
  expect(kpiConditionalPresentation(envelope, dark)).toMatchObject({ valueColor: dark.colors.attention, iconLabel: 'circle' })
  ;(envelope.dataState as InlineVisualizationDataState).datasets[0]!.rows = [[-1]]
  expect(kpiConditionalPresentation(envelope, dark)).toMatchObject({ valueColor: dark.colors.danger, iconLabel: 'decreasing' })
})

test('HTML KPI layout requirements come only from explicitly configured features', () => {
  const envelope = {
    schemaVersion: 14, visualID: 'revenue', rendererID: 'html', specRevision: 'sha256:responsive', dataRevision: 1,
    spec: {
      kind: 'kpi', title: 'Revenue', subtitle: 'Trailing 12 months',
      datasets: [
        {
          id: 'primary',
          fields: [
            { id: 'value', role: 'metric', dataType: 'decimal', nullable: false, label: 'Revenue' },
            { id: 'comparison', role: 'metric', dataType: 'decimal', nullable: false, label: 'Baseline' },
            { id: 'goal', role: 'metric', dataType: 'decimal', nullable: false, label: 'Goal' },
          ],
        },
        {
          id: 'trend',
          fields: [
            { id: 'month', role: 'category', dataType: 'date', nullable: false, label: 'Month' },
            { id: 'trend_value', role: 'metric', dataType: 'decimal', nullable: false, label: 'Revenue' },
          ],
        },
      ],
      dataBudget: { maxRows: 24, requiredCompleteness: 'complete' },
      accessibility: { title: 'Revenue', description: 'Revenue against baseline and goal' },
      interactions: [],
      value: { dataset: 'primary', field: 'value' },
      comparison: { field: { dataset: 'primary', field: 'comparison' }, reducer: 'first', label: 'Baseline' },
      goal: { field: { dataset: 'primary', field: 'goal' }, reducer: 'first', label: 'Goal' },
      trend: {
        category: { dataset: 'trend', field: 'month' },
        value: { dataset: 'trend', field: 'trend_value' },
      },
      presentation: {
        mode: 'progress', delta: 'absolute', favorableDirection: 'increase',
        missingComparison: 'show_unavailable',
        ranges: [{ minimum: 0, maximum: 10_000, label: 'On track', tone: 'success' }],
        note: 'Governed revenue',
      },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:responsive', dataRevision: 1, generation: 1, datasets: [] },
    selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as unknown as VisualizationEnvelope

  expect(kpiLayoutFeatures(envelope)).toEqual([
    'subtitle',
    'comparison',
    'progress',
    'goal',
    'status',
    'trend',
    'note',
  ])
  expect(resolveKPIWidgetLayout(envelope, { width: 320, height: 241 })).toEqual({
    kind: 'fit',
    layout: 'stacked',
    minimum: { width: 192, height: 218 },
  })
  expect(resolveKPIWidgetLayout(envelope, { width: 320, height: 242 })).toEqual({
    kind: 'fit',
    layout: 'wide',
    minimum: { width: 320, height: 242 },
  })
})

test('HTML KPI status layout is driven by qualitative ranges', () => {
  const envelope = {
    schemaVersion: 14, visualID: 'revenue', rendererID: 'html', specRevision: 'sha256:status', dataRevision: 1,
    spec: {
      kind: 'kpi', title: 'Revenue', datasets: [{ id: 'primary', fields: [{ id: 'value', role: 'metric', dataType: 'decimal', nullable: false, label: 'Revenue' }] }],
      dataBudget: { maxRows: 1, requiredCompleteness: 'complete' }, accessibility: { title: 'Revenue', description: 'Revenue' }, interactions: [],
      value: { dataset: 'primary', field: 'value' },
      presentation: { mode: 'compact', delta: 'absolute', favorableDirection: 'neutral', missingComparison: 'show_unavailable', ranges: [] },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:status', dataRevision: 1, generation: 1, datasets: [] },
    selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as unknown as VisualizationEnvelope

  expect(kpiLayoutFeatures(envelope)).not.toContain('status')
  const presentation = envelope.spec.presentation as Extract<VisualizationEnvelope['spec'], { kind: 'kpi' }>['presentation']
  presentation.ranges = [{ minimum: 0, maximum: 100, label: 'On track', tone: 'success' }]
  expect(kpiLayoutFeatures(envelope)).toContain('status')
})

function recordedAttributes(mode: 'bullet' | 'progress', state: Parameters<typeof applyKpiProgressAccessibility>[2]): Record<string, string> {
  const attributes: Record<string, string> = {}
  applyKpiProgressAccessibility({ setAttribute: (name: string, value: string) => { attributes[name] = value } }, mode, state)
  return attributes
}

test('KPI bullet meters always carry aria-valuenow, and a bullet without a value is a labelled graphic', () => {
  expect(recordedAttributes('bullet', { current: 42, currentText: '42', goalLabel: 'Target', goalText: '50', bulletMinimum: 0, bulletMaximum: 60 })).toEqual({
    role: 'meter', 'aria-label': 'Target', 'aria-valuetext': '42 of 50', 'aria-valuemin': '0', 'aria-valuemax': '60', 'aria-valuenow': '42',
  })
  const missing = recordedAttributes('bullet', { currentText: '—', goalLabel: 'Target', goalText: '50', bulletMinimum: 0, bulletMaximum: 60 })
  expect(missing).toEqual({ role: 'img', 'aria-label': 'Target: — of 50' })
  expect(missing).not.toHaveProperty('aria-valuenow')
})

test('KPI progress bars keep an indeterminate state without a current value', () => {
  expect(recordedAttributes('progress', { current: 3, currentText: '3', goalLabel: 'Goal', goalText: '10', goal: 10 })).toMatchObject({
    role: 'progressbar', 'aria-valuenow': '3', 'aria-valuemax': '10', 'aria-valuemin': '0',
  })
  const indeterminate = recordedAttributes('progress', { currentText: '—', goalText: '10', goal: 10 })
  expect(indeterminate.role).toBe('progressbar')
  expect(indeterminate).not.toHaveProperty('aria-valuenow')
})
