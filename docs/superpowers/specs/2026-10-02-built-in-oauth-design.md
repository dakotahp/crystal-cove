# Built-in OAuth sign-in

## Goal

claude.ai, the Claude mobile app, and ChatGPT can connect to Crystal Cove with no external identity provider, no OAuth shim, and no client ID or secret to paste. The operator sets two values, adds the connector URL, and signs in once with an owner password.

Today a remote setup needs either an OIDC provider (`OAUTH_ISSUER`) or a hand-built shim in front of the server that serves discovery, `/authorize`, and `/oauth/token`. Both are outside the project, and the shim also needs a fixed client ID and secret pasted into claude.ai.

## Decisions

- **Owner proves identity with a password** set in `.env`. Passkeys can come later.
- **Clients stay connected.** Short access tokens, rotating refresh tokens, reuse detection, and a 90-day maximum per sign-in.
- **The authorization server is written into the Go server** (approach A), in a new package. Not a full OAuth library, and not a second process in the image.
- **One place to sign in.** The built-in sign-in and an external OIDC provider are mutually exclusive. The static `MCP_AUTH_TOKEN` keeps working beside either one.

## Configuration

| Variable | Rule |
| --- | --- |
| `MCP_OWNER_PASSWORD` | Turns on the built-in sign-in. At least 16 characters. |
| `MCP_PUBLIC_URL` | Required with `MCP_OWNER_PASSWORD`. The issuer and the protected-resource identifier. |

Startup fails, following the fail-fast rule, when:

- `MCP_OWNER_PASSWORD` is shorter than 16 characters.
- `MCP_OWNER_PASSWORD` is set without `MCP_PUBLIC_URL`.
- `MCP_OWNER_PASSWORD` and `OAUTH_ISSUER` are both set.
- The auth store file exists but cannot be read or parsed.

At least one of `MCP_AUTH_TOKEN`, `OAUTH_ISSUER`, or `MCP_OWNER_PASSWORD` is required.

## Architecture

### `internal/authserver` (new)

Owns issuing and checking built-in tokens. The rest of the server uses two things from it: an `http.Handler` for its paths and a `Verify(ctx, token) (*auth.TokenInfo, error)` function.

| Path | Method | Purpose |
| --- | --- | --- |
| `/.well-known/oauth-authorization-server` | GET | RFC 8414 metadata (`oauthex.AuthServerMeta`) |
| `/register` | POST | RFC 7591 dynamic client registration |
| `/authorize` | GET | Sign-in page |
| `/authorize` | POST | Password check, then redirect with a code |
| `/token` | POST | `authorization_code` and `refresh_token` grants |

Metadata advertises: `issuer` = `MCP_PUBLIC_URL`, the four endpoints under it, `response_types_supported: [code]`, `grant_types_supported: [authorization_code, refresh_token]`, `code_challenge_methods_supported: [S256]`, and `token_endpoint_auth_methods_supported: [none, client_secret_post, client_secret_basic]`.

Request and response types come from `github.com/modelcontextprotocol/go-sdk/oauthex` where it has them (`AuthServerMeta`, `ClientRegistrationMetadata`, `ClientRegistrationResponse`).

Suggested files: `store.go` (persistence and grant rules), `handlers.go` (HTTP), `page.go` (sign-in HTML), `limiter.go` (wrong-password lock).

### `internal/server` changes

- `AuthConfig` gains `Builtin *BuiltinAuth` holding the authserver handler, its `Verify`, and `PublicURL`.
- With `Builtin` set, the protected-resource metadata names `PublicURL` itself as the authorization server, and the authserver paths are mounted on the mux.
- `verifyToken` checks the static token first, then the built-in or the OIDC verifier, whichever is configured.

### `internal/config` changes

Parse and validate `MCP_OWNER_PASSWORD` with the rules above.

### `cmd/crystal-cove` changes

Open the store, build the authserver, and pass it in `AuthConfig`. Log at startup that built-in sign-in is on, without the password.

## Flow

1. The client calls `/` without a token and gets a 401 with a `resource_metadata` challenge. This exists today.
2. It reads `/.well-known/oauth-protected-resource`, then `/.well-known/oauth-authorization-server`.
3. It registers at `/register` and gets a `client_id`, plus a `client_secret` if it asked for a secret-based auth method.
4. It opens `/authorize` in the browser with `client_id`, `redirect_uri`, `state`, `code_challenge`, `code_challenge_method=S256`, and optionally `resource`.
5. The page shows the client name and the host it returns to, and asks for the owner password.
6. On a correct password the server redirects to `redirect_uri` with `code` and `state`.
7. The client posts the code and `code_verifier` to `/token` and gets an access token, a refresh token, and `expires_in`.
8. The client sends the access token as a bearer token and renews it at `/token` with `grant_type=refresh_token`.

## Tokens and grants

All tokens are 32 random bytes, base64url-encoded. Only their SHA-256 hashes are stored or compared.

| Item | Lifetime | Kept |
| --- | --- | --- |
| Authorization code | 60 seconds, single use | Memory |
| Access token | 1 hour | Memory |
| Refresh token | Until rotated, at most the grant's end | Disk (hash) |
| Grant (one sign-in of one client) | 90 days from sign-in | Disk |

Rules:

- **Rotation:** every refresh returns a new refresh token, and the old one is marked used.
- **Reuse detection:** presenting a used refresh token revokes its whole grant and returns `invalid_grant`.
- **Absolute limit:** a refresh after the grant's 90 days returns `invalid_grant`, and the client must sign in again.
- **Password change:** the store records an HMAC-SHA256 fingerprint of the owner password, keyed with a random key kept in the store. At startup, a different fingerprint deletes every grant.
- **Restart:** access tokens and codes are lost, and clients renew with their refresh tokens.
- **Code exchange:** the code is bound to `client_id`, `redirect_uri`, the PKCE challenge, and the grant. A second use of a code revokes the grant it created.
- **Resource binding:** if the client sends `resource`, it must equal `MCP_PUBLIC_URL` (RFC 8707), or the request fails with `invalid_target`.

## Storage

- File: `/home/obsidian/.crystal-cove/auth.json`, beside the `vaults` folder in the same volume. It is never inside a vault, so Obsidian Sync never carries it and the vault sandbox cannot reach it. It stays there when `VAULTS_DIR` is changed.
- Mode `0600`, folder `0700`.
- Written to a temporary file in the same folder, then renamed over the old file.
- Contents: the fingerprint key, the password fingerprint, registered clients (ID, name, redirect URIs, auth method, secret hash, created time, last sign-in time), and grants (ID, client ID, created time, current refresh-token hash, used refresh-token hashes).
- Access goes through one mutex. One server process owns the file.

## Registration

- Open to anyone. No token is issued without the password.
- `redirect_uris` must be `https`, or `http` on a loopback host. They must be absolute and must not contain a fragment.
- `client_name` is shown on the sign-in page, escaped. A missing name shows "Unnamed client".
- At most 50 clients. When full, the oldest client with no active grant is removed. If every client has an active grant, registration fails with HTTP 503 and a logged warning.

## Sign-in page

- Plain server-rendered HTML with `html/template`, no JavaScript, inline styles.
- Headers: `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Cache-Control: no-store`.
- The form posts the original authorization parameters back with the password. The server validates the parameters again on POST.
- An unknown `client_id` or a `redirect_uri` that does not exactly match a registered one shows an error page and never redirects.
- Other invalid parameters redirect to `redirect_uri` with the standard `error` and `state`.

## Wrong-password lock

- One counter for the whole server, because there is one owner.
- After 5 consecutive failures, the form is locked for 1 minute. Each further failure doubles the lock, up to 1 hour. A success resets the counter.
- A locked page says how long to wait and does not check the password.
- Each failure is logged to the audit log with the remote address.

## Errors and logging

- `/token` and `/register` return RFC 6749 and RFC 7591 JSON errors: `invalid_request`, `invalid_client`, `invalid_grant`, `unsupported_grant_type`, `invalid_target`, `invalid_redirect_uri`, `invalid_client_metadata`.
- The audit log records registration, sign-in success and failure, refresh, reuse detection, grant expiry, and grant deletion after a password change. It never logs a token, code, secret, or password.

## Testing

Following the project style: real filesystems with `t.TempDir()`, `httptest`, the real MCP client over HTTP, and no mocking libraries. The clock is injected.

- **Store:** rotation, reuse revoking the grant, the 90-day limit, the password-change wipe, the client limit and eviction of clients with no active grant, atomic writes, and a corrupt file failing to open.
- **Handlers:** metadata contents, registration validation, PKCE failure, wrong and mismatched `redirect_uri`, code reuse, `resource` mismatch, the lock and its doubling, and the security headers on the page.
- **Config:** each startup rule above.
- **End to end:** register, sign in, exchange the code, call an MCP tool with the access token, refresh, confirm the old refresh token now fails and revokes the grant, and confirm the static token still works.
- **Fuzz:** a `Fuzz*` target for `/token` form parsing.
- Total coverage stays at 95% or more.
- **Manual before merge:** connect claude.ai to a real deployment and replace an existing shim. Confirm which registration auth method claude.ai uses.

## Documentation

- README remote steps: set `MCP_PUBLIC_URL` and `MCP_OWNER_PASSWORD`, proxy everything to the container, add the connector, sign in.
- `docs/oauth.md`: the built-in sign-in first, the external provider as the advanced option, and the shim section removed.
- `docs/configuration.md` and `.env.example`: the new variable.
- `docs/SECURITY.md`: the token model, storage, and lock.
- `CLAUDE.md`: auth is a static token, the built-in sign-in, or an OIDC provider. Add `internal/authserver` to the architecture list.

## Out of scope

- Passkeys.
- More than one user.
- A list of connected clients with per-client removal. A password change disconnects all of them.
- OAuth Client ID Metadata Documents.
- Changing the password without a restart.
