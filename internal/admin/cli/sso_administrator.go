package cli

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type SSOAdministratorRequest struct {
	InstanceID  string
	PrincipalID string
	Apply       bool
}

func (r SSOAdministratorRequest) Validate() error {
	if r.InstanceID == "" {
		return errors.New("the exact instance ID is required")
	}
	id, err := uuid.Parse(r.PrincipalID)
	if err != nil || id.String() != r.PrincipalID {
		return errors.New("a canonical principal UUID is required")
	}
	return nil
}

type SSOAdministratorOperations interface {
	NominateSSOAdministrator(context.Context, SSOAdministratorRequest, io.Writer) error
}

func ssoAdministratorCommand(ctx context.Context, operations Operations) *cobra.Command {
	request := SSOAdministratorRequest{}
	command := &cobra.Command{
		Use: "nominate-sso-admin", Short: "Nominate an existing OIDC principal as an instance administrator",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"leapview.dev/effect": "write", "leapview.dev/confirmation": "required"},
		RunE: func(command *cobra.Command, _ []string) error {
			if err := request.Validate(); err != nil {
				return err
			}
			operator, ok := operations.(SSOAdministratorOperations)
			if !ok {
				return errors.New("SSO administrator nomination is unavailable")
			}
			return operator.NominateSSOAdministrator(ctx, request, command.OutOrStdout())
		},
	}
	command.Flags().StringVar(&request.InstanceID, "instance", "", "exact durable instance ID")
	command.Flags().StringVar(&request.PrincipalID, "principal", "", "verified OIDC principal UUID; email matching is never used")
	command.Flags().BoolVar(&request.Apply, "apply", false, "assign the administrator role; otherwise preview only")
	return command
}
