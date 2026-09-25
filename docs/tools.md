# MCP tools

These are the tools Crystal Cove gives agents. They work the way an Obsidian vault works, unlike a generic filesystem MCP that makes an agent list directories and read raw files.

Every tool takes a `vault` name. It is optional when the server holds a single vault, which is the usual case, and required otherwise.

All paths are vault-relative and sandboxed: absolute paths and `..` escapes are rejected.

| Tool | Description |
| --- | --- |
| `list_vaults` | Names of the vaults served. Offered only when the server holds more than one vault. |
| `list_notes` | List files and folders in a vault, optionally recursive. Hidden folders (`.obsidian`, `.trash`) are excluded; pass `dir: ".trash"` to browse deleted notes. Other hidden folders cannot be listed. |
| `read_note` | Read a note, paged at 10,240 characters per call with `offset`/`next_offset` for longer notes. |
| `get_section` | Read a heading section's body (including subsections), with character paging. |
| `replace_section` | Replace a heading section's body and subsections while preserving its heading. |
| `search_notes` | Searches note names and content, ranked. A plain multi-word query finds notes holding every word in any order; a query with regex characters stays a regex (`mode` forces either). Name matches rank first, flagged `title_match`, then match count. Only Markdown notes are searched. Supports glob filters, context lines and case sensitivity. `max_results` counts notes (default 50), and each note returns 5 matching lines unless `max_lines_per_note` says otherwise (`-1` for all); `total_matches` per note counts the rest. |
| `recent_notes` | Notes changed most recently, newest first, with modified times. Takes `since` (RFC 3339) and `limit`. |
| `list_tags` | Every tag in the vault with the number of notes carrying it, most used first. Reads frontmatter `tags` and inline hashtags. |
| `find_notes` | Query by metadata instead of text: notes carrying all the given tags, and/or a frontmatter field, with the value optional so a key alone finds every note that has it. |
| `get_links` | The wikilinks a note points at, each resolved to the note it names, or flagged unresolved when that note does not exist yet. Headings, aliases and embeds are kept. |
| `get_backlinks` | The notes that link to a note, with the links they use. |
| `get_frontmatter` | One note's frontmatter fields and tags, without its body. |
| `update_frontmatter` | Add, replace or delete frontmatter fields. Untouched fields keep their value and order, and the body is unchanged. |
| `create_note` | Create a new note; fails if it already exists. |
| `append_note` | Append to a note, creating it if needed. The content starts on a new line. |
| `edit_note` | Exact find/replace; the snippet must be unique unless `replace_all` is set. |
| `move_note` | Move or rename a note. |
| `delete_note` | Move a note to the vault's `.trash` (Obsidian's own convention, recoverable everywhere). With `MCP_ALLOW_PERMANENT_DELETE=true`, `permanent: true` removes it outright. |
| `restore_note` | Undelete: move a note out of `.trash`, back to its original name or an explicit destination. |

With `MCP_READ_ONLY=true`, the server leaves out every tool that changes a note.

## Working with heading sections

`get_section` and `replace_section` select sections using `heading_path`, an
array of exact, case-sensitive heading titles. A unique suffix is enough:
`["Tasks"]` selects a unique Tasks heading, while `["Project", "Tasks"]`
disambiguates Tasks under Project. If even the full hierarchy is duplicated,
the tools reject it; use `edit_note` with unique surrounding text instead.
Use the heading's Markdown title without its opening/closing heading markers
(e.g. `"**Tasks**"` for `## **Tasks**`).

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
  "next_offset": -1
}
```

Reads return at most 10,240 Unicode characters. Pass `offset: next_offset` to
continue a truncated read; offsets are relative to the body. The returned
`heading_path` is always the full hierarchy.

Replace that body with:

```json
{
  "name": "replace_section",
  "arguments": {
    "vault": "Work",
    "path": "projects/plan.md",
    "heading_path": ["Project", "Tasks"],
    "content": "- [x] Ship the feature\n\n"
  }
}
```

Success returns `{"ok": true}`. Supply only the replacement body, **not**
`## Tasks`. The existing heading is preserved; all its old subsections are
replaced too. Empty content clears the body. Missing or ambiguous headings
fail without writing. The server adds newline separation when needed to keep
a following heading separate (a blank line for setext headings), using the
selected heading's newline style. Other bytes outside the body are preserved;
a heading at EOF without a newline gains one when adding nonempty content.

As with `edit_note`, replacement uses the current local file and has no version
precondition or transaction with concurrent sync writes. Success confirms the
local write, not completion of Obsidian Sync.
