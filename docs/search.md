# How search works

Crystal Cove gives agents four ways to find notes. Each one answers a
different question, and they are separate tools on purpose: an agent that
knows which question it is asking picks the right tool on the first call.

| Tool | Question it answers | Needs |
|---|---|---|
| `grep_notes` | Which notes hold this exact text? | Nothing extra |
| `search_notes` | Which notes are about this, or named this? | `MCP_SEMANTIC_SEARCH=true` |
| `related_notes` | Which notes are about the same thing as this note? | `MCP_SEMANTIC_SEARCH=true` |
| `find_notes` | Which notes have these tags or frontmatter fields? | Nothing extra |

The short version: use `search_notes` to find notes, and `grep_notes` for
exact words, a regex, every matching line, or a line you are about to edit.
This is the same split as "search" and "grep" in a code editor.

`find_notes` reads metadata only (tags and frontmatter) and is described in
[MCP tools](tools.md). This page covers the other three.

## grep_notes: exact search

`grep_notes` runs [ripgrep](https://github.com/BurntSushi/ripgrep) over the
vault's Markdown notes. It finds text exactly as written, ignoring case
unless `case_sensitive` is set. Nothing is indexed, so a note written a
second ago is already searchable.

### How it reads a query

A query means one of three things. `mode` picks one; without it, the server
guesses from the query:

- **Words** (the default for plain text): every word must appear somewhere
  in the note, in any order. `budget rent` finds a note holding both words,
  even on different lines.
- **Phrase**: the exact text, with no escaping. A query wrapped in double
  quotes is a phrase. Use it for text with punctuation, dashes or emoji, and
  to find a line before editing it.
- **Regex**: a ripgrep regular expression. Any of `^ $ \ . [ ] ( ) | * + ? { }`
  in the query makes it a regex.

### Names count too

A note's name is usually its subject, so `grep_notes` matches note names as
well as content. A note whose name matches the query is a result even when
no line in it matches, and it is flagged `title_match`. Names are matched
the same way the content is: all words, the phrase, or the regex.

### Ranking and pages

Ripgrep returns notes in path order, which buries the obvious note under
whatever sorts first. The server ranks every matching note before it takes
a page:

1. Notes whose name matches.
2. Then notes with more matching lines.
3. Then path order.

`limit` counts notes (default 50, at most 500), and `offset` with
`next_offset` pages through the same ranked list. Each note returns at most
5 matching lines unless `max_lines_per_note` says otherwise, and reports its
`total_matches`. A line longer than 500 characters is cut to a window around
the match. `output: "files"` returns only paths and counts, the cheap way to
choose which note to read.

Hidden folders such as `.obsidian` and `.trash` are not searched.

## search_notes: hybrid search

`search_notes` exists only with `MCP_SEMANTIC_SEARCH=true`. It is **hybrid
search**: it combines a ranking by meaning with a ranking by key words.
Each catches what the other misses:

- **Meaning** finds a note about the same topic in other words. "Feeling
  burned out" finds a note about low energy. It is weak on names and rare
  terms: a book title can mean little to a language model.
- **Key words** find names and rare terms at once. They miss a note that
  says the same thing in different words.

The rest of this section explains each part, then how they combine.

### Sections

Both rankings work on **sections** of notes, not whole notes. Each note is
split at its headings. A section shorter than 400 characters joins its
neighbor, so a note with a heading on every line does not turn into
fragments. A section longer than 1,500 characters is cut at paragraph
breaks.

Lines that repeat across many notes are left out, such as a daily-note
template's headings. A line counts as template text when it appears in at
least 10 notes, or in 1% of the vault if that is more.

Links are read as the text Obsidian shows: `[[Plan|the plan]]` becomes "the
plan" and `[[Plan#Budget]]` becomes "Plan > Budget". Brackets and link
targets are markup, not writing, so neither the model nor the excerpt sees
them.

Sections matter because one note can cover many topics. A long journal with
one short section on the query would otherwise lose to a short note that
mentions the query once. Results show the section that matched, as
`heading_path` with an excerpt, so an agent can judge a hit without reading
the whole note. When the match is a section joined from short ones, the
result points at the short section that holds the most key words of the
query, so a hit on the eighth of ten short headed entries shows that entry,
not the first. With no key word in common, it shows the start.

### Meaning: embeddings

Each section becomes a vector, a list of numbers that places its meaning in
space, so that sections about similar things sit close together. The model
is `nomic-embed-text-v1.5`, run by `llama-server` inside the container. No
note text leaves the server.

The text the model reads for a section is the note's name, the section's
heading path, and the section's text, after a prefix that marks it as a
document. A query gets a prefix that marks it as a query; the model was
trained to tell the two apart. Closeness is the cosine between the query's
vector and a section's vector.
A note scores as its closest section.

Vectors are saved under `~/.crystal-cove/semantic`, keyed by a hash of the
text, so a restart does not start over. Every 5 minutes the server checks
the vault and embeds only sections whose text changed. The first build
embeds everything, at about 2 sections a second on a CPU: close to an hour
for a 2,000-note vault.

There is no vector database. Every query is compared with every section
vector in memory, which takes tens of milliseconds for a few thousand
notes.

### Key words: BM25

BM25 ("Best Matching, version 25") is the standard algorithm for ranking
text by key words. It came out of search-engine research in the 1990s and
is the default in Elasticsearch, Lucene, and SQLite's full-text search.
Here it is a few dozen lines of Go over an index held in memory, with no
extra software.

**Words.** Text is lowercased and split into runs of letters and digits.
There is no word list and no stemming, so this works the same in any
language written with spaces. Word forms do not match: "organize" does not
find "organizing". The meaning ranking covers those. Languages written
without spaces, such as Chinese and Japanese, get little from key words;
meaning still works for them.

Each section's words include its headings and the note's name, so a word
that appears only in a heading still counts.

**The score.** For a query with words *q₁ … qₙ*, a section *S* scores:

```
score(S) = Σ  idf(qᵢ) · tf(qᵢ, S) · (k₁ + 1)
              ─────────────────────────────────────────────
              tf(qᵢ, S) + k₁ · (1 − b + b · |S| / avgLen)
```

- `tf(q, S)` is how many times the word appears in the section.
- `|S|` is the section's length in words, and `avgLen` the average length.
- `k₁ = 1.2` and `b = 0.75` are BM25's usual constants.
- `idf(q) = ln(1 + (N − df + 0.5) / (df + 0.5))`, where `N` is the number
  of sections and `df` the number holding the word.

The formula carries three ideas:

1. **Rare words count more.** That is `idf`. A word in 3 sections of 9,000
   scores high. A word in nearly every section, like "the", scores close to
   nothing. This is why there is no stop-word list.
2. **Repeats help, then level off.** That is the `tf / (tf + k₁ …)` shape.
   The second mention adds a lot, the thirtieth almost nothing, so a section
   cannot win by repeating a word.
3. **Long sections are marked down.** That is the `|S| / avgLen` term. One
   mention in a short section means more than one mention in a long one.

A note scores as its best section.

**Common words in a query.** Before scoring, the query drops words that
more than half the sections hold, unless that would drop every word. A
plain-language question then scores on its meaningful words.

**Coverage.** For each section the server also records **coverage**: the
share of the query's total `idf` that the section holds. A section holding
every key word has coverage 1. A section holding only the commonest one has
coverage near 0. Coverage decides which keyword matches are trusted, below.

The word index is rebuilt from scratch on each 5-minute check, right after
the notes are read and before any embedding. It stores a postings list (for
each word, the sections holding it and how often), which takes a few MB for
a few thousand notes.

### Combining them: meaning first, key words rescue

The meaning ranking sets the order. Key words only **rescue** clear hits
that meaning misses. These move to the top, in this order:

1. **Name.** Notes whose name holds every word of the query.
2. **Phrase.** Notes holding a quoted phrase from the query, with the words
   together as written. Headings count.
3. **Key words.** Up to 3 notes with a section of coverage 1, holding every
   key word, chosen by highest BM25 score.

Each group is in meaning order. Then the 50 notes closest in meaning
follow, and the list stops there. Past about the 50th note, closeness says
little, and stopping keeps `total` honest.

A note that holds only some of the key words keeps its place in the meaning
order. This is the most important rule in the design, and the evaluation
below explains why.

Before any vector exists, while the first index builds, key words alone set
the order, so search works seconds after start.

### Reading the results

Each result reports `matched_by`:

- `name`: the note's name holds every word of the query.
- `phrase`: the note holds a quoted phrase.
- `keyword`: one section of the note holds every key word, or key words
  alone placed it because the note has no vector yet.
- `meaning`: the note is one of the 50 closest in meaning.

A note found by meaning alone is a weaker match. When every top result is
meaning only, the vault may not hold what was asked for. A note holding only
some key words is not labelled `keyword`: one shared word, such as "heart"
in a vault of letters, would put the label on nearly every result.

`total` counts the results across every page, which is at most the 50
closest notes plus any moved up. It is not the number of notes in the vault
that match.

A query should hold one idea. Its vector sits between the ideas it mixes and
close to none of them, and no section holds every key word of a long mixed
query, so key words rescue nothing. One search per idea finds each.

There is no score in the output and no score cutoff. On the test vault, the
point where results turned to noise moved from a closeness of about 0.73 for
one query to 0.61 for another, and the least useful query had the highest
scores. No fixed number works, and an agent cannot pick one well. Use
`limit` (default 10) and the order instead. When the first page holds
nothing useful, a better query usually beats the next page.

## related_notes

`related_notes` uses the same section vectors. Given one note, it compares
every section of that note with every section of every other note, and
ranks each other note by its closest pair. A long note matches on its most
similar section, not an average of all of them. `linked` marks notes that
already link to it or from it, and `unlinked: true` leaves them out, to show
only connections the links miss.

## How the design was chosen

The combining rule was tested against meaning alone on a real personal
vault of a few thousand notes. There were 51 labelled queries in four sets. Each query had the notes
a good answer holds; a few had no answer in the vault. The score is mean
reciprocal rank: 1 over the rank of the first expected note, averaged, so
1.0 means the right note was always first.

The usual way to combine keyword and meaning rankings is **reciprocal rank
fusion** (RRF): each note scores `1 / (60 + rank)` in each list it appears
in, and the sums are compared. It was tried first.

| Design | Set 1 (dev) | Set 2 (dev) | Set 3 (held out) | Set 4 (held out) |
|---|---|---|---|---|
| Meaning only | 0.679 | 0.818 | 0.819 | 0.757 |
| RRF, BM25 per note | 0.502 | – | – | – |
| RRF, BM25 per section | 0.854 | 0.778 | – | – |
| RRF, keyword votes scaled by coverage | 0.792 | 0.786 | 0.745 | – |
| Meaning first, key words rescue | 0.679 | 0.818 | 0.890 | **0.793** |

Sets 1 and 2 were used to develop each design, so their scores flatter it.
Each held-out set was written after its design was frozen and run once.

What the tests showed:

- **BM25 per note buried long notes.** One matching section in a long
  journal lost to the length of the rest. Scoring per section fixed it, and
  so did counting heading words: one test note named its topic only in a
  heading.
- **RRF won on key words but lost on plain-language questions.** It gives a
  weak keyword match the same vote as a strong one. A plain-language
  question shares ordinary words with dozens of unrelated notes, and those
  notes pushed the right one down, in one case from 1st to 9th. Scaling votes by coverage narrowed the gap but did not close it.
- **Meaning first never lost.** Across all 51 queries it never ranks the
  expected note lower than meaning alone does. Its gains are names and rare
  titles: a book's notes went from 209th to 1st on the book's title.
- **The trade-off.** RRF's best win, a question whose key word sat in one
  section heading, went from 10th to 1st under RRF but stays at 10th now,
  because that section did not hold every key word. The strict rule gives
  up a few such wins to protect every plain-language question.

The evaluation harness is `internal/semantic/eval_test.go`. It is skipped
unless `CRYSTAL_COVE_EVAL` names a file of labelled queries, which should
stay outside the repository because it names your notes. An expected entry
written `path#heading` names a section, and the report then also counts how
often a top-5 result points at that section. To run it, start
`llama-server` with the model, then:

```
CRYSTAL_COVE_EVAL=queries.json go test ./internal/semantic -run TestEval -v
```

`CRYSTAL_COVE_EVAL_DETAIL=1` also prints each query's top results from both
rankings and how each note was found. The file's format is described in
the test.
