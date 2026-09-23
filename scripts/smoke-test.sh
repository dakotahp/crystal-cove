#!/bin/sh
# Check that a built image can actually run, which building it does not
# prove: the native sqlite module has to load under the runtime's Node, the
# tools the server shells out to have to exist, and the server binary has to
# run against the runtime's libc. No Obsidian credentials are needed.
set -eu

image="${1:?usage: smoke-test.sh <image>}"
run() { docker run --rm --entrypoint sh "$image" -c "$1"; }

echo "==> node and the sqlite module the sync client needs"
run 'node -v'
run 'node -e "require(\"/usr/local/lib/node_modules/obsidian-headless/node_modules/better-sqlite3\")"'

echo "==> the sync client runs"
run 'ob --help >/dev/null'

echo "==> ripgrep, which search_notes shells out to"
run 'rg --version | head -1'

echo "==> the server binary runs and refuses an empty configuration"
output=$(docker run --rm --entrypoint obsidian-mcp "$image" 2>&1 || true)
echo "$output" | head -1
case "$output" in
  *OBSIDIAN_EMAIL*) ;;
  *) echo "expected a configuration error naming OBSIDIAN_EMAIL, got: $output"; exit 1 ;;
esac

echo "==> smoke test passed"
