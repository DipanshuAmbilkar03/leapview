package http

import (
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageSliceForRequestPreservesEmptyArray(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/?limit=10", nil)
	request.Header.Set("X-Serving-Snapshot", "state-1")

	items, nextCursor, ok := pageSliceForRequest[string](recorder, request, []string{})
	if !ok {
		t.Fatalf("page slice failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if items == nil || len(items) != 0 || nextCursor != "" {
		t.Fatalf("empty page = %#v cursor=%q, want non-nil empty slice", items, nextCursor)
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal empty page: %v", err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("empty page JSON = %s, want []", encoded)
	}
}

func TestPageSliceForRequestRequiresServingSnapshot(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/?limit=10", nil)
	if _, _, ok := pageSliceForRequest[string](recorder, request, []string{"item"}); ok || recorder.Code != nethttp.StatusServiceUnavailable {
		t.Fatalf("missing serving snapshot status=%d ok=%v body=%s", recorder.Code, ok, recorder.Body.String())
	}
}

func TestParseAPILimitRejectsOverLimit(t *testing.T) {
	if got, err := parseAPILimit("200"); err != nil || got != 200 {
		t.Fatalf("maximum limit = %d, %v", got, err)
	}
	if _, err := parseAPILimit("201"); err == nil {
		t.Fatal("over-limit request was accepted")
	}
}

func TestDecodeOptionalJSONBodyRejectsCallerSuppliedQuery(t *testing.T) {
	request := httptest.NewRequest(nethttp.MethodPost, "/", strings.NewReader(`{"query":{"sql":"select * from secrets"}}`))
	var input struct {
		FilterState map[string]any `json:"filterState"`
	}
	if err := decodeOptionalJSONBody(request, &input); err == nil {
		t.Fatal("caller-supplied arbitrary query content was accepted")
	}
}

func TestWriteJSONErrorHidesUnclassifiedServerErrors(t *testing.T) {
	request := httptest.NewRequest(nethttp.MethodGet, "/api/v1/dashboards", nil)
	request.Header.Set("X-Request-ID", "req-leak-1")
	recorder := httptest.NewRecorder()
	writeJSONError(recorder, request, errors.New(`pq: relation "leapview.secrets" does not exist (SQLSTATE 42P01) at /var/lib/leapview`), nethttp.StatusInternalServerError)
	var body struct {
		Message   string `json:"message"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != nethttp.StatusInternalServerError || strings.Contains(recorder.Body.String(), "SQLSTATE") || strings.Contains(recorder.Body.String(), "/var/lib") {
		t.Fatalf("response leaks the cause: %d %s", recorder.Code, recorder.Body.String())
	}
	if body.RequestID != "req-leak-1" || !strings.Contains(body.Message, "req-leak-1") {
		t.Fatalf("response does not correlate by request id: %+v", body)
	}
}

func TestWriteJSONErrorKeepsDomainMessages(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"not found", errors.New(`dashboard "sales" not found`), nethttp.StatusNotFound},
		{"validation", errors.New("limit must be at least 1"), nethttp.StatusBadRequest},
		{"authorization unavailable", ErrDashboardAuthorizationUnavailable, nethttp.StatusServiceUnavailable},
		{"semantic authority unavailable", errDashboardSemanticAuthorityUnavailable, nethttp.StatusServiceUnavailable},
		{"serving snapshot", errServingSnapshotUnavailable, nethttp.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeJSONError(recorder, httptest.NewRequest(nethttp.MethodGet, "/", nil), tc.err, tc.status)
			var body struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Message != tc.err.Error() {
				t.Fatalf("message = %q, want %q", body.Message, tc.err.Error())
			}
		})
	}
}

func TestWriteAuthoringErrorHidesUnclassifiedCause(t *testing.T) {
	request := httptest.NewRequest(nethttp.MethodPost, "/api/v1/dashboard-drafts", nil)
	request.Header.Set("X-Request-ID", "req-authoring-1")
	recorder := httptest.NewRecorder()
	writeAuthoringError(recorder, request, errors.New("pq: connection to 10.1.2.3:5432 refused"))
	if recorder.Code != nethttp.StatusInternalServerError || strings.Contains(recorder.Body.String(), "10.1.2.3") {
		t.Fatalf("response leaks the cause: %d %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "req-authoring-1") {
		t.Fatalf("response does not carry the request id: %s", recorder.Body.String())
	}
}
