import type { VisualizationEnvelope } from '../../../../../generated/visualization'
import { inlineDataset, type EChartsTranslation } from './common'
import { cartesianIsHorizontal, type CartesianSpec } from './series-intent'

// Axis refinements that depend on the data or on the chart orientation rather
// than on one axis in isolation. Authored axis policy is applied afterwards by
// the decision context, so an explicit minimum, scale, or zero setting wins.
export function refineCartesianAxes(envelope: VisualizationEnvelope, spec: CartesianSpec, option: EChartsTranslation): EChartsTranslation {
  if (cartesianIsHorizontal(spec) && !['heatmap', 'boxplot', 'histogram', 'waterfall', 'candlestick'].includes(spec.mark)) {
    // ECharts draws the first category of a vertical category axis at the
    // bottom, so ranked data (largest first) reads upside down. Reading order
    // is top-down, so the default inverts the axis; an authored `inverted`
    // policy, which axis() has already translated to inverse, restores the
    // bottom-up order.
    for (const candidate of axisList(option.yAxis)) {
      if (candidate.type === 'category') candidate.inverse = candidate.inverse !== true
    }
  }
  if (measuresAreNonNegative(envelope, spec)) {
    // The value axis may not dip below zero for data that cannot: padding the
    // low side would add a meaningless negative tick. Keep room on the max side.
    for (const candidate of [...axisList(option.xAxis), ...axisList(option.yAxis)]) {
      const gap = candidate.boundaryGap
      if (candidate.type === 'value' && Array.isArray(gap) && gap[0] === '5%') candidate.boundaryGap = ['0%', gap[1]]
    }
  }
  return option
}

function axisList(axisOption: EChartsTranslation | EChartsTranslation[] | undefined): EChartsTranslation[] {
  return Array.isArray(axisOption) ? axisOption : axisOption ? [axisOption] : []
}

function measuresAreNonNegative(envelope: VisualizationEnvelope, spec: CartesianSpec): boolean {
  if (envelope.dataState.kind !== 'inline' || spec.y.length === 0) return false
  for (const measure of spec.y) {
    const dataset = inlineDataset(envelope, measure.dataset)
    const column = dataset?.columns.indexOf(measure.field) ?? -1
    if (!dataset || column < 0) return false
    for (const row of dataset.rows) {
      const value = row[column]
      if (value === null || value === undefined || value === '') continue
      if (Number(value) < 0) return false
    }
  }
  return true
}
