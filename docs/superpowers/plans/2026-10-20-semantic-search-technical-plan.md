# Technical Plan: Opt-in Semantic Search

**Date:** 2026-10-20
**Status:** Draft
**Source:** conversation; prior art is a local experiment with Ollama and `nomic-embed-text` over a 2,280-note vault

---

## Summary

Add search by meaning to Crystal Cove. When an operator sets one environment variable, the server indexes each vault with a small embedding model and offers two new tools: one finds notes that match a question by meaning, and one finds notes related to a given note. Everything runs inside the existing container, with no extra service. When the variable is not set, the server behaves exactly as it does today.

## Current State

- Tools are registered in `Server.MCPServer()` (`internal/server/server.go`); write tools are gated by a policy flag in the same way this feature will be (`server.go:207`, `addWriteTools`).
- `search_notes` is exact: ripgrep for words, phrases, and regexes (`internal/search`), ranked in `internal/server/rank.go`. It stays unchanged.
- `internal/vault/sections.go` already splits a note into heading sections with goldmark (`markdownSections`), and `walkNotes` (`internal/vault/walk.go`) walks every note while skipping hidden folders.
- The Go binary builds with `CGO_ENABLED=0` on Alpine (`Dockerfile`). External programs (`ob`, `rg`) run as child processes and are faked in tests.
- `docker-compose.override.yml` builds the image from local source, so `docker compose up --build` on a branch runs that branch. Nothing watches files today; the only ticker is the sync heartbeat (`internal/bootstrap/bootstrap.go`).

## Approach

**Inference runs in a child process.** The runtime image adds Alpine's `llama-server` package (llama.cpp, in `community` for amd64 and arm64) and bakes in the `nomic-embed-text-v1.5` GGUF model, pinned by checksum. When semantic search is on, the server starts `llama-server` in embedding mode on `127.0.0.1` and supervises it the way `bootstrap` supervises `ob sync`. A small embedder client talks to it over HTTP, so tests can use an `httptest` server instead of the real model.

**A new `internal/semantic` package owns the index.** It splits each note into sections with the existing section parser, joins sections under about 400 characters with their neighbors, and splits sections over about 1,500 characters at paragraphs. It drops a chunk whose text appears in many notes, which removes template text such as daily-note boilerplate without naming any folder or template. Vectors live in memory and in a file under `~/.crystal-cove/semantic/<vault>`, on the persistent volume. A search is a plain dot product over every vector; a few thousand notes need milliseconds, so there is no vector database.

**The index refreshes itself.** On start it loads the saved file, then a background scan re-embeds only notes whose content hash changed, and removes deleted notes. The same scan repeats every few minutes so changes from sync arrive without a file watcher. Tools answer during the first build, and say how much of the vault the index covers.

**Two tools, registered only when the flag is on.** `semantic_search` takes a question and returns notes with the best matching section and a short excerpt. `related_notes` takes a note and returns the notes whose sections are closest to any of its sections, marking the ones already linked. Both say in their descriptions that results are by meaning, not exact words, and point to `search_notes` for exact matches.

**Descriptions cross-reference each other.** Agents choose tools from their descriptions and reach for `search_notes` out of habit. So when the flag is on, `search_notes` says to use `semantic_search` for a topic in other words or when it finds little, and `get_backlinks` points to `related_notes` for notes on the same topic that do not link. When the flag is off, neither mentions tools that do not exist.

## Key Decisions

| Decision | Chose | Instead of | Why |
|---|---|---|---|
| Where inference runs | `llama-server` child process inside the image | Ollama sidecar container or an outside API | Everything stays in one container, and no vault text leaves the server |
| Inference engine | Alpine's packaged llama.cpp | ONNX Runtime through cgo, or pure-Go inference | Keeps `CGO_ENABLED=0`; works on musl for both architectures; runs the same model the experiment proved |
| Model | `nomic-embed-text-v1.5`, 8-bit GGUF | `all-MiniLM-L6-v2` or `bge-small` | It gave good results in the experiment; the smaller models save about 100 MB but are less accurate |
| Model delivery | Baked into the image | Downloaded on first use | Works offline and with a read-only root; the image grows and that is accepted |
| Feature switch | `MCP_SEMANTIC_SEARCH=true`, off by default | Always on | Beta first; when off, nothing starts and no tool appears |
| Tool surface | Two new tools | Blending vector results into `search_notes` | Agents choose fuzzy results on purpose; exact search stays predictable |
| Index storage | Flat vector file plus metadata, searched by brute force | sqlite-vec or a vector database | Vault scale is small; no new dependency or native extension |
| Chunk unit | Heading sections, small ones joined | Whole notes | Whole-note vectors blur long notes; the experiment showed both too-big and too-small chunks fail |
| Template handling | Drop chunks repeated across many notes | Skip configured folders | Works for any vault, without one person's folder names |

## Affected Areas

- `Dockerfile`: runtime stage adds `llama-server`; a new stage fetches the model and checks its checksum. The image grows from about 142 MB to 295 MB (measured).
- `internal/config`: new `MCP_SEMANTIC_SEARCH` variable.
- `internal/semantic` (new): chunking, embedder client, index, refresh loop, child-process supervision.
- `internal/server`: two tool handlers, registered only when the feature is on; they use `s.note` for path policy like the other tools.
- `cmd/crystal-cove/main.go`: starts the indexer when the flag is set.
- `scripts/smoke-test.sh`: checks that `llama-server` loads the model and embeds a string under the compose hardening.
- `docs/configuration.md`, `docs/tools.md`, README: document the variable and the tools.
- Outside consumers: every MCP client sees two new tools only when the operator opts in. The published image grows for everyone once merged.

## Risks and Unknowns

| Risk or unknown | Impact if wrong | How to resolve |
|---|---|---|
| CPU-only embedding is slower than Ollama on a Mac GPU | Measured: 54 minutes for the 2,280-note test vault (about 2 sections a second), against 4 minutes on the Mac GPU. Four parallel slots gained only 17%. | Accepted as a one-time cost: vectors are saved and later refreshes embed only changes. A smaller model would be about 4 times faster at some cost in quality, if beta testers find the wait too long |
| `llama-server` memory use | Measured: 143 MiB with one slot, 391 MiB with four | Kept at one slot; documented as the cost of opting in |
| 8-bit model results differ from the Ollama F16 model | Results worse than the experiment | Compare the same queries against the experiment's output before beta |
| Grype finds a fixable high CVE in llama.cpp | CI blocks publishing | Track Alpine updates; the package is in `community`, so updates may lag |
| `llama-server` needs a writable path despite the read-only root | Child fails to start in production compose | Smoke test runs it with the same hardening |
| Repeated-chunk threshold removes real content | A note that quotes a common passage loses that section | Start conservative and tune on the beta vault |
| Coverage gate at 95% | Supervisor and refresh error paths are hard to reach | Inject the process launcher and clock, as `bootstrap` does |

## Sequencing

1. **Image spike:** add `llama-server` and the model to the Dockerfile; confirm it embeds a string under the compose hardening on both architectures. This proves the riskiest assumption first.
2. **Index core:** `internal/semantic` chunking, embedder client, index file, and brute-force search, with tests against a fake embedder.
3. **Refresh and supervision:** background scan, content-hash updates, child-process restart. Can overlap with step 4.
4. **Tools and config:** `MCP_SEMANTIC_SEARCH`, both tools, end-to-end tests over a real MCP session.
5. **Beta:** on the work machine, `git checkout` the branch, set the variable in `.env`, and run `docker compose up --build`. Compare results with the experiment, then tune thresholds.
6. **Docs and smoke test,** then merge when the beta is good.

## Out of Scope

- Blending semantic results into `search_notes` or `find_notes`.
- Automatic tagging or link suggestions written into notes.
- Attachments, PDFs, images, and canvases.
- GPU acceleration and a choice of models.
- An external embedding service as an option.
- A separate image tag for the feature.
