package module

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/access"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

func requireAuthorizationProblem(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d (body=%q)", recorder.Code, status, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/problem+json") {
		t.Fatalf("content type = %q, want application/problem+json (body=%q)", contentType, recorder.Body.String())
	}
	var problem struct {
		Code      string `json:"code"`
		Status    int    `json:"status"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("body is not JSON: %v: %q", err, recorder.Body.String())
	}
	if problem.Code != code || problem.Status != status || problem.RequestID == "" {
		t.Fatalf("problem = %+v, want code %s status %d with a request id", problem, code, status)
	}
}

func TestWriteAuthorizationStatusUsesProblemJSONForAPIPaths(t *testing.T) {
	for status, code := range map[int]string{
		http.StatusUnauthorized:       "AUTHENTICATION_REQUIRED",
		http.StatusForbidden:          "AUTHORIZATION_DENIED",
		http.StatusNotFound:           "RESOURCE_NOT_FOUND",
		http.StatusServiceUnavailable: "AUTHORIZATION_UNAVAILABLE",
	} {
		recorder := httptest.NewRecorder()
		writeAuthorizationStatus(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/principals", nil), status)
		requireAuthorizationProblem(t, recorder, status, code)
	}
}

func TestWriteAuthorizationStatusKeepsPlaintextOutsideTheAPI(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeAuthorizationStatus(recorder, httptest.NewRequest(http.MethodGet, "/admin/system", nil), http.StatusUnauthorized)
	if recorder.Code != http.StatusUnauthorized || strings.HasPrefix(recorder.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("browser response = %d %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
}

func TestPlatformAPIAuthorizationFailuresAreProblemJSON(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("protected handler ran") })
	request := func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/v1/principals", nil) }

	recorder := httptest.NewRecorder()
	browserGuardModule(nil, Principal{}, false).RequirePlatformAdmin(next).ServeHTTP(recorder, request())
	requireAuthorizationProblem(t, recorder, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED")

	recorder = httptest.NewRecorder()
	browserGuardModule(browserGuardRepository{}, Principal{ID: "member"}, true).RequirePlatformAdmin(next).ServeHTTP(recorder, request())
	requireAuthorizationProblem(t, recorder, http.StatusForbidden, "AUTHORIZATION_DENIED")

	recorder = httptest.NewRecorder()
	browserGuardModule(browserGuardRepository{err: errors.New("role lookup failed")}, Principal{ID: "member"}, true).RequirePlatformAdmin(next).ServeHTTP(recorder, request())
	requireAuthorizationProblem(t, recorder, http.StatusServiceUnavailable, "AUTHORIZATION_UNAVAILABLE")
	if strings.Contains(recorder.Body.String(), "role lookup failed") {
		t.Fatalf("problem leaks the cause: %s", recorder.Body.String())
	}
}

func TestInstanceAPIAuthorizationFailuresAreProblemJSON(t *testing.T) {
	contract := APIGenOperationContract{
		OperationID: "exchangeProjectClaimPublisher", Method: http.MethodPost,
		Path:      "/api/v1/projects/{project}/project-claim-publisher/exchange",
		Protected: true, AuthzMode: "privilege", Action: string(access.ActionInstanceProjectClaim),
		Resolver:   string(access.TypedOperationResolverInstance),
		Extensions: map[string]any{apiGenObjectScopeExtension: "instance"},
	}
	protect := func(module *Module) http.Handler {
		authorizer, err := module.APIGenAuthorizer(nil, map[string]APIGenOperationContract{contract.OperationID: contract}, APIGenResourceResolvers{
			Instance: func(*http.Request) string { return "instance_demo" },
		})
		if err != nil {
			t.Fatal(err)
		}
		handler, ok := authorizer.Protect(contract.OperationID, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("protected handler ran") }))
		if !ok {
			t.Fatal("operation was not protected")
		}
		return handler
	}
	request := func() *http.Request {
		return httptest.NewRequest(http.MethodPost, "/api/v1/projects/project_demo/project-claim-publisher/exchange", nil)
	}

	recorder := httptest.NewRecorder()
	protect(browserGuardModule(nil, Principal{}, false)).ServeHTTP(recorder, request())
	requireAuthorizationProblem(t, recorder, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED")

	// A claim operation without a bearer token is a 401 even for an admin session.
	recorder = httptest.NewRecorder()
	protect(browserGuardModule(browserGuardRepository{admin: true}, Principal{ID: "admin"}, true)).ServeHTTP(recorder, request())
	requireAuthorizationProblem(t, recorder, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED")

	bearer := request()
	bearer.Header.Set("Authorization", "Bearer test-token")
	recorder = httptest.NewRecorder()
	protect(browserGuardModule(browserGuardRepository{}, Principal{ID: "member"}, true)).ServeHTTP(recorder, bearer)
	requireAuthorizationProblem(t, recorder, http.StatusForbidden, "AUTHORIZATION_DENIED")

	recorder = httptest.NewRecorder()
	protect(browserGuardModule(browserGuardRepository{err: errors.New("role lookup failed")}, Principal{ID: "member"}, true)).ServeHTTP(recorder, bearer)
	requireAuthorizationProblem(t, recorder, http.StatusServiceUnavailable, "AUTHORIZATION_UNAVAILABLE")
}

func TestProjectLocatorMismatchIsProblemJSON(t *testing.T) {
	contract := APIGenOperationContract{
		OperationID: "boundMetadata", Method: http.MethodGet, Path: "/api/v1/projects/{project}/metadata",
		Protected: true, AuthzMode: "authenticated", Extensions: map[string]any{apiGenObjectScopeExtension: "principal"},
	}
	module := browserGuardModule(browserGuardRepository{}, Principal{ID: "member"}, true)
	authorizer, err := module.APIGenAuthorizer(apigenRuntimeFake{project: projectgraph.ResourceID("project_demo")}, map[string]APIGenOperationContract{contract.OperationID: contract}, APIGenResourceResolvers{})
	if err != nil {
		t.Fatal(err)
	}
	handler, ok := authorizer.Protect(contract.OperationID, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("protected handler ran") }))
	if !ok {
		t.Fatal("operation was not protected")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, apigenRequest(http.MethodGet, "/api/v1/projects/foreign/metadata", map[string]string{"project": "foreign"}))
	requireAuthorizationProblem(t, recorder, http.StatusNotFound, "RESOURCE_NOT_FOUND")
}
