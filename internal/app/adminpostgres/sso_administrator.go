package adminpostgres

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/flidai/leapview/internal/access"
	accesspostgres "github.com/flidai/leapview/internal/access/postgres"
	admincli "github.com/flidai/leapview/internal/admin/cli"
	platformbootstrap "github.com/flidai/leapview/internal/platform/bootstrap/postgres"
	"github.com/flidai/leapview/internal/platform/typednil"
)

// The host operator already controls the installation credentials. This
// command requires the same exclusive instance lock as other offline commands.
func (o Operations) NominateSSOAdministrator(ctx context.Context, request admincli.SSOAdministratorRequest, out io.Writer) error {
	if out == nil {
		return errors.New("administrator nomination output is required")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	deps := o.Dependencies.withDefaults()
	cfg, err := deps.LoadConfig()
	if err != nil {
		return err
	}
	configuration, err := accessConfigForAdmin(cfg)
	if err != nil {
		return err
	}
	lock, err := deps.AcquireLock(cfg.HomeDir)
	if err != nil {
		return err
	}
	if typednil.IsNil(lock) {
		return errors.New("administrator nomination requires the instance lock")
	}
	defer lock.Release()
	pool, err := deps.OpenAccess(ctx, configuration)
	if err != nil {
		return err
	}
	if nilAccessPool(pool) {
		return errors.New("administrator nomination requires the control pool")
	}
	defer pool.Close()
	if err := deps.VerifyBaseline(ctx, pool); err != nil {
		return err
	}
	instanceID, err := platformbootstrap.New(pool).ExistingInstanceID(ctx)
	if err != nil {
		return err
	}
	if instanceID != request.InstanceID {
		return errors.New("administrator nomination instance does not match the configured installation")
	}
	key, err := accessFingerprintKey(cfg)
	if err != nil {
		return err
	}
	repo, err := accesspostgres.NewAccess(pool, accesspostgres.FingerprintConfig{Key: key})
	if err != nil {
		return err
	}
	initialized, err := repo.Initialized(ctx)
	if err != nil {
		return err
	}
	if !initialized {
		return errors.New("initialize the instance before nominating an SSO administrator")
	}
	if err := access.NominateSSOAdministrator(ctx, repo, request.PrincipalID, request.Apply); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(struct {
		InstanceID  string `json:"instanceId"`
		PrincipalID string `json:"principalId"`
		Role        string `json:"role"`
		Applied     bool   `json:"applied"`
	}{instanceID, request.PrincipalID, string(access.PlatformRoleAdmin), request.Apply})
}
