# Crystal Cove Microsite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A one-page, dark, richly designed GitHub Pages site for Crystal Cove at `crystalcove.dakotahpena.studio`, built from the Moonlit logo scene.

**Architecture:** Hand-written static files in `site/` with no build step. A Node test file (`node:test`, no dependencies) checks the page's structure and checks that every command, tool name, and security claim on the page still matches the repo's own docs. A `pages.yml` workflow runs those tests and deploys `site/` with GitHub's Pages actions.

**Tech Stack:** HTML, CSS (custom properties, container queries, `@keyframes`), vanilla JavaScript (IntersectionObserver, Clipboard API), inline SVG, self-hosted WOFF2 fonts, Node 20+ `node:test`, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-19-microsite-design.md`

## Global Constraints

- No build step, no npm dependencies, no framework. Files are served as written.
- The only third-party request the page may make is `https://api.github.com/repos/dakotahp/crystal-cove`; the page renders fully without it.
- Dark only. No light theme.
- `prefers-reduced-motion: reduce` stops every animation and shows every section.
- Tone: no "blazing", "supercharge", "10x", "revolutionary", "unhackable", "fully secure", "zero risk", no exclamation marks, no emoji in copy, no competitor names.
- Exactly one `h1`. Every `<img>` has `alt`. Text contrast at least WCAG AA.
- Palette, fonts, and copy come from the spec's "Look and feel" and "Page structure and draft copy" sections.
- Commit messages use `docs:` or `chore:` (or `ci:` for workflows), never `feat:`.
- Each commit ends with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Pre-commit: `node --test scripts/site.test.js` passes; when a workflow changes, `zizmor .` passes.

## Review Focus

1. **The docs change and the page goes stale.** A README command or a tool is renamed; the page must fail its test, not keep showing the old text. Pinned by the README-command and tool-list tests in Tasks 5 and 6.
2. **A phone-width visitor (360px).** No horizontal scroll; the hero crops around the crystal and the copy stays readable. Pinned by the overflow check in Task 8.
3. **No JavaScript, or the star-count request fails.** Every section is visible and the star pill shows "★ Star" with no number. Pinned by the `no-js` reveal test in Task 2 and the star-count fallback test in Task 7.
4. **Reduced motion.** Nothing moves; nothing stays hidden waiting for an animation. Pinned by the reduced-motion CSS test in Task 2.
5. **Clipboard unavailable** (insecure context or denied permission). The copy button must not throw; it shows "Select and copy" instead. Pinned by the copy-button test in Task 5.

---

## File Structure

```
site/
  index.html            all markup, one section per spec section
  style.css             tokens, layout, sections, motion
  script.js             reveal-on-scroll, copy buttons, demo playback, star count
  site.js               pure helpers shared by script.js and the tests (CommonJS + browser global)
  fonts/                Inter, Instrument Serif, JetBrains Mono (latin WOFF2) + OFL.txt
  images/
    favicon.svg         copy of docs/images/logo.svg
    social-preview.png  copy of docs/images/social-preview.png
  CNAME                 crystalcove.dakotahpena.studio
scripts/site.test.js    structure and drift tests
.github/workflows/pages.yml
.github/workflows/ci.yml  (modify: paths-ignore)
```

`site.js` exists so the logic in Review Focus items 3 and 5 is testable in Node without a browser. It exports, and also assigns to `window.CrystalCove` in the browser:

- `formatStars(count: number | null) -> string` — `null` → `""`, `1234` → `"1.2k"`, `987` → `"987"`.
- `copyText(text: string, clipboard: {writeText(s): Promise<void>} | undefined) -> Promise<"copied" | "manual">` — resolves `"manual"` when clipboard is missing or `writeText` rejects; never rejects.

Test helpers in `scripts/site.test.js` (no dependencies, regex on the HTML string):

- `const html = fs.readFileSync('site/index.html', 'utf8')`
- `codeBlocks(html) -> string[]` — text of every `<pre><code data-copy>` block, HTML entities decoded (`&lt; &gt; &amp; &quot; &#39;`).
- `attr(html, tag, name) -> string[]` — all values of one attribute on one tag.

---

### Task 1: Scaffold, fonts, tokens, and the test harness

**Files:**
- Create: `site/index.html`, `site/style.css`, `site/script.js`, `site/site.js`, `site/fonts/*`, `site/images/favicon.svg`, `site/images/social-preview.png`, `site/CNAME`, `scripts/site.test.js`

**Interfaces:**
- Produces: the `<head>` every later task keeps; CSS custom properties `--sky-0 --sky-1 --sky-2 --cliff --water-0 --water-1 --facet-1..5 --moon --ring --text --text-2`; font families `--font-sans: "Inter"`, `--font-serif: "Instrument Serif"`, `--font-mono: "JetBrains Mono"`; section ids `#top #nothing-missing #every-app #security #setup #tools #closing`; the helpers `codeBlocks` and `attr`.

- [ ] **Step 1: Write the failing tests** in `scripts/site.test.js`:

```js
test('page has one h1 and the hero line', () => {
  assert.strictEqual((html.match(/<h1[\s>]/g) || []).length, 1);
  assert.match(html, /Your vault, in <em[^>]*>every<\/em> Claude app\./);
});
test('head carries title, description, canonical, and share tags', () => {
  assert.match(html, /<title>Crystal Cove — Your Obsidian vault in Claude and ChatGPT<\/title>/);
  assert.match(html, /<link rel="canonical" href="https:\/\/crystalcove\.dakotahpena\.studio\/">/);
  for (const p of ['og:title', 'og:description', 'og:image', 'og:url', 'twitter:card'])
    assert.match(html, new RegExp(`(property|name)="${p}"`));
  assert.match(html, /"@type":\s*"SoftwareApplication"/);
});
test('every local asset the page references exists', () => { /* attr(html,'link','href'), attr(html,'script','src'), attr(html,'img','src'), url(...) in style.css: each relative path exists under site/ */ });
test('the page requests nothing from other hosts except the GitHub API', () => {
  /* every http(s) URL in src= attributes, <link href> other than canonical, style.css url(), and fetch( in script.js
     has host api.github.com; <a href> links are exempt */
});
test('every image has alt text', () => { /* each <img> tag has alt=" */ });
test('every section the spec names exists', () => {
  for (const id of ['top','nothing-missing','every-app','security','setup','tools','closing'])
    assert.match(html, new RegExp(`id="${id}"`));
});
test('CNAME names the custom domain', () => {
  assert.strictEqual(fs.readFileSync('site/CNAME','utf8').trim(), 'crystalcove.dakotahpena.studio');
});
test('copy avoids banned words', () => { /* visible text (tags stripped) has none of the Global Constraints words, no "!" outside code, no emoji in \p{Extended_Pictographic} except ★ */ });
```

- [ ] **Step 2: Run** `node --test scripts/site.test.js`. Expected: FAIL, `site/index.html` missing.

- [ ] **Step 3: Download fonts** (latin subset WOFF2) from `https://cdn.jsdelivr.net/npm/@fontsource/` for `inter` (400, 500, 600, 700), `instrument-serif` (400 normal and italic), `jetbrains-mono` (400, 500), into `site/fonts/` as `inter-400.woff2` etc. Add the SIL OFL 1.1 text as `site/fonts/OFL.txt`.

- [ ] **Step 4: Write the scaffold.** `index.html` with the full `<head>` (title, description from the spec's hero sub, canonical, Open Graph and Twitter tags pointing at `https://crystalcove.dakotahpena.studio/images/social-preview.png`, favicon, JSON-LD with name, description, `"license": "https://opensource.org/licenses/MIT"`, `codeRepository` URL), the nav, and empty `<section>` elements with the ids above. Each `<section>` starts with `class="reveal"`. `style.css` with `@font-face` rules (`font-display: swap`), the tokens with the spec palette values, base type, and a `.wrap` container (max width 1180px, 24px gutters, 16px under 480px). `site.js` with the two helpers from "File Structure". Copy the favicon and social image from `docs/images/`. Write `CNAME`.

- [ ] **Step 5: Run** `node --test scripts/site.test.js`. Expected: PASS.

- [ ] **Step 6: Commit** `chore: scaffold the microsite and its tests`.

---

### Task 2: Hero scene and motion — owner review checkpoint

**Files:**
- Modify: `site/index.html` (`#top`, nav), `site/style.css`, `site/script.js`
- Test: `scripts/site.test.js`

**Interfaces:**
- Consumes: tokens and ids from Task 1.
- Produces: the reusable inline SVG `<symbol id="crystal">` (5 facets, viewBox `0 0 100 220`, facet colors `--facet-1..5`) and `<symbol id="mark">` used by later sections; CSS classes `.twinkle .glow .float .drift`; `.reveal` / `.is-visible` contract (script adds `is-visible` when 15% of a section enters the viewport).

- [ ] **Step 1: Write the failing tests:**

```js
test('reduced motion stops animation and reveals every section', () => {
  const css = fs.readFileSync('site/style.css','utf8');
  const block = css.match(/@media \(prefers-reduced-motion: reduce\)\s*{([\s\S]*?)\n}/);
  assert.ok(block, 'reduced-motion block missing');
  assert.match(block[1], /animation:\s*none/);
  assert.match(block[1], /\.reveal\s*{[^}]*opacity:\s*1/);
});
test('sections are visible without JavaScript', () => {
  assert.match(html, /<html[^>]*class="no-js"/);
  const css = fs.readFileSync('site/style.css','utf8');
  assert.match(css, /\.no-js \.reveal\s*{[^}]*opacity:\s*1/);
});
test('hero offers Get started and Star on GitHub', () => {
  assert.match(html, /href="#setup"[^>]*>Get started/);
  assert.match(html, /href="https:\/\/github\.com\/dakotahp\/crystal-cove"[^>]*>[^<]*Star on GitHub/);
});
```

- [ ] **Step 2: Run** `node --test scripts/site.test.js`. Expected: the three new tests FAIL.

- [ ] **Step 3: Build the hero.** Redraw the Moonlit panorama as layered inline SVG at full viewport (`min-height: 100svh`): sky gradient, star field (about 40 stars, three twinkle timings), moon with halo top right, halo glow behind the crystal, left and right cliffs, water, crystal on the right third with its masked reflection, drifting wave strokes. Reference: Figma file `n2EPhBYpIWqSkJ44ocoERy` node `3:632` and `docs/superpowers/specs/2026-10-19-microsite-hero-mockups.html` option A. Copy on the left with the spec's eyebrow, headline, sub, and two buttons. Under 720px wide, `preserveAspectRatio="xMidYMax slice"` keeps the crystal in frame and the copy stacks above it. Nav is sticky with `backdrop-filter: blur`. `script.js` removes `no-js` from `<html>` first thing and adds `is-visible` through IntersectionObserver.

- [ ] **Step 4: Run** `node --test scripts/site.test.js`. Expected: PASS.

- [ ] **Step 5: Commit** `docs: add the microsite hero scene`.

- [ ] **Step 6: Owner review.** Serve with `python3 -m http.server -d site 8000` and give the owner the link. Stop until the owner approves the hero art, at desktop and phone width. Apply their changes before Task 3.

---

### Task 3: Nothing missing, and In every app

**Files:**
- Modify: `site/index.html` (`#nothing-missing`, `#every-app`), `site/style.css`, `site/script.js`
- Test: `scripts/site.test.js`

**Interfaces:**
- Consumes: `#crystal` symbol, `.reveal` contract.
- Produces: `.section-head` pattern (numbered label `01`, headline with one `<em>` accent, lead paragraph) reused by Tasks 4–7; demo element `#demo` with `data-state="idle|playing|done"` and a `button.replay`.

- [ ] **Step 1: Write the failing tests:**

```js
test('three pillars name sync, sign-in, and note tools', () => {
  for (const t of ['Native sync', 'Secure sign-in', 'Tools that understand notes'])
    assert.match(html, new RegExp(`<h3[^>]*>${t}</h3>`));
});
test('the demo names only real tools', () => {
  const tools = [...html.matchAll(/data-tool="([a-z_]+)"/g)].map(m => m[1]);
  assert.deepStrictEqual(tools, ['search_notes', 'append_note']);
});
test('the demo uses no vendor logos', () => {
  assert.doesNotMatch(html, /anthropic|openai/i);
});
```

- [ ] **Step 2: Run** the tests. Expected: new tests FAIL.

- [ ] **Step 3: Build both sections** with the spec's headlines and copy. Pillars are three cards, each topped by one crystal facet shape in `--facet-1`, `--facet-3`, `--facet-5`, with the "Without it:" line set smaller in `--text-2`. The demo: a phone frame (generic chat, user bubble, two tool-call chips with `data-tool`, assistant reply) beside an Obsidian-like note window (title, three existing lines, the new line, a "Synced" dot). `script.js` plays it once when `#demo` becomes visible (bubbles appear 600ms apart, the note line fades in last), sets `data-state`, and the replay button restarts it. Without JavaScript or with reduced motion, `#demo` shows its finished state.

- [ ] **Step 4: Run** the tests. Expected: PASS.

- [ ] **Step 5: Commit** `docs: add the pillars and the demo to the microsite`.

---

### Task 4: Built to face the open internet

**Files:**
- Modify: `site/index.html` (`#security`), `site/style.css`, `site/script.js`
- Test: `scripts/site.test.js`

**Interfaces:**
- Consumes: `#crystal`, `.section-head`, the copy button from Task 5 is not yet built, so this task's code block uses the same markup (`<pre><code data-copy>`) and gets its button in Task 5.
- Produces: none for later tasks.

- [ ] **Step 1: Write the failing tests:**

```js
test('the cosign command matches SECURITY.md exactly', () => {
  const sec = fs.readFileSync('docs/SECURITY.md','utf8');
  const fromDoc = sec.match(/```sh\n(cosign verify[\s\S]*?)```/)[1].trim();
  assert.ok(codeBlocks(html).some(b => b.trim() === fromDoc));
});
test('the security section links to the known limits', () => {
  assert.match(html, /href="https:\/\/github\.com\/dakotahp\/crystal-cove\/blob\/master\/docs\/SECURITY\.md#known-limits"/);
});
test('five security layers, outermost first', () => {
  const layers = [...html.matchAll(/data-layer="(\d)"/g)].map(m => m[1]);
  assert.deepStrictEqual(layers, ['1','2','3','4','5']);
});
```

- [ ] **Step 2: Run** the tests. Expected: new tests FAIL.

- [ ] **Step 3: Build the section.** An SVG of five concentric rings in `--ring` at falling opacity around the crystal, each ring a `<circle data-layer="n">`, beside an ordered list of the five layers (spec copy), each `<li data-layer="n">`. Hover or focus on either highlights both (CSS `:has()` with a `script.js` fallback that toggles `.is-active`). List items are focusable buttons so keyboard users get the same highlight. Then the cosign block with its caption, then the known-limits line.

- [ ] **Step 4: Run** the tests. Expected: PASS.

- [ ] **Step 5: Commit** `docs: add the security section to the microsite`.

---

### Task 5: Setup in four steps, with copy buttons

**Files:**
- Modify: `site/index.html` (`#setup`), `site/style.css`, `site/script.js`, `site/site.js`
- Test: `scripts/site.test.js`

**Interfaces:**
- Consumes: `copyText` from `site/site.js`.
- Produces: the copy-button behavior for every `<pre><code data-copy>` on the page: a `button.copy` inserted after each block, label "Copy" → "Copied" for 2s, or "Select and copy" when `copyText` resolves `"manual"`.

- [ ] **Step 1: Write the failing tests:**

```js
test('every setup command appears verbatim in the README', () => {
  const readme = fs.readFileSync('README.md','utf8');
  const setup = html.slice(html.indexOf('id="setup"'), html.indexOf('id="tools"'));
  const blocks = codeBlocks(setup);
  assert.ok(blocks.length >= 6);
  for (const b of blocks)
    for (const line of b.split('\n').filter(l => l.trim() && !l.trim().startsWith('#')))
      assert.ok(readme.includes(line.trim()), `not in README: ${line}`);
});
test('setup has four numbered steps', () => {
  const setup = html.slice(html.indexOf('id="setup"'), html.indexOf('id="tools"'));
  assert.strictEqual((setup.match(/<li class="step"/g) || []).length, 4);
});
test('copyText reports manual when the clipboard is missing or refuses', async () => {
  const { copyText } = require('../site/site.js');
  assert.strictEqual(await copyText('x', undefined), 'manual');
  assert.strictEqual(await copyText('x', { writeText: () => Promise.reject(new Error('denied')) }), 'manual');
  assert.strictEqual(await copyText('x', { writeText: () => Promise.resolve() }), 'copied');
});
```

- [ ] **Step 2: Run** the tests. Expected: new tests FAIL.

- [ ] **Step 3: Build the section** with the spec's four steps. Commands come from README.md's Installation and "On a cloud server" sections, unchanged (the `.env` block keeps only the lines the spec names: `OBSIDIAN_AUTH_TOKEN`, `OBSIDIAN_VAULTS`, `MCP_PUBLIC_URL`, `MCP_OWNER_PASSWORD`, with their README comments). Steps sit on a vertical line that glows step by step as each scrolls into view. Then the "runs locally too" line and a **Full setup guide** button to `https://github.com/dakotahp/crystal-cove#installation`. Wire the copy buttons in `script.js`.

- [ ] **Step 4: Run** the tests. Expected: PASS.

- [ ] **Step 5: Commit** `docs: add the setup steps to the microsite`.

---

### Task 6: What it can do

**Files:**
- Modify: `site/index.html` (`#tools`), `site/style.css`
- Test: `scripts/site.test.js`

**Interfaces:**
- Consumes: `.section-head`.

- [ ] **Step 1: Write the failing test:**

```js
test('the tool grid matches the README list exactly', () => {
  const readme = fs.readFileSync('README.md','utf8');
  const section = readme.slice(readme.indexOf('## What agents can do'), readme.indexOf('## More docs'));
  const fromReadme = [...section.matchAll(/`([a-z_]+)`/g)].map(m => m[1]).filter(n => n.includes('_')).sort();
  const onPage = [...html.matchAll(/<dt[^>]*><code>([a-z_]+)<\/code>/g)].map(m => m[1]).sort();
  assert.deepStrictEqual(onPage, fromReadme);
});
```

- [ ] **Step 2: Run** the test. Expected: FAIL.

- [ ] **Step 3: Build the grid:** three columns (Read, Find, Write), each a `<dl>` of `<dt><code>tool</code></dt><dd>one line from docs/tools.md</dd>`. The `<dd>` shows on hover and focus on wide screens and is always shown under 720px. A link to `docs/tools.md` on GitHub.

- [ ] **Step 4: Run** the tests. Expected: PASS.

- [ ] **Step 5: Commit** `docs: add the tool grid to the microsite`.

---

### Task 7: Closing, footer, and star count

**Files:**
- Modify: `site/index.html` (`#closing`, footer, nav star pill), `site/style.css`, `site/script.js`, `site/site.js`
- Test: `scripts/site.test.js`

**Interfaces:**
- Consumes: `formatStars` from `site/site.js`; `#crystal`.

- [ ] **Step 1: Write the failing tests:**

```js
test('formatStars', () => {
  const { formatStars } = require('../site/site.js');
  assert.strictEqual(formatStars(null), '');
  assert.strictEqual(formatStars(987), '987');
  assert.strictEqual(formatStars(1234), '1.2k');
});
test('star pill reads "★ Star" before any request', () => {
  assert.match(html, /class="star-pill"[^>]*>★ Star<span class="star-count"><\/span>/);
});
test('footer links and attribution', () => {
  for (const p of ['/blob/master/LICENSE', '/blob/master/CHANGELOG.md', '/releases', '/blob/master/docs/SECURITY.md'])
    assert.match(html, new RegExp(`href="https://github\\.com/dakotahp/crystal-cove${p.replace(/\./g,'\\.')}"`));
  assert.match(html, /Made by Dakotah Pena/);
});
```

- [ ] **Step 2: Run** the tests. Expected: new tests FAIL.

- [ ] **Step 3: Build them.** The closing section's background takes the water gradient; the crystal's reflection shimmers (slow vertical opacity wave) behind the spec's closing headline and two buttons. Footer per spec, no version number. `script.js` fetches `https://api.github.com/repos/dakotahp/crystal-cove` once, reads `stargazers_count`, caches it in `sessionStorage` (inside try/catch), and writes `formatStars` output into every `.star-count`; any failure leaves them empty.

- [ ] **Step 4: Run** the tests. Expected: PASS.

- [ ] **Step 5: Commit** `docs: add the closing section and footer to the microsite`.

---

### Task 8: Polish and copy pass — owner review checkpoint

**Files:**
- Modify: `site/index.html`, `site/style.css`, `site/script.js`

- [ ] **Step 1: Phone width.** At 360px and 390px wide, check `document.documentElement.scrollWidth <= window.innerWidth` in a browser console on every section; fix any overflow.
- [ ] **Step 2: Accessibility.** Keyboard through the whole page: every control is reachable with a visible focus ring; contrast of `--text-2` on the darkest background is at least 4.5:1.
- [ ] **Step 3: Lighthouse** (Chrome DevTools, mobile): Performance, Accessibility, Best Practices, and SEO each at least 95. Record the scores in the commit message.
- [ ] **Step 4: Run** `node --test scripts/site.test.js`. Expected: PASS.
- [ ] **Step 5: Commit** `docs: polish the microsite for phones and accessibility`.
- [ ] **Step 6: Owner review of the whole page and its copy.** Apply their edits, re-run the tests, commit `docs: apply microsite copy edits`.

---

### Task 9: Deploy workflow

**Files:**
- Create: `.github/workflows/pages.yml`
- Modify: `.github/workflows/ci.yml` (push and pull_request triggers)

- [ ] **Step 1: Write `pages.yml`.** Triggers: `push` to `master` with `paths: ['site/**', 'scripts/site.test.js', '.github/workflows/pages.yml']`, `pull_request` with the same paths, and `workflow_dispatch`. Top-level `permissions: {}`. Job `test` (`contents: read`): checkout with `persist-credentials: false`, setup-node 20, `node --test scripts/site.test.js`. Job `deploy` (needs `test`, only on push or dispatch from `master`, `environment: github-pages`, `permissions: { pages: write, id-token: write, contents: read }`): checkout, `actions/configure-pages`, `actions/upload-pages-artifact` with `path: site`, `actions/deploy-pages`. `concurrency: { group: pages, cancel-in-progress: false }`. Pin every action to a full commit SHA with the version in a trailing comment, matching `ci.yml`.
- [ ] **Step 2: Modify `ci.yml`:** add `paths-ignore: ['site/**', 'docs/superpowers/**']` to `push` and `pull_request`. Then check the repo's branch protection (`gh api repos/dakotahp/crystal-cove/branches/master/protection`). If required checks exist, remove the `pull_request` paths-ignore and keep only the `push` one, so site-only PRs still report the required checks.
- [ ] **Step 3: Run** `zizmor .`. Expected: no findings.
- [ ] **Step 4: Run** `node --test scripts/site.test.js`. Expected: PASS.
- [ ] **Step 5: Commit** `ci: deploy the microsite to GitHub Pages`.
- [ ] **Step 6: Hand the owner the outside-the-repo tasks** from the spec: the DNS CNAME, Pages source "GitHub Actions" with the custom domain and Enforce HTTPS, and domain verification.
