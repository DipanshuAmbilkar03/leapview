package app

import (
	"context"

	accessmodule "github.com/flidai/leapview/internal/access/module"
	agentmodule "github.com/flidai/leapview/internal/agent/module"
	analyticsmodule "github.com/flidai/leapview/internal/analytics/module"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
	workloadmodule "github.com/flidai/leapview/internal/workload/module"
)

// withAgentToolContext supplies query admission to tools that lease a raw runtime,
// such as draft preview. It carries the controller, not an active permit: each
// physical query acquires and releases its own lease in materialization.
func withAgentToolContext(ctx context.Context, principal accessmodule.Principal, scope agentmodule.Scope, admitter workloadmodule.Admitter) context.Context {
	// Durable runs already own a worker lease. Preserve that admission rather
	// than nesting an interactive preview request inside a background job.
	if _, _, admitted := workloadmodule.Current(ctx); !admitted {
		ctx = workloadmodule.WithAdmitter(ctx, admitter)
	}
	ctx = accessmodule.WithPrincipal(ctx, principal)
	ctx = withAgentCredential(ctx, principal, scope)
	if projectID, err := projectgraph.NewResourceID(scope.ProjectID); err == nil {
		ctx = analyticsmodule.WithAgentQueryMetadata(ctx, projectID, scope.PrincipalID)
	}
	return ctx
}
