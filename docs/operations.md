# Operations

## Health checks

Both endpoints need no authentication.

- `GET /health` checks only that the HTTP process is up.
- `GET /ready` requires a fresh `Fully synced` heartbeat from every vault.

A reverse proxy that checks the bearer token itself answers 401 to these paths when the request has no token. Probe them on the host (`http://127.0.0.1:8080/ready`), or send the token through the proxy.

## Logs

The sync client prints `Fully synced` every 30 seconds. The server shows that line only after other sync activity, such as a download, so the logs record each time sync caught up without repeating it. The heartbeat still counts toward `/ready` every time. To find the server's own startup lines, filter the logs: `docker compose logs | grep level=`.

Point probes that restart the container at `/health`, so a network blip that stalls sync does not restart the whole container.

## Startup and sync recovery

Startup is fail-fast. If login, any vault's `sync-setup`, or OAuth discovery fails, the container exits non-zero. Your orchestrator then surfaces the misconfiguration instead of serving a half-configured vault set.

While running, a vault becomes unready after two minutes without a successful sync heartbeat. If its sync process stays silent for five minutes, the built-in supervisor restarts that process with exponential backoff. The MCP server and the other vaults keep running.

## Running on a server

- **Persist `/home/obsidian`** (a named volume or PVC). It holds the sync client's credential store and the local vault copies. Losing it is recoverable, but costs a full re-sync on the next boot. The compose file already does this.
- **Run one replica.** Two sync processes on the same vault copy fight over the sync client's lock. On Kubernetes, use `strategy: Recreate`.

## Running without Compose

One container, credentials on the command line. Nothing is persisted here, so every restart re-syncs the vault:

```sh
docker run -d \
  -e OBSIDIAN_AUTH_TOKEN=your-token \
  -e OBSIDIAN_VAULTS="Work:vault-password,Personal" \
  -e MCP_AUTH_TOKEN=$(openssl rand -hex 32) \
  -p 127.0.0.1:8080:8080 \
  ghcr.io/dakotahp/crystal-cove:latest
```

Docker Compose is the better default, because it keeps the synced vaults and the sync client's credential store across restarts.
