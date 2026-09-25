# OAuth

You need OAuth to use Crystal Cove from claude.ai in a browser or from the Claude mobile app. Claude Code and most local MCP clients work with the static `MCP_AUTH_TOKEN` alone.

## Why claude.ai needs it

On a personal plan, a claude.ai custom connector has no field for a fixed token. Sending one as a request header is a beta limited to some organizations, so a personal account needs OAuth. See Anthropic's [connector authentication docs](https://claude.com/docs/connectors/building/authentication) for the current state.

There are two ways to give it OAuth:

- **Point `OAUTH_ISSUER` at an identity provider** you already run, such as Keycloak, Auth0 or Entra ID. This is the supported path, described below.
- **Put a small OAuth shim in front of it**, at the same host name, that serves discovery, `/authorize` and `/oauth/token`, and hands back one fixed token that you also set as `MCP_AUTH_TOKEN`. That is far less machinery than a full identity provider for a single user, and the server needs no changes: it just sees a bearer token. Your reverse proxy routes the OAuth paths to the shim and everything else to the container.

## Using an identity provider

Setting `OAUTH_ISSUER` turns the server into an OAuth 2.0 protected resource. It works with any OIDC-compliant provider (Keycloak, Auth0, Entra ID, Okta, ...):

- Bearer JWTs from the issuer are validated: signature via JWKS, issuer, lifetime, and audience, with the `azp` fallback Keycloak uses for client tokens.
- RFC 9728 protected-resource metadata is served at `/.well-known/oauth-protected-resource`, and 401 responses carry a `resource_metadata` challenge. MCP clients like claude.ai and ChatGPT use it to find your authorization server and run the flow themselves, including dynamic client registration if your provider allows it.
- The static `MCP_AUTH_TOKEN` keeps working alongside, which is handy for scripts and smoke tests.

Minimal example:

```sh
OAUTH_ISSUER=https://auth.example.com/realms/myrealm
OAUTH_AUDIENCE=crystal-cove
MCP_PUBLIC_URL=https://obsidian.example.com
OAUTH_REQUIRED_ROLES=vault-owner
```

Your provider must issue tokens whose `aud` (or `azp`) contains `OAUTH_AUDIENCE`. In Keycloak, that is a client scope with an audience mapper, made a realm default so dynamically registered MCP clients pick it up automatically.

Set `OAUTH_REQUIRED_ROLES` to a role only you hold. Without it, anyone who can get a token for the audience from your provider can use every tool. With open sign-up, that means anyone.

The other OAuth variables are in [Configuration](configuration.md).
