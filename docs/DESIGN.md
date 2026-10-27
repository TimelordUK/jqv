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
| `ctrl+d` `ctrl+u` / pgdn pgup | half page |
| `L` | open everything under the cursor (at the root: the whole document) |
| `H` | close everything under the cursor, keep it open as an outline |
| `zo` `zc` | open / close this node and everything under it |
| `zR` `zM` | open / close everything |
| `z1`…`z9` | open the whole document to that depth |
| `y` / `Y` | copy the jq path / the value as JSON |
| `?` | help: every key, from `internal/tui/keys.go` |
| `q` | quit |

Planned: `/` search, `:` or `i` into the query bar, `K` to show the AST of the query (as nvim's
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
