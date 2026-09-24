#!/bin/sh
# Protect master: changes arrive through pull requests that pass the CI
# gates, and history cannot be force-pushed or the branch deleted. Admins
# can still bypass, but only by ticking GitHub's explicit bypass box.
# vulncheck is left out: a Go vulnerability with no fix yet would block
# every merge. Needs a public repository or GitHub Pro.
set -eu

repo="${1:-dakotahp/vault-bridge}"
gh api --method PUT "repos/$repo/branches/master/protection" --input - <<'EOF'
{
  "required_status_checks": {
    "strict": false,
    "checks": [
      {"context": "test"},
      {"context": "fuzz"},
      {"context": "gosec"},
      {"context": "image"},
      {"context": "zizmor"},
      {"context": "secrets"}
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {"required_approving_review_count": 0},
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF
