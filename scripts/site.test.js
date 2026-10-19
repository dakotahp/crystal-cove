const { test } = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');

const ROOT = path.join(__dirname, '..');
const SITE = path.join(ROOT, 'site');
const read = rel => fs.readFileSync(path.join(ROOT, rel), 'utf8');
const readOptional = rel => (fs.existsSync(path.join(ROOT, rel)) ? read(rel) : '');

const html = readOptional('site/index.html');
const css = readOptional('site/style.css');
const js = readOptional('site/script.js');

const decode = s =>
  s.replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&amp;/g, '&');

const codeBlocks = source =>
  [...source.matchAll(/<pre[^>]*><code[^>]*data-copy[^>]*>([\s\S]*?)<\/code><\/pre>/g)].map(m =>
    decode(m[1].replace(/<[^>]+>/g, ''))
  );

const attr = (source, tag, name) =>
  [...source.matchAll(new RegExp(`<${tag}\\b[^>]*\\s${name}="([^"]*)"`, 'g'))].map(m => m[1]);

const cssUrls = source => [...source.matchAll(/url\(\s*['"]?([^'")]+)['"]?\s*\)/g)].map(m => m[1]);

const visibleText = source =>
  decode(
    source
      .replace(/<script[\s\S]*?<\/script>/g, ' ')
      .replace(/<style[\s\S]*?<\/style>/g, ' ')
      .replace(/<pre[\s\S]*?<\/pre>/g, ' ')
      .replace(/<code[\s\S]*?<\/code>/g, ' ')
      .replace(/<svg[\s\S]*?<\/svg>/g, ' ')
      .replace(/<[^>]+>/g, ' ')
  );

test('page has one h1 and the hero line', () => {
  assert.strictEqual((html.match(/<h1[\s>]/g) || []).length, 1);
  assert.match(html, /Your vault, in <em[^>]*>every<\/em> Claude app\./);
});

test('head carries title, description, canonical, and share tags', () => {
  assert.match(html, /<title>Crystal Cove — Your Obsidian vault in Claude and ChatGPT<\/title>/);
  assert.match(html, /<meta name="description" content="[^"]+">/);
  assert.match(html, /<link rel="canonical" href="https:\/\/crystalcove\.dakotahpena\.studio\/">/);
  for (const p of ['og:title', 'og:description', 'og:image', 'og:url', 'twitter:card'])
    assert.match(html, new RegExp(`(property|name)="${p}"`));
  assert.match(html, /"@type":\s*"SoftwareApplication"/);
});

test('every local asset the page references exists', () => {
  const refs = [
    ...attr(html, 'link', 'href'),
    ...attr(html, 'script', 'src'),
    ...attr(html, 'img', 'src'),
    ...cssUrls(css),
  ].filter(r => !/^(https?:|data:|#)/.test(r));
  assert.ok(refs.length > 0);
  for (const ref of refs) assert.ok(fs.existsSync(path.join(SITE, ref)), `missing: site/${ref}`);
});

const thirdPartyRequests = (page, styles, scripts) =>
  [
    ...attr(page, 'script', 'src'),
    ...attr(page, 'img', 'src'),
    ...(page.match(/<link\b[^>]*>/g) || []).filter(t => !/rel="canonical"/.test(t)).flatMap(t => attr(t, 'link', 'href')),
    ...cssUrls(styles),
    ...[...scripts.matchAll(/fetch\(\s*['"`]([^'"`]+)/g)].map(m => m[1]),
  ].filter(u => /^https?:/.test(u) && new URL(u).host !== 'api.github.com');

test('the page requests nothing from other hosts except the GitHub API', () => {
  assert.deepStrictEqual(thirdPartyRequests(html, css, js), []);
});

test('the third-party check catches scripts, stylesheets, fonts, and fetches', () => {
  const found = thirdPartyRequests(
    '<script src="https://cdn.example.com/a.js"></script><link rel="stylesheet" href="https://fonts.example.com/b.css"><link rel="canonical" href="https://crystalcove.dakotahpena.studio/">',
    '@font-face { src: url("https://fonts.example.com/c.woff2") }',
    "fetch('https://api.github.com/repos/x'); fetch('https://tracker.example.com/d')"
  );
  assert.deepStrictEqual(found, [
    'https://cdn.example.com/a.js',
    'https://fonts.example.com/b.css',
    'https://fonts.example.com/c.woff2',
    'https://tracker.example.com/d',
  ]);
});

test('every image has alt text', () => {
  for (const tag of html.match(/<img\b[^>]*>/g) || []) assert.match(tag, /\salt="/, tag);
});

test('every section the spec names exists', () => {
  for (const id of ['top', 'nothing-missing', 'every-app', 'security', 'setup', 'tools', 'closing'])
    assert.match(html, new RegExp(`id="${id}"`));
});

test('CNAME names the custom domain', () => {
  assert.strictEqual(read('site/CNAME').trim(), 'crystalcove.dakotahpena.studio');
});

test('copy avoids banned words, exclamation marks, and emoji', () => {
  const text = visibleText(html);
  for (const word of ['blazing', 'supercharge', '10x', 'revolutionary', 'unhackable', 'fully secure', 'zero risk'])
    assert.ok(!text.toLowerCase().includes(word), `banned word: ${word}`);
  assert.ok(!text.includes('!'), 'exclamation mark in copy');
  const emoji = text.replace(/★/g, '').match(/\p{Extended_Pictographic}/u);
  assert.strictEqual(emoji, null, `emoji in copy: ${emoji}`);
});

test('reduced motion stops animation and reveals every section', () => {
  const block = css.match(/@media \(prefers-reduced-motion: reduce\)\s*{([\s\S]*?)\n}/);
  assert.ok(block, 'reduced-motion block missing');
  assert.match(block[1], /animation:\s*none/);
  assert.match(block[1], /\.reveal\s*{[^}]*opacity:\s*1/);
});

test('sections are visible without JavaScript', () => {
  assert.match(html, /<html[^>]*class="no-js"/);
  assert.match(css, /\.no-js \.reveal\s*{[^}]*opacity:\s*1/);
});

test('hero offers Get started and Star on GitHub', () => {
  assert.match(html, /href="#setup"[^>]*>Get started/);
  assert.match(html, /href="https:\/\/github\.com\/dakotahp\/crystal-cove"[^>]*>[^<]*Star on GitHub/);
});

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

module.exports = { codeBlocks, attr };
