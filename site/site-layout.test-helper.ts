import type { Page } from '@playwright/test'

type LayoutShift = { value: number; sources: string[] }

export async function currentDocsLinkIsVisibleInSidebar(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const sidebar = document.querySelector('.site-docs-sidebar')!.getBoundingClientRect()
    const current = document.querySelector('.site-docs-link-current')!.getBoundingClientRect()
    return current.height > 0 && current.top >= sidebar.top - 1 && current.bottom <= sidebar.bottom + 1
  })
}

// Record layout-shift entries from the first paint so a test can total them
// after the streamed content has arrived.
export async function observeLayoutShifts(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const shifts: LayoutShift[] = []
    ;(window as unknown as { layoutShifts: LayoutShift[] }).layoutShifts = shifts
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries() as unknown as Array<{ value: number; hadRecentInput: boolean; sources?: Array<{ node?: Node }> }>) {
        if (entry.hadRecentInput) continue
        shifts.push({ value: entry.value, sources: (entry.sources ?? []).map((source) => (source.node as Element | undefined)?.nodeName ?? '?') })
      }
    }).observe({ type: 'layout-shift', buffered: true })
  })
}

export async function layoutShiftsOf(page: Page): Promise<{ total: number; shifts: LayoutShift[] }> {
  const shifts = await page.evaluate(() => (window as unknown as { layoutShifts: LayoutShift[] }).layoutShifts)
  return { total: shifts.reduce((sum, shift) => sum + shift.value, 0), shifts }
}
