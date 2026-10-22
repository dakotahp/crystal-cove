# syntax=docker/dockerfile:1
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG COMMIT=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/dakotahp/crystal-cove/internal/server.Commit=${COMMIT}" \
    -o /out/crystal-cove ./cmd/crystal-cove

# Build obsidian-headless separately: better-sqlite3 has no musl prebuilds
# and needs a node-gyp toolchain; build intermediates are stripped before
# the copy into the runtime stage. Its native addon is tied to the Node
# release it was compiled against, so this stage uses the runtime's own
# Alpine and apk nodejs rather than a node image.
# headless/package-lock.json pins the whole dependency tree, and only
# better-sqlite3, which must compile, runs an install script.
FROM alpine:3.24 AS headless
RUN apk add --no-cache nodejs npm python3 make g++
WORKDIR /opt/headless
COPY headless/package.json headless/package-lock.json ./
RUN npm ci --omit=dev --ignore-scripts \
    && npm rebuild better-sqlite3 \
    && cd node_modules/better-sqlite3 \
    && rm -rf deps src build/deps build/Release/obj build/Release/obj.target

# Bare Alpine runtime: apk nodejs runs the sync client, ripgrep backs
# search_notes, tini reaps the ob sync children, and llama-server runs the
# embedding model when MCP_SEMANTIC_SEARCH is on. scripts/smoke-test.sh
# opens a sqlite database to confirm the addon loads under this Node, and
# embeds a sentence to confirm the model loads.
FROM alpine:3.24
# Temporary: apk upgrade pulls in Alpine's fix for zlib CVE-2026-85091, which the
# base image lacks. Remove it once, after `docker pull alpine:3.24`,
# `docker run --rm alpine:3.24 apk list --installed zlib` shows 1.3.2-r1 or later.
RUN apk upgrade --no-cache \
    && apk add --no-cache nodejs ripgrep tini libstdc++ llama-server \
    && adduser -D -h /home/obsidian obsidian \
    && mkdir -m 755 /opt/crystal-cove
# nomic-embed-text v1.5 (Apache-2.0), pinned to a Hugging Face commit and checksum.
ADD --checksum=sha256:3e24342164b3d94991ba9692fdc0dd08e3fd7362e0aacc396a9a5c54a544c3b7 --chmod=644 \
    https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/0188c9bf409793f810680a5a431e7b899c46104c/nomic-embed-text-v1.5.Q8_0.gguf \
    /opt/crystal-cove/nomic-embed-text-v1.5.Q8_0.gguf
COPY --from=headless /opt/headless/node_modules /opt/headless/node_modules
RUN ln -s /opt/headless/node_modules/obsidian-headless/cli.js /usr/local/bin/ob
COPY --from=build /out/crystal-cove /usr/local/bin/crystal-cove
USER obsidian
ENV HOME=/home/obsidian
EXPOSE 8080
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["crystal-cove"]
