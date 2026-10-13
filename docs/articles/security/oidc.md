# OIDC

OpenID Connect provides interactive browser identity. LeapView identifies a person by the stable pair of issuer and subject. Email and display name are profile metadata and may change without creating a new identity.

## Before you begin

Choose one trusted OIDC provider, a stable provider ID, and the exact external HTTPS origin served by the trusted reverse proxy. Obtain a test user without administrator privileges and access to both provider and LeapView logs.

Roll out identity in this order:

1. Register one confidential web client with an exact callback URI.
2. Store its secret and configure the complete LeapView OIDC tuple.
3. Configure trusted proxy, host, cookie, CSRF, and clock boundaries.
4. Bind the test principal or provisioned group to a narrow role.
5. Validate configuration and verify login, logout, denial, and audit behavior.

## Register the client

Create a confidential web application in the identity provider. Register the exact public callback URL:

```text
https://dash.example.com/auth/{provider_id}/callback
```

The provider ID must contain only route-safe letters, numbers, dots, underscores, or dashes. Use one stable value; changing it changes the callback route and requires a coordinated provider update.

Configure LeapView:

```sh
LEAPVIEW_OIDC_PROVIDER_ID=entra
LEAPVIEW_OIDC_ISSUER_URL=https://login.microsoftonline.com/<tenant>/v2.0
LEAPVIEW_OIDC_CLIENT_ID=<client-id>
LEAPVIEW_OIDC_CLIENT_SECRET=<client-secret>
LEAPVIEW_OIDC_CALLBACK_URL=https://dash.example.com/auth/entra/callback
LEAPVIEW_OIDC_SCOPES="openid profile email"
```

Production validation requires the issuer and callback to use unambiguous HTTPS URLs and treats issuer, client ID, client secret, and callback as an all-or-none set. URL userinfo, query strings, and fragments are rejected; issuer paths remain supported for tenant-specific providers. Store the client secret in the deployment secret manager.

## Microsoft Business single-tenant sign-in

For an organization's Microsoft work accounts, register a confidential **Web** application in that organization's Microsoft Entra tenant. Choose **Accounts in this organizational directory only (Single tenant)** and register exactly `https://dash.example.com/auth/azureadv2/callback`, replacing the origin with LeapView's public HTTPS origin. Use the application's client ID, a client-secret value held in the deployment secret manager, and the directory's tenant UUID. See Microsoft's [redirect URI configuration](https://learn.microsoft.com/en-us/entra/identity-platform/how-to-add-redirect-uri) and [authorization code flow](https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-auth-code-flow).

The built-in Microsoft provider uses these settings instead of the generic OIDC tuple above:

```sh
# Example UUIDs: replace them with this organization's tenant and application IDs.
LEAPVIEW_AZURE_TENANT=11111111-2222-4333-8444-555555555555
LEAPVIEW_AZURE_CLIENT_ID=66666666-7777-4888-8999-aaaaaaaaaaaa
LEAPVIEW_AZURE_CLIENT_SECRET=<client-secret-value-from-secret-manager>
LEAPVIEW_AZURE_CALLBACK_URL=https://dash.example.com/auth/azureadv2/callback
LEAPVIEW_LOCAL_AUTH=false
LEAPVIEW_DEV_QUICK_LOGIN=false
LEAPVIEW_DEV_AUTH_BYPASS=false
LEAPVIEW_COOKIE_SECURE=true
LEAPVIEW_PRODUCTION=true
```

Set a concrete tenant UUID; leaving the tenant empty selects the Microsoft `common` endpoint and does not establish this single-tenant boundary. Configure the remaining production host, proxy, and CSRF settings described below. Entra application assignment and consent must allow the intended administrators and users to sign in.

Both administrators and ordinary users enter the same Microsoft sign-in flow at `/auth/azureadv2`. With local authentication disabled, an unauthenticated protected-page visit starts that flow automatically and returns to the requested page after authentication. An existing Microsoft browser session may let Entra complete sign-in without asking for credentials. Entra decides whether account selection, consent, MFA, or Conditional Access requires interaction; LeapView does not force a new password prompt or guarantee silent sign-in. See Microsoft's [browser sign-in flow](https://learn.microsoft.com/en-us/entra/identity-platform/app-sign-in-flow).

Assign LeapView privileges explicitly to the principal identified by the verified issuer and subject. A tenant administrator, email address, email domain, or successful first sign-in does not create a LeapView platform administrator. An OIDC identity with the same email as an existing local administrator remains a separate principal. Ordinary users still need their own project grants or role bindings.

The one-shot instance initializer creates a local administrator. Initialize through that local path once, keep local operator access available during setup, and have the intended Microsoft administrator complete OIDC sign-in to create its distinct principal. Obtain that principal's immutable UUID from the signed-in profile or the existing administrator's principal list and verify its issuer and subject. Stop the LeapView instance, then use the trusted installation operator's offline nomination command with the exact durable instance ID and principal UUID:

```sh
# Preview the exact existing OIDC principal; this does not assign a role.
leapview admin nominate-sso-admin --instance <instance-id> --principal <principal-uuid>
# Explicitly assign platform administration to that same principal.
leapview admin nominate-sso-admin --instance <instance-id> --principal <principal-uuid> --apply
```

The command requires an initialized matching instance and an exclusive instance lock. It validates an existing enabled OIDC user and records the assignment in the access audit; it does not search by email, link identities, or create a local password. After nomination, set `LEAPVIEW_LOCAL_AUTH=false` as shown above, restart, and verify Microsoft administrator access before completing the cutover. Project role bindings remain separate from the instance-wide `platform_admin` role.

These settings describe the deployment contract. A local signed OIDC fixture can demonstrate the protocol and explicit role separation, but does not verify a live Microsoft tenant, its app registration, consent, or Conditional Access policy.

## Configure the reverse proxy

Terminate TLS at a maintained trusted proxy and ensure the application sees the correct public scheme and host used by the callback. Set exact allowed hosts. Enable proxy-header trust only when that proxy overwrites client-supplied forwarding headers.

Secure cookies must remain enabled for browser auth. With secure cookies enabled, LeapView uses browser-enforced `__Host-` names for session and authentication-state cookies; switching from an insecure development boundary to this production boundary requires a fresh sign-in. Authentication and authenticated responses are marked `no-store`, and provider failures return generic client messages rather than upstream details. Clock synchronization matters for token and state validation on both LeapView and the identity provider.

## Understand identity mapping

The issuer URL and the token's subject claim form identity. Do not map identity by email alone: email addresses can be renamed or reassigned. Profile changes may update display metadata while privileges remain attached to the same principal.

LeapView intentionally does not treat OIDC group claims as the enterprise group source of truth. Use SCIM for directory users, groups, and membership, then use LeapView grants and role bindings for product authorization.

SCIM and OIDC currently store separate external identity keys. Matching their email metadata does not connect the principals or transfer SCIM group membership to an OIDC session. A deployment relying on preprovisioned directory access needs an explicit trusted identity mapping; do not resolve that gap by linking accounts on email alone.

## Assign access

A successful login can still result in no visible project resource. OIDC proves who the user is; it does not grant product access. Bind a provisioned or known principal/group to an appropriate project role or explicit grant.

Test with a non-administrator user. An owner account can hide missing group provisioning or role binding because it already has broad access.

## Harden the provider

Use provider MFA and conditional access. Restrict who may use the client, protect client-secret rotation, and monitor provider-side sign-in events. Keep redirect URI lists narrow and remove retired callbacks.

LeapView currently delegates the MFA decision to the provider and does not authorize from `acr` or `amr` assurance claims. Read the [MFA security decision boundary](/docs/security/mfa-boundary) before describing the deployment's assurance level or requiring application-side step-up.

When rotating the client secret, install the new value through the secret manager and coordinate restart without exposing either value. Verify login before revoking the old provider credential where the provider supports overlap.

## Validate the configuration

Start LeapView with production validation enabled before changing traffic. It must reject partial OIDC configuration, non-HTTPS production issuer or callback URLs, malformed provider IDs, and insecure cookie boundaries. Compare the configured callback with the provider registration character for character, including path and provider ID.

Validate that the reverse proxy overwrites forwarding headers, the application trusts only that proxy, and the public host is explicitly allowed. Keep the previous authentication path available to an operator until the new flow is verified.

## Verify identity and access

Use the non-administrator test account to complete these checks:

1. Sign in through the provider and confirm the expected issuer-subject principal is used.
2. Verify only explicitly granted resources and actions in the server-bound Project are available.
3. Sign out and confirm the browser session is no longer accepted.
4. Remove or suspend access and verify the next authorization check denies it.
5. Inspect audit and provider logs for the same event without raw tokens or secrets.

Repeat a login after changing only profile metadata such as display name; it should update the same identity rather than create a new one.

## Troubleshooting

If the provider rejects the request, compare the callback URL character for character. If callback reaches LeapView but state or cookie validation fails, check HTTPS, cookie security, allowed hosts, CSRF key consistency, proxy headers, and clock skew. If login succeeds but access is empty, inspect principal identity and grants rather than the OIDC handshake.

Preserve correlation timestamps and inspect both provider logs and LeapView audit/application logs without recording raw tokens.

## Next steps

Configure [SCIM provisioning](/docs/security/scim) for directory lifecycle, review [Roles, grants, and policies](/docs/security/authorization), and complete the broader [Authentication and authorization](/docs/enterprise-auth) checklist.

The configuration above shows the workflow. Use the generated [environment variable reference](/docs/configuration) for the complete OIDC, proxy, host, cookie, and production-validation contract.
