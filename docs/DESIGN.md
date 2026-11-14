# jqv — design

> **jqv** (working name) is a fast terminal workbench for JSON. Open a document, take it apart,
> and put it back together with jq, the way a mechanic strips an engine on the bench.

This is a living document. Settled decisions are stated plainly; open questions are marked
**OPEN** and tracked in [WISHLIST.md](WISHLIST.md).

---

## 1. Why

| Tool | What's good | What hurts |
|---|---|---|
| jq | Enormously powerful | Write-only at the prompt: edit, run, squint at the error, repeat. Hard to learn by trial |
| jless | Fast, vim-keyed tree viewer | View only: no queries, no output, nothing to build with |
| fx / jnv / ijq | Interactive jq of a kind | Either a viewer or a query box, not a bench with both; weak at explaining errors |
| sql-cli (own) | SQL over flat JSON | Needs flat data; nested JSON has to be shaped first, which is jq's job |
| SSMS | A work area: object tree, query, results grid side by side | It's for SQL Server |

**jqv's promise:** *see the whole document, try a jq idea in a keystroke, and see
exactly where it went wrong.*

## 2. Principles

1. **Built for the owner's daily use.** Ergonomics decided by use beat theory.
   Downloads are not a goal.
2. **Fast.** Startup and every keystroke must feel instant on any document you'd look at at
   work (up to tens of MB). Measure before optimising, and once something has been measured,
   don't let it regress.
3. **The document is never changed.** Queries, filters and flattening produce *output*;
   the source stays as it was loaded, key order included.
4. **jq in-process.** gojq as a library: no process spawn, and we get the AST, error positions
   and custom functions. A "run with real jq" check is allowed, never required.
5. **Keyboard first, vim-shaped.** Every action has a key. The mouse is optional.
6. **Pretty is a feature.** Colour, alignment and calm chrome all matter; a tool you enjoy
   looking at is a tool you open.
7. **Portable.** Windows Terminal (pwsh) first, Linux and macOS terminals always. No X, no browser.
8. **Teach jq.** Errors say what was wrong *and* what to try. Recipes and the AST view exist to
   build understanding, not to hide jq.

## 3. Architecture

```
cmd/jqv            flags, input (file / stdin), --print, starts the TUI
internal/doc       ordered JSON tree: Node{Kind, Key, Index, Raw, Children, Parent}; Path(); Value()
internal/tree      view state: which nodes are open; the flat visible-row list; row labels
internal/tui       bubbletea model (pure Update) + view (lipgloss); options.go is the :set table
internal/config    config.toml: reads the [set] table, collects problems, never fails
internal/filter    the filter stack: jq layers over an array of records; Lift to one jq program
— planned —
internal/query     gojq wrapper: parse, compile, run with a timeout and cancel; error → position + hint
internal/shape     shape index: every path, the keys and types seen under it (autocomplete, schema view)
internal/table     array-of-objects → columns/rows (output mode, CSV/sql-cli export)
internal/recipe    named, parameterised jq snippets from a TOML file
```

**Why our own parser (C-001).** encoding/json and gojq decode objects into Go maps, which have
no order; gojq also *sorts* keys on output. A viewer has to show keys in the order they were
written, so `doc` builds its tree from `json.Decoder` tokens. gojq receives `Node.Value()`.
Query *output* comes out of gojq in sorted-key order; that's acceptable for now (OPEN: Q-010).

**The row list.** `tree` keeps a flat slice of visible rows (node + depth), rebuilt on
expand/collapse. Scrolling, cursor movement and rendering only touch what is on screen, so the
size of the document matters only when rebuilding. If rebuilds ever show up in timings, the
fix is incremental splicing of the slice (C-006), not a different design.

## 4. The work area (target layout)

```
┌─ jqv  orders.json ─────────────────────────────────────────── {4} ┐
│ ▾ {4}                        │ Q  .orders[] | select(.qty > 500)    │
│ │ source: "trd-platform"     │    stage 2/2 · 2 results · 0.3 ms    │
│ │ ▾ orders [3]               ├─ output (tree | table | raw) ────────┤
│ │ │ ▸ [0] {9}                │ ▾ [2]                                │
│ │ │ ▸ [1] {10}               │ │ ▸ [0] {9}                          │
├──────────────────────────────┴──────────────────────────────────────┤
│ .orders[1]                                       NORMAL      6/14   │
└─────────────────────────────────────────────────────────────────────┘
```

Built in this order (see the wishlist milestones): the document tree (M1), then the query pane
and output (M2), then shape and autocomplete (M3).

## 5. Keys

Vim-shaped, inside the TUI only. Chords use a pending first key shown in the status bar.

| Key | Action |
|---|---|
| `j` `k` / arrows | down / up |
| `h` / left | close this container, or go to the parent |
| `l` / right | open this container, or step into it |
| `enter` / space | toggle |
| `gg` `G` / home end | top / bottom |
| `ctrl+f` `ctrl+b` / pgdn pgup | page down / up |
| `ctrl+d` `ctrl+u` | half page |
| `[{` `]}` | start / end of the enclosing object (vim's; arrays are skipped, so it climbs by records) |
| `%` | start of a block to its last row and back; opens a closed block; from a scalar, the start of its block |
| `{` `}` | previous / next block at this level, skipping scalars: opens it and puts it at the top of the screen. From a scalar, moves from the block it is in. Blocks left behind stay open |
| `(` `)` | previous / next element of the nearest array around the cursor, from any depth inside the element; opens it and puts it at the top of the screen; stops at the ends. On an array's own row, into its last / first element |
| `ctrl+g` | where am I: row n of m, % through the document, depth |
| `:` | command line: `:123` row, `:set` an option (§13), `:q` |
| `L` | open everything under the cursor (at the root: the whole document) |
| `H` | close everything under the cursor, keep it open as an outline |
| `zo` `zc` | open / close this node and everything under it |
| `zR` `zM` | open / close everything |
| `z1`…`z9` | open the whole document to that depth |
| `y` / `Y` | copy the jq path / the value: a string as its text without quotes (it is pasted into something), anything else as JSON |
| `/` `?` `n` `N` | search forward / back, next / previous (smartcase; `\c` `\C` force). The cursor follows as you type; esc puts everything back |
| `f` | finder: type, and every match is listed under the tree, which follows the selection. `up` `down` `ctrl+j` `ctrl+k` `pgup` `pgdown` move, enter lands, esc goes back |
| `F` | the last search as a finder list, on the match at the cursor |
| `z/` | collapse everything except the paths down to each match |
| `esc` | clear the search highlight |
| `zz` `zt` `zb` | scroll the cursor to the middle / top / bottom |
| `zl` | lock the cursor to the middle row, or unlock (vim's `scrolloff=999`; the ends of the document still stop it). `zt` and `zb` end the lock |
| `F1` | help: every key, from `internal/tui/keys.go` |
| `q` | quit |

Planned: a key into the query bar (`i`, since `:` is now vim's command line), `K` to show the AST of the query (as nvim's
LSP hover does). **Keymap as data** (T-005) once there are enough keys to argue about.
Keys must avoid Zellij / tmux / Windows Terminal defaults (same list as hit's DESIGN §14.1).

## 6. Input

- `jqv file.json`, or JSON piped in (`… | jqv`). With piped stdin, keys come from the console
  (`tea.WithInputTTY`).
- One top-level value. **OPEN:** JSON Lines / several concatenated values (C-004).
- `--print` renders the tree as plain text (same labels as the TUI) and exits: for scripts,
  for diffing, and for tests.

## 7. Testing

**Changes must never quietly undo behaviour we've already agreed on.** Every layer is tested from day one;
CI runs it all on Windows and Linux.

| Layer | How | What it proves |
|---|---|---|
| doc unit | table tests | key order, paths and quoting, scalars, errors |
| doc fuzz | `FuzzParse` | never panics; anything accepted means what encoding/json says it means |
| tree golden | `testdata/*.golden`, `-update` to re-bless | expand/collapse/depth produce exactly these rows |
| TUI model | key messages into `Update`, no terminal | each key does what §5 says; cursor follows collapses; scroll keeps cursor visible |
| TUI view | width/height invariants + plain-text golden | nothing overflows at 20/40/120 columns; layout doesn't drift |
| CLI | `run()` with fake stdin/stdout | `--print`, stdin, error messages |
| filter stack | corpus `testdata/filter/cases.toml` over `trades.json`; each case also lifted to one jq program and run through gojq and, if on PATH, real jq | layers keep exactly these records, and mean what jq means |
| lessons | `learn/*.md`: input, `jq` and output blocks; `./scripts/learn.ps1` writes the outputs (§17) | every query gives what the page says, under gojq and real jq |
| query (planned) | corpus `testdata/queries/*.jq` + input + expected output | gojq results and our error hints don't drift |

### 7.1 Regression workflow

1. Something from daily use is wrong.
2. **First a failing test or fixture**: the JSON goes in `testdata/`, the keys or query in a test.
3. Fix, then commit with the wishlist ID.
4. The fixtures only grow, so daily use gradually becomes the test suite.

`./scripts/test.ps1` runs what CI runs: gofmt check, vet, test, a short fuzz pass.

## 8. Tech choices

- **Go**: fast builds, single static binary, trivial cross-compile.
- **TUI**: bubbletea + lipgloss + charmbracelet/x/ansi (width-aware truncation).
- **jq**: `github.com/itchyny/gojq` in-process.
- **Clipboard**: `atotto/clipboard` (native on Windows; xclip/wl-copy on Linux). OSC 52 for
  remote sessions later (T-007).
- **Config** (when needed): TOML, as in hit.

## 9. Finding things (owner's priority, 2026-10-28)

Navigation and finding come before querying. Most of the time the question is "where is it?",
not "compute this".

**The problem with a flat fuzzy filter.** fzf-style filtering treats every row as a line of
text. In a TeamCity project list, typing `elastic` keeps the rows that mention it and wipes
out the projects nested under them, which are what you came for. Typing `orderid` leaves a
column of `orderId: …` lines with nothing to say which order each belongs to. Structure is
the point of JSON, so finding must keep it.

Four tools, from cheapest to richest. Each one is useful without the others.

### 9.1 Search and jump (V-007, T-011)

- `/` forward, `?` back, `n`/`N` repeat, as in vim. Matches keys and scalar values.
- **Smart-case**, as in hit: an all-lower-case query ignores case; any capital makes it exact.
- **Quoted (V-014).** `/"AE"` matches a whole key or string, written the way the tree shows it:
  not TAKE, not `AEX`, and `"2"` is not the number 2. Bare `ad` is inside every `Trade…` on a FIX
  dictionary. While the closing quote is still to come, `"AE` means *starts with*, so incremental
  search has something to follow. Smartcase and `\c` `\C` apply. Only a leading quote is special.
- Landing on a match inside a closed container opens its parents. The status bar shows
  `match 3/17`, and every visible match is highlighted.
- **Incremental (V-013).** The cursor moves as the pattern is typed, from where the prompt
  opened, with the count beside it. Until enter it is only a preview: moving on closes what the
  last look opened (`Tree.Peek` / `Unpeek`), and esc restores the cursor, the scroll, the open
  nodes and the previous search.
- **Cost.** Measured 2026-10-31 (Ryzen 7950X, order-shaped JSON): a walk of the whole document
  is 22 ms at 10 MB and 110 ms at 50 MB. The first character and backspace pay that. A pattern
  that extends the last one filters its hits instead (`Query.Refines`, `find.Filter`), so the
  cost falls with the hit count: 76 ms from `o` to `ol` at 50 MB, the worst case, where nearly
  every record matches, and next to nothing once the pattern is selective. `match n/m` is a
  binary search. Fine for now; if 50 MB feels slow in daily use, the fix is to debounce.
- **Jump labels** (flash.nvim / leap / hop): press the jump key, type one or two characters,
  each visible match gets a one- or two-letter label, press it to land. This only covers
  what is on screen, and gets there in three or four keys.
- Help moves from `?` to `F1` in the same change (T-012).
- **Fold to matches** (`z/`, V-011): after a search, close everything and open only the
  paths down to each match: search `olivia`, press `z/`, and see Olivia and every user with a
  friend called Olivia, everyone else one closed line. Later the matches can come from a jq
  expression instead of text (V-012).

### 9.2 Structure-aware filter (V-008)

A filter hides rows; it never flattens them. For a query `q`, a node is shown when:

1. it matches `q` (key or value), or
2. it is an **ancestor** of a match, so you can see the path down to it, or
3. it is **inside a matching container**, so a matching project keeps its sub-projects.

The same tree widget draws the result, with matches highlighted, so you can open, close, copy
paths and search inside it as usual. Clearing the filter returns to the full tree with the
cursor on the same node.

**OPEN:** whether rule 3 is always on, or a toggle; and whether "dim the rest" is a useful
second mode. Settle both against the real TeamCity fixture.

### 9.3 Finder list (V-009)

Telescope-style: `f`, type a phrase, and get a list of hits, one per line: the path of the
parent, then the row as the tree draws it, so a column of `_name:` lines still says which record
each belongs to. Good for "show me every place this ID appears".

- **Under the tree, full width,** as vim's quickfix window: paths are long, and the tree needs
  its width too. Its height is fixed (two fifths of the body) so the tree does not jump about
  during typing. On a very short terminal the list hides and the tree still follows.
- **The tree follows the selection,** opening the way to each hit and closing it again when the
  selection moves on, so stepping through 20 hits leaves no trail. Enter keeps the last one open.
- **It is the same search as `/`:** same matching, smartcase included, and enter leaves it as
  the last search, so `n` `N` `z/` carry on through it. That also makes it a quickfix list:
  `F` opens the last search, from `/` or `f`, as a list on the match at the cursor. `/` then `F`
  is how to see a search you already ran. Substring, not fuzzy: §9 says why.
- **No cap on hits.** The list draws only what is on screen, so 100,000 hits cost the same to
  show as 20. The cost of many hits is the search itself, and that is the same with a cap.

### 9.4 Knowing where you are, and lifting it (V-010, Q-006, Q-013, Q-014)

Where the cursor is matters more than the exact path:

- **Generalised path.** On `.orders[3].id` the useful path is usually `.orders[].id`, the
  field across every order. The status bar or info panel shows both.
- **Field facts.** For a field inside an array of objects: how many elements have it, how
  many distinct values, a few samples, min/max/sum for numbers. These are computed on demand
  for the cursor's field only, so they are cheap.
- **Lift into a query.** A key puts the generalised path into the query bar (Q-006).
  Selecting several sibling fields (`id`, `symbol`, `qty`) gives
  `.orders[] | {id, symbol, qty}`, shown as a grid (Q-013). On a number, an aggregate
  (`[.orders[].qty] | add`) is shown at once and can be lifted too (Q-014).

The query that comes out is a **starting point**: it's written into the bar to be read and
edited, which is how jq gets learnt.

## 10. The REPL: get close, then edit (direction, 2026-10-28)

Most real work is a handful of operations: pick some fields, filter to a value, flatten
into a grid, maybe sort or count. A typical goal is "the order rows for this order id, as a
grid of id, symbol, qty". Typing that in jq by hand every time is the friction jqv exists
to remove.

- **Verbs, not hand-typing (R-005).** A few one-key operations, each filled from the cursor:
  *pick fields*, *filter where = this*, *sort by*, *group/count*, *flatten*. Each writes
  plain jq into the query bar and appends to what is already there, so verbs chain:
  `.orders[] | select(.id == "ORD-1002") | {id, symbol: .instrument.symbol, qty}`.
- **The output is a grid when it can be** (V-002): an array of flat-ish objects is shown as
  columns by default.
- **Completion knows the document (R-006, Q-007).** Keys valid at this point in the path,
  values seen for a field inside `select(...)`, and verbs listed beside the builtins.
- **jq stays visible.** Nothing is hidden behind a GUI: verbs only write text, and the text
  is what runs. You learn jq by watching what the verbs produce.

Order of work: the navigation layer first (§9.1, §5), then Q-001/Q-002 (the bar and the
engine), then verbs, starting from the cursor (Q-006, Q-013), then completion.

### 10.1 Two modes, and the filter stack first (owner, 2026-11-05)

The REPL has two kinds of work, and they need different results:

1. **Projection**: the data is reshaped. Flatten into a grid, pick fields by navigating to
   them, add aggregates (Q-013, Q-014, V-002, R-005). The output is new data.
2. **Filter**: nothing is reshaped. A **stack of conditions** narrows the document, and the
   result is still the document, shown in the same tree. It's a structural grep for homing
   in on the nodes that matter.

The filter comes first: it's simpler, needs no output pane, and each layer is already a legal
jq `select`.

**Building a layer from the cursor (Q-015).** On `.orders[3].qty`, a key opens a small prompt
pre-filled with the field; type `> 50`. The layer is the generalised path plus the condition,
written as plain jq: `.orders[] | select(.qty > 50)`. The record is the element of the
nearest array around the cursor (as `(` `)` use, T-022); the condition is on the path from
there to the field.

**What is shown.** Each matching record is shown whole, with the path to it from the root.
Non-matching records are hidden, as in the structure-aware filter (§9.2, V-008), so this is
that filter with jq conditions instead of text. Clearing the stack gives the full tree back
with the cursor where it was.

**The stack.** Layers AND together: `qty > 50`, then on `status` add `== "open"`. The stack is
visible, with each layer in jq, and the top layer can be popped. Because every layer is real jq,
the stack can later be lifted whole into the query bar as one `select(... and ...)`.

**Settled (owner, 2026-11-05).** Keep it simple, and keep it jq: this is fragments of jq glued
together, not a language of our own.

- **Show only what is left.** Filtered-out records are hidden; the status bar counts them.
- **You get what you asked for.** `.qty < 50` then `.qty > 50` leaves nothing, with no warning.
- **jq's meaning, exactly.** A layer is a jq condition on one record, and a record stays when it
  says true (not false or null) at least once. Matching strings is jq's own toolkit:
  `== "EUR"` exact, `contains("EU")` substring, `test("^eur"; "i")` regex with flags,
  `startswith` / `endswith`, `length > 20`, `IN("A"; "B")`, `ascii_downcase`.
- **A missing field is null**, so `.instrument.symbol == "EUR"` quietly drops a record with no
  instrument, as wanted.
- **A layer that fails on a record drops it, and is counted** (`3 records failed: cannot
  index string`). jq itself would stop at the first failure; in a grep, a record the condition
  can't apply to is simply not a match. `?` and `//` are jq's way to say so explicitly, and the
  corpus shows both. Note `//` does **not** catch errors, in jq or gojq: `.a.b? // ""` does.
- **A single `=` is refused**, with the `==` form suggested: in jq it assigns, the result is a
  record, which is true, so every record would pass.
- **One array of records per stack**, chosen by the first layer. Later layers are relative to
  the same record, nested objects included (`.instrument.symbol`); for a nested array use
  jq's `any(.parties[]?; .role == "seller")`.
- **Clear all** in one key brings everything back. **`:w! file.json`** writes what is left (V-016).
- **Every stack is one jq program** (`filter.Lift`): `.trades |= map(select(any((L1)?; .)) | …)`,
  the whole document with only the kept records. The corpus runs every case both ways, through
  gojq and real jq, so the stack never means anything jq wouldn't.

**The corpus** is `testdata/filter/`: `trades.json`, built with a trap per record (no instrument,
lower case, a string where an object should be, a qty that is a string), and `cases.toml`:
layers in, the ids kept and the failures counted out, with a line on why. New cases come from
daily use, as fixtures do (§7.1).

**Later: an IDE for jq.** Most stacks will be one or two simple conditions. The same bench can
grow into writing longer queries with completion and help (Q-007, Q-008, R-006), still plain jq.

## 11. Size: knowing our limits (2026-10-29)

jqv must never do what Notepad++ does with a file you didn't check first: eat memory until it
falls over. It knows its own limit and says so before it gets into trouble.

**Measured** (order-shaped JSON, our parser, 2026-10-29):

| Input | Parse | Nodes | Memory |
|---|---|---|---|
| 10 MB | 0.1 s | 0.8 M | 77 MB |
| 50 MB | 0.6 s | 4 M | 350 MB |
| 200 MB | 2.2 s | 16 M | 1.4 GB |

About 11 ms and 7 MB of memory per MB of JSON. Queries will add more (gojq needs `Value()`,
another copy).

**The rule (C-010, done).** A document is loaded whole, up to `--max-size` (default 100 MB, so
about 1 s and 700 MB). A file over the limit is refused before reading a byte; piped input
stops at the limit. Either way the message gives the size, the limit, and what to do. The
full-featured tree stays simple because it never has to cope with a document it can't hold.

**Big files are usually many small documents.** The real large cases are logs: one JSON message
per line, gigabytes in total, as with FlexTrade's drop-copy feed of gRPC messages logged as
JSON. Each message is small; the problem is finding the few you want. So the answer for big
input is not a lazy tree over one giant value but a **lines mode** (C-011):

- Scan once and index the byte offset of each line (fast, little memory).
- Run jq over each line as a stream, never holding the whole file. Matching lines become a
  normal in-memory document (an array), with the whole tree available again.
- Show the first N lines straight away as a preview while the scan runs.

**Following a tree of ids (R-007).** In that feed, a child order names its parent but not the
root, so "every message for this parent order" needs the transitive closure of ids. It is one
streaming pass in jq, since a log has parents before children:

```
jq -c -n --arg root P1 'foreach inputs as $m ({ids: {($root): true}};
    if .ids[$m.orderId] or .ids[$m.parentId // ""] then .ids[$m.orderId] = true | .out = $m
    else .out = null end;
    .out // empty)' feed.jsonl
```

Only the id set is held. This replaced a Python pre-processor plus `grep -E`. In jqv it becomes
a verb: on an id, "follow the tree from here", asking which fields are the id and the parent
(guessed from the shape) and running the pass above.

**One giant value** (a 1 GB single array) is the case we do not chase. OPEN, only if real use
asks: stream the top-level array's elements (`json.Decoder` can) as if they were lines.

## 12. Other formats: translate in, stay JSON (2026-10-29)

jqv is a JSON tool. But the owner's docstore of config is a mix of JSON, YAML and XML, and the
same questions apply to all of it: where is this setting, what's under this node, which
environments differ. So other formats are **translated on load** into the same `doc` tree
(C-012), and from then on nothing knows the difference: tree, search, `z/`, copy path,
queries.

- **One-way.** We read YAML and XML; we never write them back. Output is JSON.
- **Key order kept.** The translators build `doc.Node`s directly (YAML through yaml.v3's node
  tree, XML through `encoding/xml` tokens), so they keep document order, as the JSON parser does.
- **XML has no single right answer.** Attributes, text beside child elements, and repeated
  elements (one `<add>` is an object, two are an array) all need a convention. **OPEN:** follow
  an existing tool's mapping, most likely yq's, so a query written for one works in the other.
  Document the mapping with fixtures, one per awkward case.
- **Say what happened.** The header shows the source format, and a path copied from a
  translated document is a path in the *JSON*, not an XPath.
- Note: .NET's `appsettings.json` is already JSON. The XML is `web.config` / `app.config`,
  `.csproj` and friends.

**A conversion pipeline, not a second tool** (owner, 2026-11-07). jqv stays a JSON tool:
each translator is a front door into the one tree, and `--print-json` (C-015) is the back door
out, so a YAML file can go on into plain jq or sql-cli. Once a format is in, everything works
on it unchanged, including the REPL: `jqv repl manual.yml` runs jq on jq's own manual.

**YAML (C-013, done 2026-11-07).** yaml.v3's node tree is walked into `doc.Node`s
(`doc.ParseYAML`); fixtures in `testdata/yaml/`, one per awkward case, with the JSON each gives.

- Picked by extension (`.yml`, `.yaml`); stdin is JSON. Content sniffing waits for C-012.
- YAML 1.2 types, as yaml.v3 resolves them: `3` a number, `true` a boolean, `~` and an
  empty value null, `yes` a **string**. `0x1F`, `0o17` and `1_000` become JSON numbers;
  decimal numbers keep their digits, so big integers keep their precision.
- What JSON can't hold stays a string as written: timestamps (`2026-11-07`), `.inf`, `.nan`.
- Anchors and aliases are expanded; merge keys (`<<: *base`, or a list) put the merged keys
  where the `<<` stands, less those the mapping sets itself. Expansion is capped (20M values)
  against alias bombs.
- Several documents (`---`) become an array; empty ones, as after a trailing `---`, are
  left out.
- Keys are strings: `1:` is `"1"`. A list or mapping as a key is an error.
- The title says it: `ci.yml · yaml→json`.

## 13. Options and config (agreed 2026-11-05)

Some behaviour is a matter of taste that should survive a restart: row numbers on, compact
rows on. vim settles this with one idea, used here too: **an option is a `:set` name, and the
config file is a list of `:set`s.** There is one table of options, so anything in the config
can be changed for the session with `:set`, and anything `:set` can change can go in the config.

**The table (T-025).** `internal/tui/options.go` lists each option once: long name, short
alias, type (bool or int), default, one line of help. `:set` grows to vim's forms over it:
`nu`, `nonu`, `nu!`, `compactwidth=60`, `nu?` (show the value), and `:set` alone (what differs
from the defaults). F1 lists the options from the same table, as it lists keys.

**The file (C-016).** `config.toml` in the user config directory, as hit does:
`%APPDATA%\jqv\config.toml` on Windows, `~/.config/jqv/config.toml` elsewhere
(`os.UserConfigDir`). `JQV_CONFIG` points somewhere else, which is how tests stay away from the
real file. A missing file is not a problem: most people have none.

```toml
[set]
number       = true
compact      = true   # small objects of scalars on one row (§14)
compactwidth = 60
```

- Keys are the long names; the short aliases are for typing, not for files.
- **A bad config never stops jqv**, as in hit: an unknown option or wrong type is skipped and
  reported on the status line at start-up (`config: unknown option "numbr"`), and the rest
  is used. A typo that silently did nothing would look like a setting that doesn't work.
- Order of precedence: defaults, then the file, then `:set` during the session. `:set` is
  never written back; the file is edited by hand, as a vimrc is.
- `[repl]` is the REPL's (§18): `pipe = "bat -l json"`, what a bare `:!` runs; `lines = 60`,
  where a line's output is cut (0 shows all). The REPL reports
  its problems under its banner and leaves `[set]` to the viewer.
- Later tables, not now: `[keys]` (T-005), a theme (T-008).

**Built (2026-11-05):** `number`/`nu`, `relativenumber`/`rnu`, `scrolllock` (the same switch
as `zl`) and `depth` (how far the document opens at the start; `:set depth=3` reopens it, as
`z3` does). `compact` and `compactwidth` arrive with V-015. Others join when something earns one.

- `:set nonu` also turns off `rnu`, so it always clears the gutter. That was agreed with T-014
  and is kept, though vim leaves `rnu` on.
- `-d` on the command line beats the config's `depth`.
- `--print` does not read the config, so its output doesn't depend on whose machine it runs on.
- F1 lists the options and the path the config is read from.

## 14. Compact rows (V-015, agreed 2026-11-05)

The FIX dictionary is mostly arrays of tiny objects: `{"_name": "BeginString", "_required": "Y"}`
costs three rows when open and says nothing when closed (`{2}`). A compact row shows it whole:

```
▾ field [27]
│ ▸ [0] {_name: "BeginString", _required: "Y"}
│ ▸ [1] {_name: "BodyLength", _required: "Y"}
```

**A compact row is just a closed container with its contents shown inline.** Nothing new in the
tree's state: open and closed work as now, and only the way a closed row is drawn changes.

- **Small** means: every child is a scalar, and the inline text fits in `compactwidth`
  characters (default 60). A fixed width rather than the terminal's, so the same rows are
  compact whatever the window size, and tests can pin them. A row wider than the screen is cut
  with `…` as any row is.
- **Arrays count too**: `tags: ["a", "b"]`.
- **With `compact` on**, the bulk opens (`zR`, `zo`, `L`, `z1`–`z9`) leave small containers
  closed, so they show inline. Off, they open as today, and closed rows show `{2}`.
- **Expand one** with `l` / enter as usual (it opens to rows), and close it back with `h`.
  That's the "lift it out for a moment" case, with no new keys.
- **Toggle a whole region**: `zi` on a node flips every small container under it between
  inline and open; at the root, the whole document. (vim's `zi` toggles folding as a whole.)
  `:set compact` / `nocompact` does the same for everything and sets the mode for bulk opens.
- **Search** opens a compact row to land on the field inside it, as it opens any closed
  parent. (Rejected: highlighting the match inside the inline row and landing on the row.)
- A closed **large** container stays `{27}`. Previewing those too (`{id: "ORD-1001", …}`) is
  V-006, a separate choice made later by use.

## 15. Where this is going: a pipeline bench (owner, 2026-11-05)

Many iterations from now, jqv is an IDE for jq pipelines. The model is SQL written as a chain of
`WITH a AS (…), b AS (SELECT … FROM a WHERE …)`: a pipeline of small intermediate tables, each
built from the ones before, with the final result at the end and **any stage open to look at**.
The purist's objection (it's inefficient) rarely matters in practice; what matters is that
each piece can be seen, trusted and reused.

How you would work:

1. **Build a stage from fragments**: a filter stack (§10.1), a projection (Q-013), a verb
   (R-005), with the result in front of you as you go.
2. **Lock it in** when you're happy: it becomes a **named stage**, saved as a recipe.
3. **Glue stages together** into a pipeline, and move around it: run up to any stage, see its
   output, change one stage and watch everything after it follow (Q-004).
4. **Reuse**: a saved stage is a component for the next pipeline, with grammar help,
   completion and suggested recipes as you write (Q-005, Q-007, Q-008, R-006).

**It stays plain jq.** jq already has every piece this needs:

| The idea | jq |
|---|---|
| a named stage (a CTE) | `def open_eur: .trades \|= map(select(…));` |
| the pipeline | `open_eur \| by_account \| totals` |
| a value kept for later stages | `. as $all \| …` |
| a library of saved stages | a module: `import "jqv/trading" as t;` then `t::open_eur` |

So a recipe is a `def` in a module file, a pipeline is a jq program, and anything jqv saves runs
with plain jq outside it. jqv's job is the understanding: knowing where the stages are, what each
one gives, and how they fit together. It does not invent a language.

Each step on the way should be useful on its own, which is why the filter stack comes first.
The wishlist items on this path: Q-015 → Q-004 → Q-016 → R-008, with Q-005/Q-007/Q-008/R-006
for the help.

## 16. The expression reader (proposal, 2026-11-05)

The REPL starts small: **the value under the cursor is `.`**, and you write jq against it and
see the answer as you type. On `"  hello  "`, `trim` shows `"hello"`, `trim | length` shows
`5`, and `.[:5]` shows `"  hel"`, which makes it obvious that the trim should have come first.
Everything later (filter layers, projections, stages) is this same line with more context.
It's also how jqv teaches jq: you try a function on a real value and see what it does.

**First use: filtering.** The first job of the reader is building filter layers (§10.1): all the
ways to isolate records. So the first catalogue is the functions that **test** a value (give
true or false) and the ones that **shape** a value so it can be tested.

| On a | Tests | Shapers to test after |
|---|---|---|
| string | `== "x"`, `contains`, `test(re; flags)`, `startswith`, `endswith`, `IN(…)` | `trim`, `ascii_downcase`, `length`, `.[:3]`, `split(",")`, `tonumber` |
| number | `> 50`, `>= 50`, `== 50`, `IN(…)` | `floor`, `fabs`, `tostring` |
| array | `length > 0`, `any(.[]; …)`, `all(.[]; …)`, `index(x)`, `contains([x])` | `length`, `first`, `last`, `map(…)`, `sort` |
| object | `has("k")`, `.k == …` | `keys`, `length`, `to_entries` |
| null / missing | `== null`, `!= null` | `// "default"`, `?` |

**Completion by example, not by inference.** To know what fits after `trim |`, jqv doesn't
work out types from a grammar: it runs the expression up to the `|` on the real value, sees a
string came out, and offers string functions. The answer is always right for this document,
and it costs one evaluation of a short expression on one value. In a filter layer, it runs on
a sample of records instead of one, and offers what fits the types that actually came out.

**The catalogue is data** (Q-008): per function, the input kinds, what it gives, a one-line
explanation, and an example with its result. Names and arity are checked against gojq's own
`builtins`, and every example is run through gojq and real jq in a test, as the filter corpus
is, so the help never lies and the jq/gojq differences show up. The help panel shows each
example **applied to the value under the cursor**: `trim → "hello"`, not just what `trim`
does in general.

**Errors teach** (§2, principle 8): `trim` on a number fails with jq's message plus a hint
(`tostring | trim`); an empty result says which step stopped producing values.

**In a filter layer** the reader has a record around it: the line starts at the field, as
`.instrument.symbol | `, with the caret after it. As you type it shows the result on this record,
plus `kept 12 of 40` across all of them. The jq program the whole stack makes (`filter.Lift`) is
shown under it, so you see real jq as you learn.

**Order of work**, each step useful on its own:

1. **The reader on the cursor** (Q-017): a key opens a line where `.` is the cursor's value;
   the result shows as you type; esc leaves. Proposed key `!`, after vim's "filter this
   through".
2. **The catalogue** (Q-008) for strings and numbers, tests and shapers first, with its
   checked examples.
3. **Completion by example** (Q-018): after `|`, a list of what fits, with each item applied to
   the current value.
4. **Filter layers** (Q-015 TUI): the reader with a record and a kept count, pushed onto the
   stack with enter.

**OPEN:** whether `!` suits the reader; how many records to sample when completing in a
filter layer.

## 17. Lessons: jq worked out on the page (Q-019, 2026-11-06)

The owner wants to learn jq properly while building the reader (§16), and to know exactly what
it should do before building it. Rather than Jupyter (a Python kernel, notebooks that diff
badly, and nothing on a work machine without a laptop), the lessons are **markdown in `learn/`,
checked by `go test`**: an input block, a `jq` block, and the output block jq gives. They read
on GitHub like a notebook, and the test runs every query through gojq and, if on the PATH, real
jq, so a page never drifts from what jq does. It's the filter corpus's idea (§10.1) in a form
you can read.

- `./scripts/learn.ps1` writes each query's output in under it (from real jq when present, so
  key order is jq's). `-Check` doesn't write: it fails where a block says something else, so you
  can write the answer you expect first and see where you were wrong.
- Errors are checked as failing only, since gojq and jq word them differently.
- Each lesson ends with **In jqv**: what the page means for the reader. Questions found there go
  to §16 or §10.1. The first: a filter layer works on one array, but `.[].people[]` reaches
  records spread over several (lesson 3).
- `examples/jq` (R-007) stays as it is: whole real-world programs, one directory each. Lessons
  are short steps with prose around them.
- Later the lessons can feed the catalogue (Q-008), and jqv could open one with its input
  loaded (`jqv learn/02-strings.md`), but neither is built.

## 18. The scratchpad REPL on the command line (Q-020, owner 2026-11-07)

The reader (§16) and the bench (§15) need an engine before they need a screen. `jqv repl
file.json` is that engine's first front end: a plain line REPL in the terminal, used daily
to find out what the reader should do. The TUI's reader (Q-017) is its second front end, over
the same package (`internal/bench`), so nothing here is thrown away.

**Scratch by default: hack, push, hack.** A line is jq run against the current `.`, and its
outputs are printed. Nothing is kept: try `.[1:]`, `.[1:][]`, `.[1:][].people[0]`, fire and
forget. When a line is right, `:push` makes it a **stage**, and from then on `.` is what the
stages give. `:pop` takes the last one back. The stages joined with `|` are one jq program
(`:program` prints it), so what you built runs in plain jq.

```
. ❯ .[1:]
[{"team":"beta",...}]
. ❯ :push
.[1:] ❯ .[].people[0].name
"Cat"
```

- **jq's meaning, exactly.** A line runs as `stages | line` on the whole document, so a stage
  with several outputs feeds each to the line, as `|` does in jq.
- **Values with no input just work.** `[range(1; 11)]` ignores `.`, so there is no `-n` mode.
- **`$root`** is always the whole document, whatever the stages: `.[] | select(.id == $root.x)`.
  `:program` starts with `. as $root |` when a stage uses it, so it still runs in plain jq.
- **`$_`** is the last line's result (an array when it gave several), for scratch work like
  `$_ | length`. A stage can't use it, since it changes with every line.
- **Bounded.** A line runs with a timeout (5 s), and only the first 50 outputs are printed, with
  the count of the rest, so `range(1e9)` or `repeat(1)` can't hang the session.
  What a line shows also stops after 60 lines (Q-039, `[repl] lines`, 0 for all), with
  `… N more lines (M in all): :pp shows it all, :! pages it`, so `.` on a big document doesn't
  scroll 26,000 lines past. Only the display is cut: `$_`, `:pp`, `:!` and `:wq` see the whole
  value, and `:pp` is the way to ask for all of it. 60 so the 50 one-line outputs above fit.
- **Errors teach** (§16): jq's message plus a hint where a common mistake is behind it, such as
  a field asked of an array ("after a slice, add `[]`", lesson 4). A parse error points at the
  place with `^`, and says what's behind it when the line shows it: `. age` is `.` and then
  a stray word, so the hint is "a field is .age, with no space after the dot". `sub("\(.*"; "")` meant a
  literal paren, but `\(` in a string starts an interpolation, so the hint says to write `\\(`.
  `[range(6)] as $i | map(. + 2)` fails on the document, because `as` names a value and
  leaves `.` as it was; when a line with `X as $v | body` fails (and the body doesn't start
  from `$v`), the hint says so with the line's own X and body rewritten:
  `[range(6)] | map(. + 2)`, `$v` in the body becoming `.`. A line that gives only nulls says so and lists the keys of `.` (the first
  20 and the count, Q-042), since a misspelt field (`.nam`) is null in jq, not an error.
  With stages pushed, a line that fails or gives only nulls but works on `$root` was written
  for the whole document: the hint says what `.` is now and gives `$root | line`, or `:pop`
  (Q-041), instead of "check the spelling".
- Output is compact when it fits on a line of 100, pretty otherwise. Keys come out sorted by
  gojq (Q-010 stays open).
- On a terminal the line is jqv's own editor (below). Piped lines, and `jqv repl --plain`,
  are read whole after enter, with the console's own editing.

**Commands** start with `:` (a line of jq never does): `:push [expr]` (`:p`), `:pop`,
`:reset` (drop every stage), `:stages`, `:program`, `:tree [expr]`, `:pp [expr]`, `:! cmd`,
`:scope [expr]` (§22), `:cls` (`:clear`), `:help [name]`, `:try N [.]`, `:wq [expr]` (`:x`), `:q`.

- **`:wq`** (Q-030, done 2026-11-08) quits and hands back what was built: the stages, then
  the last line run since the last `:push` (what `:push` would add) or the expr given, as a
  shell command, `jq '.[] | .people[] | .name' people.json`. Quoted for pwsh on Windows and
  sh elsewhere; a YAML file is `jqv --print-json ci.yml | jq '…'`. It is printed, copied to
  the clipboard when the lines are typed, and with `--emit FILE` written there too. A
  program can't type into its shell's prompt, so the README has a pwsh function that runs
  the REPL with `--emit` and puts the command in PSReadLine's history: quit, press up, run.
  A line that uses `$_` or doesn't parse isn't handed back; the session stays.

- **`:tree`** (Q-005, with Q-004's "output after each piece"): gojq's parse tree, laid out
  with what each step gives on the document. `.[].people[]` is one term with three steps
  (`.[]`, `.people`, `[]`); `a | b` is parts, each run with the parts before it. The first step
  that fails shows the error and its hint, and the steps after it say `not reached`, so the
  tree says where a query breaks. A binding (`. as $x | …`) stays whole. Later: open
  `select(…)`, `map(…)` and operators to show their insides.
- **`:structure [N] [expr]`** (`:st`, Q-040): the shape of `.`, or of expr's outputs, as an
  outline (§21). `N` limits the depth.
- **Output.** Compact when it fits, as before. `:pp` prints the last result (or an
  expression's) as jq does, indented. `:! cmd args` pipes it, indented, into a program:
  `:! bat -l json`, `:! clip`. The command is split on spaces, with no shell, and runs only on
  `:!`, never as you type.
- **`:! ` with no command** runs config.toml's `[repl] pipe` (§13), so the program you always
  reach for (`bat -l json`) is one keystroke: `:!`. A command given still wins (`:! clip`).
- **`:cls`** clears the screen; the stages, `$_` and the console's history stay. (`:clear`
  used to drop the stages; that is `:reset` now, since everyone expects `clear` to clear.)

- **`:help name`** (Q-021, done 2026-11-07) is jq's own manual: `internal/catalog` embeds
  `manual.yml` from jqlang/jq at tag jq-1.8.2 (CC BY 3.0, `internal/catalog/NOTICE`) and
  parses it on the first `:help`. To move to a new jq, replace the file and run the tests.
  - A name is what a title's code spans say: `map(f)` is `map`, and operators as written
    (`:help |`, `:help ?//`, `:help +=`). A title or section works too (`:help math`). Anything
    else is searched for in titles and text, and the hits are listed with what to type.
    `:help jq` lists every section with its names; `:help functions` lists every function.
  - Entries whose titles name nothing (Math, Dates, Convert to/from JSON, SQL-Style
    Operators, Format strings) answer to the names their text puts in backticks and gojq
    defines: `:help sin`, `:help strftime`, `:help tojson`, `:help INDEX`, and `:help @csv`.
  - **jqv's notes** (Q-044, owner 2026-11-12): the manual lists the math functions with no
    examples, so `:help remainder` only found the Math section, and nothing pointed to `%`.
    `internal/catalog/jqv.yml` holds jqv's own entries in the manual's YAML shape, parsed
    after it, so a function they cover gets their entry (`:help pow`, `:help remainder`,
    `:help modf`), and one the manual has keeps its own first (`:help floor` gives the
    manual's, then the notes'). Entries can carry `aliases`, words they are found by that
    aren't jq names and are never offered by Tab (`:help modulo`, `:help sum`). The examples
    run in the same tests as the manual's, under gojq and real jq. The section is titled
    "(jqv's notes)", and `:try` says "as jqv's notes say", so nothing passes as jq's text.
  - **A call with the wrong number of arguments** (Q-043) gets a hint from gojq's
    `builtins` and the titles' forms: `remainder takes 2 arguments, not 1: remainder(x; y).
    It ignores what comes in; to use it, pass . as an argument: remainder(.; 10)`. One that
    works on `.` says to pipe: `sqrt(16)` → `16 | sqrt`. A name gojq doesn't define gets no
    hint yet (spelling suggestions later); nor does a def of the line's own.
  - The entry's text is shown as the manual has it, then its examples numbered:
    `[1] trim, ltrim, rtrim  on " abc "  →  "abc", "abc ", " abc"`.
  - **`:try N`** runs example N on its own input and says whether it gave what the manual
    says. **`:try N .`** runs it as a line on the current `.` instead. Either way it is then
    the last line, so `:push` keeps it: read, try on your data, keep.
  - **It can't lie.** The test runs all 250 examples under gojq and real jq (on the PATH, or
    `$JQ`). Real jq 1.8.2 agrees with every one. gojq differs on 8 (`have_decnum`,
    `$__loc__`, regex flags `x` and `n`, `match`'s array form), listed in `catalog.Differs`
    with why, and shown under the example. Functions the manual has and gojq lacks
    (`keys_unsorted`, `input_filename`, `stderr`, …) are in `catalog.NotInGojq`, shown on
    the entry. Both lists are checked both ways, so a gojq upgrade that closes one shows up.
  - The manual is also just a document: `jqv repl internal/catalog/manual.yml` (§12) runs jq
    on it, and `[.sections[].entries[]?.examples[]?] | length` is 250.

**Tab completion (Q-036, proposal, owner 2026-11-10).** The catalogue behind `:help` already
knows every name jq has, so it's the first source for completion. Tab completes the word
before the cursor; it starts in `:help` and grows into the jq line itself.

- **Where.** `:help comb<Tab>` → `:help combinations`. On a line of jq, `sel<Tab>` →
  `select(`. The lexer (§19) says what the word is, so nothing is offered inside a string or
  a comment, and `.sel<Tab>` (a field) is left for keys (Q-007, R-006).
- **One match** completes. A function that takes arguments gets its `(`, with the cursor
  inside; one that doesn't (`length`, `keys`) doesn't.
- **Several matches** cycle, as PowerShell's Tab does on Windows (PSReadLine
  `TabCompleteNext`), which the owner uses daily: each Tab puts the next candidate in place,
  `shift+tab` goes back, and esc puts back what was typed. The candidates show under the line
  (`3 of 5: combinations · combinations(n) · …`).
- **A hint as it lands**: the signature from the manual's title and the first line of its
  text, on the line under the editor (`combinations, combinations(n): all combinations of the
  arrays' elements`). F1 on the word still opens the whole entry.
- **What's offered, in order of building:** `:` commands and `:help` names; jq's functions
  and keywords (`if`, `reduce`, `as`, `def`, `try`, …) from the catalogue and gojq's builtins;
  then keys at this point in the document (Q-007, R-006); then functions that fit what the line
  so far gives (Q-018); then recipes (R-009), which insert a skeleton:
  `red<Tab>` offers `reduce` and `reduce .[] as $x (0; . + $x)`.
- **Tested as the rest is:** a completer is a pure function of the line and the cursor,
  giving the candidates; the editor's handler is driven with key messages, no terminal.

**OPEN:** whether Tab on an empty line does anything (list the commands?); how many candidates
show under the line before it says `… and 12 more`.

**First step done (2026-11-11).** The editor has a `Completer`: the line and cursor in, a
span to replace and `Candidate`s out (an insert, how it's listed, a hint, and how far back
from its end the cursor lands, for skeletons later). The editor does the cycling and knows no
jq: Tab puts the next candidate in place, shift+tab the one before, esc puts back what was
typed, and any other key keeps the one shown. A single candidate goes straight in, with its
hint under the line until the next key. While cycling, the candidates show under the line,
the one in place reversed, as many as fit the width around it (`… 12 more`), then its hint,
dim. `assist.Complete` offers:
- `:` commands, read from `:help`'s own text, so the two can't drift apart;
- after `:help`, the catalogue's names and sections, any case;
- on a jq line, or in the argument of `:push`, `:pp`, `:tree`, `:lex` and `:fmt`: gojq's
  functions (`sel` → `select(`, `len` → `length`; a function gets its `(` when it can't be
  called bare), keywords, the defs written in the line (`def older(n): …; old` → `older(`),
  and `$ENV`, `$root`, `$_` and the line's own `$variables`. The exact name comes first,
  then alphabetical order. The hint is the manual's title and the first sentence of its
  text. Inside a string, a comment, a `.field` or a module's `a::b`, nothing is offered.

Answers to the OPEN questions, for now: Tab on an empty line or after a space offers nothing,
and the list is as wide as the terminal. The context's defs (§20, Q-037) are the next source,
and keys from what `.` is at the cursor (§22, Q-007) the one after.

**The line editor (Q-025, core done 2026-11-08).** jqv reads the input line itself (inline
Bubble Tea: output scrolls above as before, the line at the bottom is ours), which is what
every key below needs. Piped scripts keep the plain line reader, so the session goldens
don't change, and `--plain` keeps it on a terminal too. The same editor is the TUI's reader
(Q-017).

- **Done: the core** (`internal/editor`, no jq in it): a `Buffer` (runes and a cursor),
  `History` (up/down, no blanks or repeats, the line being typed kept while walking), a
  `Keymap` from Bubble Tea key names to actions, and a `Model` that is a Bubble Tea model.
  `Reader.ReadLine` runs one inline program per line, so output printed between lines
  scrolls as it did. A finished line's view ends in a newline, because Bubble Tea clears
  the line it leaves the cursor on.
- **Keys now**: history on up/down; `ctrl+a`/`ctrl+e`, `home`/`end`; `ctrl+left`/
  `ctrl+right` by word (PSReadLine's; `alt+b`/`alt+f` would hit Zellij's floating panes);
  `ctrl+w`, `ctrl+k`, `ctrl+u`, `ctrl+delete` cut, `ctrl+y` puts the cut back; `esc` empties
  the line, `ctrl+c` abandons it (`^C`, a new prompt, not quit), `ctrl+d` on an empty line
  quits; `ctrl+l` clears; a paste is one line, its line breaks made spaces, until Q-026.
  A word, for moves and cuts, is letters, digits and `_`: `ctrl+w` after `.people[].name`
  leaves `.people[].`.
- **Hosts plug in by action**: a host binds a key to an action of its own and gives a
  `Handler`, which gets the line and the cursor and returns text to print above; the line
  stays. The REPL binds `F1` to `:help` on the word under the cursor (`editor.WordAt` with
  jq's word: letters, digits, `_`, `@`, `$`, so `@csv` and `$ENV` work), or `:help` on none.
  The examples it lists are then `:try`'s.
- **Or by submitting a line of their own**: a `Submitter` gets the line and cursor and gives
  a line to run instead, for what needs the terminal to itself (a handler prints while the
  editor draws). The editor finishes with that line, shown as what ran, not added to
  history, and the line being typed comes back at the next prompt, cursor and all. The REPL
  binds `F2` to a bare `:!`: the last result into `[repl] pipe` (`bat -l json`).
- **History is kept across sessions** (R-001, 2026-11-08) in a file jqv owns, in hit's
  format with hit's own packages (`github.com/TimelordUK/hit/history`, hit's C-044):
  append-only JSONL at `%LOCALAPPDATA%\jqv\history.jsonl` (`$XDG_STATE_HOME/jqv` or
  `~/.local/state/jqv` elsewhere, `JQV_HISTORY` to move it). `internal/history` adapts it.
  - Every line typed is a `cmd` record (`cmd` the line, `cwd` the document's full path,
    `sh` `"jqv"`, a session id) with its `end` (`exit` 0, or 1 for jq that didn't parse or
    run, and the time taken). Nothing typed is lost, broken lines included.
  - Up-arrow recalls up to 1000 lines: the ones that ran, each once, newest first, this
    document's before the others. A broken line from an earlier session doesn't come back
    on up; the current session's do, as typed, so a mistake can be fixed.
  - Only lines typed into the editor are kept; piped scripts, tests and `--plain` neither
    read nor write it. A history that can't be written is said once, never fatal.
  - Not a per-document file: queries travel between documents (`to_entries`, `paths`), and
    the `cwd` field gives "this document's first" without splitting them. The finished
    pipeline also lands in the shell's own history (and hit's) when `:wq`'s command is run.
  - **Up with text on the line searches** (R-011, owner 2026-11-12, as
    zsh-history-substring-search): only the entries containing it, newest first, each press
    one older; down past the newest gives back what was typed. The note under the line says
    where the walk is, `history 1/4 matching "def"` (`history 3/120` on an empty line, `no
    history matching "zzz"`). Changing a recalled line and pressing up searches again on the
    changed text. Substring rather than PSReadLine's prefix, so `def` finds `… | def`.
    Case-sensitive, as jq is. Later: the match highlighted in the line.
  - Next: `ctrl+r`, a search over it ranked by hit's `search` (match × recency × frequency,
    scoped to this document or all), and `:save name` for the ones worth keeping (R-002);
    the enhanced TUI reads the same file.
- A handler can also move the cursor, which is how stepping (below) works.
- Typed keys never put control characters in the line. On Windows, Bubble Tea sends a
  modifier pressed alone (ctrl, before ctrl+up) as a typed NUL; it went into the line
  unseen and broke stepping (found 2026-11-08, `TestBareModifierTypesNothing`).
- `jqv repl --show-keys` prints each key the editor gets and what it is bound to, quoted
  when unprintable, and any sequence Bubble Tea doesn't know: the way to see what Zellij or
  a terminal actually sends before deciding a key doesn't work there.

**Run to the cursor (Q-029, done 2026-11-08, owner from daily use).** A long line that fails
(`.[].people[] | select(.age > 30) | reduce .[] as $item (0; . + $item)`: cannot add number
and string) is looked at stage by stage without retyping it:

- **`F5`** runs the line up to the cursor and prints it above, `▸` and the program run, then
  its output; the line stays. "Up to the cursor" is the text before it, on to the end of the
  part the cursor is in (the next `|`, `,` or `;` at its level, or the bracket it is in
  closing), with whatever is still open closed: on `select` in `[.[].people[] |
  select(.age > 30) | .age] | add` it runs `[.[].people[] | select(.age > 30)]`. On
  `if … then .b` it adds ` end`; in a string's `\(…)` it closes both.
- **`ctrl+up` / `ctrl+down`** step the cursor back and on to the end of each stage (before
  each `|`, at any depth, and before the bracket that ends a bracketed pipeline) and run
  there. With multi-line entries (Q-026), one stage a line, these are just up and down.
- It runs as a line does, so `:push` keeps the part run, and `$_` is its result. In `:push`,
  `:pp`, `:tree` and `:lex`, the argument is the jq; other commands say F5 runs jq.
- `jqsyntax` finds this structure itself (gojq's AST has no positions; since Q-031, from its
  own lexer, §19): strings and their
  `\(…)`, comments, brackets, `if`/`end`, `|` but not `|=`, and `as $x |`, which is a
  binding's body and not a stage. A cut that doesn't parse (inside `reduce`'s source) runs
  the bracket around it instead; stops that run nothing new are skipped. Fuzzed for panics;
  stepping along a corpus of real queries must always give gojq something it parses.

- **Keys**, the shell's where there is one, and clear of Zellij/tmux/Windows Terminal (§5):
  `ctrl+l` clears (as bash, zsh and PSReadLine), `ctrl+r` searches history (R-001, persisted),
  `F1` on a word gives its help (PSReadLine's F1 does this for a command). Not `ctrl+h`:
  terminals send it as backspace, and it is Zellij's move mode.
- **Help as you type**: on a name the catalogue has, a dim line under the input,
  `trim, ltrim, rtrim: trims leading and trailing whitespace · F1`. `F1` prints the whole
  `:help` above and keeps the line being typed.
- **Tab expands by example** (R-006, Q-018): `.[].people[].` then tab runs `.[].people[]` on the
  document and offers the keys that actually came out (`name`, `age`); after `|`, the functions
  that fit what came out. The AST (`:tree`) says where the expression stops, so the part
  before the cursor is what's run.
- **Several lines** (Q-026): an unfinished entry (an open bracket, a trailing `|`) goes on
  to the next line, as Python's REPL and pwsh's backtick do; `shift+enter` breaks the line on
  purpose (PSReadLine's AddLine). Enter runs the whole entry; jq doesn't care about the line
  breaks, and history keeps the entry as typed. **OPEN:** a key to join an entry back onto one
  line. `ctrl+m` and `ctrl+j` are enter and newline in a terminal, and `alt+hjkl` are Zellij's
  focus keys, so it may be a command (`:join`), or automatic for `:push` and `:program`.
- **Colour as you type** (Q-027; the plan is §19). The line is redrawn on every key, so each token can be
  styled. gojq keeps its lexer private and its AST has no positions, so jqv gets a small jq
  tokenizer of its own (strings with `\(…)`, numbers, `.field`, `$var`, keywords, `@format`,
  comments, operators), fuzzed against gojq so the two agree on what parses. Beyond syntax,
  colour can say what jqv knows: a function the catalogue has, a name gojq doesn't define
  (red: `keys_unsorted`), a field that isn't there on the document (by example, as tab
  is), the rest of the line after a parse error underlined, matching brackets. bubbles'
  `textarea` draws its text in one style, and bubbline may or may not take a highlighter:
  check before the spike. It likely tips the choice to a small editor model of our own, on
  Bubble Tea, taking ideas from `textarea`.

**Built in pieces, not one editor (proposal, owner 2026-11-07).** This will take a while to get
right, and the REPL is the place to do it, so the TUI gets it later whole. So the editor is
a core that knows nothing about jq, and everything jq-ish plugs into it:

| Piece | Package (proposed) | Knows |
|---|---|---|
| Core: text, cursor, several lines, history, keymap as data (T-005) | `internal/editor` | keys and text, no jq |
| Tokenizer: the line's structure with positions (started: `UpTo`, `Stops`) | `internal/jqsyntax` | jq's lexical grammar |
| Providers, each a small interface the core calls (started: F1, F5, stepping, as Handlers) | `internal/assist` | jq, the catalogue, the document |
| Front ends: the REPL's inline program, the TUI's reader | `cmd/jqv`, `internal/tui` | the screen |

- **Providers**, each optional, each tested alone:
  - **Highlighter**: tokens to styles (Q-027).
  - **Hinter**: the dim line under the input (`:help`'s first sentence).
  - **Completer**: tab, by example (R-006, Q-018).
  - **Checker**: a live parse error, for the underline.
  - **Expander**: snippets with placeholders, which is how recipes (R-002) come in: `group by X
    and count` inserts `group_by(▮X) | map({key: .[0].▮X, count: length})` and tab moves
    between the placeholders; in the TUI, R-003 fills them from the cursor. Q-028.
  - A provider gets the line, the cursor and a way to run jq on the current `.`
    (`bench.Session`). It never gets the screen.
- The core is a Bubble Tea model (`Update`, `View`), so both front ends host the same thing
  and it is tested as the TUI is: key messages in, model out (§7), no terminal.
- Keys come from a table (T-005), so F1, `ctrl+l`, tab and the rest are bindings to
  actions, and config.toml can change them later.
- **Order**, each step useful alone: the core with history and the plain keys (Q-025); the
  Hinter and F1, since the catalogue exists; the tokenizer and the Highlighter (Q-027); the
  Completer (R-006); the Expander and recipes (Q-028).

Planned: filter layers (Q-015) as `:where`, live results as you type, error hints that point at
`:help` (Q-023), and the help applied to the cursor's value in the TUI (Q-008).

**Tested like the TUI**: `bench.Session` takes lines and gives text, with no terminal.
`internal/bench/testdata/*.session` are typed lines (the first names the input in `testdata/`),
checked against `*.golden` transcripts; `-update` re-blesses. A bug from daily use becomes a
session first (§7.1). Piped into `jqv repl`, a script of lines echoes as a transcript too.

## 19. A lexer first, then colour (Q-031, Q-027, agreed 2026-11-08)

Colour doesn't need a parser; it needs a lexer. `jqsyntax`'s `scan` isn't one yet: it finds
what cutting a pipeline needs (pipes, brackets, `if`/`end`, `\(…)`, `as $x |`) and calls the
rest `other`. Names, numbers, operators, `.field`, `$var` and `@csv` are all `other`; spaces and
comments aren't tokens at all; `def`, `reduce`, `foreach`, `try`, `label`, `import`, `include`,
`and` and `or` aren't seen as keywords; and only `|=` is read as a two-character operator. What
it does have, bracket depth and string nesting, is the hard part, and rainbow brackets come
from it nearly free.

So the next piece is a **total lexer**, built and tested on its own. It's also what tab
expansion (R-006), syntax-error hints (Q-003) and, later, a parser with positions for `def`s,
scripts and the debugger stand on.

| Colour | Needs |
|---|---|
| strings, numbers, comments, `.field`, `$var`, `@format`, keywords, the pipe | the lexer |
| a call versus a field (`select` versus `.select`) | the lexer: a name not after `.`, `$` or `@` |
| builtin versus your `def` versus unknown (red: `keys_unsorted`) | the lexer, gojq's names (`catalog.builtins()`), the `def` names in the text |
| rainbow brackets, a mismatched one in red, the one matching the cursor | the lexer's depth |
| a field that isn't on the document | by example: run `UpTo` on the document, as tab will |

What does need a parser, and isn't part of this: a `$var` used outside its scope, a `def`
called with the wrong number of arguments, jumping to a `def`, and stepping into one in a
debugger. gojq's AST has no positions, so that parser will be ours, built on this lexer.

**Order**, each step useful and tested alone:

1. **`jqsyntax.Lex(text) []Token`** (Q-031). Kinds: space, comment, string (its `\(` and `)`
   edges and escapes as their own tokens), number, field (`.foo`, `."foo"`), `.` and `..`,
   variable, format, keyword, name, pipe, operator, bracket, error. Positions in runes.
   - Properties, fuzzed: the tokens joined give the text back exactly; text gojq parses has
     no error tokens; half-typed text always lexes (an unterminated string runs to the end,
     since the line is always mid-typing).
   - A table of traps: `.end` and `{if: 1}` (a field and a key, not keywords), `$__loc__`,
     `@base64d "…\(x)…"`, `?//`, `//=`, `.[`, `..`, `1e3`, `a::b` module names, and jq 1.7+'s
     comment continued after a backslash (check what gojq does).
   - Before writing it, read gojq's own lexer (`lexer.go`) for the exact rules (name
     characters, number forms), so we match it rather than guess.
   - **Done 2026-11-09** (`lex.go`). We can't call gojq's lexer: it's unexported, drops spaces
     and comments, works in bytes and stops at the first error. So ours copies its rules:
     ASCII names, its keyword list, its number states (`1.` and `.5` are numbers; `1a`, `1e`
     and `1.2.` are errors as far as gojq reads them), `a::b`, and a comment carried on by a
     `\` at the end of its line (gojq does this). What we decided along the way:
     - Each token has a **`Depth`**: the brackets open around it, a string's `\(` counted. A
       bracket and its closer share the depth outside them, so rainbow colour reads Depth.
     - A closer that matches nothing open (`[)]`) is an **Error**, so a mismatch shows up
       without a pass of its own.
     - **Key** is a kind of its own: a name just inside `{` or after its comma (`{if: 1}`,
       `{a}`). gojq lexes `if` there as a keyword, and its parser allows it as a key.
     - `."foo"`: the `.` is a Field and the string follows as string tokens.
     - gojq takes a NUL as the end of the text; we call it an Error, as jq does.
     - Non-ASCII outside a string is an Error per rune (`.café` is `.caf` then `é`), as gojq
       won't read it either.

   **`:lex [expr]`** (Q-034) shows how a line is read: the tokens as TestLex's cases are
   written (`[name:select][bracket:(]…`), then each bracket with its depth, then any Error
   with its position. It takes half-typed lines, since they are what it is for, and a
   misread line can be pasted straight into a test.

   Stepping inside a bracket (Q-033, from daily use) needs one more layer, not a parser: a
   bracket tree over these tokens, telling a collect `[` from a call's `(`, an object `{`
   and an interpolation. Then "in" runs the stage the bracket is in, then its inside as a
   stream (`[A | B]` gives `A | B`; `map(f)` gives `… | .[] | f`), where today every stop
   inside a bracket runs it closed.
2. **`scan` rebuilt on `Lex`**, so there is one tokenizer. `TestUpTo`, `TestStops`,
   `TestStopsParse` and `FuzzUpTo` must stay green unchanged.
   - **Done 2026-11-09.** They did. Checked further by fuzzing the old `scan` against the new
     on text gojq parses (138M cases, both `UpTo` and `Stops`). They differed only where the
     old one misread jq, and those are now test cases: `{if: .a} | .b` (the key read as an
     `if`, so the line had no stops at all), `try ..catch` (`.catch` taken as a field),
     `A::as` (a module's name taken as `as`), a comment ending at `\r`, and one carried on by
     a `\`.
3. **A paint hook in the editor**: an optional `Painter` that gives styled spans for the line;
   `View` draws them, cursor still reversed. The editor stays free of jq. The REPL's
   Highlighter (`internal/assist`) maps token kinds to lipgloss styles, a table config.toml
   can override later. Tested through a marker renderer
   (`[fn:select]([field:.age] [op:>] [num:30])`), not ANSI, so the goldens read.
   - **Done 2026-11-10.** `editor.Painter` gives `Span`s, each a run and a style function;
     `editor.Render` draws them, the cursor reversed inside its span's style. A finished
     line keeps its colour in the scrollback. `assist.Highlight(Palette)` is the jq painter:
     the whole line, or the argument of `:push`, `:pp`, `:tree` and `:lex`; other commands
     stay plain. `Colours()` is the tree view's palette (fields and keys as its keys,
     strings and numbers as its values), adaptive for light and dark. Tests use a palette
     that writes markers. lipgloss asks the terminal whether it is dark the first time it
     is used, so `Bind` asks before the editor reads keys, or the answer would arrive as
     typing.
4. **Brackets**: rainbow by depth, a mismatched one in red, the one matching the cursor marked.
   - Rainbow and mismatch done with step 3 (2026-11-10): brackets and a string's `\(…)` take
     three colours in turn by `Depth`; a closer that matches nothing is lexed as an Error
     (red, underlined). Marking the bracket that matches the cursor is still to do.
5. **Colour that knows things**: builtin, your `def`, unknown; fields not on the document come
   with tab (R-006), since both run `UpTo`.
6. **A read-only layout** (Q-035, done 2026-11-10, ahead of Q-033 by the owner's choice):
   `jqsyntax.Layout` lays a line out by its structure, for reading. `:fmt [expr]` prints the
   last line (or expr) laid out and coloured as the line is (`assist.Paint`, the same spans
   as the editor's), and **`F8`** prints the line being typed laid out, keeping the line. F5
   stays run-to-cursor, and is kept for the debugger later.
   ```
   [.[].people[] | select(.age > 30)] | reduce .[].age as $i (0; . + $i)
   ```
   ```
   [
       .[].people[]
       | select(.age > 30)
   ]
   | reduce .[].age as $i (0; . + $i)
   ```
   - The rule is one rule: **a bracket with a pipeline anywhere inside it is opened up**, its
     inside indented 4, its closer back on a line of its own at the opener's indent; one
     without stays as written. Every `|` of an opened level starts a line, `as $x |` included.
     Inside an opened `[ ]` or `{ }` each comma ends a line; inside an opened `( )` each `;`
     (reduce's and a call's arguments). A `,` at the top or in `( )` doesn't break, since
     there it's the comma operator, which binds tighter than `|`.
   - `if … end` opens the same way when it has a pipeline in it, an `elif`, or is longer than
     60 characters (owner 2026-11-10: an `elif` of objects stayed on one long line): the
     condition on the `if`'s line, each branch indented under `then`, `elif` and `else`, and
     the brackets around it opened too. A short `if . then 1 else 2 end` stays on its line.
     A string's `\(…)` never opens. A
     `;` outside any bracket ends a `def`, so it ends a line; so does a comment.
   - Spaces are one space, and nothing else is changed: fuzzed so that the layout has the
     same tokens as the text, parses wherever the text does, and laying it out again changes
     nothing. Half-typed text lays out as far as it goes.
   - Known looseness, to settle by use: `map({…})` opens both brackets
     (`map(` / `{` / … / `}` / `)`) where hugging them (`map({` … `})`) would read better;
     a pipeline in an `if`'s condition breaks the condition; a `def`'s body pipes sit at the
     top's indent.
   - Next on this path: the editor showing an entry in this layout as you type (Q-026), and
     Q-033's stepping moving along its lines.

## 20. A script context: working on fragments (proposal, owner 2026-11-11)

The REPL so far runs one line at a time and stacks stages. The owner wants it closer to a Lisp
IDE: a program built from small named pieces, each one quick to reach, edit and run alone, then
put together into the pipeline that is the answer. jq already has the pieces (§15): a named
piece is a `def`, and the answer is a program that calls them. So the REPL keeps a program
being built, which we call a **context**, and every line runs inside it.

**Proposed model.** Recommendations until the owner confirms them; the questions are at the end.

- **A context is a jq program in the making**: its `def`s, and a **main** pipeline. There is
  one at the start, `Global`. Written out, it is plain jq: the defs, then main. `:wq` hands it
  back like that, and later `:w file.jq` saves it.
- **A line that is only defs goes into the context.** `def peeps: .[].people[];` replaces any
  `peeps` with the same number of arguments (jq tells `f/0` from `f/1`, so we do too). It is
  also run on `.` and shown in brief, `peeps ▸ 7 outputs (objects)`, so you see straight away
  what it gives.
- **Every other line runs with the context's defs in front of it**, so `peeps | .name` just
  works. The line is still parsed alone first, so a syntax error's caret stays on what you
  typed.
- **Fragments run on the document.** A def with no arguments runs on `.`, the document after
  the stages, as the owner suggested, so each piece can be debugged on its own. Later a def
  can be given an input (`:on peeps` makes `older` run on what `peeps` gives) and its
  arguments (`:run older(30)`), kept in the file as a comment so they survive a save.
- **Main is the stages, grown up.** `:push` stays as it is. `:push as name` keeps the stage as
  `def name: …;` too, and main then calls it by name (`peeps | older(30) | names`). That is
  Q-016's named stages without a new idea: the stages are main, and a named stage is a def.
- **Getting around:** `:defs` lists the defs and what each gives, as `:stages` does. `:e
  peeps` puts `def peeps: .[].people[];` on the line to edit, and enter puts it back. Tab
  completes def names, both after `:e` and in jq. Once on the line, F5, ctrl+up/down, `:fmt`
  and `:tree` work on its body as on any line. A key that steps the line from one def to the
  next is the obvious next ergonomic step; it waits until use shows how often it's wanted.
- **Expanding goes through completion.** Tab replaces a span of the line with a candidate
  (§18, Q-036). The same move covers more than finishing a name: `reduce` expanded into its
  skeleton (R-009), a call expanded into its def's body ("inline"), or a def pulled in by
  name. So the completer is built to take sources, and the context's defs are one of them.
- **Later, refactoring** from the lexer and a bracket tree (Q-033): lift the stage under the
  cursor out into a def (`:lift name`), inline a call, rename a def and its calls, list who
  calls a def.
- **More than one context**, later: `:ctx trading` makes or switches to one. Another
  context's defs are reachable as `trading::f`, through gojq's module loader serving contexts
  from memory, so a saved context is a real jq module (R-008). `Global` is the default and
  needs no prefix.
- **The workbench (Q-032) is this, plus files:** `:load team-report.jq` fills a context from
  a file, and `:w` writes it back.

**Notes from the other laptop (2026-11-10)**, written before this section and folded in:

- **A line of only defs is unambiguous.** jq refuses one as a program ("Top-level program not
  given (try ".")"), so keeping it rather than running it can never take a line that meant
  something else.
- **Replace in place, not at the end.** jq binds a name to the def before it: in
  `def f: 1; def g: f + 1; def f: 10; g`, `g` is still 2. So a def typed again must take the
  old one's place in the context's list, or the defs after it (`ud` calling `peeps`) keep
  the old body. That is the difference between editing here and retyping the text into jq,
  and worth a line in a lesson.
- **The prompt can show the def names** (`[peeps ud] . ❯`) rather than their text, which is
  what happens today when a def is pushed as a stage (`:push def peeps: …; .`). That works
  already, because a def reaches everything after its `;`, but it fills the prompt, and
  `:pop` can't take one def back alone. `:undef name` should say which defs call it.
- **An alternative to editing on the line: focus.** `:focus ud` makes a def's body the
  pipeline being built, so `:push`/`:pop`, F5 and stepping build it a stage at a time
  (prompt `ud: peeps ❯`), and `:focus` alone goes back to main with the stages joined as its
  new body. Only defs without arguments could be focused. `:e` covers most of this more
  simply; focus is worth it if a def's body grows long enough to want stages of its own.

**Order**, each step useful alone:

1. **Tab completion** (Q-036), with sources: commands, `:help` names, jq's functions,
   keywords and variables, and the defs written in the line itself. The context's defs
   plug in at step 2.
2. **The context** (Q-037): def lines kept, the defs put in front of every line, `:defs`,
   `:e name`, `:undef name`, defs as a completion source, and `:wq` writing defs then main.
3. **`:push as name`**: main as named stages (Q-016).
4. **Files** (Q-032): `:load` and `:w` for `.jq`.
5. **Inputs and arguments** for defs (`:on`, `:run`).
6. **Refactoring** (Q-038): lift, inline, rename.

**OPEN:** whether a def line shows only the summary of what it gives or the values too; the
syntax `:push as name` versus `:push name`; a key to step through the defs; what main is when
nothing has been pushed (the last line?); whether `Global::` shows in the prompt.

## 21. Structure: the document's shape (Q-040, 2026-11-12)

Before writing jq against a document you need its shape: which keys sit under which arrays,
what types they hold, which are optional, which hold a handful of codes. `:structure` prints
that, and `internal/shape` keeps it as data for what comes later: key completion (Q-007,
R-006), the schema view (V-005), cursor info (V-010) and suggestions (R-010).

**Types come from the document tree, not the jq lexer.** `jqsyntax` reads jq programs; the
document is `internal/doc`'s ordered tree, where every node already knows its JSON type.
`shape.FromNode` walks it once.

- **A place** is a generalised path: array indices folded to `[]`, so `.orders[3].id` and
  `.orders[7].id` are both `.orders[].id`. Each place keeps how many values it saw and of
  which types, array lengths (fewest..most), number ranges, and up to 1000 distinct scalar
  values with their counts. Keys come in the order first seen.
- **One line per place**, labelled as a jq step (`.orders`, `[]`, `."$id"`) so it can be
  copied. Two folds keep regular documents short. An array that holds only arrays shares its
  line with its elements (`.fills[]  len 0..2  object ×3`). An object with one key, always
  present, shares its line with that key (`.fix.messages.message[]`).
- **What a line says:** the types (`string ×93`, or `array ×90 | object ×2` when mixed);
  `missing in N` when the key is absent from some of the objects that could hold it (it needs
  care in a `select`); array lengths; and for scalars, the values when there are at most 5
  (`"N" ×19 "Y" ×7`, worth a `select` or `group_by`), or else `all distinct` (a key for
  `INDEX`), `N distinct`, the number range, and an example.
- **Null beside one other type is optional, not mixed.** Any other mix gets `⚠ mixed`.
- **Array or single object** gets a note under the outline, with the jq that walks both:
  `⚠ .fix.messages.message[].field is an array or a single object; to walk both: … | if type
  == "array" then .[] else . end`. XML converted to JSON gives a lone child as an object, so
  `.field[]` then walks the child's values. FIX4.4 has five such places.
- **What it runs on.** Bare `:structure`, with no stages, uses the doc tree, so keys come in
  the order written. After stages, or with an expr, it uses gojq's values. A stream is taken
  together (`3 outputs, taken together:`). gojq's maps have no order, so `shape.KeyOrder` puts
  them back. An object that passed through unchanged (`.[]`, a slice, a select) is matched by
  its set of keys and gets them exactly as written. One jq built gets them by where each key
  first appears in the document, then sorted. Ranking by name alone wasn't enough: FIX's
  fields have `_number` before `_name`, but `_name` appears first, in the header.
- **Depth:** `:structure 2 .fix` shows two levels, then `… 14 places below`. A leading
  number is taken as the depth because a number alone has no structure worth asking for.

**Next:** a corpus of real documents (TeamCity's project list, k8s YAML, drop-copy JSON
Lines) as golden outlines in `internal/shape/testdata`; key completion from the shape (Q-007);
suggestions from what a place holds (R-010); the TUI's schema view on the same data (V-005).
OPEN: whether string formats (dates, numeric strings like FIX's `_number`) should be
recognised as kinds of their own. They would help suggestions (`tonumber` before a sort).

## 22. Knowing what `.` is: scope, keys and checks (proposal, owner 2026-11-14)

A SQL editor knows the schema, so it can complete columns and underline a misspelt one. jqv
knows more than a schema, because it has the document. So at any point in a line it can know
what `.` is there. One engine answers that, and three things use it. **Tab** offers the keys
at that place, with their types. **Checks** mark a field that isn't there. **A context
display** shows what `.` is as you type, like a debugger's stack: the dim line in the REPL,
and later a fixed panel beside the TUI's editor (T-027).

The case behind the checks is the owner's own line, on
`testdata/employees_5-level_10-KB_formatted.json`:

```
.[].employee | {"eid": .id, "ename": .name, "epos": .position, "pid": .projects[0].id}
```

`pid` is `null` on all six rows, and jq says nothing, because the key is `projectId`. A
misspelt field in jq is a silent null, never an error. Only something that knows the shape
can say so.

**The question at the cursor has two parts.** The first is the **input**: what `.` is for the
term the cursor is in. The second is the **term**: the path typed so far in that term
(`.projects[0].na`). Tab completes the last step of the term from the shape at the step before
it. The checker asks the same of every field in the line.

**The input is a jq program, derived from the line.** It is made from the lexer's tokens and
a bracket tree (the same one Q-033's stepping needs), by these rules:

| Construct | `.` inside it is |
|---|---|
| `A \| B` | in B: A's outputs |
| `A, B`, `A and B`, `A + B`, `A == B` … | the input of the whole |
| `( … )`, `[ … ]` | the input of the bracket |
| `{k: V}`, `{(K): V}` | the input of the object; `{name}` is `{name: .name}` |
| `map(f)`, `map_values(f)`, `sort_by(f)`, `group_by(f)`, `unique_by(f)`, `min_by(f)`, `max_by(f)`, `any(f)`, `all(f)` | each element: `input \| .[]` |
| `with_entries(f)` | `input \| to_entries[]` |
| `select(f)`, `if C then A else B end`, `first(f)`, `limit(n; f)`, `del(f)` and the rest | the input (the default for an argument) |
| `X as $v \| body` | body: the input; `$v`: `input \| X` |
| `reduce S as $x (I; U)`, `foreach` | I: the input; U: the accumulator, taken as I's output; `$x`: `input \| S` |
| `try A catch B` | in B: the error, a string |
| `def f: body;` | unknown: it depends on the caller |

For the owner's line with the cursor after `.projects[0].`, the input is `.[].employee` and
the term is `.projects[0].`. Inside `select(.status == "Pending")` within `map(…)`, it is
`… | .[]`.

**Walk it when it's a path, run it when it isn't.** When the input is a plain path (`.`,
`.[]`, `.a.b[]`, `.[0]`, slices, `?`), the answer is a walk of the document's shape. That
shape is built once from the doc tree, with keys in the order they were written, and no jq
runs. Otherwise gojq runs the input program on the current `.` (after the stages), in
process, with a short timeout and an output cap. The outputs are shaped (`shape.FromValues`
with `KeyOrder`). Results are cached by the program's text, so cycling Tab, typing more of a
term or rechecking the line doesn't run it again. If the cap or the timeout is hit, the shape
is marked **sampled**, and every claim from it is weakened: "not in the first 1000 values"
rather than "unknown". The term, always a path, is then walked on that shape step by step,
so the shape at every step is known.

- **Why by example, not inferred types.** Static inference would need a type rule for every
  builtin, and still couldn't know which records `select(.status == "Pending")` keeps. gojq
  already knows jq's meaning exactly. The static walk is kept for paths, where it is plainly
  right and saves a run on every keystroke. Inference over the shape is still possible later,
  for places that can't be run (a def's body).
- **Bounded, like a line** (§18): nothing on the keystroke path spawns a process, and a slow
  input (`range(1e9)`) only costs its timeout, and only once, because of the cache.

**Variables.** `$root` is the whole document's shape; `$v` from `X as $v` is `input | X`;
`$x` in `reduce`/`foreach` is `input | S`; `$_` is the last result's; `$ENV` is an object of
strings. A term can start from a variable, so `$e.em<Tab>` works as `.em<Tab>` does.

**Frames: the stack.** Every construct that encloses the cursor is a frame, with a label, its
span in the line, its input program and its shape. `scope.At(line, cursor)` gives the frames
from the outermost in, then the term. For the line above, with the cursor after
`.projects[0].`:

```
$root              array len 6 of object {employee}
.[]                object {employee}
.employee          object {id name position department projects}
{… "pid": ▮}       . is the object's input
.projects[0]       object {projectId projectName startDate tasks}
```

The TUI's panel draws this (T-027), and the REPL prints it on a key. **A frame shows one
level only**: the type, array lengths, then keys up to the width and `… 912 keys`. So FIX4.4
can't flood it, and `:structure` stays the way to see the depth.

**Tab on keys (Q-007).** After `.`, `.na`, `$v.` or `[].`, the candidates are the keys at that
place, matched by prefix. A key that isn't an identifier goes in quoted (`."$id"`). The hint
under the line is the place's `:structure` line, from the same formatter:
`name  string ×6  all distinct  e.g. "Jodi Reyes"`, `projects  array len 1 of object (4
keys)`, `completionDate  string ×8  missing in 4`. That puts the type in the status as each
candidate lands.

**Checks (Q-046).** Every field in the line, not only the one at the cursor, is checked
against its frame. The painter colours it and the dim line says why when the cursor is on it:

- **error:** what jq would refuse on every value seen: `.x` on an array, a string or a number
  (`.projects.x` gives `Cannot index array with "x"`), and `.[]` on a scalar. The hint gives
  the form that works: `.projects[].x` or `.projects[0].x`.
- **warning:** a key never seen at that place, which jq would quietly turn into null. It
  comes with the nearest keys there: same ending or containment first, then spelling
  (`id` → `projectId`). On a sampled shape it says "not in the first 1000 values".
- **indexes past the end** (owner, 2026-11-14): jq gives null for an index out of range,
  positive or negative, as quietly as for a missing key (`.projects[9]`, `.projects[-9]`,
  and `.projects[9].projectId` null down the path; checked against jq 1.8.2). The shape
  keeps each array place's shortest and longest length. An index at or past the longest
  (or below minus it) is a **warning**: `projects has 1 element in all 6 records: [9] is
  always null`. One between the shortest and longest is a **note**: `len 2..4: [3] is null
  where shorter`. To say how many arrays are shorter, the shape would keep a count per
  length. Slices out of range give `[]`, not null, and aren't checked. The REPL's all-null
  note says "a field that isn't there" here too, which is wrong for an index: the check
  names the index instead.
- Not checked: `.x?`, def bodies, and terms whose input failed to run (that error is the
  line's own hint's job when it runs). When a line run with enter gives only nulls, the
  all-null note (§18) can name the unknown field rather than listing keys.

**The context line (Q-047).** While typing, with nothing else to show, the dim line under the
input shows the type of the term at the cursor:
`.[].employee | .projects[0]  object {projectId projectName startDate tasks}`, and on an
array `array len 1..3 of object`. Completion's candidates and hints replace it while Tab is
cycling. F3 prints the frames above the line, as F8 prints the layout, and keeps the line.

**Where it lives.** `internal/scope` is pure. It takes the line, the cursor and a source (the
document's shape and a runner for jq, bounded) and gives the frames, the term, the shape at
the cursor, and diagnostics with spans. `jqsyntax` supplies tokens and the bracket tree;
`shape` supplies the shapes and the line format; `assist` turns a scope into candidates,
paint spans and the dim line; the TUI's panel reads a scope directly. It is tested from a
table of lines with `▮` for the cursor and the input and term expected, then golden frames on
the employees, FIX4.4 and filebeat documents.

**Order**, each step useful alone:

1. **The input program**: the bracket tree, the rules above, `scope.Input(line, cursor)`, and
   `:scope [line]` in the REPL to print the frames, as `:lex` does (for checking it on real
   lines and turning a misread one into a test).
2. **Shapes for frames**: walk or run, the cache, sampling.
3. **Tab on keys** with type hints (Q-007).
4. **Checks**: colour, the reason on the dim line, the nearest keys.
5. **The context line and F3** (Q-047).
6. **Values** (R-006, owner 2026-11-14): on the other side of a comparison with a field,
   Tab cycles the values seen at that field's place. `.[].employee | select(.name == "Jo<Tab>`
   gives `"Jodi Reyes"`, and `select(.status == <Tab>` cycles `"Completed"`, `"In Progress"`,
   `"Pending"`, the most frequent first, each hint saying how often it was seen
   (`×8 of 12`). The same works after `!=`, `<`, `>`, inside `IN(…)`, and in the string
   argument of `startswith`, `endswith`, `test` and `contains` on that field. **Against
   explosion:** the shape keeps at most 1000 distinct values a place (`shape.MaxDistinct`).
   What has been typed inside the string filters them, by prefix and then by substring, and
   the editor already lists only what fits the width (`… 412 more`). A place with more
   distinct values than the cap says so in the hint (`first 1000 of more`), so a missing
   value isn't taken as absent. A value goes in as JSON text (`"In Progress"`, `47`,
   `true`), so quoting can't break the line. Numbers offer their range in the hint
   (`13..73`) as well as the values.
7. **The TUI panel** (T-027).

**Step 1 done (2026-11-14).** `scope.At(line, cursor)` walks the lexer's tokens once, keeping
a stack of frames (the top, each bracket, and `if` and `def` at the same depth), each with the
stages finished in it. The input at the cursor is the frame's base, then those stages, joined
with `|`. A binding stage (`X as $v`) is kept only while something after it uses `$v`, so
`.[] as $e | .x | .▮` has input `.x`. A `reduce`'s update is the accumulator, taken as
`S as $x | init`. The right side of `|=` runs on its left side's paths; `+=` and the rest
run on the input, as jq does. `findTerm` reads back from the cursor over fields, indexes,
quoted keys and `?` to a head: `.`, `..`, a variable, or a bracket (`(.a | .b).c`,
`first(.[]).x`). After something that isn't a path (`0[1].▮`, a reduce's `(…)`) it gives no
term, rather than a wrong one. `:scope [expr]` (`:sc`) prints the stack, with ▮ for the
cursor. It is tested by a table of lines (`internal/scope`), a `scope.session` golden on the
employees file, and `FuzzAt`. On any line gojq parses, at every cursor, every program it
gives must parse too. The fuzzer found the cases the table missed: a space before a call's
`(` or an index's `[` (`A (0;0)`, `0 [0:]`), a pipe in a slice's bound, a comment inside a
stage (it swallowed the rest once joined onto one line), and a call with thousands of
arguments (labels are now built once, at the end).

**Step 2 done (2026-11-14).** `scope.Shapes.Of(program)` gives what a program yields, as a
shape. It splits the program at its top-level pipes and takes the longest tail that is only
keys and `[]`. With nothing before that tail, it walks the input's shape (the document's,
when there are no stages). Otherwise it runs the head through gojq (150 ms, 1000 outputs,
`bench.ScopeTimeout`/`ScopeMax`) and walks the tail on what came out. Only keys and `[]` on
arrays are walked, because their counts are exactly what jq would give. An index, a slice,
`[]` on objects, a key asked of arrays or nulls, and a path going on past a sometimes-missing
key are all run instead, which also gives jq's own error. A walk says when a key isn't at
its place at all (`Missing`, for the checks) and how many objects lack it (`Absent`, the
nulls jq gives). Runs are kept by program text until the stages or `$_` change, so typing
a term runs each prefix once. `shape.Brief` is one level, as the frames show it:
`array len 6 of object ×6 {employee}`, keys an object sometimes lacks marked `?`, as many
keys as fit and then `… 903 more`. `:scope` now shows it for every frame, the term and each
variable. Measured on FIX4.4: the document's shape takes about 9 ms, once; after that `At`
and every `Of`, run or walked, take well under a millisecond.

**Step 3 done (2026-11-14).** Tab offers keys (`assist.CompleteOn`, bound by the REPL;
`Complete` stays the pure fallback for names). When the cursor is in a field (`.na▮`, `.▮`,
`."na▮`, `$e.▮`), the candidates are the keys of what the term's path gives
(`Scope.Value`, through `Shapes.Of`). In an object's key place (`{na▮`) they are the input's
keys, as shorthand (`{name}` is `{name: .name}`). Matching settles two of the OPEN questions:
first the key typed exactly, then keys starting with it, then keys containing it in any case
(`.id▮` and `.ID▮` give `projectId`). Within each group keys come in the document's order,
not sorted, because `id name position department projects` reads as the record does. A key
that isn't an identifier goes in quoted (`."$id"`); typing inside quotes keeps them. The
hint is the key and its `Brief` (`projects  array ×6  len 1 of object ×6 {…}`), then
`missing in 4 objects` when some lack it, and `(of the first outputs)` after a cut run.
Where the place has no keys (a string, a def's body) nothing is offered, rather than jq's
function names. Tab on a whole key holding an array offers nothing more for now: the hint
says it is an array.

**Step 4 done (2026-11-14).** `scope.Check(line, shapes)` checks every field and integer
index in the line. For a field, the scope at its end gives the term, the program for what
it is asked of (`Value`) and that program's shape. For an index, the scope at its `[` gives
the program for the array (`Here`, the term with its key). The findings:

- **error:** a key asked of arrays only (`.x: .projects is an array here; a key is asked of
  its elements: .projects[].x or .projects[0].x`), or of strings or numbers.
- **warning:** a key no object there has, with the nearest keys. Nearest means containing it
  or contained in it in any case, then within a few edits, a swap of neighbours counting as
  one (`nmae` → `name`). An index at or past the longest array there (or below minus it):
  `[9]: … has 1 element in all 6 arrays here, so [9] is always null`. Negative indexes are
  read from `[-` and a number, as the lexer splits them.
- **note:** a key some objects lack (`missing in 4 of the 12 objects`), an index past some
  arrays' ends (`[3] is null in the 6 of 12 arrays that are shorter`, from a count of arrays
  by length the shape now keeps), and a key that starts some keys there. The last is the
  word being typed, so `.na` says `keys starting "na": name` rather than warning at every
  key you type.
- Skipped: `.x?` and `[9]?`; a key after one already wrong (only the first wrong step is
  marked, `.junk` in `.junk.more`); places with only nulls, or mixed types, where jq's answer
  depends on the value; def bodies.

In the REPL, the painter (`assist.Checked`) draws a warning's field underlined in yellow and
an error's in red, in place of its own colour. Notes aren't drawn. The editor has a `Hinter`
now, a dim line under the input whenever Tab's candidates or a completion's hint aren't
showing. `assist.CheckHint` puts the reason there when the cursor is on what a check found
(`⚠ .id: no such key in the 6 objects here, so jq gives null; did you mean .projectId?`). The
painter and the hinter share one `Check` per line. `:scope` lists the line's checks after the
stack, which is how they are tested in a session; `FuzzAt` runs `Check` on every input too.

Not modelled yet: `try … catch` (the catch's `.` is the error), `?//` alternatives, an index
on an `if … end` (`end [0]` is taken as an array), `label` beyond keeping its variable, and
a def's body, which is unknown (its parameters too).

**OPEN:** whether Tab on a whole key that holds an array also offers `.projects[]` and
`.projects[0]`; what `.` is in a
def's body (unknown, or the input at its first call); the run's timeout and sample size (150
ms and 1000 outputs to start, measured on FIX4.4 and filebeat); whether checks run on every
key or after a pause.
