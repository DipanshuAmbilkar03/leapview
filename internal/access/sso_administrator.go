package access

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// NominateSSOAdministrator is a trusted installation-operator boundary.
// It never discovers or links a recipient by email and is not an HTTP action.
func NominateSSOAdministrator(ctx context.Context, repo Repository, principalID string, apply bool) error {
	id, err := uuid.Parse(principalID)
	if err != nil || id.String() != principalID {
		return errors.New("a canonical principal UUID is required")
	}
	validate := func(repository Repository) (Principal, error) {
		principal, err := repository.PrincipalByID(ctx, principalID)
		if err != nil {
			return Principal{}, err
		}
		if principal.Kind != PrincipalKindUser || principal.AccessDisabled() {
			return Principal{}, ErrForbidden
		}
		reader, ok := repository.(PrincipalIdentityManagementRepository)
		if !ok {
			return Principal{}, errors.New("identity management is unavailable")
		}
		identity, err := reader.PrincipalIdentityManagement(ctx, principalID)
		if err != nil {
			return Principal{}, err
		}
		if identity.Source != IdentityManagementExternal || identity.Provider != "oidc" {
			return Principal{}, errors.New("administrator nomination requires an existing OIDC identity")
		}
		return principal, nil
	}
	if !apply {
		_, err := validate(repo)
		return err
	}
	audited, ok := repo.(AuditedMutationRepository)
	if !ok {
		return errors.New("atomic administrator audit is unavailable")
	}
	return audited.RunAuditedMutation(ctx, func(tx Repository) (AuditEventInput, error) {
		principal, err := validate(tx)
		if err != nil {
			return AuditEventInput{}, err
		}
		_, err = tx.SetPlatformRole(ctx, PlatformRoleInput{
			PrincipalID: principal.ID, Email: principal.Email, DisplayName: principal.DisplayName, Role: PlatformRoleAdmin,
		})
		return AuditEventInput{
			Action: "platform_admin.nominated", ResourceKind: "principal", ResourceID: principal.ID,
			Status: "success", MetadataJSON: `{"surface":"offline_operator","identitySource":"oidc"}`,
		}, err
	})
}
