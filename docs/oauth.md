# OAuth

You need OAuth to use Crystal Cove from claude.ai in a browser, from the Claude mobile app, or from ChatGPT. Claude Code and most local MCP clients work with the static `MCP_AUTH_TOKEN` alone.

## Built-in sign-in

Set two values:

```sh
MCP_PUBLIC_URL=https://obsidian.example.com
MCP_OWNER_PASSWORD=<at least 16 characters>
```

Crystal Cove then is its own OAuth server. An app registers itself, opens a Crystal Cove page where you enter the owner password, and goes back connected. You paste no client ID or secret.

- **Staying connected:** an app gets an access token for one hour and renews it by itself. One sign-in lasts at most 90 days, then you sign in again.
- **Disconnecting every app:** change `MCP_OWNER_PASSWORD` and restart the container.
- **A stolen renewal token:** when an old one is used again, that app's sign-in ends at once.
- **Wrong passwords:** after five, the page locks for a minute, and the lock doubles with each further miss, up to an hour.
- **Storage:** `/home/obsidian/.crystal-cove/auth.json`, beside the vaults and never inside one, so Obsidian Sync never carries it. It holds only hashes of tokens and secrets.

The page shows which app asks and which site you return to. If either looks wrong, do not enter the password.

## Using an identity provider

If you already run Keycloak, Auth0, Entra ID, or another OIDC provider, Crystal Cove can accept its tokens instead. Use this or the built-in sign-in, not both.

Setting `OAUTH_ISSUER` turns the server into an OAuth 2.0 protected resource:

- Bearer JWTs from the issuer are validated: signature via JWKS, issuer, lifetime, and audience, with the `azp` fallback Keycloak uses for client tokens.
- RFC 9728 protected-resource metadata is served at `/.well-known/oauth-protected-resource`, and 401 responses carry a `resource_metadata` challenge. MCP clients use it to find your provider and run the flow themselves, including dynamic client registration if your provider allows it.
- The static `MCP_AUTH_TOKEN` keeps working alongside.

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
