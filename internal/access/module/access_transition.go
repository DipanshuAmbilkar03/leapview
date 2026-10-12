package module

import "github.com/flidai/leapview/internal/access"

// Keep module consumers source-compatible while the semantic transition
// contract remains independent of browser authentication and HTTP adapters.
type AccessTransitionRoleIntent = access.AccessTransitionRoleIntent
type AccessTransitionGrantIntent = access.AccessTransitionGrantIntent
type AccessTransitionIntent = access.AccessTransitionIntent
type AccessTransitionPlan = access.AccessTransitionPlan
