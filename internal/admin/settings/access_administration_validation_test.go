package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/access"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// failingRepository panics if validation lets a command reach storage.
type failingRepository struct{ access.Repository }

func TestValidateAccessAdministrationCommandRejectsMalformedPrincipalFields(t *testing.T) {
	tests := []struct {
		name    string
		command AccessAdministrationCommand
		message string
	}{
		{"oversized display name", AccessAdministrationCommand{Action: "create_principal", Email: "a@example.com", DisplayName: strings.Repeat("n", 3000)}, "Display name must be 120 characters or fewer."},
		{"oversized email", AccessAdministrationCommand{Action: "create_principal", Email: strings.Repeat("a", 300) + "@example.com", DisplayName: "Ada"}, "Email must be 254 characters or fewer."},
		{"email without domain", AccessAdministrationCommand{Action: "create_principal", Email: "ada", DisplayName: "Ada"}, "Enter an email address such as name@example.com."},
		{"email with display name syntax", AccessAdministrationCommand{Action: "create_principal", Email: "Ada <ada@example.com>", DisplayName: "Ada"}, "Enter an email address such as name@example.com."},
		{"control characters in name", AccessAdministrationCommand{Action: "create_principal", Email: "a@example.com", DisplayName: "Ada\x00Lovelace"}, "Display name cannot contain control characters."},
		{"oversized rename", AccessAdministrationCommand{Action: "update_principal", PrincipalID: "p", DisplayName: strings.Repeat("é", 121)}, "Display name must be 120 characters or fewer."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateAccessAdministrationCommand(NormalizeAccessAdministrationCommand(test.command))
			var fieldErr *AccessAdministrationFieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, test.message, err.Error())
		})
	}
}

func TestValidateAccessAdministrationCommandAcceptsBoundaryValues(t *testing.T) {
	command := AccessAdministrationCommand{Action: "create_principal", Email: "ada@example.com", DisplayName: strings.Repeat("é", 120)}
	require.NoError(t, ValidateAccessAdministrationCommand(NormalizeAccessAdministrationCommand(command)))
	require.NoError(t, ValidateAccessAdministrationCommand(AccessAdministrationCommand{Action: "block_principal", PrincipalID: "p"}))
}

func TestApplyAccessAdministrationCommandRejectsInvalidInputBeforeTouchingStorage(t *testing.T) {
	_, err := ApplyAccessAdministrationCommand(context.Background(), failingRepository{}, "actor", AccessAdministrationCommand{Action: "create_principal", Email: "a@example.com", DisplayName: strings.Repeat("x", 3000)})
	var fieldErr *AccessAdministrationFieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "displayName", fieldErr.Field)
}

func TestAccessAdministrationUserErrorHidesDatabaseDiagnostics(t *testing.T) {
	cause := fmt.Errorf("insert principal: %w", &pgconn.PgError{Code: "23514", ConstraintName: "principal_display_name_check", Message: `new row for relation "principal" violates check constraint`})
	message, internal := AccessAdministrationUserError(cause)
	require.True(t, internal)
	require.NotContains(t, message, "SQLSTATE")
	require.NotContains(t, message, "constraint")
	require.NotEmpty(t, message)

	message, internal = AccessAdministrationUserError(&AccessAdministrationFieldError{Field: "email", Message: "Enter an email address such as name@example.com."})
	require.False(t, internal)
	require.Equal(t, "Enter an email address such as name@example.com.", message)

	message, internal = AccessAdministrationUserError(errors.New("you cannot delete your own account"))
	require.False(t, internal)
	require.Equal(t, "you cannot delete your own account", message)
}
