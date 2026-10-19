# MCP tools

These are the tools Crystal Cove gives agents. They work the way an Obsidian vault works, unlike a generic filesystem MCP that makes an agent list directories and read raw files.

Every tool takes a `vault` name. It is optional when the server holds a single vault, which is the usual case, and required otherwise.

All paths are vault-relative and sandboxed: absolute paths and `..` escapes are rejected.

A call on a note that does not exist fails with `not_found: no note at "<path>" in vault "<name>"`. A call with an unknown or missing argument fails with an error that lists the tool's parameters and which of them are required.

| Tool | Description |
| --- | --- |
| `list_vaults` | Names of the vaults served. Offered only when the server holds more than one vault. |
| `list_notes` | List files and folders in a vault, optionally recursive. Hidden folders (`.obsidian`, `.trash`) are excluded; pass `dir: ".trash"` with `recursive: true` to browse deleted notes. Other hidden folders cannot be listed. Pages at 200 entries (`limit` up to 1,000) with `offset`/`next_offset`, and reports the listing's `total`. Entries come in path order; `order: "desc"` reverses it, so `limit: 3` on a folder of dated notes lists the three newest. |
| `read_note` | Read a note, paged at 10,240 characters per call with `offset`/`next_offset` for longer notes. The page size is fixed; there is no `limit`. Returns the note's `version` (see [Edits and sync](#edits-and-sync)). |
| `read_notes` | Read up to 20 notes in one call, such as every note `find_notes` returned. Each note returns its first page with its `version`, in the order asked; the call returns at most 51,200 characters in all. A note cut short by either limit reports `truncated` and `next_offset` (`0` when the budget ran out before it, with `budget_exhausted` set); continue it with `read_note`. A note that cannot be read reports only `path` and `error`, so it cannot pass for an empty note, and the others still come back. |
| `get_section` | Read a heading section's body (including subsections), with character paging. Returns the section's `version`. |
| `edit_section` | Change one heading section: `append` lines after its own text, `prepend` them below the heading, or `replace` the whole body (which needs the section's `version`). |
| `search_notes` | Searches note names and content, ranked. A plain multi-word query finds notes holding every word in any order; a query wrapped in double quotes, or `mode: "phrase"`, finds that exact text with no escaping; a query with regex characters stays a regex (`mode` forces any reading). Name matches rank first, flagged `title_match`, then match count. Only Markdown notes are searched. Supports glob filters, context lines and case sensitivity. `limit` counts notes (default 50, up to 500), paged with `offset`/`next_offset` over the whole ranked list, with the `total` of matching notes, and each note returns 5 matching lines unless `max_lines_per_note` says otherwise (`-1` for all; `0` means the default); `total_matches` per note counts the rest. A line longer than 500 characters is cut to a window around the match and flagged `text_truncated`. `output: "files"` leaves the lines out and keeps each note's `path`, `total_matches` and `title_match`: the cheap way to choose which note to read. |
| `recent_notes` | Notes changed most recently, newest first, with modified times. Takes `since` (RFC 3339). Pages at 20 notes (`limit` up to 1,000) with `offset`/`next_offset`, and reports the `total`. |
| `list_tags` | Every tag in the vault with the number of notes carrying it, most used first. Reads frontmatter `tags` and inline hashtags. Pages like `list_notes`: 200 tags (`limit` up to 1,000) with `offset`/`next_offset`, and reports the vault's `total` distinct tags, so `limit: 20` gives the main topics. |
| `find_notes` | Query by metadata instead of text: notes carrying all the given `tags`, and/or a field named by `frontmatter_key`, with `frontmatter_value` optional so a key alone finds every note that has it. There is no free-text `query`; that is `search_notes`. Pages at 50 notes (`limit` up to 1,000) with `offset`/`next_offset`, and reports the `total` matches. |
| `get_links` | The wikilinks a note points at, each resolved to the note it names, or flagged unresolved when that note does not exist yet. Headings, aliases and embeds are kept. |
| `get_backlinks` | The notes that link to a note, with the links they use. |
| `get_frontmatter` | One note's frontmatter fields and tags, without its body. Dates come back as written (`2026-06-06`), so writing one back leaves the file unchanged. |
| `update_frontmatter` | Add, replace or delete frontmatter fields. Untouched fields keep their value and order, and the body is unchanged. |
| `create_note` | Create a new note; fails if it already exists. |
| `append_note` | Append to a note, creating it if needed. The content starts on a new line. |
| `edit_note` | Exact find/replace. `edits` is a list of `{find, replace, replace_all}` applied in order, each to the text the edits before it left, so several changes to one note take one call. Each snippet must be unique unless `replace_all` is set. If any edit fails, nothing is written and the error names the edit (`edit 2 of 3: text not found`). Takes an optional `version` and returns the new one. |
| `move_note` | Move or rename a note. With `update_links: true`, rewrites the `[[links]]` to it in every note (headings, aliases and embeds kept, code left alone) and lists the notes it changed in `links_updated_in`. A bare `[[Name]]` link stays bare unless another note shares the new name; a `[[folder/Name]]` link gets the new path. `links_updated` says whether links were rewritten at all. Also restores a deleted note: pass its path inside `.trash`, and leave `new_path` out to put it back where it was deleted from. |
| `delete_note` | Move a note to the vault's `.trash`, keeping its folder (`Projects/Plan.md` goes to `.trash/Projects/Plan.md`) so `move_note` can put it back where it was. A note trashed by Obsidian itself, which keeps only the file name, goes back to the vault root. With `MCP_ALLOW_PERMANENT_DELETE=true`, `permanent: true` removes it outright. |

With `MCP_READ_ONLY=true`, the server leaves out every tool that changes a note.

## Edits and sync

Obsidian Sync can write a note at any moment, so no edit overwrites a change
it did not see:

- Every edit reads the note, applies the change, and just before it writes
  checks that the note still holds what it read. If sync wrote meanwhile, the
  edit is applied again to the new text, up to three times.
- An agent's edit can also rest on a read from minutes ago. `read_note`
  returns a `version`, a short hash of the text. Pass it to `edit_note` and
  the edit is refused when the note has changed since, so the agent reads it
  again instead of undoing someone's change. `edit_note` returns the new
  `version` for a following edit. `get_section` and `edit_section` work the
  same way for one section, and a `replace` requires the version.
- Obsidian, on a device where a note is open and being typed in, saves its
  own copy over changes that reach it meanwhile. So a write waits until the
  note has gone 10 seconds without a change from another device, and is
  refused with an error if it is still changing after 30 seconds.
  `MCP_WRITE_QUIET_SECONDS` and `MCP_WRITE_WAIT_SECONDS` set the two times.
  The server learns of an edit only once sync delivers it, so a write in the
  first moments of someone's typing can still be lost on that device.
- A new note's path cannot differ from an existing note or folder only in
  letter case, such as `plan.md` beside `Plan.md`. The server keeps the two
  apart, but macOS, Windows, iOS and Android treat them as one file, so sync
  would merge them and lose one note's text. Renaming a note to change only
  its own case is allowed.

## Working with heading sections

`get_section` and `edit_section` select sections using `heading_path`, an
array of exact, case-sensitive heading titles. A unique suffix is enough:
`["Tasks"]` selects a unique Tasks heading, while `["Project", "Tasks"]`
disambiguates Tasks under Project. If even the full hierarchy is duplicated,
the tools reject it; use `edit_note` with unique surrounding text instead.
Use the heading's Markdown title without its opening/closing heading markers
(e.g. `"**Tasks**"` for `## **Tasks**`).

A heading that is not found fails with the note's heading paths in
`heading_path` form, closest first, so an agent that misremembered a heading
can retry without reading the whole note. Closeness ignores case, spacing,
and dash and quote differences, but only to order the list: the tools never
pick a heading the agent did not name exactly. At most 50 are listed.

A section's body starts immediately after its heading and ends before the next
heading of equal or higher rank, or at EOF. It includes nested subsections and
blank lines. Both `#`-style and underlined (setext) headings are supported.
Headings inside code blocks, blockquotes, lists, HTML blocks, and initial YAML
frontmatter are excluded. Text before the first heading has no section; use
`read_note` / `edit_note` for that text. An unclosed initial YAML frontmatter
block is treated as extending to EOF.

For a note containing `# Project`, `## Tasks`, and `## Done`, read Tasks with:

```json
{
  "name": "get_section",
  "arguments": {
    "vault": "Work",
    "path": "projects/plan.md",
    "heading_path": ["Project", "Tasks"]
  }
}
```

The structured result contains:

```json
{
  "heading_path": ["Project", "Tasks"],
  "level": 2,
  "content": "- [ ] Ship the feature\n\n",
  "offset": 0,
  "total_characters": 24,
  "truncated": false,
  "next_offset": -1,
  "version": "3f2a9c1b7d4e6a08"
}
```

Reads return at most 10,240 Unicode characters. Pass `offset: next_offset` to
continue a truncated read; offsets are relative to the body. The returned
`heading_path` is always the full hierarchy. `version` identifies this
section's text only, so an edit elsewhere in the note leaves it valid.

`edit_section` changes the section in one of three modes:

- `append` adds lines after the section's own text, before its first
  subheading. This is how to add an entry under a heading such as `## Log`.
  Blank lines that end the section's text stay after the new lines.
- `prepend` adds lines right below the heading, after any blank lines there.
- `replace` replaces the whole body, subsections included. It needs the
  `version` from `get_section`, so it cannot remove text the agent has not
  seen. Empty content clears the body.

Add a task to that section with:

```json
{
  "name": "edit_section",
  "arguments": {
    "vault": "Work",
    "path": "projects/plan.md",
    "heading_path": ["Project", "Tasks"],
    "mode": "append",
    "content": "- [ ] Write the release notes"
  }
}
```

Success returns `{"ok": true, "version": "..."}`, the section's new version for
a following edit. Supply only lines of the body, **not** `## Tasks`; the heading
and every byte outside the section are kept. Missing or ambiguous headings fail
without writing. Any mode given a `version` refuses the edit when the section
has changed since. The server adds line breaks where needed so the new text and
a following heading stay on their own lines (a blank line before a setext
heading), in the note's own line-break style. Success confirms the local write,
not completion of Obsidian Sync.
