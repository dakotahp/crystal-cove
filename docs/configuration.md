# Configuration

Crystal Cove reads all of its settings from environment variables. With Docker Compose, put them in a `.env` file next to `docker-compose.yml`. The [example file](../.env.example) lists every variable with a comment.

| Variable | Required | Description |
| --- | --- | --- |
| `OBSIDIAN_EMAIL` | yes* | Obsidian account email. Not needed with `OBSIDIAN_AUTH_TOKEN`. |
| `OBSIDIAN_PASSWORD` | yes* | Obsidian account password. Not needed with `OBSIDIAN_AUTH_TOKEN`. |
| `OBSIDIAN_AUTH_TOKEN` | no | An existing `obsidian-headless` session token, used instead of email and password. Required for accounts with MFA enabled. |
| `OBSIDIAN_VAULTS` | yes | Comma-separated remote vault names, each optionally `Name:encryption-password`. |
| `OBSIDIAN_VAULT_PASSWORD` | no | End-to-end encryption password applied to vaults that don't carry their own. |
| `MCP_AUTH_TOKEN` | yes* | Static bearer token (API key) clients may present. At least 32 characters (`openssl rand -hex 32`); the server refuses to start with a shorter one. Optional when `OAUTH_ISSUER` is set; at least one of the two is required. |
| `OAUTH_ISSUER` | no | OpenID Connect issuer URL. Setting it delegates auth to that provider, see [OAuth](oauth.md). Must be `https`, except on a loopback host. |
| `OAUTH_AUDIENCE` | with `OAUTH_ISSUER` | Audience tokens must carry in `aud` (or `azp`, the authorized-party fallback used by e.g. Keycloak client tokens). |
| `MCP_PUBLIC_URL` | with `OAUTH_ISSUER` | This server's canonical public URL, used as the protected-resource identifier. |
| `OAUTH_INTERNAL_ISSUER` | no | Alternative base URL for fetching discovery/JWKS (e.g. cluster-internal). The discovery document must still report `OAUTH_ISSUER` as its issuer. |
| `OAUTH_SCOPES` | no | Comma-separated scopes advertised as `scopes_supported`. Default `openid,profile,email`. |
| `OAUTH_REQUIRED_ROLES` | no | Comma-separated roles the token must **all** carry (Keycloak `realm_access`/`resource_access`, or a flat `roles` claim), in addition to a valid audience. Empty (default) = audience-only, and the server logs a warning at startup. Recommended: without it, anyone who can get a token for the audience from your provider, which with open sign-up means anyone, can use every tool. |
| `OBSIDIAN_DEVICE_NAME` | no | Device name shown in sync version history. Defaults to `CrystalCove-` plus 8 random hex characters; set it explicitly so restarts reuse one device identity. |
| `VAULTS_DIR` | no | Where vaults are synced locally. Defaults to `~/vaults`. |
| `HOST_PORT` | no | Docker Compose only: the host port to publish (default `8080`). The container always listens on 8080. |
| `PORT` | no | Without Compose, the port the server listens on (default `8080`). With Compose, the container always uses 8080, and `PORT` in `.env` still works as the host port when `HOST_PORT` is not set. |
| `BIND_ADDRESS` | no | Docker Compose only: the host address to publish on. Defaults to `127.0.0.1`, so only this machine can reach the plain-HTTP port. `0.0.0.0` exposes it to every network the host is on. |
| `MCP_READ_ONLY` | no | `true` leaves out every tool that changes a note, so clients can only read and search. Default `false`. |
| `MCP_ALLOW_PERMANENT_DELETE` | no | `true` lets `delete_note` remove notes outright, including notes already in `.trash`. Default `false`: deletes only move notes to `.trash`. |
