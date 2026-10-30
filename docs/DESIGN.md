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
internal/tui       bubbletea model (pure Update) + view (lipgloss)
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
| `:` | command line: `:123` row, `:set nu` / `rnu` / `nonu`, `:q` |
| `L` | open everything under the cursor (at the root: the whole document) |
| `H` | close everything under the cursor, keep it open as an outline |
| `zo` `zc` | open / close this node and everything under it |
| `zR` `zM` | open / close everything |
| `z1`…`z9` | open the whole document to that depth |
| `y` / `Y` | copy the jq path / the value as JSON |
| `/` `?` `n` `N` | search forward / back, next / previous (smartcase; `\c` `\C` force) |
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
- Landing on a match inside a closed container opens its parents. The status bar shows
  `match 3/17`, and every visible match is highlighted.
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

Telescope-style: type a phrase and get a list of hits, one per line, as `path   matched text`.
This view is about *where*, so it shows paths rather than collapsing them. Moving through the
list moves the tree beside it; enter lands there and closes the list. Good for "show me every
place this ID appears" when there are many hits spread across the document.

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
