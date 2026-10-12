package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flidai/leapview/internal/access"
)

func TestAgentEditingRequiresPlatformAdminRatherThanProjectAdmin(t *testing.T) {
	for _, isPlatformAdmin := range []bool{false, true} {
		model := ReadModel{AuthConfigured: true, CurrentPrincipal: func(*http.Request) (Principal, bool) { return Principal{ID: "principal"}, true }, CurrentEffectiveCapabilities: func(context.Context, string) ([]access.Capability, error) {
			return []access.Capability{access.CapabilityProjectAdmin}, nil
		}, PlatformAdmin: func(context.Context, string) (bool, error) { return isPlatformAdmin, nil }}
		data, err := model.agentData(httptest.NewRequest(http.MethodGet, "/admin/agent", nil))
		if err != nil {
			t.Fatal(err)
		}
		if data.CanWrite != isPlatformAdmin {
			t.Fatalf("platformAdmin=%t canWrite=%t", isPlatformAdmin, data.CanWrite)
		}
	}
}

type principalPresentationReader struct{ AccessReader }

func (principalPresentationReader) ListPrincipals(context.Context, access.PrincipalFilter) ([]access.Principal, error) {
	return []access.Principal{{ID: "blocked", Kind: access.PrincipalKindUser, BlockedAt: "2026-10-01T00:00:00Z", LastSeenAt: "2026-10-02T00:00:00Z"}}, nil
}

func TestPrincipalsDataRetainsBlockStatusAndActivity(t *testing.T) {
	rows, err := (ReadModel{}).principalsData(httptest.NewRequest(http.MethodGet, "/admin/principals", nil), principalPresentationReader{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].BlockedAt != "2026-10-01T00:00:00Z" || rows[0].LastSeenAt != "2026-10-02T00:00:00Z" {
		t.Fatalf("principal presentation = %#v", rows)
	}
}
