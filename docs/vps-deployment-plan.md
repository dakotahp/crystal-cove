# VPS deployment plan: phone access to the vault

Goal: reach the Obsidian vault from claude.ai on the web and on the iPhone,
including CarPlay, with the Obsidian account keeping MFA on.

Status when this was written: the stack runs on a laptop against the real
vault. Sync reached `Fully synced`, `/ready` returned 200, and MCP served its
tools over an authenticated endpoint. DNS for the new host is set, and the
Caddy block is copied on the server and waits for a port.

## Shape

```
phone / claude.ai
      |  HTTPS
   Caddy (TLS, existing)
      |-- OAuth paths      -> auth.py        (existing shim)
      `-- everything else  -> container      (127.0.0.1:8787)
```

The container holds the sync client and the MCP server. Caddy and `auth.py`
already exist on the server and are reused unchanged.

## Why OAuth stays

claude.ai on a personal plan cannot send a fixed API key. Request-header auth
(`static_headers`) is a beta limited to some organizations, so it is not
available here. claude.ai therefore needs an OAuth flow, which `auth.py`
provides: it serves discovery, `/authorize` and `/oauth/token`, and returns one
fixed token.

That token is what this server checks. Set `MCP_AUTH_TOKEN` to the same value
`auth.py` hands out, and leave `OAUTH_ISSUER` unset.

Running a real identity provider (Keycloak, Auth0) is the documented
alternative. It is a whole extra service for a single user, so it is rejected
for now. A small single-user OAuth mode built into this server would remove
`auth.py` later. That is a future change, not part of this plan.

## Steps

1. **Run it beside the current setup.** The working stack keeps serving its
   own host name. Nothing about it changes until the new one is trusted.
2. **Get a session token on the server.** Run `ob login` over SSH once and
   answer the MFA prompt. A new sign-in does not revoke tokens held elsewhere,
   so the laptop and the server can each hold one. Copy the token from
   `~/.config/obsidian-headless/auth_token`.
3. **Write the server's `.env`**: `OBSIDIAN_AUTH_TOKEN` from step 2,
   `OBSIDIAN_VAULTS` with the vault name, `OBSIDIAN_VAULT_PASSWORD` when the
   vault is end-to-end encrypted, because sync-setup fails without it,
   `MCP_AUTH_TOKEN` set to the token `auth.py` returns, and
   `OBSIDIAN_DEVICE_NAME=vps-mcp` so Sync version history names the device
   clearly.
4. **Keep the port on loopback.** The compose file publishes to `127.0.0.1`
   by default. Leave `BIND_ADDRESS` unset: `0.0.0.0` on a public server
   exposes the MCP endpoint directly, with no TLS and no Caddy in front.
5. **Point the copied Caddy block at that port** and reload Caddy.
6. **Start it and watch the first sync.** Wait for `Fully synced` in the logs
   and a 200 from `/ready` before connecting anything.
7. **Add the connector in claude.ai** with the new host name, and set the
   write tools to "Always allow". A tool approval prompt may not render in
   CarPlay, so an approval-on-demand setup can fail while driving.
8. **Cut over** by leaving the old connector switched off for a week. Retire
   it once the new one holds.

## Checks before trusting it

- The log line `using OBSIDIAN_AUTH_TOKEN, skipping ob login` appears at boot.
- A request with no bearer token gets 401.
- `search_notes` finds a known note, and `read_note` returns its text.
- A write from the phone appears on the laptop through Sync.
- After `docker compose restart`, sync resumes with no login prompt.

## Known gaps

- **No `instructions` field.** This server does not send MCP `instructions`, so
  the vault conventions in `.mcp-instructions.md` never reach the model. The
  current setup injects them with `wrapper.mjs`. This matters most on a phone,
  where explaining the vault every time is not practical. It is the next
  feature to add.
- **Search has no ranking.** Results come back in ripgrep order, not by
  relevance. Frontmatter is searchable today, because ripgrep reads the whole
  file as text, so frontmatter properties are searchable.
- **Token lifetime is unknown.** Obsidian does not document how long a session
  token stays valid. If sync starts failing to authenticate, repeat step 2.
- **Disk.** The server holds a second full copy of the vault in a Docker
  volume, on top of the copy the current setup syncs.

## Keep the laptop instance running

The laptop copy is the cheapest way to find the next set of gaps. Claude Code
accepts a fixed token, so it needs no OAuth:

```sh
claude mcp add --transport http vault-local http://127.0.0.1:8787/ \
  --header "Authorization: Bearer <MCP_AUTH_TOKEN>"
```

Use it for real work and collect what annoys you. That list should decide what
gets built next, ahead of any guess made here.
