package settings

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	// MaxPrincipalEmailLength is the RFC 5321 forward-path limit.
	MaxPrincipalEmailLength = 254
	// MaxPrincipalDisplayNameLength counts characters, not bytes.
	MaxPrincipalDisplayNameLength = 120
)

// AccessAdministrationFieldError is a user-correctable validation failure tied to one form field.
// Its message is written for the person filling in the form and is always safe to display.
type AccessAdministrationFieldError struct {
	Field   string
	Message string
}

func (e *AccessAdministrationFieldError) Error() string { return e.Message }

// ValidateAccessAdministrationCommand checks principal profile fields before any storage work so
// malformed input never reaches a database constraint. The command must already be normalized.
func ValidateAccessAdministrationCommand(command AccessAdministrationCommand) error {
	switch command.Action {
	case "create_principal":
		if command.Email != "" {
			if err := validatePrincipalEmail(command.Email); err != nil {
				return err
			}
		}
		return validatePrincipalDisplayName(command.DisplayName)
	case "update_principal":
		return validatePrincipalDisplayName(command.DisplayName)
	}
	return nil
}

func validatePrincipalEmail(email string) error {
	if utf8.RuneCountInString(email) > MaxPrincipalEmailLength {
		return &AccessAdministrationFieldError{Field: "email", Message: fmt.Sprintf("Email must be %d characters or fewer.", MaxPrincipalEmailLength)}
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Name != "" || parsed.Address != email {
		return &AccessAdministrationFieldError{Field: "email", Message: "Enter an email address such as name@example.com."}
	}
	return nil
}

func validatePrincipalDisplayName(name string) error {
	if utf8.RuneCountInString(name) > MaxPrincipalDisplayNameLength {
		return &AccessAdministrationFieldError{Field: "displayName", Message: fmt.Sprintf("Display name must be %d characters or fewer.", MaxPrincipalDisplayNameLength)}
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return &AccessAdministrationFieldError{Field: "displayName", Message: "Display name cannot contain control characters."}
	}
	return nil
}

// AccessAdministrationUserError returns the message to show an administrator for a failed command.
// internal reports that err carried storage diagnostics that were replaced by a generic message;
// the caller must log err instead of displaying it.
func AccessAdministrationUserError(err error) (message string, internal bool) {
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) {
		return err.Error(), false
	}
	switch databaseError.Code {
	case "22001", "22021", "22P05", "23514":
		return "One of the values is too long or uses characters that are not allowed. Shorten it and try again.", true
	case "23505":
		return "That value is already in use.", true
	}
	return "The change could not be saved. Try again, and contact an administrator if it keeps failing.", true
}
