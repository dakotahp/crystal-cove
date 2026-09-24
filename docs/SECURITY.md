# Security

This server holds a complete copy of a personal knowledge base and exposes it to AI assistants, on a machine that may face the internet. That shapes every design decision below.

## What it protects

- **The vault's contents.** Notes, and the Obsidian Sync credentials that
  reach them.
- **The host.** A connected assistant must not be able to read or write
  anything outside the vault.
- **The account.** A leaked credential must be revocable without losing the vault.

## Boundaries

### The MCP endpoint is authenticated, always

Every MCP request carries a bearer token. There is no unauthenticated mode and no way to disable the check.

- **Static token** (`MCP_AUTH_TOKEN`): compared in constant time, so a
  wrong token takes the same time to reject as a right one and cannot be guessed byte by byte. The server refuses to start with a token shorter than 32 characters, so a placeholder or a short word never guards a live vault.
- **OpenID Connect** (`OAUTH_ISSUER`): tokens are validated against the
  provider's published keys, with issuer, audience and expiry checked, and optional required roles (`OAUTH_REQUIRED_ROLES`) to bind the endpoint to specific principals. The server advertises RFC 9728 protected-resource metadata so clients can find the authorization server themselves.
- Both can run side by side.

Only two endpoints are unauthenticated, and neither reads the vault:
`/livez` reports that the process is alive, and `/readyz` reports whether
every vault has a recent sync heartbeat.

### TLS is the operator's job, and it is required

The container speaks plain HTTP on port 8080. Credentials travel in a
header, so a public deployment must terminate TLS in front of it with a
reverse proxy, an ingress or a tunnel. The compose file publishes the
container's port to loopback only, so the proxy is the only way in; Docker's
published ports bypass host firewalls such as ufw, so this default matters. Tokens
belong in headers, never in a URL, where they would land in proxy logs and
browser history.

### The vault is the only reachable part of the filesystem

Two layers, because one is not enough:

1. **Path sandboxing.** Every path a tool receives is resolved through a
   single function that rejects absolute paths and any `..` that would
   escape the vault root. The file is then opened through Go's `os.Root`,
   which refuses a symlink that leads outside the vault, so a link placed
   in the vault folder cannot expose or overwrite the host. No tool handler
   builds paths on its own.
2. **Notes only.** Tools additionally require a `.md` path outside hidden
   folders, so a client cannot read or rewrite Obsidian's own
   configuration, a stylesheet, or a dotfile that happens to sit in the
   vault. `list_notes` refuses hidden folders the same way, so it cannot
   enumerate `.obsidian` either. The vault's `.trash` stays reachable,
   because delete and restore work through it.

### Secrets stay out of logs, arguments and the repository

- Credentials arrive only through environment variables.
- The sync token passes to the sync client through the environment, not on a command line, so it never appears in the process table.
- Command logging masks password arguments before writing them.
- Child processes get only what they need. ripgrep sees `PATH` alone, and
  the sync client does not see the MCP token or the passwords it receives
  as arguments.
- `.env` is ignored by git, and the repository carries only placeholders.

### Writes cannot corrupt a note

- A note is replaced by writing a temporary file and renaming it over the
  original, so a reader, including the sync client watching the folder,
  sees either the old note or the new one, and an interrupted write leaves
  the original intact.
- Deletes move a note to the vault's `.trash` by default, where it stays
  recoverable from any device; permanent deletion is explicit.
- Creating a note fails if it already exists, and an edit must match
  exactly once unless the caller asks for every occurrence.

### The container runs with little privilege

A multi-stage build produces a small Alpine runtime holding only Node, the
sync client, ripgrep and an init process. It runs as an unprivileged user,
with `tini` as PID 1 to reap the sync children. The compose file drops
every Linux capability and blocks privilege escalation, since nothing in
the container needs either. The image is built and
published by CI rather than by hand.

## Keeping dependencies current

A dependency is the most likely way a vulnerability arrives, so this is
deliberate rather than occasional.

- The Go module set is intentionally small: the MCP SDK, an OIDC library, a
  Markdown parser and a YAML parser. Fewer dependencies, less to audit.
- `go.mod` and `go.sum` pin every version, direct and indirect, and Go
  verifies checksums on every build.
- The sync client, which holds the account token and vault passwords, is
  installed from `headless/package-lock.json`, so a build never picks up a
  release nobody reviewed. Only its sqlite module, which must compile, runs
  an install script.
- Dependabot watches Go modules, the sync client, the Dockerfile's base
  images and the CI actions every week. Routine patch updates arrive grouped; a major version
  arrives on its own, where it gets read.
- Dependabot security alerts and automated security fixes are enabled, so a
  known vulnerability opens a pull request without waiting for the weekly
  run.
- Every update is a pull request that must pass CI and be reviewed. Nothing
  updates itself into the published image.

## What CI proves before an image ships

1. Formatting, `go vet`, and gosec static security analysis. A gosec
   warning that is a false positive is silenced on its own line with the
   reason, never for the whole project.
2. The full test suite, with total coverage held at 95% or above.
3. A short fuzz run against the path sandbox and the rule for which notes
   the write tools may change.
4. A smoke test that runs the built image: it opens a database with the
   native sqlite module under the runtime's Node, runs the sync client and
   ripgrep, and checks the server refuses to start with an empty
   configuration. Building an image does not prove it runs, so this gate
   exists.
5. A Grype scan of the built image, covering Alpine packages, the sync
   client's npm tree and the Go binary. A high or critical vulnerability
   that has a fix stops the image, since updating is then all it takes.

Publishing waits on all of it. Each published image carries an SBOM and
full build provenance as registry attestations, so what it contains and
which workflow built it can be checked with `docker buildx imagetools
inspect`.

CI also rebuilds and republishes `:latest` every week without the layer
cache, so Alpine, Node and Go security patches reach the image even when
nothing in this repository changes. Versioned tags are not rebuilt.

CI audits its own workflows with zizmor, which catches injectable
expressions, over-broad tokens, unpinned actions and cache poisoning.

CI also runs `govulncheck` with the Go builder image the Dockerfile names,
so it checks the standard library that ships. A finding turns CI red but
does not hold back publishing: the fix for a Go vulnerability arrives by
rebuilding, so blocking the rebuild would block the fix.

## Known limits

Stated plainly, because a security document that claims everything is
covered is not useful.

- **A static token is a shared secret.** Anyone holding it has the vault's
  full tool set. Use OpenID Connect where individual identity matters, and
  rotate the token by restarting with a new one.
- **Notes are untrusted input to a language model.** A note can contain
  text that tries to steer an assistant into doing something you did not
  ask for. No server-side check can fully prevent that. Keep write tools
  behind your client's approval settings when that risk matters, and
  consider a dedicated vault. One door is closed on the server: tools
  cannot change `mcp-instructions.md`, so a steered assistant cannot plant
  instructions that every later session would receive.
- **Single tenant.** One credential set, one account. This is not a
  multi-user service, and it does not try to be.
- **No rate limiting and no audit log.** A reverse proxy can add the first.
  The second is not implemented.
- **Vault data sits unencrypted on the host**, in the container's volume,
  protected by the host's own disk encryption and access control. An
  end-to-end encrypted vault is decrypted here, because the server has to
  read the notes to serve them.
- **The sync token grants access to the Sync account's vaults.** Treat it
  like a password: keep it in `.env`, and revoke it with `ob logout` if a
  machine holding it is lost.
- **Account password mode puts the password in the container's process
  table.** Logging in with `OBSIDIAN_EMAIL` and `OBSIDIAN_PASSWORD` passes
  the password to the sync client as a command-line argument, so anything able to list processes inside that container could read it. Supplying `OBSIDIAN_AUTH_TOKEN` instead avoids this, and is also what an account with MFA needs.

## Reporting a problem

Open an issue describing what you found, how to reproduce it, and what it lets an attacker do. If you have a fix, a pull request is welcome; link it to the issue.

If the problem is serious enough that a public description would put
existing deployments at risk, open an issue asking for a private channel
instead of posting the details, and the discussion will move there.
