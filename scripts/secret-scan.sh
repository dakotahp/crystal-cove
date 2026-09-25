#!/bin/sh
# Scan every commit for committed secrets with gitleaks. Findings are
# printed with their values redacted. Run it from a normal clone: in a git
# worktree, .git points outside the mounted folder.
set -eu

# gitleaks reports success when git fails and nothing is scanned, so the
# folder is marked safe for the container's user and a scan of zero commits
# counts as a failure.
output=$(docker run --rm -v "$PWD:/repo" -w /repo \
  -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' \
  ghcr.io/gitleaks/gitleaks:v8.30.1 git . --redact --no-banner --verbose 2>&1) && status=0 || status=$?
echo "$output"
if echo "$output" | grep -Eq '(^|[^0-9])0 commits scanned'; then
  echo "no commits were scanned"
  exit 1
fi
exit "$status"
