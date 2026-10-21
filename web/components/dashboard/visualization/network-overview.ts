import type { VisualizationEnvelope } from '../../../generated/visualization'

export function networkConnections(envelope: VisualizationEnvelope | undefined, search = ''): { source: string; target: string; value: string }[] {
  const spec = envelope?.spec
  if (!envelope || spec?.kind !== 'hierarchy' || (spec.mark !== 'graph' && spec.mark !== 'sankey') || envelope.dataState.kind !== 'inline' || !spec.source || !spec.target) return []
  const dataset = envelope.dataState.datasets.find(dataset => dataset.id === spec.source!.dataset)
  if (!dataset) return []
  const source = dataset.columns.indexOf(spec.source.field), target = dataset.columns.indexOf(spec.target.field)
  const value = spec.value ? dataset.columns.indexOf(spec.value.field) : -1
  if (source < 0 || target < 0) return []
  const query = search.trim().toLocaleLowerCase()
  return dataset.rows.map(row => ({ source: String(row[source] ?? ''), target: String(row[target] ?? ''), value: value >= 0 ? String(row[value] ?? '') : '' }))
    .filter(row => !query || `${row.source} ${row.target}`.toLocaleLowerCase().includes(query))
}
