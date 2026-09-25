#!/bin/sh
# Protect master: changes arrive through pull requests that pass the CI
# gates, and history cannot be force-pushed or the branch deleted. Checks
# are pinned to GitHub Actions (app 15368), so no other app can report a
# passing "test". Admins are not bound: they can merge past failing checks
# and push to master directly. No approval is required, which suits a solo
# maintainer; raise required_approving_review_count once others contribute.
# vulncheck is left out: a Go vulnerability with no fix yet would block
# every merge. Needs a public repository or GitHub Pro.
set -eu

repo="${1:-dakotahp/vault-bridge}"
gh api --method PUT "repos/$repo/branches/master/protection" --input - <<'EOF'
{
  "required_status_checks": {
    "strict": false,
    "checks": [
      {"context": "test", "app_id": 15368},
      {"context": "fuzz", "app_id": 15368},
      {"context": "gosec", "app_id": 15368},
      {"context": "image", "app_id": 15368},
      {"context": "zizmor", "app_id": 15368},
      {"context": "secrets", "app_id": 15368}
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {"required_approving_review_count": 0},
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF
