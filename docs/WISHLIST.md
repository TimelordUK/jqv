# jqv — wishlist

Every idea goes here, however wild. We refine them by using the tool.

**IDs:** `C-` core (document model, parsing, performance) · `Q-` query (gojq, errors, AST,
autocomplete) · `V-` views and output (tree, table, raw, export) · `R-` recipes and history ·
`T-` the TUI's layout, keys and chrome. IDs are permanent. Don't renumber; mark items dropped instead.

**Milestones:** `M1` daily-drivable viewer · `M2` query bench · `M3` shape-aware (autocomplete,
schema) · `later` · `?` undecided

**Status:** `idea` → `planned` → `doing` → `done` · `dropped`

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
| C-009 | Large-file stance: measure 10/50/100 MB; decide on lazy children if needed | later | idea |

## Query

| ID | Item | MS | Status |
|---|---|---|---|
| Q-001 | `internal/query`: gojq parse/compile/run with timeout and cancel; corpus tests (`testdata/queries`) | M2 | planned |
| Q-002 | Query bar: evaluate as you type (debounced), results into the output pane | M2 | planned |
| Q-003 | Errors with position: underline the bad token, a plain-English hint, a suggested fix | M2 | planned |
| Q-004 | Pipeline stepper: split at top-level `\|`, show the output after each stage, move between stages | M2 | idea |
| Q-005 | AST view (`K`, like nvim LSP hover): the parsed query as a tree | M2 | idea |
| Q-006 | Cursor node → query: `y` is done; also "insert path into query", and "filter to this" | M2 | idea |
| Q-007 | Autocomplete paths/keys from the shape index (C-side: `internal/shape`) | M3 | idea |
| Q-008 | Autocomplete jq builtins with signatures and one-line docs | M3 | idea |
| Q-009 | `--query` / `-q` non-interactive: run jq with our error hints, print the result | M2 | idea |
| Q-010 | Output key order: gojq sorts keys. Decide whether to re-order output by the source's order where possible | ? | idea |
| Q-011 | "Check with real jq" (if on PATH): run the same query and diff, for gojq differences | later | idea |
| Q-012 | Custom functions available in every query (`flat`, `paths_to(k)`, `table`) | M2 | idea |

## Views and output

| ID | Item | MS | Status |
|---|---|---|---|
| V-001 | Output pane as a tree (same widget as the document) | M2 | idea |
| V-002 | Table mode: array of objects → columns from the union of keys, nested values summarised | M2 | idea |
| V-003 | Raw mode: pretty-printed JSON with syntax colour | M2 | idea |
| V-004 | Export output: JSON / JSONL / CSV to a file or clipboard; hand-off to sql-cli | M2 | idea |
| V-005 | Schema view: the shape index shown as a tree (key, types seen, how often, example) | M3 | idea |
| V-006 | Collapsed containers show a one-line preview (`{id: "ORD-1001", status: …}`) | M1 | idea |
| V-007 | Search in the tree (`/`, `n`/`N`), matches in keys and values, opening parents to show them | M1 | idea |

## Recipes and history

| ID | Item | MS | Status |
|---|---|---|---|
| R-001 | Query history, persisted, recalled with up/down and a finder (hit-style) | M2 | idea |
| R-002 | Recipes: named, parameterised jq snippets in TOML ("group by X and count", "find key K anywhere") | M2 | idea |
| R-003 | Recipe picker that fills parameters from the cursor node (a key, a path) | M3 | idea |
| R-004 | Save a session: document path, open nodes, query, output mode | later | idea |

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
