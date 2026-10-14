import { expect, test } from 'bun:test'
import { buildFlowGridCooperatively } from './topology-flow'

test('cooperative flow grid keeps the synchronous row-major coordinates and values', async () => {
  const flowWidth = 4.5
  const flowHeight = 3.25
  const noiseSize = 0.003
  const sampled: Array<[number, number]> = []
  let clock = 0
  let yields = 0
  let queuedTaskRan = false
  setTimeout(() => { queuedTaskRan = true }, 0)

  const grid = await buildFlowGridCooperatively({
    flowWidth,
    flowHeight,
    noiseSize,
    calculateCell: (x, y) => {
      sampled.push([x, y])
      return Math.sin(x * 1000) + Math.cos(y * 1000)
    },
    timeBudgetMs: 3,
    now: () => clock++,
    yieldToMainThread: () => new Promise((resolve) => setTimeout(() => {
      yields += 1
      resolve()
    }, 0)),
  })

  const expectedGrid: number[][] = []
  const expectedCoordinates: Array<[number, number]> = []
  for (let rowIndex = 0; rowIndex < flowHeight; rowIndex += 1) {
    const row: number[] = []
    for (let columnIndex = 0; columnIndex < flowWidth; columnIndex += 1) {
      const x = columnIndex * noiseSize
      const y = rowIndex * noiseSize
      expectedCoordinates.push([x, y])
      row.push(Math.sin(x * 1000) + Math.cos(y * 1000))
    }
    expectedGrid.push(row)
  }

  expect(grid).toEqual(expectedGrid)
  expect(sampled).toEqual(expectedCoordinates)
  expect(yields).toBeGreaterThan(0)
  expect(queuedTaskRan).toBe(true)
})

test('cooperative flow grid discards partial work after cancellation', async () => {
  let clock = 0
  let samples = 0
  let cancelled = false

  const grid = await buildFlowGridCooperatively({
    flowWidth: 8,
    flowHeight: 8,
    noiseSize: 0.003,
    calculateCell: () => { samples += 1; return samples },
    shouldCancel: () => cancelled,
    timeBudgetMs: 1,
    now: () => { clock += 2; return clock },
    yieldToMainThread: async () => { cancelled = true },
  })

  expect(grid).toBeUndefined()
  expect(samples).toBe(1)
})
