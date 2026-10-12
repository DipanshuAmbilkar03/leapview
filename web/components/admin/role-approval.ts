import { html, nothing } from 'lit'
import type { AccessAdministrationSignal, AccessRoleAssignmentSignal } from '../../generated/signals'

export function renderRoleApproval(assignment: AccessRoleAssignmentSignal, signal: AccessAdministrationSignal) {
  if (assignment.status !== 'Pending activation' || !signal.rolePresets.some((preset) => preset.role === assignment.role)) return nothing
  const approve = (event: Event) => {
    if (signal.roleMutationUnavailableReason || signal.loading) return
    ;(event.currentTarget as HTMLElement).dispatchEvent(new CustomEvent('lv-access-admin-command', {
      bubbles: true, composed: true,
      detail: { action: 'grant_role', bindingId: assignment.bindingId, subjectType: assignment.subjectType, subjectId: assignment.subjectId, role: assignment.role, expectedRevision: signal.policyRevision },
    }))
  }
  return html`<button class="role-approval" type="button" aria-label=${`Approve ${assignment.role} access for ${assignment.subjectName || assignment.subjectId}`} ?disabled=${signal.loading || !!signal.roleMutationUnavailableReason} @click=${approve}>Approve access</button>`
}
