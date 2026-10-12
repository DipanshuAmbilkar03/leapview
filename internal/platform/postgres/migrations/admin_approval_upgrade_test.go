package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminApprovalUpgradePreservesExistingRoleHistory(t *testing.T) {
	pool, db, provider := newDemoUpgradeDatabase(t)
	_, err := provider.UpTo(t.Context(), 66)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO access.authorization_policy(target_id,project_id,environment,revision,digest) VALUES('approval-upgrade','project','production',1,'sha256:'||repeat('a',64)); INSERT INTO access.authorization_policy_revision(target_id,project_id,environment,revision,digest) VALUES('approval-upgrade','project','production',1,'sha256:'||repeat('a',64)); INSERT INTO access.authorization_policy_role_binding(target_id,project_id,environment,revision,id,subject_kind,subject_id,role,capabilities) VALUES('approval-upgrade','project','production',1,'legacy','principal','user','viewer','["RESOURCE_READ"]')`)
	require.NoError(t, err)
	var before string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT row_to_json(binding)::text FROM access.authorization_policy_role_binding binding WHERE target_id='approval-upgrade'`).Scan(&before))
	_, err = provider.Up(t.Context())
	require.NoError(t, err)
	require.NoError(t, VerifyGoose(t.Context(), db))
	var after string
	var approved bool
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT (to_jsonb(binding)-'admin_approved')::json, admin_approved FROM access.authorization_policy_role_binding binding WHERE target_id='approval-upgrade'`).Scan(&after, &approved))
	// JSON key order changes under jsonb; compare decoded objects.
	require.JSONEq(t, before, after)
	require.False(t, approved, "existing assignments must not be retroactively approved")
	_, err = pool.Exec(t.Context(), `INSERT INTO access.authorization_policy_role_binding(target_id,project_id,environment,revision,id,subject_kind,subject_id,role,capabilities,admin_approved) VALUES('approval-upgrade','project','production',1,'forged','principal','other','viewer','["RESOURCE_READ"]',true)`)
	require.Error(t, err, "legacy assignments cannot carry immediate approval")
}
