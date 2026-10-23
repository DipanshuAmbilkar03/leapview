package duckdb

import (
	"strings"

	"github.com/flidai/leapview/internal/analytics/connectionbinding"
	"github.com/flidai/leapview/internal/analytics/connectors"
	semanticmodel "github.com/flidai/leapview/internal/analytics/model"
)

// A target probe owns an isolated session with one attached connection. Its
// internal SQL alias is independent of the durable resource ID, which can
// contain namespace punctuation and is not a semantic or SQL identifier.
const targetRuntimeConnectionAlias = "target_probe"

func ApplyTargetBinding(
	logical semanticmodel.Connection,
	binding connectionbinding.TargetBinding,
	snapshot connectionbinding.CredentialSnapshot,
) (semanticmodel.Connection, error) {
	if err := binding.Validate(); err != nil || !binding.Enabled ||
		strings.TrimSpace(logical.Kind) != binding.ConnectorKind ||
		binding.AuthenticationMode != connectionbinding.AuthenticationExternalBundle &&
			binding.AuthenticationMode != connectionbinding.AuthenticationNone {
		return semanticmodel.Connection{}, connectionbinding.ErrIncompatibleBinding
	}
	resolved := logical
	if binding.AuthenticationMode == connectionbinding.AuthenticationNone {
		if logical.Access != semanticmodel.ConnectionAccessPublic {
			return semanticmodel.Connection{}, connectionbinding.ErrIncompatibleBinding
		}
		resolved.Access = semanticmodel.ConnectionAccessPublic
	} else if logical.Access == semanticmodel.ConnectionAccessPublic {
		return semanticmodel.Connection{}, connectionbinding.ErrIncompatibleBinding
	}
	resolved.Host = binding.Endpoint.Host
	resolved.Port = binding.Endpoint.Port
	resolved.Database = binding.Endpoint.Database
	resolved.Username = binding.Endpoint.SourceIdentity
	resolved.SSLMode = binding.Endpoint.TLSMode
	if binding.Endpoint.ObjectScope != "" {
		resolved.Scope = binding.Endpoint.ObjectScope
	}
	if err := applyEndpointRuntimeOptions(&resolved.RuntimeOptions, binding.Endpoint.Options); err != nil {
		return semanticmodel.Connection{}, connectionbinding.ErrIncompatibleBinding
	}
	resolved.Credentials = semanticmodel.ConnectionCredentials{}
	if binding.AuthenticationMode == connectionbinding.AuthenticationExternalBundle {
		if err := snapshot.Use(func(values map[string]string) error {
			resolved.Auth = make(semanticmodel.ConnectionAuth, len(values))
			for key, value := range values {
				resolved.Auth[key] = value
			}
			return nil
		}); err != nil {
			return semanticmodel.Connection{}, connectionbinding.ErrInvalidCredentialBundle
		}
	} else {
		resolved.Auth = nil
	}
	if credentialOverridesEndpoint(resolved) {
		clear(resolved.Auth)
		return semanticmodel.Connection{}, connectionbinding.ErrInvalidCredentialBundle
	}
	validated, err := resolved.Validate(targetRuntimeConnectionAlias)
	if err != nil {
		clear(resolved.Auth)
		return semanticmodel.Connection{}, connectionbinding.ErrInvalidCredentialBundle
	}
	return validated, nil
}

// applyEndpointRuntimeOptions is the only boundary where target endpoint
// option strings enter the typed connection runtime. Reader options never
// cross this boundary; endpoint options are limited to the connector-specific
// filesystem hints represented by ConnectionRuntimeOptions.
func applyEndpointRuntimeOptions(runtime *semanticmodel.ConnectionRuntimeOptions, values map[string]string) error {
	if runtime == nil {
		return connectionbinding.ErrIncompatibleBinding
	}
	for key, value := range values {
		switch key {
		case "path":
			runtime.Path = value
		case "data_path":
			runtime.DataPath = value
		default:
			return connectionbinding.ErrIncompatibleBinding
		}
	}
	return nil
}

// credentialOverridesEndpoint reports whether a database connection carries a
// connection_string next to its governed endpoint. DuckDB's database attach
// uses the connection string verbatim, so it would replace the host that the
// outbound destination policy resolved and checked (loopback, link-local, or a
// plaintext sslmode=disable target) rather than complement it. A target
// binding's endpoint is the only destination a database credential may reach.
func credentialOverridesEndpoint(connection semanticmodel.Connection) bool {
	spec, ok := connectors.LookupConnection(connection.Kind)
	if !ok || spec.AttachKind != connectors.AttachDatabase || strings.TrimSpace(connection.Host) == "" {
		return false
	}
	_, present := connection.Auth["connection_string"]
	return present
}
