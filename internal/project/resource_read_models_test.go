package project

import (
	"encoding/json"
	"strings"
	"testing"

	semanticmodel "github.com/flidai/leapview/internal/analytics/model"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
	refreshschedule "github.com/flidai/leapview/internal/refresh/schedule"
)

func TestSourceAssetPayloadProjectsSchemaAndFields(t *testing.T) {
	source := semanticmodel.Source{
		Format: "csv", Connection: "warehouse", Path: "s3://bucket/orders.csv",
		SchemaMode: "compatible",
		Fields:     map[string]semanticmodel.SourceField{"order_id": {Type: "int", Description: "Order ID"}},
		Schema:     semanticmodel.TableSchema{Columns: []semanticmodel.ColumnSchema{{Name: "order_id", Ordinal: 0, PhysicalType: "BIGINT"}}},
	}
	payload := SourceAssetPayload(source)
	if payload["Format"] != "csv" || payload["Connection"] != "warehouse" || payload["Path"] != "s3://bucket/orders.csv" {
		t.Fatalf("source payload = %#v", payload)
	}
	fields, ok := payload["Fields"].(map[string]any)
	if !ok || fields["order_id"] == nil {
		t.Fatalf("source fields = %#v, want order_id", payload["Fields"])
	}
	if payload["SchemaMode"] != "compatible" {
		t.Fatalf("source schema mode = %#v, want compatible", payload["SchemaMode"])
	}
	schema, ok := payload["Schema"].(map[string]any)
	if !ok || len(schema["columns"].([]any)) != 1 {
		t.Fatalf("source schema = %#v, want one column", payload["Schema"])
	}
}

func TestConnectionAssetPayloadOmitsCredentialMaterial(t *testing.T) {
	connection := semanticmodel.Connection{
		Kind: "postgres", Scope: "warehouse", Path: "db.example", Root: "orders",
		Credentials: semanticmodel.ConnectionCredentials{Provider: "env", Secret: "PROD_PASSWORD", Region: "eu-west-1"},
	}
	payload := ConnectionAssetPayload(connection)
	if payload["Kind"] != "postgres" || payload["Scope"] != "warehouse" || payload["credentials_configured"] != true {
		t.Fatalf("connection payload = %#v", payload)
	}
	serialized := strings.ToLower(string(mustJSON(t, payload)))
	for _, forbidden := range []string{"prod_password", "eu-west-1", "provider", "secret"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("connection payload contains credential material %q: %s", forbidden, serialized)
		}
	}
}

func TestRefreshPipelineAssetPayloadProjectsPublicSchedule(t *testing.T) {
	pipeline := refreshschedule.Definition{
		ID: "pipeline:sales", Name: "sales-refresh", SemanticModelID: projectgraph.ResourceID("semantic:sales"),
		Timezone: "Europe/Copenhagen", ConcurrencyPolicy: refreshschedule.ConcurrencyForbid,
		Schedules: []refreshschedule.Schedule{{Expression: "0 * * * *"}},
	}
	payload := RefreshPipelineAssetPayload(pipeline)
	if payload["SemanticModel"] != "semantic:sales" || payload["SemanticModelID"] != "semantic:sales" {
		t.Fatalf("pipeline payload = %#v", payload)
	}
	schedules, ok := payload["Schedules"].([]any)
	if !ok || len(schedules) != 1 {
		t.Fatalf("pipeline schedules = %#v, want one schedule", payload["Schedules"])
	}
	entry := schedules[0].(map[string]any)
	if entry["Cron"] != "0 * * * *" || payload["Timezone"] != "Europe/Copenhagen" {
		t.Fatalf("pipeline schedule = %#v", entry)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	// The helpers already perform a JSON round trip. Formatting through the
	// same standard encoder keeps this test independent of map iteration order.
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestConnectionAssetPayloadCredentialsRequired(t *testing.T) {
	for _, tc := range []struct {
		name       string
		connection semanticmodel.Connection
		want       bool
	}{
		{"managed without provider", semanticmodel.Connection{Kind: "managed"}, false},
		{"public access without provider", semanticmodel.Connection{Kind: "s3", Scope: "s3://open/", Access: semanticmodel.ConnectionAccessPublic}, false},
		{"explicit none provider", semanticmodel.Connection{Kind: "s3", Scope: "s3://open/", Credentials: semanticmodel.ConnectionCredentials{Provider: "none"}}, false},
		{"omitted provider on external connector", semanticmodel.Connection{Kind: "postgres", Scope: "warehouse"}, true},
		{"env provider on external connector", semanticmodel.Connection{Kind: "s3", Scope: "s3://b/", Credentials: semanticmodel.ConnectionCredentials{Provider: "env", Secret: "S3"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := ConnectionAssetPayload(tc.connection)
			if got := payload["credentials_required"]; got != tc.want {
				t.Fatalf("credentials_required = %v, want %v (payload %#v)", got, tc.want, payload)
			}
		})
	}
}
