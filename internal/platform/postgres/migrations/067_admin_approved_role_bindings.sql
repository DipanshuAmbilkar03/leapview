-- +goose Up
SET LOCAL ROLE leapview_control_owner;
ALTER TABLE access.authorization_policy_role_binding
 ADD COLUMN admin_approved boolean NOT NULL DEFAULT false,
 ADD CONSTRAINT authorization_policy_role_binding_admin_approved_typed
 CHECK (NOT admin_approved OR (permission_profile IS NOT NULL AND permission_profile = 'leapview.permissions/v1'));
RESET ROLE;

-- +goose Down
SET LOCAL ROLE leapview_control_owner;
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'admin-approved role binding history is immutable';
END $$;
-- +goose StatementEnd
RESET ROLE;
