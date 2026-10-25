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
| `grep_notes` | Exact search of note names and content, ranked: the tool for exact words, a regex, every matching line, or a line to edit. With semantic search on, `search_notes` is the better first search. A plain multi-word query finds notes holding every word in any order; a query wrapped in double quotes, or `mode: "phrase"`, finds that exact text with no escaping; a query with regex characters stays a regex (`mode` forces any reading). Name matches rank first, flagged `title_match`, then match count. Only Markdown notes are searched. Supports glob filters, context lines and case sensitivity. `limit` counts notes (default 50, up to 500), paged with `offset`/`next_offset` over the whole ranked list, with the `total` of matching notes, and each note returns 5 matching lines unless `max_lines_per_note` says otherwise (`-1` for all; `0` means the default); `total_matches` per note counts the rest. A line longer than 500 characters is cut to a window around the match and flagged `text_truncated`. `output: "files"` leaves the lines out and keeps each note's `path`, `total_matches` and `title_match`: the cheap way to choose which note to read. |
| `recent_notes` | Notes changed most recently, newest first, with modified times. Takes `since` (RFC 3339). Pages at 20 notes (`limit` up to 1,000) with `offset`/`next_offset`, and reports the `total`. |
| `list_tags` | Every tag in the vault with the number of notes carrying it, most used first. Reads frontmatter `tags` and inline hashtags. Pages like `list_notes`: 200 tags (`limit` up to 1,000) with `offset`/`next_offset`, and reports the vault's `total` distinct tags, so `limit: 20` gives the main topics. |
| `find_notes` | Query by metadata instead of text: notes carrying all the given `tags`, and/or a field named by `frontmatter_key`, with `frontmatter_value` optional so a key alone finds every note that has it. There is no free-text `query`; that is `grep_notes` (or `search_notes` with semantic search on). Pages at 50 notes (`limit` up to 1,000) with `offset`/`next_offset`, and reports the `total` matches. |
| `get_links` | The wikilinks a note points at, each resolved to the note it names, or flagged unresolved when that note does not exist yet. Headings, aliases and embeds are kept. |
| `get_backlinks` | The notes that link to a note, with the links they use. |
| `get_frontmatter` | One note's frontmatter fields and tags, without its body. Dates come back as written (`2026-06-06`), so writing one back leaves the file unchanged. |
| `update_frontmatter` | Add, replace or delete frontmatter fields. Untouched fields keep their value and order, and the body is unchanged. |
| `create_note` | Create a new note; fails if it already exists. |
| `append_note` | Append to a note, creating it if needed. The content starts on a new line. |
| `daily_note_path` | The path of a day's daily note, and whether it `exists` yet. Never creates it. See [Daily notes](#daily-notes). |
| `read_daily_note` | Read a day's daily note, paged like `read_note`. A note not created yet returns `exists: false` and its path, and stays uncreated. |
| `append_daily_note` | Add `content` to the end of a day's daily note, on a new line unless `inline` is set. Creates the note from the daily note template when it is missing, and reports `created`. |
| `prepend_daily_note` | Add `content` at the start of a day's daily note, below its frontmatter, followed by a line break unless `inline` is set. Creates a missing note as `append_daily_note` does. |
| `edit_note` | Exact find/replace. `edits` is a list of `{find, replace, replace_all}` applied in order, each to the text the edits before it left, so several changes to one note take one call. Each snippet must be unique unless `replace_all` is set. If any edit fails, nothing is written and the error names the edit (`edit 2 of 3: text not found`). Takes an optional `version` and returns the new one. |
| `move_note` | Move or rename a note. With `update_links: true`, rewrites the `[[links]]` to it in every note (headings, aliases and embeds kept, code left alone) and lists the notes it changed in `links_updated_in`. A bare `[[Name]]` link stays bare unless another note shares the new name; a `[[folder/Name]]` link gets the new path. `links_updated` says whether links were rewritten at all. Also restores a deleted note: pass its path inside `.trash`, and leave `new_path` out to put it back where it was deleted from. To move every note in a folder, use `move_folder`: one `move_note` per note leaves the emptied folders behind. |
| `move_folder` | Move or rename a folder with everything in it: notes, attachments and other files of any type, and its subfolders, keeping their layout. Fails if `new_path` exists, if `new_path` is inside `path`, or if either is the vault root or a hidden folder such as `.trash`. With `update_links: true`, rewrites links to every moved file with the same rules as `move_note`, and keeps relative links inside the moved notes working. Reports `files_moved` and `links_updated_in`. Parent folders it leaves stay, even when empty. |
| `delete_note` | Move a note to the vault's `.trash`, keeping its folder (`Projects/Plan.md` goes to `.trash/Projects/Plan.md`) so `move_note` can put it back where it was. A note trashed by Obsidian itself, which keeps only the file name, goes back to the vault root. With `MCP_ALLOW_PERMANENT_DELETE=true`, `permanent: true` removes it outright. |
| `search_notes` | Beta, with `MCP_SEMANTIC_SEARCH=true`. Hybrid search (keyword + semantic): finds notes by name, key words, quoted phrase, and meaning in one ranked list. Each note reports how it was found as `matched_by`, and the section that shows it as `heading_path` with an `excerpt`. Supports `folders` and `exclude_folders`. Pages at 10 notes with `offset`/`next_offset`. See [Semantic search](#semantic-search-beta). |
| `related_notes` | Beta, with `MCP_SEMANTIC_SEARCH=true`. The notes closest in meaning to one note, compared section by section. `linked` marks notes that already link to or from it, and `unlinked: true` leaves those out. Pages like `search_notes`. |

With `MCP_READ_ONLY=true`, the server leaves out every tool that changes a note.

## Daily notes

The daily note tools match the `daily:path`, `daily:read`, `daily:append` and
`daily:prepend` commands of the Obsidian CLI. They find the note the way
Obsidian's Daily notes core plugin does, from its settings in
`.obsidian/daily-notes.json`: the folder, the date format of the name, and the
template. Obsidian Sync carries that file by default, as core plugin settings.
A vault without it uses Obsidian's defaults: the vault root and `YYYY-MM-DD`.

- Every tool takes an optional `date`: `YYYY-MM-DD`, `today`, `yesterday` or
  `tomorrow`. Without it, the tools use today in the server's time zone,
  `MCP_TIMEZONE` (default UTC).
- A new note starts from the template, with `{{title}}`, `{{date}}` and
  `{{time}}` filled in for that day. A format after a colon works too, such as
  `{{date:dddd}}`. Other template syntax, such as the Templater plugin's,
  stays as written.
- Date formats use moment.js tokens, with English month and weekday names.
  Tokens that depend on a locale, such as `gggg` and `ww`, are refused with an
  error instead of giving a different name than Obsidian would. ISO weeks
  (`GGGG-[W]WW`) work.

These differ from the CLI on purpose: reading never creates the note, and the
CLI has no `date`. Like `append_note`, an append adds a line break only when
the note does not already end with one.

## Semantic search (beta)

`grep_notes` finds the exact words you give it. With semantic search on, `search_notes` finds notes about the same topic too, even when they use other words: "feeling burned out" can find a note about low energy. `related_notes` uses the same idea to find notes connected to one note, including notes it never links to.

`search_notes` is hybrid search: notes are ranked by meaning, and key words rescue the clear hits that meaning misses, such as a book title or a rare name. These move to the top, in this order:

1. **Name.** Notes whose name holds every word of the query.
2. **Phrase.** Notes holding words in double quotes together, as written.
3. **Key words.** Up to three notes with a section holding every key word of the query, chosen by BM25, the standard keyword ranking.

Each group is in meaning order, and the rest of the notes follow in meaning order. A note that holds only some of the key words keeps its meaning place: a question in plain words shares ordinary words with many unrelated notes, and letting those words move notes up pushes the right note down.

Key words are scored per section, and a note counts as its best section, so one matching section in a long note is not lost. Headings and the note's name count as words of each section. A word that few sections hold counts more than a word most hold, so "the" and "of" count for almost nothing in any language. Word forms do not match ("organize" does not find "organizing"); the meaning ranking covers those.

Each result reports `matched_by`: `name`, `phrase`, `keyword` (the note holds key words), and `meaning` (one of the 50 notes closest in meaning). A note found by meaning alone is a weaker match. When every top result is meaning only, the vault may not hold what was asked for.

Turn it on with `MCP_SEMANTIC_SEARCH=true`. The server then runs a small embedding model, `nomic-embed-text-v1.5`, inside the container with `llama-server`. No note text leaves the server.

- **Index.** Each note is split at its headings. Short sections join their neighbors, and long ones split at paragraphs. Lines that repeat across many notes, such as a daily-note template, are left out. The vectors are saved under `~/.crystal-cove/semantic`, so a restart does not start over.
- **Freshness.** The server checks the vault every 5 minutes and embeds only the sections that changed. The keyword ranking is rebuilt on each check, in memory.
- **First build.** Embedding runs on the CPU at about 2 sections a second, so the first index of a 2,000-note vault (about 8,000 sections) takes close to an hour. It happens once: later checks embed only what changed. The tools work while the first index builds, and `index` in each result reports how many notes are done. Key words, names and phrases work as soon as the notes are read, before any section is embedded. Before the notes are read, the tools return an error that says the index is still being built.
- **Cost.** The embedding server uses about 150 MB of memory, plus the vectors (about 25 MB for 8,000 sections). Queries take tens of milliseconds. The image includes the model either way. With the setting off, nothing starts and the tools do not appear.

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
