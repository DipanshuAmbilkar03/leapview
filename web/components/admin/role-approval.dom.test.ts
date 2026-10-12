import { afterAll, beforeAll, expect, test } from 'bun:test'
import { startAdminPageTestFixture, stopAdminPageTestFixture } from './admin-page.test-fixture'

let fixture: Awaited<ReturnType<typeof startAdminPageTestFixture>>
beforeAll(async () => { fixture = await startAdminPageTestFixture() })
afterAll(async () => { await stopAdminPageTestFixture(fixture) })

test('pending role can be approved using the same identity, role, binding and current revision', async () => {
  const page = await fixture.browser.newPage()
  try {
    await page.goto(fixture.baseURL)
    await page.waitForFunction(() => customElements.get('lv-admin-page'))
    await page.evaluate(async () => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ page: { kind: 'admin', title: 'User', active: 'principal-detail' }, adminAccess: {
        principals: [{ id: 'user', kind: 'user', email: 'user@example.test', displayName: 'User', groups: [], capabilities: {} }], selectedPrincipalId: 'user', groups: [], sessions: [], rolePresets: [{ role: 'viewer', name: 'Viewer', permissions: ['dashboard.read'] }], permissionCatalog: [], policyRevision: 7, loading: false, activity: [],
        roleAssignments: [{ bindingId: 'binding-user', role: 'viewer', subjectType: 'principal', subjectId: 'user', subjectName: 'User', status: 'Pending activation', permissions: ['dashboard.read'] }],
      } })
      ;(window as any).approvalCommands = []
      document.addEventListener('lv-access-admin-command', (event) => (window as any).approvalCommands.push((event as CustomEvent).detail))
    })
    await page.getByRole('button', { name: 'Approve viewer access for User', exact: true }).click()
    expect(await page.evaluate(() => (window as any).approvalCommands)).toEqual([{ action: 'grant_role', bindingId: 'binding-user', subjectType: 'principal', subjectId: 'user', role: 'viewer', expectedRevision: 7 }])
    await page.evaluate(async () => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ adminAccess: { roleAssignments: [{ bindingId: 'binding-user', role: 'viewer', subjectType: 'principal', subjectId: 'user', status: 'Active', permissions: ['dashboard.read'] }] } })
    })
    await page.getByText('1 permission across 1 area · Active', { exact: true }).waitFor()
    expect(await page.getByRole('button', { name: /Approve .* access/ }).count()).toBe(0)
  } finally { await page.close() }
})
