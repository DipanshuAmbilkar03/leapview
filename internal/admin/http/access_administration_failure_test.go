package http

import (
	"bytes"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	adminsettings "github.com/flidai/leapview/internal/admin/settings"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestAccessAdministrationFailureMessageLogsCauseAndHidesSQLState(t *testing.T) {
	var logs bytes.Buffer
	handler := Handler{ReadModel: ReadModel{Logger: slog.New(slog.NewTextHandler(&logs, nil))}}
	request := httptest.NewRequest(nethttp.MethodPost, "/admin/principals/commands", nil)
	cause := &pgconn.PgError{Code: "23514", ConstraintName: "principal_display_name_check", Message: `new row for relation "principal" violates check constraint "principal_display_name_check"`}

	message := handler.accessAdministrationFailureMessage(request, adminsettings.AccessAdministrationCommand{Action: "create_principal"}, cause)

	require.NotContains(t, message, "SQLSTATE")
	require.NotContains(t, message, "constraint")
	require.Contains(t, logs.String(), "access administration command failed")
	require.Contains(t, logs.String(), "principal_display_name_check")
	require.Contains(t, logs.String(), "create_principal")

	logs.Reset()
	message = handler.accessAdministrationFailureMessage(request, adminsettings.AccessAdministrationCommand{Action: "create_principal"}, &adminsettings.AccessAdministrationFieldError{Field: "email", Message: "Enter an email address such as name@example.com."})
	require.Equal(t, "Enter an email address such as name@example.com.", message)
	require.Empty(t, logs.String())
}
