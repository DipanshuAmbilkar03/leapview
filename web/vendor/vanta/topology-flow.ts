export type CooperativeFlowGridOptions<T> = {
  flowWidth: number
  flowHeight: number
  noiseSize: number
  calculateCell: (x: number, y: number) => T
  shouldCancel?: () => boolean
  yieldToMainThread?: () => Promise<void>
  timeBudgetMs?: number
  now?: () => number
}

const DEFAULT_TIME_BUDGET_MS = 4

export async function buildFlowGridCooperatively<T>({
  flowWidth,
  flowHeight,
  noiseSize,
  calculateCell,
  shouldCancel = () => false,
  yieldToMainThread = yieldToMainThreadTask,
  timeBudgetMs = DEFAULT_TIME_BUDGET_MS,
  now = () => performance.now(),
}: CooperativeFlowGridOptions<T>): Promise<T[][] | undefined> {
  const rowCount = Math.ceil(flowHeight)
  const columnCount = Math.ceil(flowWidth)
  const totalCellCount = rowCount * columnCount
  const grid = new Array<T[]>(rowCount)
  let cellIndex = 0
  let rowIndex = 0
  let columnIndex = 0
  let row: T[] = []
  let sliceStartedAt = now()

  while (cellIndex < totalCellCount) {
    if (shouldCancel()) return undefined

    row.push(calculateCell(columnIndex * noiseSize, rowIndex * noiseSize))
    cellIndex += 1
    columnIndex += 1

    if (columnIndex === columnCount) {
      grid[rowIndex] = row
      rowIndex += 1
      columnIndex = 0
      row = []
    }

    if (cellIndex < totalCellCount && now() - sliceStartedAt >= timeBudgetMs) {
      await yieldToMainThread()
      if (shouldCancel()) return undefined
      sliceStartedAt = now()
    }
  }

  return shouldCancel() ? undefined : grid
}

function yieldToMainThreadTask(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0))
}
