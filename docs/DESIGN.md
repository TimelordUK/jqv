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
- `[repl]` is the REPL's (§18): `pipe = "bat -l json"`, what a bare `:!` runs. The REPL reports
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
- **Errors teach** (§16): jq's message plus a hint where a common mistake is behind it, such as
  a field asked of an array ("after a slice, add `[]`", lesson 4). A parse error points at the
  place with `^`, and says what's behind it when the line shows it: `. age` is `.` and then
  a stray word, so the hint is "a field is .age, with no space after the dot". A line that gives only nulls says so and lists the keys of `.`, since a
  misspelt field (`.nam`) is null in jq, not an error.
- Output is compact when it fits on a line of 100, pretty otherwise. Keys come out sorted by
  gojq (Q-010 stays open).
- Line editing and up-arrow history are the console's own for now: the line is read whole,
  after enter, so no key reaches jqv while you type. The line editor below changes that.

**Commands** start with `:` (a line of jq never does): `:push [expr]` (`:p`), `:pop`,
`:reset` (drop every stage), `:stages`, `:program`, `:tree [expr]`, `:pp [expr]`, `:! cmd`,
`:cls` (`:clear`), `:help [name]`, `:try N [.]`, `:q`.

- **`:tree`** (Q-005, with Q-004's "output after each piece"): gojq's parse tree, laid out
  with what each step gives on the document. `.[].people[]` is one term with three steps
  (`.[]`, `.people`, `[]`); `a | b` is parts, each run with the parts before it. The first step
  that fails shows the error and its hint, and the steps after it say `not reached`, so the
  tree says where a query breaks. A binding (`. as $x | …`) stays whole. Later: open
  `select(…)`, `map(…)` and operators to show their insides.
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
    `:help jq` lists every section with its names.
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

**The line editor (Q-025, planned, owner 2026-11-07).** jqv reads the input line itself
(inline Bubble Tea: output scrolls above as now, the line at the bottom is ours), which is
what every key below needs. It means redoing what the console gives for free (history, word
moves, paste), so it comes once the commands have settled. Piped scripts keep the plain line
reader, so the session goldens don't change. The same editor is the TUI's reader (Q-017).

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

Planned: filter layers (Q-015) as `:where`, live results as you type, error hints that point at
`:help` (Q-023), and the help applied to the cursor's value in the TUI (Q-008).

**Tested like the TUI**: `bench.Session` takes lines and gives text, with no terminal.
`internal/bench/testdata/*.session` are typed lines (the first names the input in `testdata/`),
checked against `*.golden` transcripts; `-update` re-blesses. A bug from daily use becomes a
session first (§7.1). Piped into `jqv repl`, a script of lines echoes as a transcript too.
