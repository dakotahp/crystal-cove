<p align="center">
  <img src="docs/images/banner.svg" alt="Crystal Cove: Obsidian MCP with native sync and note-aware tools. A purple crystal rises from a moonlit sea cove." width="100%">
</p>

# Crystal Cove

Cloud Obsidian Vault, ready for Claude and ChatGPT, in one container. Native sync, note tools, and sign-in are all built in, so there is nothing else to set up. (Works locally on your laptop, too.)

<div>
<a href="https://github.com/dakotahp/crystal-cove/actions/workflows/ci.yml"><img src="https://github.com/dakotahp/crystal-cove/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
<a href="https://github.com/dakotahp/crystal-cove/pkgs/container/crystal-cove"><img src="https://img.shields.io/badge/ghcr.io-crystal--cove-blue?logo=docker" alt="Container image"></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="MIT license"></a>
</div>

<!-- toc -->

- [What's in the box](#whats-in-the-box)
- [The Why](#the-why)
- [Which setup do you need?](#which-setup-do-you-need)
- [Installation](#installation)
  - [On your computer](#on-your-computer)
  - [On a cloud server](#on-a-cloud-server)
- [What agents can do](#what-agents-can-do)
- [More docs](#more-docs)

<!-- /toc -->

## What's in the box

One container holds every part an assistant needs to work in your vault:

- **Obsidian Sync.** Obsidian's official [headless client](https://help.obsidian.md/install/headless) keeps the vault in sync. No need for the desktop app or CLI.
- **An MCP server.** Tools that read, search, and edit notes the way Obsidian sees them, over the [Model Context Protocol](https://modelcontextprotocol.io).
- **Sign-in.** A built-in sign-in page for claude.ai, the Claude mobile app, and ChatGPT.
- **Secure by design.** A read-only container, soft deletes to the vault's `.trash`, and a readiness check that waits for sync.

You only need only:

- An [Obsidian Sync](https://obsidian.md/sync) subscription.
- Docker.
- For use from a phone or claude.ai: A virtual private server with HTTPS URL, if you want to make your vault accessible to cloud agents.

## The Why

<p align="center">
  <img src="docs/images/trifecta.png" alt="Three overlapping circles: native sync, reach from anywhere, and tools that understand notes. Only the center, where all three meet, is the full setup. Native sync plus reach alone means primitive searching. Native sync plus note tools alone means laptop only, no mobile. Reach plus note tools alone means reinventing the wheel." width="100%">
</p>

Obsidian has no first-party way to give an agent your vault from a cloud server. Its CLI needs the desktop app to be running and has many idiosyncrasies.

An assistant needs three things at once to work well in a vault, and most other projects give two:

1. **The official sync engine.** Nothing here reimplements Obsidian Sync, and nothing depends on a desktop app that is awake.
2. **Reach from anywhere.** MCP over HTTPS with sign-in, so a browser, a phone, or a cloud agent can connect.
3. **Tools that understand notes.** A vault is more than a folder of Markdown files. Tools that know about names, tags, links, and sections use fewer tokens than `read_file` and `list_directory`.

Crystal Cove packs all three into one container. That means:

- **Claude** (claude.ai, Claude Code, Claude Desktop) and **ChatGPT** can read, search, create, edit, and organize your notes.
- Every write syncs to your Obsidian apps within seconds. A note you write on your phone is visible to your assistant.
- It runs all the time wherever you host containers: a NAS, a VPS, Kubernetes, or a Raspberry Pi (images are amd64 and arm64).

## Which setup do you need?

The same container works in two places, and agents see the same tools in both:

|          | **On your computer**                                         | **On a server**                                              |
| -------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Connects | Claude Code, Cursor, and other MCP clients on that computer  | claude.ai, the Claude mobile app, ChatGPT, and coding agents anywhere |
| Address  | `http://127.0.0.1:8080/`                                     | `https://obsidian.example.com/`                              |
| Sign-in  | A token you generate                                         | The built-in sign-in page and your password                  |
| You need | Docker                                                       | Docker, a domain name, and HTTPS (a reverse proxy or tunnel) |
| Good for | Daily agent work where you don't need remote access, in place of a plugin or the Obsidian CLI | Your vault from a phone, and agents that run while your computer is off |

## Installation

To use Crystal Cove, you copy the docker-compose file to where you will run it, sign into Obsidian Sync, and hook it up to your agents. Accounts with multi-factor authentication and end-to-end encrypted vaults both work.

**1. Download the compose file** into a new folder:

```sh
mkdir crystal-cove && cd crystal-cove
curl -O https://raw.githubusercontent.com/dakotahp/crystal-cove/master/docker-compose.yml
```

**2. Log in to Obsidian Sync.** The native Obsidian Sync command asks for your email, password, and MFA code if you have one, then prints a session token:

```sh
docker run --rm -it ghcr.io/dakotahp/crystal-cove:latest \
  sh -c 'ob login && cat ~/.config/obsidian-headless/auth_token'
```

Run it on the machine where the container will run. Each login is a separate Obsidian Sync device, and it does not sign out your other devices. If the token stops working, do this step again.

**3. Find your vault's name** as Obsidian Sync spells it:

```sh
docker run --rm -e OBSIDIAN_AUTH_TOKEN=<token from step 2> \
  ghcr.io/dakotahp/crystal-cove:latest ob sync-list-remote
```

The list also shows an ID for each vault. You need only the name.

**4. Create a `.env` file** next to `docker-compose.yml`. There are three secrets, and each comment says what it unlocks:

```sh
# Unlocks your Obsidian account: the token from step 2.
OBSIDIAN_AUTH_TOKEN=

# The vault name from step 3.
OBSIDIAN_VAULTS=Personal

# Unlocks an end-to-end encrypted vault. Not your account password.
#OBSIDIAN_VAULT_PASSWORD=

# Unlocks this server for agents that send a token, such as Claude Code.
# Make one with: openssl rand -hex 32
MCP_AUTH_TOKEN=
```

**Is your vault end-to-end encrypted?** If you chose an encryption password when you set up Obsidian Sync for it, yes. Set `OBSIDIAN_VAULT_PASSWORD`, or the container stops with "Password not provided".

The [example file](.env.example) lists every option. Now follow the section for where you run it.

### On your computer

Use this method if you only want to use this on one computer, it is easy to install locally to avoid going through the Obsidian CLI and Desktop app, whether you have both on the computer or not.

**1. Start it and wait for the first sync:**

```sh
docker compose up -d
docker compose logs -f          # wait for "Fully synced", then Ctrl-C
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/ready   # 200
```

The first sync downloads the whole vault into a Docker volume, so a large vault takes a few minutes. If port 8080 is in use, set `HOST_PORT` in `.env` to a different port and use that port in these URLs.

**2. Connect an agent.** For Claude Code:

```sh
claude mcp add --scope user --transport http obsidian http://127.0.0.1:8080/ \
  --header "Authorization: Bearer $MCP_AUTH_TOKEN"
```

`--scope user` makes the vault available in every folder. Do not use `--scope project`: it writes the token into a `.mcp.json` file that is meant to be committed. Then `claude mcp list` shows it as connected.

Other MCP clients take the same URL and `Authorization` header over "Streamable HTTP".

**Stopping:** `docker compose stop` keeps the synced copy for next time. `docker compose down -v` also deletes it, so the next start syncs from the beginning.

### On a cloud server

Use this method if you wish to use your vault from a phone, from claude.ai in a browser, or from agents that run while your computer is off. It makes your vault truly available form anywhere, securely. Sign-in is built in. You only need to create a password and a domain with TLS/SSL, and the apps do the rest.

**1. Turn on sign-in.** Add to `.env`:

```sh
# The address your apps will use. HTTPS, with no path.
MCP_PUBLIC_URL=https://obsidian.example.com

# The password you will login with when you connect your AI service. Minimum of 16 characters required.
MCP_OWNER_PASSWORD=

# The name Obsidian Sync version history shows for edits from the server.
OBSIDIAN_DEVICE_NAME=crystal-cove
```

**2. Give it an HTTPS address.** This is the only part the container does give you. It speaks plain HTTP on `127.0.0.1:8080`, and tokens travel in a header, so put an HTTPS proxy in front of it. [Caddy](https://caddyserver.com) is easy and recommended, and handles the HTTPS the certificate for you. The only Caddy configuration you'll need is:

```
obsidian.example.com {
	reverse_proxy localhost:8080
}
```

Traefik, nginx, a platform ingress, or a [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) also work. Point them at the port in `HOST_PORT` if you changed it. **Do not** set `BIND_ADDRESS=0.0.0.0`: that exposes the server without HTTPS.

**3. Start it and wait for the first sync:**

```sh
docker compose up -d
docker compose logs -f          # wait for "Fully synced", then Ctrl-C
curl -s -o /dev/null -w '%{http_code}\n' https://obsidian.example.com/ready   # 200
```

A 502 means the proxy cannot reach the container. Make sure it points at the same port as `HOST_PORT`.

**4. Connect your assistant.**

- **claude.ai / Claude Desktop / Claude mobile:** add a custom connector with your URL like `https://obsidian.example.com/`. Leave the client ID and secret empty. A Crystal Cove sign-in page will open and enter your owner password from step 1, and you should return to Claude connected.
- **ChatGPT:** add an MCP connector (Settings → Connectors) with the same URL, and sign in the same way.
- **Claude Code:** use the same `claude mcp add` command as on your computer, with your server's URL.

Adjust tool permissions as you see fit. If you intend to use it hands-free, for example in a car, set may need to set all write tools to "Always allow".

Already run an identity provider such as Keycloak or Auth0? Crystal Cove can use it in place of the built-in sign-in. See [docs/oauth.md](docs/oauth.md).

## What agents can do

Agents get tools that work with notes, not raw files:

- **Read:** `read_note`, `get_section`, `get_frontmatter`, `list_notes`, `recent_notes`
- **Find:** `search_notes` (by name and content, ranked), `find_notes` (by tags and frontmatter), `list_tags`, `get_links`, `get_backlinks`
- **Write:** `create_note`, `append_note`, `edit_note`, `edit_section`, `update_frontmatter`, `move_note` (also restores from the trash), `delete_note`

Note deletes are soft by moving the file to the vault's `.trash` so it will be recoverable. Set `MCP_READ_ONLY=true` to give agents only the read and find tools. See [docs/tools.md](docs/tools.md) for every tool's details.

You can also give agents guidance about your vault, like an AGENTS.md file. Put it in a note called `mcp-instructions.md` at the vault root. See [docs/vault-instructions.md](docs/vault-instructions.md). The advantage of this is to describe your vault if you have [a particular organization structure](https://www.buildingasecondbrain.com/para) or have certain concepts so the agent's context goes beyond folders with notes.

## More docs

- [Configuration](docs/configuration.md): every environment variable
- [OAuth](docs/oauth.md): signing in from claude.ai and ChatGPT
- [MCP tools](docs/tools.md): what each tool does
- [Vault instructions](docs/vault-instructions.md): guidance for agents
- [Operations](docs/operations.md): health checks, recovery, and running without Compose
- [Contributing](docs/CONTRIBUTING.md): developing this project
- [Security](docs/SECURITY.md): Crystal Cove takes security is extremely seriously because the remote installation method exposes your vault to the open internet by design.

---

Started as a fork of [Obsidian-Hosted-Mcp](https://github.com/andyjmorgan/Obsidian-Hosted-Mcp) by Andrew Morgan.

*Obsidian is a trademark of Dynalist Inc. This project is not affiliated
with or endorsed by Obsidian; it simply drives the official headless sync client.*
