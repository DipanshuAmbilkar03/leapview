import { format } from 'echarts'
import { compactScrollLegendGeometry } from './compact-scroll-legend'
import { finiteNumber } from './view-state-layout'

export function hasBottomLegend(value: unknown): boolean {
  const legends = Array.isArray(value) ? value : [value]
  return legends.some(isHorizontalBottomLegend)
}

export function hasTopLegend(value: unknown): boolean {
  const legends = Array.isArray(value) ? value : [value]
  return legends.some((entry) => {
    if (!entry || typeof entry !== 'object' || Array.isArray(entry)) return false
    const legend = entry as Record<string, unknown>
    return legend.show !== false && legend.orient !== 'vertical' && legend.top !== undefined && legend.top !== 'auto'
  })
}

function isHorizontalBottomLegend(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value)
    && (value as Record<string, unknown>).orient !== 'vertical'
    && (value as Record<string, unknown>).bottom !== undefined)
}

export function proportionalSideLegend(legend: unknown): 'left' | 'right' | undefined {
  const entries = Array.isArray(legend) ? legend : [legend]
  for (const entry of entries) {
    if (!entry || typeof entry !== 'object' || Array.isArray(entry) || entry.orient !== 'vertical') continue
    if (entry.left !== undefined) return 'left'
    if (entry.right !== undefined) return 'right'
  }
  return undefined
}

export function compactProportionalSideLegend(legend: unknown): unknown {
  const legends = Array.isArray(legend) ? legend : [legend]
  const result = legends.map((entry) => {
    if (!entry || typeof entry !== 'object' || Array.isArray(entry)) return entry
    const source = entry as Record<string, unknown>
    if (source.orient !== 'vertical' || (source.left === undefined && source.right === undefined)) return entry
    return { ...source, orient: 'horizontal', top: 'auto', bottom: 0, left: 'center', right: 'auto' }
  })
  return Array.isArray(legend) ? result : result[0]
}

export function compactProportionalLegendTitle(graphic: unknown, movedSide: 'left' | 'right'): unknown {
  if (!Array.isArray(graphic)) return graphic
  return graphic.map((entry) => {
    if (!entry || typeof entry !== 'object' || Array.isArray(entry)) return entry
    const title = entry as Record<string, unknown>
    if (title.type !== 'text' || title.top !== 4 || title[movedSide] !== 8) return entry
    return { ...title, top: 'auto', bottom: 28, left: 8, right: 'auto' }
  })
}

export function desktopLegend(value: unknown, resetProportionalSide = false, resetFunnelSide = false, width = Infinity): unknown {
  const legends = Array.isArray(value) ? value : [value]
  const result = legends.map((entry) => {
    if ((resetProportionalSide || resetFunnelSide) && entry && typeof entry === 'object' && !Array.isArray(entry) && entry.orient === 'vertical' && (entry.left !== undefined || entry.right !== undefined)) {
      return { ...entry, left: entry.left ?? 'auto', right: entry.right ?? 'auto', height: 'auto',
        ...(resetFunnelSide ? { textStyle: { ...entry.textStyle, width: entry.textStyle?.width ?? null,
          overflow: entry.textStyle?.overflow ?? null, ellipsis: entry.textStyle?.ellipsis ?? null },
        tooltip: entry.tooltip ?? { show: false } } : {}),
      }
    }
    if (!isHorizontalBottomLegend(entry)) return entry
    const legend = entry
    // Clear compact sizing, retaining the scroll component and its selection state.
    const restored = {
      ...legend, type: 'scroll', left: 'center', right: 'auto', width: 'auto', height: 'auto',
      itemWidth: finiteNumber(legend.itemWidth) ?? 25,
      itemHeight: finiteNumber(legend.itemHeight) ?? 14,
      textStyle: {
        ...((legend.textStyle && typeof legend.textStyle === 'object' && !Array.isArray(legend.textStyle))
          ? legend.textStyle as Record<string, unknown>
          : {}),
        width: null, overflow: null, ellipsis: null, backgroundColor: null,
      },
    }
    return legendNeedsPaging(legend, width) ? { ...restored, ...compactScrollLegendGeometry(restored, width, false) } : restored
  })
  return Array.isArray(value) ? result : result[0]
}

export function compactLegend(value: unknown, width: number): unknown {
  const legends = Array.isArray(value) ? value : [value]
  const result = legends.map((entry) => {
    if (!isHorizontalBottomLegend(entry)) return entry
    const legend = entry
    const responsive = {
      ...legend,
      type: 'scroll', bottom: 0, left: 'center', right: 'auto',
      width: Math.max(0, width - 16), height: 24,
      pageIconColor: (legend.textStyle as Record<string, unknown> | undefined)?.color,
      pageTextStyle: legend.textStyle,
    }
    return { ...responsive, ...compactScrollLegendGeometry(responsive, width) }
  })
  return Array.isArray(value) ? result : result[0]
}

export function legendNeedsPaging(value: unknown, width: number): boolean {
  return (Array.isArray(value) ? value : [value]).some((legend) => {
    if (!isHorizontalBottomLegend(legend)) return false
    const data = Array.isArray(legend.data) ? legend.data : []
    const text = legend.textStyle as Record<string, any> ?? {}
    const font = `${text.fontStyle ?? 'normal'} ${text.fontWeight ?? 'normal'} ${text.fontSize ?? 12}px ${text.fontFamily ?? 'sans-serif'}`
    const totalWidth = data.reduce((total: number, item: any) => {
      const name = typeof item === 'object' ? item.name : item
      const label = typeof legend.formatter === 'function' ? legend.formatter(name) : name
      return total + format.getTextRect(String(label ?? ''), font).width + (finiteNumber(legend.itemWidth) ?? 25) + 5
    }, Math.max(0, data.length - 1) * (finiteNumber(legend.itemGap) ?? 10))
    return totalWidth > width - 26
  })
}
