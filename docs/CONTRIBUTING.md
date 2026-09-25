# Contributing

Suggestions and proposals are welcome. Create an issue and @ mention me to notify me.

## Development

```sh
go test ./...                       # requires ripgrep on PATH for integration tests
go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out
docker build -t crystal-cove .
```

The table of contents at the top of this README is generated. After you add or rename a heading, run `node scripts/generate-toc.js` (Node 18 or newer) and commit the result.

CI enforces `gofmt`, `go vet`, a 95% total coverage gate, and an up-to-date table of contents, then publishes a multi-arch (amd64/arm64) image to GHCR. Every push to `master` updates `:latest`.

Releases are automatic. Write commit messages, or squash-merge PR titles, as [Conventional Commits](https://www.conventionalcommits.org): `fix:` makes a
patch release, `feat:` makes a minor release, and `feat!:` makes a breaking one. [release-please](https://github.com/googleapis/release-please) collects
them into a release PR that updates `CHANGELOG.md` and the version in
`internal/server/server.go`. Merging that PR tags `vX.Y.Z`, creates a GitHub release, and publishes the image as `:X.Y.Z` and `:X.Y`.

## 