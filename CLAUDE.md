# CLAUDE.md

Guidance for AI agents working in this repository.

## Goal

An internet-hostable MCP server for Obsidian vaults. One Docker container:

1. Logs into Obsidian Sync using the **official** headless client
   (`obsidian-headless` npm package, the `ob` CLI) — never a
   reverse-engineered sync protocol.
2. Bootstraps the configured vaults to `~/vaults/<name>` and keeps them
   continuously synced.
3. Serves MCP tools over streamable HTTP to read, search, create, edit,
   move, and delete notes. Writes propagate back to the user's devices
   through Obsidian Sync.

Deliberate MVP boundaries (do not "fix" these without being asked):

- Text/markdown only; no charts, canvases, or attachments handling.
- Auth is a static bearer token (`MCP_AUTH_TOKEN`), an OIDC provider
  (`OAUTH_ISSUER` + `OAUTH_AUDIENCE` + `MCP_PUBLIC_URL`), or both. OIDC is
  provider-agnostic: discovery + JWKS only, validated in
  `internal/oidcauth`; the server advertises RFC 9728 protected-resource
  metadata so MCP clients can bootstrap the OAuth flow.
- Obsidian account auth is either `OBSIDIAN_EMAIL` + `OBSIDIAN_PASSWORD`, or
  `OBSIDIAN_AUTH_TOKEN` (an existing `ob` session token). Accounts with MFA
  must use the token: `ob login` would prompt for a code that nothing can
  answer, and running it with credentials revokes the token already in use.
- MCP `instructions` come from a vault-root file, re-read for each session so
  guidance edited on another device arrives once it syncs:
  `.mcp-instructions.md` (server-local; Obsidian Sync does not carry arbitrary
  dotfiles) or `mcp-instructions.md` (an ordinary note, so it syncs). Several
  contributing vaults are labelled `## Vault: <name>`. Missing or unreadable
  files are not an error: guidance never blocks serving.
- Deletes are soft by default: notes move to the vault's `.trash`
  (Obsidian's own convention) so they sync and stay recoverable.
- `read_note` returns at most 10,240 characters per call
  (`vault.ReadPageSize`) with `offset`/`next_offset` paging — chosen
  deliberately for LLM context-window hygiene.

## Architecture

```
cmd/obsidian-mcp/      thin main: config -> bootstrap -> serve (testable run())
internal/config/       env parsing/validation (Getenv + rand injected for tests)
internal/bootstrap/    drives the ob CLI: login, sync-setup per vault, and a
                       supervisor that restarts `ob sync --continuous` with
                       exponential backoff (reset after stable runs)
internal/vault/        sandboxed filesystem ops rooted at one vault; ALL path
                       handling goes through open(), which rejects absolute
                       paths and `..` escapes, then works through an os.Root
                       so symlinks cannot lead outside the vault
internal/notes/        YAML frontmatter parsing, tag collection (frontmatter
                       `tags` plus inline hashtags, code fences excluded),
                       frontmatter rewriting that preserves key order and body,
                       and wikilink parsing plus name-based link resolution
internal/search/       ripgrep runner; parses `rg --json` events; hidden dirs
                       (.obsidian, .trash) excluded because rg skips hidden
                       files by default; --no-ignore so stray ignore files
                       can't hide notes
internal/oidcauth/     OIDC bearer-token validation for any compliant IdP:
                       discovery (with internal-issuer override), JWKS via
                       go-oidc, audience check with Keycloak-style azp
                       fallback; tests run a real fake IdP over httptest
internal/server/       MCP tool registration (official modelcontextprotocol/
                       go-sdk, typed handlers), HTTP handler with bearer auth
                       middleware, process /livez, and sync-aware /readyz
```

Key invariants:

- **Fail-fast boot**: any login or `sync-setup` failure exits non-zero;
  never start serving a partially configured vault set.
- **Sync-aware readiness**: every vault must emit a recent `Fully synced`
  heartbeat. A silent continuous-sync child is restarted by the supervisor;
  `/livez` must remain process-only so transient network failures do not
  restart the whole container.
- **Search reads a query as words unless it looks like a regex.** Two or more
  plain words mean "a note holding all of them, in any order"; any of the
  characters in `search.regexChars` make the query a regex. `mode` overrides
  the guess. Ranking lives in `server/rank.go`: name match, then how many of
  the query's words the note covers, then match count, then path order.
- **`search_notes` covers names as well as content.** A note's name is
  usually its subject, so a content-only search misses the note a person
  would have opened by name. `vault.MatchTitles` walks for name matches and
  the handler puts them first, flagged `title_match`; content search stays in
  `internal/search`.
- **Metadata queries are separate from text search.** `find_notes` filters by
  tags and frontmatter; `search_notes` searches words. Scans read every note
  on demand: a vault of a few thousand notes is fast enough, so there is no
  index yet. A note whose frontmatter does not parse is skipped during a scan
  rather than failing it, but the single-note tools report the error.
- **Writes replace a note atomically**: `vault.writeAtomic` writes a temporary
  file beside the target and renames it over, so the sync client watching the
  folder never reads a half-written note and an interrupted write leaves the
  original intact. Keep new write paths on it. One consequence: a read-only
  note can be replaced, because rename depends on the directory.
- **Note tools work on notes only.** `requireNote` rejects anything without a
  `.md` extension and any path inside a hidden folder except the trash, which
  delete and restore need. The vault sandbox stops escapes but allows every
  file inside the vault, so this policy lives in the handlers.
- **A single-vault server takes no vault name.** `Server.vault("")` resolves to
  the only vault, and errors naming them all when there are several.
- **Path sandboxing lives in `internal/vault`**, not in tool handlers.
  New file operations must go through `open()` and the `os.Root` it
  returns, never `os` calls on a joined path, which would follow a symlink
  out of the vault.
- **Secrets never reach logs**: `bootstrap.redact()` masks `--password`
  values; keep that property when adding ob invocations.
- **External processes are injected for tests**: `ob` via a fake script on
  PATH, ripgrep via `search.RunFunc`. Follow the same pattern for new
  process dependencies; do not mock `os/exec` any other way.
- Multiple vaults are served by one process; vault names come from
  `OBSIDIAN_VAULTS` (`Name` or `Name:e2e-password`, comma-separated) with
  `OBSIDIAN_VAULT_PASSWORD` as the shared fallback.

The Docker image is three stages: Go builder → `alpine` stage with apk
`nodejs` and `npm` that installs `obsidian-headless` from the lockfile in
`headless/` (better-sqlite3 has
no musl prebuilds, so a node-gyp toolchain is installed there and build
intermediates stripped) → bare `alpine` runtime with apk `nodejs`, `ripgrep`,
`tini`, running as a non-root user. The build and runtime stages must share
one Alpine release: the sqlite addon only loads under the Node it was
compiled for, which `scripts/smoke-test.sh` checks by opening a database. Keep the runtime stage minimal; the
image-size target is "as small as possible" (~142MB today).

## Methodology

TDD is crucial here — treat it as a regular checkpoint, file by file and
feature by feature, not something to backfill at the end.

- **Bugs**: reproduce with a failing test first, then fix, then confirm the
  test passes. No bug fix lands without the test that would have caught it.
- **New features / builds**: build the piece, test it, and only then
  proceed to the next piece. After each file or feature, stop and run the
  full check (`gofmt`, `go vet`, `go test ./...`) before moving on — do not
  batch several features and test at the end.

## Quality gates

CI (`.github/workflows/ci.yml`) enforces, in order:

1. `gofmt -l .` must be empty.
1b. `node --test scripts/generate-toc.test.js` must pass: the README table of
   contents is generated, so after changing a heading run
   `node scripts/generate-toc.js` and commit the result.
2. `go vet ./...` must pass.
3. `go test ./... -coverprofile=coverage.out -covermode=atomic` must pass.
4. **Total coverage must be ≥ 95%** (currently ~96%). If you add code,
   add tests in the same change; prefer refactoring untestable error
   branches away (injection, restructuring) over excluding them.
4b. `scripts/smoke-test.sh` runs against a freshly built image: building it
   proves nothing about whether it runs, so this opens a database with the
   native sqlite module under the runtime's Node, runs `ob` and `rg`, and
   checks the server binary refuses an empty configuration. No credentials
   needed. Grype then scans the image and fails on a fixable high or
   critical vulnerability.
4c. Security jobs: a 20-second fuzz run per `Fuzz*` target, gosec
   (silence a false positive on its own line with `#nosec <rule> --
   reason`, never project-wide), `scripts/vulncheck.sh` (govulncheck under
   the Dockerfile's Go builder image), and zizmor on the workflows. Fuzz and
   gosec gate publishing; govulncheck and zizmor only turn CI red.
   Workflow actions are pinned to commit SHAs; keep new ones pinned.
5. On push to `master`, and weekly on a schedule without the layer cache:
   multi-arch (amd64/arm64) image publish of `:latest` to
   `ghcr.io/dakotahp/vault-bridge`, with an SBOM and provenance attached. Versioned images come from
   `.github/workflows/release-please.yml`, not from a tag push: a tag made
   with the default token starts no other workflow, so that workflow builds
   and pushes `:X.Y.Z` and `:X.Y` itself once a release is created.

Releases use Conventional Commits (`feat:`, `fix:`, `feat!:`). Never edit
`CHANGELOG.md`, `.release-please-manifest.json`, or the `Version` constant in
`internal/server/server.go` by hand; release-please owns them.

Conventions:

- Strict idiomatic Go; no panics in library code; errors wrapped with
  `%w` and enough context to act on.
- Tests use real filesystems (`t.TempDir()`), real ripgrep when present
  (skip otherwise), fake `ob` shell scripts on PATH, and a real MCP client
  session over HTTP for end-to-end coverage — keep new tests in that style
  rather than introducing mocking frameworks.
- Run before committing:
  `gofmt -l . && go vet ./... && go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1`

## Reference material (not committed)

`.reference/` is gitignored and may contain clones of
`kepano/obsidian-skills` and `MarkusPfundstein/mcp-obsidian` for
inspiration. Never commit it. The closest prior art is
`alexjbarnes/vault-sync`; this project intentionally differs by using the
official headless client and serving multiple vaults per container.
