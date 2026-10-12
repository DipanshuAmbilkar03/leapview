package snapshot

import "errors"

// WithAdminApprovedRoleBindings overlays only typed, administrator-approved
// assignments on the current graph. Unapproved additions remain staged. The
// captured graph, grants, data policies, and serving identity are retained.
func (s AuthorizationSnapshot) WithAdminApprovedRoleBindings(current []RoleBinding) (AuthorizationSnapshot, error) {
	restricted, err := s.RestrictToCurrentRoleBindings(current)
	if err != nil {
		return AuthorizationSnapshot{}, err
	}
	byID := make(map[string]RoleBinding)
	for _, binding := range restricted.roleBindings {
		byID[binding.ID] = binding
	}
	for _, binding := range current {
		if !binding.AdminApproved {
			continue
		}
		if !binding.TypedRoleBinding() {
			return AuthorizationSnapshot{}, errors.New("admin-approved assignment must use typed permissions")
		}
		// The approval belongs to the mutable target policy, not to the immutable
		// serving artifact. Runtime authority remains its exact permission pairs.
		binding.AdminApproved = false
		byID[binding.ID] = binding
	}
	bindings := make([]RoleBinding, 0, len(byID))
	for _, binding := range byID {
		bindings = append(bindings, binding)
	}
	return NewAuthorizationSnapshotWithRoleBindings(s.identity, s.project, bindings, restricted.grants, s.dataPolicies)
}
