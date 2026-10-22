import { expect, test } from 'bun:test'
import { JSDOM } from 'jsdom'
import type { VisualizationEnvelope } from '../../../../generated/visualization'
import { defaultRendererContext } from '../host-controller'
import { adapter } from './html'

function kpi(status: VisualizationEnvelope['status'], rows: unknown[][]): VisualizationEnvelope {
  return {
    schemaVersion: 14, visualID: 'revenue', rendererID: 'html', specRevision: 'sha256:load', dataRevision: 1,
    spec: {
      kind: 'kpi', title: 'Revenue',
      datasets: [
        { id: 'primary', fields: [{ id: 'value', role: 'metric', dataType: 'decimal', nullable: true, label: 'Revenue' }] },
        { id: 'prior', fields: [{ id: 'value', role: 'metric', dataType: 'decimal', nullable: true, label: 'Prior' }] },
      ],
      dataBudget: { maxRows: 1, requiredCompleteness: 'complete' }, accessibility: { title: 'Revenue', description: 'Revenue' }, interactions: [],
      value: { dataset: 'primary', field: 'value' },
      comparison: { field: { dataset: 'prior', field: 'value' }, reducer: 'first', label: 'Previous' },
      presentation: { mode: 'compact', delta: 'relative', favorableDirection: 'increase', missingComparison: 'show_unavailable', ranges: [] },
    },
    dataState: {
      kind: 'inline', specRevision: 'sha256:load', dataRevision: 1, generation: 1,
      datasets: [
        { id: 'primary', specRevision: 'sha256:load', dataRevision: 1, generation: 1, columns: ['value'], rows, completeness: 'complete' },
        { id: 'prior', specRevision: 'sha256:load', dataRevision: 1, generation: 1, columns: ['value'], rows, completeness: 'complete' },
      ],
    },
    selection: [], highlights: [], status, diagnostics: [],
  } as VisualizationEnvelope
}

test('a loading KPI card renders a labelled skeleton and swaps to values when data arrives', async () => {
  const dom = new JSDOM('<!doctype html><body><div id="host"></div></body>')
  const previous = { document: Object.getOwnPropertyDescriptor(globalThis, 'document'), window: Object.getOwnPropertyDescriptor(globalThis, 'window') }
  Object.defineProperty(globalThis, 'document', { configurable: true, value: dom.window.document })
  Object.defineProperty(globalThis, 'window', { configurable: true, value: dom.window })
  try {
    const host = dom.window.document.getElementById('host')!
    const handle = await adapter.mount(host, kpi({ kind: 'loading' }, []), defaultRendererContext)
    const card = host.querySelector('article')!
    expect(card.getAttribute('aria-busy')).toBe('true')
    expect(card.getAttribute('aria-label')).toContain('Loading Revenue.')
    expect(host.querySelectorAll('.lv-kpi-skeleton').length).toBe(2)
    expect(host.textContent).not.toContain('Unavailable')
    expect(host.textContent).not.toContain('—')

    await handle.update(kpi({ kind: 'ready' }, [[110]]), 0b11111, defaultRendererContext)
    expect(host.querySelector('article')!.hasAttribute('aria-busy')).toBe(false)
    expect(host.querySelector('.lv-kpi-skeleton')).toBeNull()
    expect(host.querySelector('.lv-visualization-kpi')!.textContent).toBe('110')
    handle.dispose()
  } finally {
    for (const [name, descriptor] of Object.entries(previous)) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor)
      else delete (globalThis as Record<string, unknown>)[name]
    }
    dom.window.close()
  }
})
