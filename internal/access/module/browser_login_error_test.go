package module

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginErrorMessagesCoverPasswordChangeOutcomesAndIgnoreUnknownCodes(t *testing.T) {
	for code, want := range map[string]string{
		"password_policy":            "at least 12 characters",
		"current_password_incorrect": "temporary password is incorrect",
		"too_many_attempts":          "Too many sign-in attempts",
	} {
		request := httptest.NewRequest("GET", "/login?error="+code, nil)
		if got := loginErrorCode(request); got != code {
			t.Errorf("loginErrorCode(%q) = %q", code, got)
		}
		if got := loginErrorMessage(request); !strings.Contains(got, want) {
			t.Errorf("loginErrorMessage(%q) = %q, want it to contain %q", code, got, want)
		}
	}
	for _, code := range []string{"unknown", "<script>", ""} {
		request := httptest.NewRequest("GET", "/login?error="+code, nil)
		if loginErrorCode(request) != "" || loginErrorMessage(request) != "" {
			t.Errorf("code %q reached the login page", code)
		}
	}
}
