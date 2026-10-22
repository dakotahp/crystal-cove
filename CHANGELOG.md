# Changelog

## [0.20.0](https://github.com/dakotahp/crystal-cove/compare/v0.19.0...v0.20.0) (2026-10-22)


### Features

* **server:** add daily note tools matching the Obsidian CLI ([c14241e](https://github.com/dakotahp/crystal-cove/commit/c14241e40b510870980251fb308c89bb043f3b54))
* **server:** add daily note tools matching the Obsidian CLI ([c17a85b](https://github.com/dakotahp/crystal-cove/commit/c17a85b627ec6953310c45c12669f8052b72ee53))

## [0.19.0](https://github.com/dakotahp/crystal-cove/compare/v0.18.0...v0.19.0) (2026-10-20)


### Features

* **server:** add move_folder to move a whole folder with its links ([c5ded84](https://github.com/dakotahp/crystal-cove/commit/c5ded84276a5198982cf2d58374b371b0a50a6ff))

## [0.18.0](https://github.com/dakotahp/crystal-cove/compare/v0.17.0...v0.18.0) (2026-10-19)


### Features

* add add_tags and remove_tags to update_frontmatter ([49421b4](https://github.com/dakotahp/crystal-cove/commit/49421b4db71773bcd2caf4f85b42ae8a7bd04fd9))
* add folder, missing-field, tag-count and field filters to find_notes ([cf9cb31](https://github.com/dakotahp/crystal-cove/commit/cf9cb31df892f91bfa060d3e4babc89a7bad239e))
* add max_chars to read_notes for skimming many notes ([c768c02](https://github.com/dakotahp/crystal-cove/commit/c768c0272854519357f22d5dd7243e3734a619d4))
* add unresolved_links and orphans, and resolve links as Obsidian does ([f21ed7b](https://github.com/dakotahp/crystal-cove/commit/f21ed7b69e37f3a228becb298788faac3e746985))
* vault-wide link checks, tag merging, find_notes filters, read_notes previews ([7e792f7](https://github.com/dakotahp/crystal-cove/commit/7e792f7d0059b71c53c746d621b4a48eaecb784d))


### Bug Fixes

* update Markdown links and relative links when move_note updates links ([0b21e05](https://github.com/dakotahp/crystal-cove/commit/0b21e05c2569deeb8455f5f6a1e668bcefe692c3))

## [0.17.0](https://github.com/dakotahp/crystal-cove/compare/v0.16.0...v0.17.0) (2026-10-19)


### ⚠ BREAKING CHANGES

* search_notes reports total and next_offset instead of truncated.
* search_notes and find_notes take limit instead of max_results, and find_notes reports next_offset and total instead of truncated.

### Features

* page list_tags with offset and limit like list_notes ([c7e93dc](https://github.com/dakotahp/crystal-cove/commit/c7e93dcf67bf6d86b8f66189d1f7b0ef9ef70106))
* page search_notes with offset and next_offset ([c7b6b4d](https://github.com/dakotahp/crystal-cove/commit/c7b6b4d2c518ef0f727ff8ea5a3e2054809e23ff))
* use limit, offset and next_offset across listing tools ([9c3049f](https://github.com/dakotahp/crystal-cove/commit/9c3049fc5f50e1b7850dc4e4e5692eb5ae64a01b))

## [0.16.0](https://github.com/dakotahp/crystal-cove/compare/v0.15.0...v0.16.0) (2026-10-17)


### Features

* return the new note's version from create_note ([15efab9](https://github.com/dakotahp/crystal-cove/commit/15efab912c33eea7463763a6565b1a6630ae9dbe))

## [0.15.0](https://github.com/dakotahp/crystal-cove/compare/v0.14.0...v0.15.0) (2026-10-15)


### Features

* hold a write while another device is still editing the note ([3300cbb](https://github.com/dakotahp/crystal-cove/commit/3300cbb317f5c42e3bd0e88198278d16d51865af))


### Bug Fixes

* refuse note paths that differ from an existing one only in letter case ([ee26305](https://github.com/dakotahp/crystal-cove/commit/ee2630548fcfa9867628a11200f77692c84736dd))
* serialize writes within a vault so parallel writes cannot drop each other ([2be0597](https://github.com/dakotahp/crystal-cove/commit/2be059719096d10971826c23f87efac0a3c712d5))

## [0.14.0](https://github.com/dakotahp/crystal-cove/compare/v0.13.0...v0.14.0) (2026-10-14)


### Features

* show the running version and commit on /health and at startup ([710c41f](https://github.com/dakotahp/crystal-cove/commit/710c41f1d19fe9206eac6dfd5c0126231ef695e7))


### Bug Fixes

* accept mode auto and list parameter choices as enums ([bd35dc4](https://github.com/dakotahp/crystal-cove/commit/bd35dc4be5b57cbcee290b4306e438a2a914fef1))
* leave read fields out of a read_notes entry that failed ([f2f500e](https://github.com/dakotahp/crystal-cove/commit/f2f500e275388d4cc7f47c8273de3d84f4b4ec79))

## [0.13.0](https://github.com/dakotahp/crystal-cove/compare/v0.12.3...v0.13.0) (2026-10-14)


### ⚠ BREAKING CHANGES

* edit_note no longer takes find, replace and replace_all at the top level; pass them as one entry in edits.

### Features

* add exact-phrase mode to search_notes ([987c7bc](https://github.com/dakotahp/crystal-cove/commit/987c7bcc8f5fa11a9be2a2cf3e8d6d5064b494f0))
* add files output to search_notes and clip long lines ([4215bcc](https://github.com/dakotahp/crystal-cove/commit/4215bcc07d56c2b4679820c2358c9125608cc694))
* add order to list_notes for newest-first listings ([b8efb8b](https://github.com/dakotahp/crystal-cove/commit/b8efb8b61605550bce3bd1abdaa9974d65b05546))
* add read_notes to read several notes in one call ([4815c09](https://github.com/dakotahp/crystal-cove/commit/4815c09bea5e721bc90f8278bbe2d9ec5abe883a))
* list a note's headings when a section is not found ([df6dc50](https://github.com/dakotahp/crystal-cove/commit/df6dc502fd20604c21168d1af4d9dcd8e50dbb8e))
* take a list of edits in edit_note ([53450da](https://github.com/dakotahp/crystal-cove/commit/53450dab38ccbfc606fa199aab57e7e80a24acb6))

## [0.12.3](https://github.com/dakotahp/crystal-cove/compare/v0.12.2...v0.12.3) (2026-10-10)


### Bug Fixes

* say a hidden file is hidden, not inside a hidden folder ([b9c5d4e](https://github.com/dakotahp/crystal-cove/commit/b9c5d4ef00aeadbe4133b75ea40c0fd717f454e8))
* upgrade runtime base packages to patch zlib CVE-2026-85091 ([f6fbce1](https://github.com/dakotahp/crystal-cove/commit/f6fbce1c428a72b8562e29c265733f5dbd7ca0ea))

## [0.12.2](https://github.com/dakotahp/crystal-cove/compare/v0.12.1...v0.12.2) (2026-10-07)


### Bug Fixes

* apply search_notes glob to title matches ([b326337](https://github.com/dakotahp/crystal-cove/commit/b326337ce3ea0725b771d18dfc42a5f228b7a09a))
* say frontmatter tools list fields alphabetically ([#43](https://github.com/dakotahp/crystal-cove/issues/43)) ([ba431d2](https://github.com/dakotahp/crystal-cove/commit/ba431d23caeb61cbed0f56d4a2bbf5a4e2cf5e23))

## [0.12.1](https://github.com/dakotahp/crystal-cove/compare/v0.12.0...v0.12.1) (2026-10-06)


### Bug Fixes

* make tool errors and results easier for agents to act on ([3e8a100](https://github.com/dakotahp/crystal-cove/commit/3e8a1005ddef2b03ff85a0c5278e0bef790142ef))

## [0.12.0](https://github.com/dakotahp/crystal-cove/compare/v0.11.0...v0.12.0) (2026-10-03)


### Features

* serve the logo as a favicon ([829dafa](https://github.com/dakotahp/crystal-cove/commit/829dafa77a0f0a750f27fcccc191a6411d1cf799))
* serve the logo as a favicon ([#36](https://github.com/dakotahp/crystal-cove/issues/36)) ([b9b0877](https://github.com/dakotahp/crystal-cove/commit/b9b08775cf61d2c2337a72eeb1848ab1cf42153c))

## [0.11.0](https://github.com/dakotahp/crystal-cove/compare/v0.10.0...v0.11.0) (2026-10-03)


### Features

* style the built-in sign-in page ([0f83a55](https://github.com/dakotahp/crystal-cove/commit/0f83a55c9727c5bb7c31610c73a28ebede17161b))


### Bug Fixes

* limit client registration to 10 per minute ([b24ff4f](https://github.com/dakotahp/crystal-cove/commit/b24ff4f4508c6e27baa8591a0fc0314972848e5d))
* show sign-in errors as a page instead of redirecting ([910d4c4](https://github.com/dakotahp/crystal-cove/commit/910d4c4e4e5c2cd611b3a0b83d7646da1dddb8fa))

## [0.10.0](https://github.com/dakotahp/crystal-cove/compare/v0.9.0...v0.10.0) (2026-10-02)


### Features

* install from the published image without cloning ([d0a9f8e](https://github.com/dakotahp/crystal-cove/commit/d0a9f8e3e83728b246683fa56244d1f423bd3da8))


### Bug Fixes

* log a repeated "Fully synced" line once ([0b2094c](https://github.com/dakotahp/crystal-cove/commit/0b2094c30cbae317a77654d508460e1ca1f234df))
* name OBSIDIAN_VAULT_PASSWORD when an encrypted vault has no password ([022a811](https://github.com/dakotahp/crystal-cove/commit/022a811dcecee3c708915694b45179f896519689))

## [0.9.0](https://github.com/dakotahp/crystal-cove/compare/v0.8.0...v0.9.0) (2026-09-25)


### ⚠ BREAKING CHANGES

* **server:** the replace_section tool is removed. Call edit_section with mode replace and the version from get_section instead.
* **server:** the restore_note tool is removed. Call move_note with the note's path inside .trash instead; new_path replaces restore_note's to.
* probes on /livez, /readyz, or /healthz now get a 401 and mark the container unhealthy. Point liveness probes at /health and readiness probes at /ready. /healthz used to mean ready, so a probe that only drops the "z" now checks the process instead of sync.

### Features

* advertise the Crystal Cove emblem as the MCP server icon ([f753542](https://github.com/dakotahp/crystal-cove/commit/f753542814c2ad660dfadbc0d8f12ead4c67e703))
* advertise the Crystal Cove emblem as the MCP server icon ([#21](https://github.com/dakotahp/crystal-cove/issues/21)) ([1b5afcf](https://github.com/dakotahp/crystal-cove/commit/1b5afcfd71c75c9e5dd3ff3ec9ea8911380e4140))
* refuse an edit built on an outdated read of the note ([5d41501](https://github.com/dakotahp/crystal-cove/commit/5d41501000b36d7fa7ce8a552e8c3be33dfc4b80))
* rename the health endpoints to /health and /ready ([7757a45](https://github.com/dakotahp/crystal-cove/commit/7757a4577b9a6bab3dbfeb832ab269eb30d33797))
* **server:** fold restore_note into move_note ([3f4b90e](https://github.com/dakotahp/crystal-cove/commit/3f4b90ec267392867101cacac829a2a5100972e0))
* **server:** offer list_vaults only when the server holds several vaults ([24fa7c8](https://github.com/dakotahp/crystal-cove/commit/24fa7c8c57278206a8c0bae0d0479098e3e543c7))
* **server:** page list_notes at 200 entries ([17a84b1](https://github.com/dakotahp/crystal-cove/commit/17a84b1d340a06479417d00e6909b21c52af08f3))
* **server:** replace replace_section with edit_section and its append, prepend and replace modes ([b6333d5](https://github.com/dakotahp/crystal-cove/commit/b6333d54ce8516ab5a72d99f6fe3b190878c0db8))
* **server:** update links to a moved note with move_note update_links ([0e1cad1](https://github.com/dakotahp/crystal-cove/commit/0e1cad1230d38d1cf358a02b37b2c4ac6a52a8f1))
* **vault:** keep a deleted note's folder in the trash so a restore puts it back ([fd5e14a](https://github.com/dakotahp/crystal-cove/commit/fd5e14a9cb746bc03a16156a05c08bc8c4a27220))


### Bug Fixes

* **search:** find every note holding all query words, and rank before limiting ([2bccca9](https://github.com/dakotahp/crystal-cove/commit/2bccca9e59ca99a36e4118070c492516f51a5f3e))
* **search:** find every note holding all query words, and rank before limiting ([#24](https://github.com/dakotahp/crystal-cove/issues/24)) ([125581e](https://github.com/dakotahp/crystal-cove/commit/125581e3d7be3b5cd129574fa6ade2367731e191))
* **vault:** close the vault root with defer in Append ([2980b64](https://github.com/dakotahp/crystal-cove/commit/2980b6461bffeea93ad71a97a988b2cbdd70f529))
* **vault:** never overwrite a change sync writes during an edit ([2ad3ad5](https://github.com/dakotahp/crystal-cove/commit/2ad3ad5d0f1af7c92a282a7188492193bd277305))
* **vault:** start appended content on a new line ([5b054f4](https://github.com/dakotahp/crystal-cove/commit/5b054f497e603d154b8d6d7b63253ba524286a5b))
* **vault:** write created and appended notes atomically ([b9c1dce](https://github.com/dakotahp/crystal-cove/commit/b9c1dce3b660c4f2554233c6acf21e4b73372951))

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
