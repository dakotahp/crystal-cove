#!/bin/sh
# Check that a built image can actually run, which building it does not
# prove: the native sqlite module has to load under the runtime's Node, the
# tools the server shells out to have to exist, and the server binary has to
# run against the runtime's libc. No Obsidian credentials are needed.
set -eu

image="${1:?usage: smoke-test.sh <image>}"
# The same restrictions docker-compose.yml applies, so a change that needs a
# writable root filesystem or a capability fails here first.
hardened="--read-only --tmpfs /tmp --tmpfs /home/obsidian:uid=1000,gid=1000 --cap-drop ALL --security-opt no-new-privileges:true"
run() { docker run --rm $hardened --entrypoint sh "$image" -c "$1"; }

echo "==> node and the sqlite module the sync client needs"
run 'node -v'
# Requiring better-sqlite3 does not load its native addon; opening a
# database does, and that is where a Node ABI mismatch shows up.
run 'node -e "
  const dir = require(\"path\").dirname(require(\"fs\").realpathSync(\"/usr/local/bin/ob\"));
  const Database = require(require.resolve(\"better-sqlite3\", { paths: [dir] }));
  new Database(process.env.HOME + \"/probe.db\").prepare(\"select 1\").get();
"'

echo "==> the sync client runs"
run 'ob --help >/dev/null'

echo "==> ripgrep, which search_notes shells out to"
run 'rg --version | head -1'

echo "==> the server binary runs and refuses an empty configuration"
output=$(docker run --rm $hardened --entrypoint obsidian-mcp "$image" 2>&1 || true)
echo "$output" | head -1
case "$output" in
  *OBSIDIAN_EMAIL*) ;;
  *) echo "expected a configuration error naming OBSIDIAN_EMAIL, got: $output"; exit 1 ;;
esac

echo "==> smoke test passed"
