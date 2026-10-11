package app

import (
	deploymentmodule "github.com/flidai/leapview/internal/deployment/module"
	refreshmodule "github.com/flidai/leapview/internal/refresh/module"
)

// nativeAPIIdempotencyOwners lists commands whose native handler owns the
// durable operation and its replay contract.
func nativeAPIIdempotencyOwners() map[string]struct{} {
	return map[string]struct{}{
		// Plans commit their operation, plan, event and audit together. Builds
		// own reservation/attempt fencing and reconcile indeterminate physical
		// effects before replay. An outer HTTP attempt would quarantine a lost
		// response or cache a temporary busy result ahead of those authorities.
		deploymentmodule.CreateDeliveryPlanOperationID:                  {},
		deploymentmodule.BuildDeliveryPlanOperationID:                   {},
		refreshmodule.CreateRefreshRunOperationID:                       {},
		refreshmodule.CancelRefreshRunOperationID:                       {},
		deploymentmodule.PlanProjectCandidateSynchronizationOperationID: {},
		"exchangeProjectClaimPublisher":                                 {},
	}
}
