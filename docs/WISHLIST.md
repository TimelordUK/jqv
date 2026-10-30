# jqv — wishlist

Every idea goes here, however wild. We refine them by using the tool.

**IDs:** `C-` core (document model, parsing, performance) · `Q-` query (gojq, errors, AST,
autocomplete) · `V-` views and output (tree, table, raw, export) · `R-` recipes and history ·
`T-` the TUI's layout, keys and chrome. IDs are permanent. Don't renumber; mark items dropped instead.

**Milestones:** `M1` daily-drivable viewer · `M2` query bench · `M3` shape-aware (autocomplete,
schema) · `later` · `?` undecided

**Status:** `idea` → `planned` → `doing` → `done` · `dropped`

**Owner's priorities (2026-10-28):** navigation and finding first, then sensible filtering,
then lifting from the cursor into queries. Rough order: V-007 search → V-010 cursor info →
T-011 jump labels → V-008 structure-aware filter → V-009 finder list → Q-001 onwards, with
Q-006 / Q-013 / Q-014 as the first query features because they start from the cursor.
The navigation layer should feel like vim before the REPL starts (V-007 with T-012, then
T-013–T-015). The REPL's job is to get close quickly: usually a grid of selected fields,
filtered (the order rows for one order id), built by R-005 verbs, not typed out.

---

## Core

| ID | Item | MS | Status |
|---|---|---|---|
| C-001 | Ordered JSON tree from the token stream (key order kept), jq paths with quoting, `Value()` for gojq, fuzzed against encoding/json | M1 | done |
| C-002 | Read a file or piped stdin; keys from the console when stdin is piped | M1 | done |
| C-003 | `--print` / `-d N`: the tree as text, no TUI | M1 | done |
| C-004 | JSON Lines and concatenated values: open as an array of documents | M1 | idea |
| C-005 | Load timing in the status bar (parse ms, node count), like hit's `HIT_TIMING` | M1 | idea |
| C-006 | Incremental row splicing on expand/collapse if rebuilds ever show in timings | later | idea |
| C-007 | Reload on file change (`:e`, or watch); keep open nodes by path | M2 | idea |
| C-008 | Parse errors point at line:col with the surrounding text, and still open what parsed | M1 | idea |
| C-009 | Large-file stance (DESIGN §11): measured 2026-10-29, ~7x memory and ~11 ms per MB. Whole-document loading stays simple; big input goes to lines mode (C-011). Leaner nodes only if limits bite | later | done |
| C-010 | Size guard: `--max-size` (default 100 MB); refuse a big file before reading, stop piped input at the limit, say what to do instead | M1 | done |
| C-011 | Lines mode for big JSON Lines logs: index line offsets, preview the first N, stream jq over lines, matches become a normal document | M2 | idea |
| C-012 | Other formats in (DESIGN §12): translate on load into the same tree, so every key, search and query works unchanged. Pick by extension, then by content. The header shows the translation (`web.config · xml→json`) | M2 | idea |
| C-013 | YAML → JSON (k8s, pipelines, compose). Key order kept; anchors expanded; multi-document files become an array | M2 | idea |
| C-014 | XML → JSON (.NET `web.config` / `app.config`, `.csproj`, NuGet, Maven). Mapping chosen to match an existing tool (yq's is the likely pick) so queries carry over; element order kept. `testdata/FIX4.4.json` is one online converter's answer (attributes as `_name`, repeated elements as arrays): compare against it. It puts attributes last, so a message's name is at the end of its block; ours should put them first. With user guidance, nominate an attribute as the key so a repeated element becomes a keyed map (`TradeCaptureReport: {…}`) rather than an array (owner, 2026-10-30): to try when conversions start | M2 | idea |
| C-015 | `--print-json`: write the translated JSON out, so a YAML or XML file can be piped into jq or sql-cli | M2 | idea |

## Query

| ID | Item | MS | Status |
|---|---|---|---|
| Q-001 | `internal/query`: gojq parse/compile/run with timeout and cancel; corpus tests (`testdata/queries`) | M2 | planned |
| Q-002 | Query bar: evaluate as you type (debounced), results into the output pane | M2 | planned |
| Q-003 | Errors with position: underline the bad token, a plain-English hint, a suggested fix | M2 | planned |
| Q-004 | Pipeline stepper: split at top-level `\|`, show the output after each stage, move between stages | M2 | idea |
| Q-005 | AST view (`K`, like nvim LSP hover): the parsed query as a tree | M2 | idea |
| Q-006 | Lift the cursor into the query (DESIGN §9.4): on `.orders[3].id` the query starts as `.orders[].id`, the generalised path, not the one element. `y` (exact path) is done; also "filter to this" | M2 | idea |
| Q-007 | Autocomplete paths/keys from the shape index (C-side: `internal/shape`) | M3 | idea |
| Q-008 | Autocomplete jq builtins with signatures and one-line docs | M3 | idea |
| Q-009 | `--query` / `-q` non-interactive: run jq with our error hints, print the result | M2 | idea |
| Q-010 | Output key order: gojq sorts keys. Decide whether to re-order output by the source's order where possible | ? | idea |
| Q-011 | "Check with real jq" (if on PATH): run the same query and diff, for gojq differences | later | idea |
| Q-012 | Custom functions available in every query (`flat`, `paths_to(k)`, `table`) | M2 | idea |
| Q-013 | Pick several fields (visual select or mark `id`, `symbol`, `qty` inside one order) → query `.orders[] \| {id, symbol, qty}` → shown as a grid (V-002). A starting point to edit, not a finished query | M2 | idea |
| Q-014 | Aggregate lift: on a number inside an array of objects (`qty` in `.orders[]`), show sum/min/max/avg/count straight away, and lift `[.orders[].qty] \| add` into the query. Later: small inline operations | M2 | idea |

## Views and output

| ID | Item | MS | Status |
|---|---|---|---|
| V-001 | Output pane as a tree (same widget as the document) | M2 | idea |
| V-002 | Table mode: array of objects → columns from the union of keys, nested values summarised | M2 | idea |
| V-003 | Raw mode: pretty-printed JSON with syntax colour | M2 | idea |
| V-004 | Export output: JSON / JSONL / CSV to a file or clipboard; hand-off to sql-cli | M2 | idea |
| V-005 | Schema view: the shape index shown as a tree (key, types seen, how often, example) | M3 | idea |
| V-006 | Collapsed containers show a one-line preview (`{id: "ORD-1001", status: …}`) | M1 | idea |
| V-007 | Vim search (DESIGN §9.1): `/` forward, `?` back, `n`/`N` repeat, wrap with vim's messages; keys and values; smartcase with `\c`/`\C`; opens closed parents and centres an off-screen match; `match 3/17` in the status bar; matches highlighted; esc clears. Later: incremental (move while typing), search history | M1 | done |
| V-008 | Structure-aware filter (DESIGN §9.2): hide what doesn't match but keep each match's ancestors (so you see where it lives) and a matching container's whole subtree (TeamCity: `elastic` keeps the projects nested under it). Fixture from a real TeamCity response | M1 | idea |
| V-009 | Telescope-style finder (DESIGN §9.3): type a phrase, get a scrollable list of hits (path + matched text + a little context); moving through the list moves the tree; enter lands there | M1 | idea |
| V-010 | Cursor info (DESIGN §9.4): beside the path, the generalised path (`.orders[].id`), type, position among siblings, and for a field in an array of objects how many elements have it, distinct count, sample values, min/max for numbers | M1 | idea |
| V-011 | Fold to matches (`z/`, owner's idea 2026-10-29): collapse everything except the paths down to each match of the last search, cursor on the first. Unlike the filter (V-008) nothing is hidden: non-matching siblings stay as one closed line each | M1 | done |
| V-012 | Fold to a jq expression: the same as `z/` but the matches are what a jq expression selects (`paths(.. \| strings \| test("olivia";"i"))`, `.[] \| select(.age > 90)`), so structure-aware conditions can drive what opens | M2 | idea |

## Recipes and history

| ID | Item | MS | Status |
|---|---|---|---|
| R-001 | Query history, persisted, recalled with up/down and a finder (hit-style) | M2 | idea |
| R-002 | Recipes: named, parameterised jq snippets in TOML ("group by X and count", "find key K anywhere") | M2 | idea |
| R-003 | Recipe picker that fills parameters from the cursor node (a key, a path) | M3 | idea |
| R-004 | Save a session: document path, open nodes, query, output mode | later | idea |
| R-005 | **The everyday 90%** as a few verbs, so they are never hand-typed: pick fields → grid, filter where field = value (on `"ORD-1002"`: `.orders[] \| select(.id == "ORD-1002")`), sort by, group and count, flatten nested to columns. Each verb fills from the cursor and writes plain jq into the bar, so it can be edited and chained (DESIGN §10) | M2 | idea |
| R-007 | Follow a tree of ids (DESIGN §11): on an id, collect every message linked to it through parent fields, in one streaming pass. FlexTrade drop copy (gRPC messages as JSON lines): children name their parent, never the root | M2 | idea |
| R-006 | Contextual completion in the query bar: after `.orders[].` offer the keys seen there; after `select(.status == ` offer the values seen; verbs (R-005) offered beside builtins | M3 | idea |

## TUI

| ID | Item | MS | Status |
|---|---|---|---|
| T-001 | Tree view: vim keys, open/close, zo/zc/zR/zM/z1–z9, gg/G, half pages, indent guides, colour by type | M1 | done |
| T-002 | Copy the path (`y`) and the value (`Y`) of the cursor node | M1 | done |
| T-003 | Split layout: document · query · output, resizable, focus moves with a key | M2 | idea |
| T-004 | Help overlay (`?`) from the keymap table in `tui/keys.go`; a test fails if a listed key does nothing | M1 | done |
| T-005 | Keymap as data in config.toml | later | idea |
| T-006 | Mouse: click to open/close, wheel to scroll | later | idea |
| T-007 | OSC 52 copy for SSH sessions | later | idea |
| T-008 | Themes (the palette is one table today) | later | idea |
| T-009 | Long strings: wrap or show in a detail pane rather than only truncate | M1 | idea |
| T-010 | Single-key bulk open/close from the cursor: `L` opens the whole subtree, `H` closes everything below but keeps the node open (outline). `zo`/`zc` stay | M1 | done |
| T-011 | Jump labels, like flash.nvim / leap / hop (DESIGN §9.1): press the jump key, type one or two characters, every visible match gets a short label, press the label to land. Reaches anything on screen in 3–4 keys | M1 | idea |
| T-012 | `?` is for backward search (V-007), so help moves to `F1` (owner, 2026-10-28: a natural vim environment) | M1 | done |
| T-013 | Re-centre, vim style: `zz` cursor to middle, `zt` top, `zb` bottom | M1 | done |
| T-014 | Row numbers in a gutter: `:set nu`, `:set rnu` (relative; with both, the cursor row is absolute as in vim), `:set nonu`. Numbers are rows on show, not file lines | M1 | done |
| T-015 | Counts before motions (`5j`, `3n`), so relative numbers pay off | M1 | idea |
| T-016 | Full pages: `ctrl+f` / `ctrl+b` and PgDn/PgUp move a screen less two rows of context; `ctrl+d` / `ctrl+u` stay half pages | M1 | done |
| T-017 | `ctrl+g`: file, row n of m with vim's `--n%--`, how far through the whole document (stable as nodes open and close), depth | M1 | done |
| T-018 | `:` command line, a small vim subset: `:123` go to row, `:set nu/rnu/nonu/nornu/nu!`, `:q`. Unknown commands say so. Grows only when a command earns it (`:e`, `:w out.json` later) | M1 | done |
| T-019 | `[{` `]}`: start / end of the enclosing object, as vim. A search hit on a field at the tail of a record gets to the record's top without closing anything | M1 | done |
| T-020 | `%`: start of a block to its last row and back. Closing brackets are not rows, so the last row stands in for one, and the way back is remembered because one row can end several blocks | M1 | done |
| T-021 | `{` `}`: step through the blocks at this level (one FIX message to the next), opening each on landing and putting it at the top of the screen | M1 | done |
| T-022 | `(` `)`: iterate the array the cursor is in, element by element, from any depth inside the element (Nobel prizes, or the laureates of one). Unlike `{` `}` it never moves across object keys | M1 | done |
