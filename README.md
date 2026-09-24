<p align="center">
  <img src="https://obsidian.md/images/obsidian-logo-gradient.svg" alt="Obsidian logo" width="120">
</p>

<h1 align="center">Obsidian Hosted MCP</h1>

<p align="center">
  Give Claude, ChatGPT, and any other MCP client full access to your
  <a href="https://obsidian.md">Obsidian</a> vaults — from anywhere.
</p>

<p align="center">
  <a href="https://github.com/andyjmorgan/Obsidian-Hosted-Mcp/actions/workflows/ci.yml"><img src="https://github.com/andyjmorgan/Obsidian-Hosted-Mcp/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/andyjmorgan/Obsidian-Hosted-Mcp/pkgs/container/obsidian-hosted-mcp"><img src="https://img.shields.io/badge/ghcr.io-obsidian--hosted--mcp-blue?logo=docker" alt="Container image"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="MIT license"></a>
</p>

## Why this exists

Giving an AI assistant real access to an Obsidian vault needs three things
at once. Every other approach I tried has two of them.

1. **The official sync engine.** The vault copy is kept by Obsidian's own
   [headless client](https://help.obsidian.md/install/headless), so nothing
   here reimplements Obsidian Sync, and nothing depends on the desktop app
   being awake or on a socket into it.
2. **Reach from anywhere.** It speaks the
   [Model Context Protocol](https://modelcontextprotocol.io) over HTTP with
   authentication, so a browser, a phone, or a cloud agent can use it while
   your computer is off.
3. **Tools that understand notes.** Tags, frontmatter, wikilinks, and a
   search that ranks a note named after your query above a passing mention
   of it. Not `read_file` and `list_directory` over a folder.

Drop any one and the result fails in a specific way. Reimplement sync and
you are trusting a guess about someone else's protocol with your notes.
Skip the remote part and it only works on one machine, so no phone and no
cloud. Ship generic file tools and the assistant cannot find anything,
because a note's name and its frontmatter are where the meaning lives.

That means:

- **Claude** (claude.ai, Claude Code, Claude Desktop) and **ChatGPT**
  (connectors / deep research) can read, search, create, edit, and organize
  your notes.
- Every write syncs back to your phone, tablet, and desktop within seconds —
  and every note you jot down on your phone becomes visible to your
  assistant.
- It runs 24/7 wherever you host containers: a NAS, a VPS, Kubernetes, or a
  Raspberry Pi (images are amd64 + arm64).

### It is also the better local option

The remote half is optional. Because the tools understand notes and nothing
depends on the desktop app, the same server is the most capable way to give
a **local** agent your vault: point Claude Code at `127.0.0.1` and you get
tag queries, frontmatter edits, backlinks and ranked search, with no
Obsidian window open and no socket to race against. One interface on your
laptop, your work machine and your phone, instead of one tool for local
work and a different one for everything else.

Requirements: an Obsidian account with a **Sync subscription**.

If the account has MFA enabled, no one can type the code inside a container,
so set `OBSIDIAN_AUTH_TOKEN` instead of the email and password. Run `ob login`
once on any machine, answer the MFA prompt, then copy the token from
`~/.config/obsidian-headless/auth_token` (Linux) or
`~/.obsidian-headless/auth_token` (macOS). The server then skips `ob login`
and uses that session. Repeat the step if the token ever stops working.

## Which setup do you need?

The same container covers two quite different jobs, and the tools are the
same either way. Start with the first one: it is simpler, it needs no
domain name and no OAuth, and it is a complete setup on its own rather than
a trial run.

| | **On one machine** | **On a server** |
| --- | --- | --- |
| Who connects | Agents on that machine: Claude Code, Cursor, and other local MCP clients | claude.ai in a browser, the Claude mobile app, and anything else on the internet |
| Reached at | `http://127.0.0.1:8787/` | `https://notes.example.com/` |
| Auth | A static bearer token you generate | OAuth, because claude.ai cannot send a fixed token (see below) |
| You also need | Docker | A domain name, TLS, and a reverse proxy |
| Good for | Daily agent work on a laptop or work computer, in place of a plugin or the Obsidian CLI, with no desktop app running | Reaching your vault from a phone, and agents that run while your computer is off |

Both keep the vault in sync through Obsidian Sync, so a note written by an
agent on one machine appears on your phone and your desktop like any other
edit.

## Run it on one machine

This is the quickest way to try the server, and it is enough for daily work
with a local agent.

**You need:** Docker, an Obsidian account with a
[Sync](https://obsidian.md/sync) subscription, and the vault's end-to-end
encryption password if you set one.

**1. Get a sync token.** In a terminal, log in once with the official
headless client and answer any MFA prompt:

```sh
npx -y obsidian-headless login
cat ~/.obsidian-headless/auth_token          # macOS
cat ~/.config/obsidian-headless/auth_token   # Linux
```

**2. Write a `.env`** next to `docker-compose.yml`:

```sh
OBSIDIAN_AUTH_TOKEN=the-token-from-step-1
OBSIDIAN_VAULTS=YourVaultName
#OBSIDIAN_VAULT_PASSWORD=only-if-the-vault-is-encrypted
MCP_AUTH_TOKEN=generate-one-with-openssl-rand-hex-32
PORT=8787
```

Not sure of the vault's name? `OBSIDIAN_AUTH_TOKEN=... npx -y
obsidian-headless sync-list-remote --json` lists them as Obsidian Sync
spells them.

**3. Start it and wait for the first sync:**

```sh
docker compose up -d
docker compose logs -f          # wait for "Fully synced", then Ctrl-C
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8787/readyz   # 200
```

The first sync downloads the whole vault into a Docker volume, so a large
vault takes a few minutes.

**4. Point an agent at it.** For Claude Code:

```sh
claude mcp add --scope user --transport http vault-local http://127.0.0.1:8787/ \
  --header "Authorization: Bearer $MCP_AUTH_TOKEN"
```

`--scope user` makes the vault available in every folder. Without it, Claude
Code adds the server to the current folder only. Avoid `--scope project`: it
writes the token into a `.mcp.json` file that is meant to be committed.

Then `claude mcp list` should show it connected. Other MCP clients take the
same URL and `Authorization` header.

**Stopping and cleaning up:** `docker compose stop` leaves the synced copy
in place for next time. `docker compose down -v` also deletes the volume
holding the vault copy, so the next start re-syncs from scratch.

**Worth knowing before you run this on a work computer.** The container
keeps a full copy of the vault on that machine, in a Docker volume, and it
stays a registered Obsidian Sync device until you remove it. The MCP
endpoint listens on localhost only, and `MCP_AUTH_TOKEN` keeps other local
processes from using it.

## Starting it without compose

One container, credentials on the command line. Use `OBSIDIAN_AUTH_TOKEN`
instead of the email and password if the account has MFA enabled, and note
that nothing is persisted here, so every restart re-syncs the vault:

```sh
docker run -d \
  -e OBSIDIAN_EMAIL=you@example.com \
  -e OBSIDIAN_PASSWORD=your-account-password \
  -e OBSIDIAN_VAULTS="Work:vault-password,Personal" \
  -e MCP_AUTH_TOKEN=$(openssl rand -hex 32) \
  -p 8080:8080 \
  ghcr.io/andyjmorgan/obsidian-hosted-mcp:latest
```

Docker Compose is the better default, because it keeps the synced vaults
and the sync client's credential store across restarts:

```sh
cp .env.example .env   # fill in credentials, vaults, and a generated token
docker compose up -d
```

Watch the logs: the container logs in, connects each vault, starts
continuous sync, and then serves MCP on port 8080. Health endpoints are
unauthenticated: `GET /livez` checks the HTTP process, while `GET /readyz`
requires a fresh `Fully synced` heartbeat from every vault. `/healthz` is a
backwards-compatible alias for `/readyz`.

## Connect your AI assistant

The endpoint is the server root (`/`) over streamable HTTP.

**Claude Code**

```sh
claude mcp add --scope user --transport http obsidian https://your-host/ \
  --header "Authorization: Bearer <MCP_AUTH_TOKEN>"
```

**claude.ai / Claude Desktop** — add a custom connector with URL
`https://your-host/`. With OAuth configured (below), the connector
discovers the flow and signs you in. A static token only works where the
client offers a request-header field, which a personal plan does not; see
[What claude.ai needs](#what-claudeai-needs).

**ChatGPT** — add an MCP connector (Settings → Connectors) pointing at
`https://your-host/`. OAuth-configured servers let ChatGPT run the
authorization flow itself.

**Anything else** — `npx @modelcontextprotocol/inspector`, transport
"Streamable HTTP", URL `https://your-host/`, header
`Authorization: Bearer <token>`.

## Run it on a server

Do this when you want your vault from a phone, from claude.ai in a browser,
or from agents that run while your computer is off. Everything in the
single-machine setup still applies; the rest of this section is what the
internet adds.

1. **Pick a host** that can run a container and be reached over HTTPS. TLS
   is non-negotiable — tokens travel in a header. A reverse proxy
   (Caddy/Traefik/nginx), a platform ingress, or a
   [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)
   all work; the container itself speaks plain HTTP on 8080.
2. **Publish the port to loopback only**, so the proxy is the only way in.
   The compose file's `"${PORT:-8080}:8080"` binds every interface, which on
   a public host exposes the MCP endpoint directly, without TLS. Change it
   to `"127.0.0.1:${PORT:-8080}:8080"`.
3. **Give the server its own sync token and device name.** Run `ob login`
   over SSH for a token of its own, and set `OBSIDIAN_DEVICE_NAME` so Sync
   version history names it clearly. A new login does not revoke tokens held
   elsewhere, so your laptop keeps working.
4. **Use a dedicated vault (or account) first.** The server can create,
   edit, move, and delete notes. Deletes are soft by default (they land in
   the vault's `.trash` and stay recoverable everywhere), but start with a
   test vault until you trust your setup.
5. **Persist `/home/obsidian`** (a named volume or PVC). It holds the sync
   client's credential store and the local vault copies; losing it is
   recoverable but costs a full re-sync on next boot.
6. **Run one replica.** Two sync processes on the same vault copy fight
   over the sync client's lock. On Kubernetes use `strategy: Recreate`.
7. **Pick auth**: a static API key (`MCP_AUTH_TOKEN`), OAuth via your
   identity provider (below), or both side by side. Which one you can use
   depends on the client, so read the next section before choosing.

### What claude.ai needs

A bearer token is enough for Claude Code and for most local MCP clients. It
is **not** enough for claude.ai in a browser or the Claude mobile app: on a
personal plan, a custom connector has no field for a fixed token. Sending
one as a request header is a beta limited to some organizations, so a
personal account needs OAuth. See Anthropic's
[connector authentication docs](https://claude.com/docs/connectors/building/authentication)
for the current state.

That leaves two ways to reach this server from a phone:

- **Point `OAUTH_ISSUER` at an identity provider** you already run, such as
  Keycloak, Auth0 or Entra ID. This is the supported path, described below.
- **Put a small OAuth shim in front of it**, at the same host name, that
  serves discovery, `/authorize` and `/oauth/token`, and hands back one
  fixed token that you also set as `MCP_AUTH_TOKEN`. That is far less
  machinery than a full identity provider for a single user, and the server
  needs no changes: it just sees a bearer token. Your reverse proxy routes
  the OAuth paths to the shim and everything else to the container.

Once the connector is added, set the write tools to "Always allow" if you
plan to use it hands-free, for example in a car. A tool approval prompt may
not be answerable there.

Startup is fail-fast: if login, any vault's `sync-setup`, or OAuth
discovery fails, the container exits non-zero so your orchestrator surfaces
the misconfiguration instead of serving a half-configured vault set.
During continuous operation, a vault becomes unready after two minutes
without a successful sync heartbeat. If its sync child stays silent for five
minutes, the built-in supervisor terminates and restarts that child with
exponential backoff; the MCP process and other vault syncs remain running.

## Configuration

| Variable | Required | Description |
| --- | --- | --- |
| `OBSIDIAN_EMAIL` | yes* | Obsidian account email. Not needed with `OBSIDIAN_AUTH_TOKEN`. |
| `OBSIDIAN_PASSWORD` | yes* | Obsidian account password. Not needed with `OBSIDIAN_AUTH_TOKEN`. |
| `OBSIDIAN_AUTH_TOKEN` | no | An existing `obsidian-headless` session token, used instead of email and password. Required for accounts with MFA enabled. |
| `OBSIDIAN_VAULTS` | yes | Comma-separated remote vault names, each optionally `Name:encryption-password`. |
| `OBSIDIAN_VAULT_PASSWORD` | no | End-to-end encryption password applied to vaults that don't carry their own. |
| `MCP_AUTH_TOKEN` | yes* | Static bearer token (API key) clients may present. Optional when `OAUTH_ISSUER` is set; at least one of the two is required. |
| `OAUTH_ISSUER` | no | OpenID Connect issuer URL. Setting it delegates auth to that provider — see below. |
| `OAUTH_AUDIENCE` | with `OAUTH_ISSUER` | Audience tokens must carry in `aud` (or `azp`, the authorized-party fallback used by e.g. Keycloak client tokens). |
| `MCP_PUBLIC_URL` | with `OAUTH_ISSUER` | This server's canonical public URL, used as the protected-resource identifier. |
| `OAUTH_INTERNAL_ISSUER` | no | Alternative base URL for fetching discovery/JWKS (e.g. cluster-internal). The discovery document must still report `OAUTH_ISSUER` as its issuer. |
| `OAUTH_SCOPES` | no | Comma-separated scopes advertised as `scopes_supported`. Default `openid,profile,email`. |
| `OAUTH_REQUIRED_ROLES` | no | Comma-separated roles the token must **all** carry (Keycloak `realm_access`/`resource_access`, or a flat `roles` claim), in addition to a valid audience. Empty (default) = audience-only. Use to bind the endpoint to specific principals (e.g. an owner role) regardless of which client the token came from. |
| `OBSIDIAN_DEVICE_NAME` | no | Device name shown in sync version history. Defaults to `ObsidianMCP-` plus 8 random hex characters; set it explicitly so restarts reuse one device identity. |
| `VAULTS_DIR` | no | Where vaults are synced locally. Defaults to `~/vaults`. |
| `PORT` | no | HTTP listen port, default `8080`. |

## OAuth: delegate auth to your identity provider

Setting `OAUTH_ISSUER` turns the server into an OAuth 2.0 protected
resource. It is provider-agnostic — anything OIDC-compliant works
(Keycloak, Auth0, Entra ID, Okta, ...):

- Bearer JWTs from the issuer are validated (signature via JWKS, issuer,
  lifetime, and audience — with the `azp` fallback Keycloak uses for
  client tokens).
- RFC 9728 protected-resource metadata is served at
  `/.well-known/oauth-protected-resource`, and 401 responses carry a
  `resource_metadata` challenge — so MCP clients like claude.ai and ChatGPT
  discover your authorization server and run the flow (including dynamic
  client registration, if your IdP allows it) without any manual token
  handling.
- The static API key keeps working alongside, which is handy for scripts
  and smoke tests.

Minimal example:

```sh
OAUTH_ISSUER=https://auth.example.com/realms/myrealm
OAUTH_AUDIENCE=obsidian-mcp
MCP_PUBLIC_URL=https://obsidian.example.com
```

Your IdP must issue tokens whose `aud` (or `azp`) contains
`OAUTH_AUDIENCE` — in Keycloak that's a client scope with an audience
mapper, made a realm default so dynamically-registered MCP clients pick it
up automatically.

Every tool takes a `vault` name. It is optional when the server holds a single
vault, which is the usual case, and required otherwise.

## Vault instructions

A vault can tell the model how it is organised. At startup the server reads
one file from each vault root and sends it to MCP clients as the server's
`instructions`, the same field other connectors use for usage guidance. Use it
for folder conventions, note naming, and anything you would otherwise repeat
in every conversation.

Two file names are accepted, in this order:

| File | Syncs? | Use it when |
| --- | --- | --- |
| `.mcp-instructions.md` | No | You keep the guidance on the server only. Obsidian Sync does not carry arbitrary dotfiles, so you place this one by hand. |
| `mcp-instructions.md` | Yes | You want to edit the guidance in Obsidian from any device. It is an ordinary note. |

The first one found per vault wins. Vaults without either file add nothing. If
several vaults supply guidance, each section is labelled `## Vault: <name>`.

The file is re-read for each new session, so an edit made on another device
reaches the next conversation once it syncs. The boot log reports how many
characters were loaded at startup.

## MCP tools

| Tool | Description |
| --- | --- |
| `list_vaults` | Names of the vaults served. |
| `list_notes` | List files and folders in a vault, optionally recursive. Hidden folders (`.obsidian`, `.trash`) are excluded; pass `dir: ".trash"` to browse deleted notes. |
| `read_note` | Read a note, paged at 10,240 characters per call with `offset`/`next_offset` for longer notes. |
| `get_section` | Read a heading section’s body (including subsections), with character paging. |
| `replace_section` | Replace a heading section’s body and subsections while preserving its heading. |
| `search_notes` | Searches note names and content, ranked. A plain multi-word query finds notes holding every word in any order; a query with regex characters stays a regex (`mode` forces either). Name matches rank first, flagged `title_match`, then word coverage, then match count. Supports glob filters, context lines and case sensitivity. `max_results` counts notes (default 50), and each note returns 5 matching lines unless `max_lines_per_note` says otherwise (`-1` for all); `total_matches` per note counts the rest. |
| `recent_notes` | Notes changed most recently, newest first, with modified times. Takes `since` (RFC 3339) and `limit`. |
| `list_tags` | Every tag in the vault with the number of notes carrying it, most used first. Reads frontmatter `tags` and inline hashtags. |
| `find_notes` | Query by metadata instead of text: notes carrying all the given tags, and/or a frontmatter field, with the value optional so a key alone finds every note that has it. |
| `get_links` | The wikilinks a note points at, each resolved to the note it names, or flagged unresolved when that note does not exist yet. Headings, aliases and embeds are kept. |
| `get_backlinks` | The notes that link to a note, with the links they use. |
| `get_frontmatter` | One note's frontmatter fields and tags, without its body. |
| `update_frontmatter` | Add, replace or delete frontmatter fields. Untouched fields keep their value and order, and the body is unchanged. |
| `create_note` | Create a new note; fails if it already exists. |
| `append_note` | Append to a note, creating it if needed. |
| `edit_note` | Exact find/replace; the snippet must be unique unless `replace_all` is set. |
| `move_note` | Move or rename a note. |
| `delete_note` | Move a note to the vault's `.trash` (Obsidian's own convention, recoverable everywhere); `permanent: true` removes it outright. |
| `restore_note` | Undelete: move a note out of `.trash`, back to its original name or an explicit destination. |

All paths are vault-relative and sandboxed: absolute paths and `..` escapes
are rejected.

### Working with heading sections

`get_section` and `replace_section` select sections using `heading_path`, an
array of exact, case-sensitive heading titles. A unique suffix is enough:
`["Tasks"]` selects a unique Tasks heading, while `["Project", "Tasks"]`
disambiguates Tasks under Project. If even the full hierarchy is duplicated,
the tools reject it; use `edit_note` with unique surrounding text instead.
Use the heading's Markdown title without its opening/closing heading markers
(e.g. `"**Tasks**"` for `## **Tasks**`).

A section's body starts immediately after its heading and ends before the next
heading of equal or higher rank, or at EOF. It includes nested subsections and
blank lines. Both `#`-style and underlined (setext) headings are supported.
Headings inside code blocks, blockquotes, lists, HTML blocks, and initial YAML
frontmatter are excluded. Text before the first heading has no section; use
`read_note` / `edit_note` for that text. An unclosed initial YAML frontmatter
block is treated as extending to EOF.

For a note containing `# Project`, `## Tasks`, and `## Done`, read Tasks with:

```json
{
  "name": "get_section",
  "arguments": {
    "vault": "Work",
    "path": "projects/plan.md",
    "heading_path": ["Project", "Tasks"]
  }
}
```

The structured result contains:

```json
{
  "heading_path": ["Project", "Tasks"],
  "level": 2,
  "content": "- [ ] Ship the feature\n\n",
  "offset": 0,
  "total_characters": 24,
  "truncated": false,
  "next_offset": -1
}
```

Reads return at most 10,240 Unicode characters. Pass `offset: next_offset` to
continue a truncated read; offsets are relative to the body. The returned
`heading_path` is always the full hierarchy.

Replace that body with:

```json
{
  "name": "replace_section",
  "arguments": {
    "vault": "Work",
    "path": "projects/plan.md",
    "heading_path": ["Project", "Tasks"],
    "content": "- [x] Ship the feature\n\n"
  }
}
```

Success returns `{"ok": true}`. Supply only the replacement body, **not**
`## Tasks`. The existing heading is preserved; all its old subsections are
replaced too. Empty content clears the body. Missing or ambiguous headings
fail without writing. The server adds newline separation when needed to keep
a following heading separate (a blank line for setext headings), using the
selected heading's newline style. Other bytes outside the body are preserved;
a heading at EOF without a newline gains one when adding nonempty content.

As with `edit_note`, replacement uses the current local file and has no version
precondition or transaction with concurrent sync writes. Success confirms the
local write, not completion of Obsidian Sync.

## Development

```sh
go test ./...                       # requires ripgrep on PATH for integration tests
go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out
docker build -t obsidian-hosted-mcp .
```

CI enforces `gofmt`, `go vet`, and a 95% total coverage gate, then publishes
a multi-arch (amd64/arm64) image to GHCR: `:latest` from `main` and semver
tags from `v*` releases.

## Security

[docs/SECURITY.md](docs/SECURITY.md) describes the whole picture: what is
protected, where the boundaries are, how dependencies are kept current,
what CI proves before an image ships, the known limits, and how to report a
problem. The short version:

- Every MCP request is authenticated, by a constant-time token check or by
  an OpenID Connect provider. Only the two health endpoints are open, and
  neither reads the vault.
- Run behind TLS and publish the container's port to loopback; the token
  travels in a header.
- Tools reach notes and nothing else: paths are sandboxed to the vault, and
  restricted to `.md` files outside hidden folders.
- Secrets arrive by environment variable, stay out of logs and the process
  table, and `.env` is never committed.
- Dependabot watches Go modules, base images and CI actions weekly, with
  security alerts and automated fixes on.

---

*Obsidian is a trademark of Dynalist Inc. This project is not affiliated
with or endorsed by Obsidian; it simply drives the official headless sync
client.*
