package module

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/access"
)

func TestLoginRedirectsSignedInUsersToTheApp(t *testing.T) {
	store := testStore(t)
	if _, err := store.repository.SetPlatformRole(t.Context(), access.PlatformRoleInput{
		PrincipalID: DevelopmentPrincipalID, Email: "dev@localhost", DisplayName: "Local Developer", Role: access.PlatformRoleAdmin,
	}); err != nil {
		t.Fatal(err)
	}
	auth := mustNewAuth(t, store.repository, AuthConfig{LocalAuth: true, DevelopmentLogin: true, CSRFKey: strings.Repeat("k", 32)})
	module, err := newSurface(surfaceConfig{Repository: func() (access.Repository, error) { return store.repository, nil }, Auth: auth})
	if err != nil {
		t.Fatal(err)
	}
	signIn := httptest.NewRecorder()
	auth.DevelopmentLogin(signIn, httptest.NewRequest(http.MethodPost, "/auth/development/login", nil))
	cookies := signIn.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("development login cookies = %#v", cookies)
	}
	login := func(target string, signedIn bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if signedIn {
			request.AddCookie(cookies[0])
		}
		recorder := httptest.NewRecorder()
		module.Login(recorder, request)
		return recorder
	}

	if response := login("/login", true); response.Code != http.StatusFound || response.Header().Get("Location") != "/" {
		t.Fatalf("signed-in /login = %d location %q, want redirect to /", response.Code, response.Header().Get("Location"))
	}
	if response := login("/login", false); response.Code != http.StatusOK {
		t.Fatalf("signed-out /login = %d, want the form", response.Code)
	}
	// An explicit outcome such as a denied page must still be explained
	// instead of bouncing the user back into the page that rejected them.
	if response := login("/login?error=forbidden", true); response.Code != http.StatusOK {
		t.Fatalf("signed-in /login?error=forbidden = %d, want the explanation", response.Code)
	}
}
