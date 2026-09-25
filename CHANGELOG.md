# Changelog

## [0.8.0](https://github.com/dakotahp/crystal-cove/compare/v0.7.0...v0.8.0) (2026-09-25)


### ⚠ BREAKING CHANGES

* an http OAUTH_ISSUER on a non-loopback host now stops the server at startup.
* a static bearer token shorter than 32 characters now stops the server at startup. Generate one with openssl rand -hex 32.
* delete_note refuses permanent: true, and deletes inside .trash, unless MCP_ALLOW_PERMANENT_DELETE=true is set.
* an http OAUTH_ISSUER on a non-loopback host now stops the server at startup.
* a static bearer token shorter than 32 characters now stops the server at startup. Generate one with openssl rand -hex 32.

### Features

* add a read-only mode and make permanent delete opt-in ([440a3c4](https://github.com/dakotahp/crystal-cove/commit/440a3c4de8e67ceac4b22419bcb6588d1318e207))
* log every tool call to an audit trail ([2f1232e](https://github.com/dakotahp/crystal-cove/commit/2f1232e4e29c22b680b0d9ca3eff41cd856825e4))
* log refused bearer tokens to the audit trail ([e036931](https://github.com/dakotahp/crystal-cove/commit/e0369319059b8c8f56790baeaad77a4cd61934c7))
* rename the project to Crystal Cove ([e350baf](https://github.com/dakotahp/crystal-cove/commit/e350bafdd753863cf73e866ed64c9704d5c45409))
* warn at startup when OIDC has no required roles ([f52bfe9](https://github.com/dakotahp/crystal-cove/commit/f52bfe997ef10570f41a5382f9885382f7d3ab8b))


### Bug Fixes

* build the sync client against the runtime's Node ([5c54c1f](https://github.com/dakotahp/crystal-cove/commit/5c54c1fabd86dd52647cce864ec9fdd2c5180130))
* build the sync client against the runtime's Node ([5dddb40](https://github.com/dakotahp/crystal-cove/commit/5dddb40df5a27f3e04509926dbca951499ed8805))
* keep server secrets out of child process environments ([bb8eb2f](https://github.com/dakotahp/crystal-cove/commit/bb8eb2f77502a731023c9935289a4a4aa3dbb30f))
* keep server secrets out of child process environments ([be8afa5](https://github.com/dakotahp/crystal-cove/commit/be8afa5a3ff5d8fdffd3b172f8ff010fa9bab213))
* pin master's required checks to GitHub Actions ([713ab54](https://github.com/dakotahp/crystal-cove/commit/713ab542c4a5bbbd5a932ec47938e518ed29257d))
* pin the Obsidian sync client with a lockfile ([882c26b](https://github.com/dakotahp/crystal-cove/commit/882c26bce4768384fbf6a05b3aeceb40d206427b))
* pin the Obsidian sync client with a lockfile ([abbdc05](https://github.com/dakotahp/crystal-cove/commit/abbdc055ed5d0f453ff9ffdfc2d3843f2b9b9e0e))
* publish the compose port on loopback and drop container privileges ([0ec6533](https://github.com/dakotahp/crystal-cove/commit/0ec65331dae1c5bb4e4a2a9d160030a0c91ce3d1))
* publish the compose port on loopback and drop container privileges ([1d87404](https://github.com/dakotahp/crystal-cove/commit/1d87404ff76d74e349196a175cbf23587dabd2a3))
* refuse MCP_AUTH_TOKEN values shorter than 32 characters ([1b4958a](https://github.com/dakotahp/crystal-cove/commit/1b4958a1debd33094b852d9966ccbe33621eaa84))
* refuse MCP_AUTH_TOKEN values shorter than 32 characters ([cc562fb](https://github.com/dakotahp/crystal-cove/commit/cc562fb87d4c57cd867f0ff5aecd57ae7460c420))
* require an https OAUTH_ISSUER off loopback ([b75e013](https://github.com/dakotahp/crystal-cove/commit/b75e013886bd7b7b77f81b5d62985f0a534c898a))
* require an https OAUTH_ISSUER off loopback ([e27d101](https://github.com/dakotahp/crystal-cove/commit/e27d101a082bdc6a378d031dad49b6eafe7d93e5))
* run the container with a read-only root filesystem ([144822d](https://github.com/dakotahp/crystal-cove/commit/144822d58457c8854c46b8241f48de95c81842f5))
* stop list_notes from listing hidden folders ([d319283](https://github.com/dakotahp/crystal-cove/commit/d319283ce18782d96bb8aef93cd87ee53934b752))
* stop list_notes from listing hidden folders ([52bbcad](https://github.com/dakotahp/crystal-cove/commit/52bbcad8efc2e8e4b825fbf0b47cf21de573b59e))
* stop symlinks from leading tools outside the vault ([d80bcfc](https://github.com/dakotahp/crystal-cove/commit/d80bcfcfeccb73b38c4443868749505806073b6d))
* stop symlinks from leading tools outside the vault ([3f05987](https://github.com/dakotahp/crystal-cove/commit/3f059873a75ffd05f0174782b745e4f74d51eaea))
* stop tools from changing the synced instructions note ([a61ab3b](https://github.com/dakotahp/crystal-cove/commit/a61ab3b6a64abaf686d1e9196ac0ffb452110077))
* stop tools from changing the synced instructions note ([2d35329](https://github.com/dakotahp/crystal-cove/commit/2d3532900a79f291d3347bf3599759fb32f396b4))
