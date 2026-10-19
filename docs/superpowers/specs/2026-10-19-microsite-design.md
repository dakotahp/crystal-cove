# Crystal Cove microsite: design

## Intent

A one-page marketing site for Crystal Cove, hosted on GitHub Pages at
`crystalcove.dakotahpena.studio`. It exists for discovery and search, and to
give the project a brand and a face. It sells the project the way a product
page does, even though every path leads to the GitHub repo.

**Reader.** An Obsidian user who already has Claude working with their vault
on a laptop (Claude Code, a local MCP server, the file system). What they do
not have is the vault everywhere else: the Claude phone app, claude.ai in a
browser, ChatGPT, a second computer. They can run one Docker container but
are not Docker experts.

**What the reader should do, in order:** set the project up, and star the
repo.

**The one idea to remember.** Crystal Cove is complete. Other Obsidian MCP
servers each miss at least one of three things: native Obsidian Sync,
secure cloud sign-in (OAuth), or tools that understand notes. Crystal Cove
has all three, and once it runs, connecting is close to one click: add the
URL in Claude or ChatGPT and enter a password.

**Security is a major theme.** The server puts a whole knowledge base on the
open internet. The project's security work (`docs/SECURITY.md`) is a reason
to trust it over a quickly assembled alternative, and the page should show
that work.

## Goals

- A striking, deliberately rich page that leans into the logo's night-cove
  scene. Showing off is the point. archify.si is the reference for polish and
  for "sells the product", not for colors, content, or length.
- The hero line, "Your vault, in every Claude app.", with ChatGPT named in
  the first sentence after it. ChatGPT has equal billing in the copy.
- Every claim on the page is true today and traceable to the README or
  `docs/`.
- Fast, small, and free of third-party requests.

## Non-goals

- More than one page. Docs stay in the repo; the site links to them.
- A static site generator. Hugo was considered and rejected: one page gains
  nothing from templates.
- A light theme.
- Analytics, cookies, or any tracking.
- Localization.

## Anti-goals (would make it a failure even if it works)

- **Boastful or "tech bro" tone.** No "blazing", "supercharge", "10x",
  "revolutionary", exclamation marks, or emoji. Calm, plain, confident
  sentences. The README's voice is the model.
- **Jabs at other projects.** Never name competitors. Never say "unlike
  vibe-coded projects". The security section makes that point by showing
  the work, not by saying it.
- **Overclaiming.** No "unhackable", "fully secure", or "zero risk". The page
  links to the known limits.
- **Motion that pulls the eye off the copy.** Animation is slow and ambient.

## Constraints

- Lives in `site/` on `master` of this repo. No `gh-pages` branch, no
  separate repo.
- Plain HTML, CSS, and a little JavaScript. No build step, no npm
  dependencies, no framework.
- Deployed by a new `.github/workflows/pages.yml` using GitHub's Pages
  actions (`actions/upload-pages-artifact`, `actions/deploy-pages`), pinned
  to commit SHAs like every other action here. It triggers on pushes to
  `master` that touch `site/**` and on `workflow_dispatch`. Permissions are
  the minimum Pages needs (`pages: write`, `id-token: write`,
  `contents: read`). It must pass zizmor.
- `ci.yml` gets `paths-ignore: ['site/**', 'docs/superpowers/**']` on its
  push and pull-request triggers, so a site-only change does not run the Go
  suite or build images. (A builder should confirm this does not break any
  required status check on PRs; see "Left to the builder".)
- Commits for the site use `docs:` or `chore:`, never `feat:`, so
  release-please does not cut a release.
- The Go quality gates (gofmt, vet, coverage) do not see `site/`; nothing
  to change there.

## Look and feel

**Source of truth for the brand:** the Figma file "Crystal Cove Logo"
(`n2EPhBYpIWqSkJ44ocoERy`), variant **A1 · Moonlit**, and the panorama
banner (node `3:632`). The repo already ships `docs/images/logo.svg`,
`docs/images/banner.svg`, and `docs/images/social-preview.png` from it.
A3 · Teal spook is not used.

**New art (decided by the spec author).** The banner is a 3:1 panorama and
the page needs more: a full-viewport hero, a phone crop around the crystal,
separate layers for animation (stars, glow, waves), and an underwater
closing scene. The builder redraws and extends the scene as SVG, matching
A1 · Moonlit in shapes, colors, and style. The Figma file is readable
through the Figma MCP for reference. The owner approves the new art in
pass 1.

**Mockups.** `2026-10-19-microsite-hero-mockups.html` beside this file
holds the three hero directions the owner reviewed. "A · Panorama" was
chosen. It is an HTML fragment made for a preview frame, so some wrapper
styles are missing when opened directly.

**Palette** (sampled from the logo; the builder may refine against Figma):

| Role | Value |
|---|---|
| Sky, top to horizon | `#110c2c` → `#231a52` → `#33256f` |
| Cliffs | `#05040b` |
| Water | `#1d2858` → `#0a0f2b` |
| Crystal facets, light to dark | `#e4dcff`, `#a98bfa`, `#8b5cf6`, `#7c3aed`, `#5b21b6` |
| Moon | `#f6ebc6` |
| Ring / accent line | `#c4b5fd` |
| Text | `#f3efff`; secondary `#b7acdf` |

**Type.** Inter for text and headings. Instrument Serif italic for one
accent word or phrase per headline (archify's device, used sparingly).
JetBrains Mono for code. All three are self-hosted as WOFF2 in `site/fonts/`
(subset to Latin), so the page makes no request to Google.

**Motion.** Stars twinkle, the crystal's glow slowly breathes, the crystal
floats a few pixels, waves drift. Sections fade and rise in as they scroll
into view. With `prefers-reduced-motion: reduce`, all of it stops and every
section is simply visible.

**Dark only.** The night scene is the brand.

## Page structure and draft copy

The copy below is a first draft. The owner will edit it in the browser
mockups before it is final; the builder should treat wording as editable
and structure as decided.

### Nav (sticky, translucent over the scene)

Logo mark + "Crystal Cove" · Features · Security · Setup · Docs (repo
`docs/`) · a "★ Star" pill with the live star count (see "Left to the
builder").

### Hero — layout "A · Panorama", calmed down

The README banner scene at full viewport: sky, moon top right, cliffs,
crystal and its reflection on the right third, water across the bottom.
Copy on the left. Chosen over a centered "monument" layout because the
hero must carry context, not just a headline; from that layout it keeps
the stillness and the small number of elements.

- Eyebrow: `Obsidian MCP server · self-hosted`
- Headline: **Your vault, in *every* Claude app.** ("every" in the serif
  italic)
- Sub: "Claude and ChatGPT read and write your Obsidian notes on your phone,
  on the web, and on every computer. One container, with sync, sign-in, and
  note tools built in."
- Buttons: **Get started** (to the setup section) · **★ Star on GitHub**

No "Works with" logo row. On narrow screens the scene crops around the
crystal and the copy sits above it.

### 1 · Nothing missing

Headline: **Everything a vault needs. *Nothing missing.***

Three pillars, each drawn as one facet of the crystal (light, mid, dark
facet color). Each says what it is and what breaks without it, as in the
README's "The Why":

1. **Native sync.** Obsidian's official headless client keeps the vault
   current. The AI's edits reach your phone and laptop like any other edit.
   Without it: a copy that drifts, or a desktop app that must stay awake.
2. **Secure sign-in.** OAuth, the sign-in flow Claude and ChatGPT
   connectors expect. Enter one password and you are connected. Without it:
   the vault stays on your laptop.
3. **Tools that understand notes.** Names, tags, links, backlinks, sections,
   frontmatter. Fewer tokens than reading raw files. Without them: an
   assistant that greps a folder.

### 2 · In every app

Headline: **Ask from anywhere. *It lands in Obsidian.***

A demo, built in HTML/CSS (not a screenshot), side by side:

- Left: a phone frame with a Claude-style chat. The user asks something
  like "Add 'call the landlord' to today's note". The assistant shows a
  tool call (`search_notes`, then `append_note`) and confirms.
- Right: an Obsidian-style note window where the new line appears, with a
  small "Synced" indicator.

It plays once when scrolled into view and can be replayed. Under it, one
line: "Works in claude.ai, the Claude apps, Claude Code, and ChatGPT."
The chat must not use Anthropic's or OpenAI's logos or exact UI; it is a
generic, clearly illustrative chat.

### 3 · Built to face the open internet

Headline: **Your notes on the internet. *Locked down.***

Lead: "This server holds your whole knowledge base on a machine that faces
the internet. Every design decision starts there."

The visual is the logo's ring: the crystal in the center, concentric rings
around it, one per layer. Hovering or tapping a ring (or its list item)
highlights it. Five layers, from `docs/SECURITY.md`:

1. **Sign-in, always.** Every request needs a token; there is no open mode.
   OAuth with PKCE, rotating refresh tokens, and a lockout after wrong
   passwords.
2. **The vault and nothing else.** Paths cannot escape the vault and
   symlinks cannot lead out. Tools touch notes only, never Obsidian's
   settings.
3. **Notes stay intact.** Writes never overwrite a sync change and wait
   while you type on another device. Deletes go to the trash. A read-only
   mode removes every write tool.
4. **A locked-down container.** Non-root, read-only filesystem, every Linux
   capability dropped.
5. **Checked before it ships.** Static analysis, fuzzing, secret scanning,
   and an image vulnerability scan on every change. Every image is signed.

Below the rings, a code block with the real `cosign verify` command from
`docs/SECURITY.md`, captioned "Check that an image came from this repo
before you run it."

Closing line: "We list the known limits too." linking to
`docs/SECURITY.md#known-limits`.

### 4 · Setup

Headline: **Running in *four steps.***

The README's cloud-server path has about seven steps. The page groups them
into four, and every command a reader needs is on the page (owner's
decision), so a reader can succeed without leaving it. Nothing is hidden:
the HTTPS proxy is its own step. Each step has a short sentence and
copyable code blocks (copy button, "Copied" feedback). Commands must match
the README exactly at build time:

1. **Download and sign in.** Make a folder and `curl -O` the compose
   file. Run the `docker run … ob login` command, which prints your
   Obsidian Sync token. Then the `ob sync-list-remote` command, which
   lists your vault names.
2. **Fill in `.env`.** The token, the vault name, `MCP_PUBLIC_URL`, and
   `MCP_OWNER_PASSWORD` (16 or more characters). One line notes that
   end-to-end encrypted vaults also need `OBSIDIAN_VAULT_PASSWORD`.
3. **Add HTTPS and start.** The two-line Caddy config, a note that any
   reverse proxy or a Cloudflare Tunnel works, then `docker compose up -d`
   and wait for "Fully synced".
4. **Connect.** Add your URL as a custom connector in Claude or ChatGPT,
   and enter your password.

Then a quiet line: "Only want it on your laptop? It runs locally too." and
a **Full setup guide** button to the README's Installation section.

### 5 · What it can do

Headline: **Tools that read notes *the way you do.***

A compact grid in three columns, matching the README's grouping:

- **Read:** `read_note`, `read_notes`, `get_section`, `get_frontmatter`,
  `list_notes`, `recent_notes`
- **Find:** `search_notes`, `find_notes`, `list_tags`, `get_links`,
  `get_backlinks`
- **Write:** `create_note`, `append_note`, `edit_note`, `edit_section`,
  `update_frontmatter`, `move_note`, `delete_note`

Each tool has a one-line description on hover/focus (from `docs/tools.md`).
A link to `docs/tools.md` for details.

### 6 · Closing, under the waterline

The page "sinks" below the water: the background shifts to the water
gradient, and the crystal's reflection shimmers behind the content.

Headline: **Your vault, *everywhere you ask.***

Buttons: **★ Star on GitHub** · **Get started**

Footer: logo mark, "MIT License", links to GitHub, Docs, Security,
Changelog, and Releases. Small attribution: "Made by Dakotah Pena." No
version number (spec author's call): the site deploys only when `site/`
changes, so a typed version goes stale at the next release.

## Files

```
site/
  index.html
  style.css
  script.js          # motion triggers, copy buttons, demo playback, star count
  fonts/             # self-hosted WOFF2
  images/            # logo, scene parts, social image (SVG where possible)
  CNAME              # crystalcove.dakotahpena.studio (harmless with Actions deploys)
.github/workflows/pages.yml
```

## Search and sharing

- `<title>`: "Crystal Cove — Your Obsidian vault in Claude and ChatGPT"
- Meta description from the hero sub.
- Open Graph and Twitter card tags using `docs/images/social-preview.png`
  (1280×640), copied into `site/images/`.
- Canonical URL `https://crystalcove.dakotahpena.studio/`.
- `favicon.svg` from the logo.
- JSON-LD `SoftwareApplication` with name, description, license, and repo
  URL.
- Semantic headings (one `h1`), alt text on every meaningful image,
  visible focus states, and text contrast of at least WCAG AA on the dark
  backgrounds.

## Owner's tasks outside the repo

- DNS: CNAME `crystalcove` → `dakotahp.github.io`.
- Repo Settings → Pages: source "GitHub Actions", custom domain
  `crystalcove.dakotahpena.studio`, Enforce HTTPS on.
- Verify `dakotahpena.studio` in GitHub account settings, to block domain
  takeover.

## Process

The owner wants to spend a long time refining the design. Build in
passes, and show each pass in the browser before the next. Each pass is
previewed locally (a static file server on `site/`, or the brainstorming
visual companion). The owner's feedback and copy edits come back in chat,
and the builder applies them. Nothing deploys until pass 6.

1. Hero at full fidelity, desktop and phone.
2. Sections 1–3.
3. Sections 4–6.
4. Copy pass with the owner.
5. Polish: motion, phone layout, accessibility, Lighthouse.
6. Deploy workflow.

## Left to the builder

- Exact spacing, sizes, and breakpoints.
- Whether the scene is one SVG or layered SVGs/CSS for parallax.
- The star count: fetch it in the browser from the GitHub API (one
  unauthenticated request, cached in `sessionStorage`, hidden on failure),
  or drop the number and keep the "★ Star" label. Either is fine; the
  page must render fully without it. A fetch to `api.github.com` is the
  one allowed third-party request.
- Whether `paths-ignore` in `ci.yml` blocks merging site-only PRs because
  of required status checks. If it does, use a small always-passing job
  for site-only changes, or skip the `paths-ignore` change.
- Tool descriptions for section 5, taken from `docs/tools.md`.
