import { expect, test } from 'bun:test'
import * as echarts from 'echarts'
import { defaultRendererContext } from '../../host-controller'
import { EChartsHandle, echartsOption } from '../echarts'
import { CategoryColorRegistry } from './category-colors'
import { cartesianFixture } from '../echarts-test-fixtures'

test('bar legends reflect conditional fills and retain their series identity and authored label', () => {
  const envelope = cartesianFixture('bar')
  if (envelope.spec.kind !== 'cartesian' || envelope.dataState.kind !== 'inline') throw new Error('invalid fixture')
  envelope.spec.conditionalFormatting = [{
    id: 'variance', target: 'mark_fill', field: { dataset: 'primary', field: 'value' },
    rule: { kind: 'rules', rules: [{ operator: 'less_than', value: 0, style: { color: 'danger' } }], defaultStyle: { color: 'success' }, nullStyle: { color: 'neutral' } },
  }]
  envelope.spec.presentation.legendItems = [{ value: 'value', label: 'Revenue variance' }]
  envelope.dataState.datasets[0]!.rows = [['Below', -20], ['Above', 10]]
  const option = echartsOption(envelope, defaultRendererContext) as any
  expect(option.legend.formatter('value')).toBe('Revenue variance')
  expect(option.legend.data).toEqual([{ name: 'value', itemStyle: { color: {
    type: 'linear', x: 0, y: 0, x2: 1, y2: 0, colorStops: [
      { offset: 0, color: defaultRendererContext.colors.danger }, { offset: 0.5, color: defaultRendererContext.colors.danger },
      { offset: 0.5, color: defaultRendererContext.colors.success }, { offset: 1, color: defaultRendererContext.colors.success },
    ],
  } } }])
  envelope.dataState.datasets[0]!.rows = [['Above', 10]]
  expect((echartsOption(envelope, defaultRendererContext) as any).legend.data[0].itemStyle.color).toBe(defaultRendererContext.colors.success)
  envelope.spec.presentation.legend = 'hidden'
  expect((echartsOption(envelope, defaultRendererContext) as any).legend).toBeUndefined()
})

for (const variant of ['single measure', 'multiple measures', 'category series']) {
  test(`ordinary bars retain every ${variant} legend entry and native selection`, () => {
    const envelope = cartesianFixture('bar', variant === 'multiple measures' ? ['label', 'revenue', 'forecast'] : ['label', 'value'])
    if (envelope.spec.kind !== 'cartesian' || envelope.dataState.kind !== 'inline') throw new Error('invalid fixture')
    let names = ['value']
    if (variant === 'multiple measures') {
      envelope.spec.datasets[0]!.fields[1]!.label = 'Revenue'
      envelope.spec.datasets[0]!.fields[2]!.label = 'Forecast'
      names = ['Revenue', 'Forecast']
    } else if (variant === 'category series') {
      envelope.spec.datasets[0]!.fields.push({ id: 'group', role: 'dimension', dataType: 'string', nullable: false, label: 'Group' })
      envelope.spec.series = { dataset: 'primary', field: 'group' }
      envelope.dataState.datasets[0]!.columns.push('group')
      envelope.dataState.datasets[0]!.rows = [['A', 10, 'Retail'], ['A', 20, 'Wholesale'], ['B', 30, 'Online']]
      names = ['Online', 'Retail', 'Wholesale']
    }
    names.sort()
    const chart: any = echarts.init(null, null, { renderer: 'svg', ssr: true, width: 768, height: 400 })
    const handle = new EChartsHandle({} as HTMLElement, {} as HTMLElement, chart, new CategoryColorRegistry())
    const activeNames = () => chart.getModel().getCurrentSeriesIndices()
      .map((index: number) => chart.getModel().getSeriesByIndex(index))
      .filter((series: any) => series.subType === 'bar').map((series: any) => series.name).sort()
    const legendLabels = () => {
      const model = chart.getModel().getComponent('legend')
      const labels: string[] = []
      chart.getViewOfComponentModel(model).getContentGroup().traverse((element: any) => {
        if (element.type === 'text') labels.push(element.style.text)
      })
      return labels.sort()
    }
    try {
      handle.mount(envelope, defaultRendererContext)
      void handle.whenReady().catch(() => {})
      handle.resize(768, 400)
      chart.resize({ width: 768, height: 400 })
      expect(legendLabels()).toEqual(names)
      expect(activeNames()).toEqual(names)
      const selectedName = names[0]!
      chart.dispatchAction({ type: 'legendUnSelect', name: selectedName })
      expect(activeNames()).toEqual(names.filter((name) => name !== selectedName))
      handle.resize(375, 400)
      chart.resize({ width: 375, height: 400 })
      expect(legendLabels()).toEqual(names)
      expect(chart.getModel().getComponent('legend').isSelected(selectedName)).toBe(false)
      expect(activeNames()).toEqual(names.filter((name) => name !== selectedName))
      chart.dispatchAction({ type: 'legendSelect', name: selectedName })
      expect(activeNames()).toEqual(names)
    } finally { handle.dispose() }
  })
}
