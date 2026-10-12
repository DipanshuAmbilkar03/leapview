package postgres

import (
	"context"

	accessdb "github.com/flidai/leapview/internal/access/postgres/internal/db"
)

// RevokeAllSessionsForPrincipal revokes browser, desktop, and authoring sessions,
// including their access and refresh credentials, in the caller's transaction.
// Independently issued personal API tokens and service secrets remain intact.
func (r *Repository) RevokeAllSessionsForPrincipal(ctx context.Context, pid string) error {
	pid, err := uuidID("principal id", pid)
	if err != nil {
		return err
	}
	principalID, err := pgUUID(pid)
	if err != nil {
		return err
	}
	tx, ownTx, err := r.txOrBegin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if ownTx {
			_ = tx.Rollback(ctx)
		}
	}()
	queries := accessdb.New(tx)
	if err := queries.RevokePrincipalSessions(ctx, principalID); err != nil {
		return err
	}
	if err := queries.RevokePrincipalAuthoringSessions(ctx, principalID); err != nil {
		return err
	}
	if err := queries.RevokePrincipalOAuthSessions(ctx, principalID); err != nil {
		return err
	}
	if ownTx {
		return tx.Commit(ctx)
	}
	return nil
}
