# jqv

A fast terminal workbench for JSON: a vim-keyed tree of the document, with jq (gojq,
in-process) for taking it apart. Working name.

Early days: the tree viewer works; the query bench is next. Learning jq alongside it: [learn/](learn/README.md). See [docs/DESIGN.md](docs/DESIGN.md)
and [docs/WISHLIST.md](docs/WISHLIST.md).

Install (needs Go):

```
go install github.com/TimelordUK/jqv/cmd/jqv@latest
# or, where proxy.golang.org is blocked:
git clone https://github.com/TimelordUK/jqv; cd jqv; go build -o jqv.exe ./cmd/jqv
```

Use:

```
jqv testdata/orders.json
Get-Content big.json | jqv
jqv --print -d 3 testdata/orders.json
jqv --max-size 300 big.json     # default limit 100 MB
jqv ci.yml                      # YAML is translated to JSON on load (key order kept)
jqv --print-json ci.yml | jq .  # ...and written out as JSON for other tools
```

| Key | Action |
|---|---|
| `j` `k` `h` `l` / arrows | move; `h`/`l` close/open or go to parent/child |
| `enter` / space | toggle |
| `gg` `G` · `ctrl+f` `ctrl+b` · `ctrl+d` `ctrl+u` | top/bottom · page · half page |
| `ctrl+g` | where am I |
| `:123` · `:set …` · `:q` | go to row · an option (below) · quit |
| `[{` `]}` · `%` | start / end of the enclosing object · start of a block to its end and back |
| `{` `}` · `(` `)` | previous / next block at this level · previous / next element of this array (each opened as you land) |
| `L` / `H` | open whole subtree / fold everything below it (at the top: whole doc) |
| `zo` `zc` · `zR` `zM` · `z1`–`z9` | open/close subtree · everything · to depth |
| `y` / `Y` | copy jq path / value |
| `/` `?` `n` `N` | search forward / back, next / previous; lower-case ignores case; the cursor follows as you type; `"AE"` matches a whole key or string |
| `f` / `F` | finder: every match as a list, the tree following the selection / the last search as a list |
| `z/` | after a search: fold everything except the paths to the matches |
| `zz` `zt` `zb` | re-centre |
| `zl` | lock the cursor to the middle row, or unlock |
| `F1` | help: every key |
| `q` | quit |

## Options and config

Options are set for the session with `:set`, vim style, or for every session in
`config.toml`. F1 lists them, with the path the config is read from.

| Option | Short | What |
|---|---|---|
| `number` | `nu` | row numbers in a gutter |
| `relativenumber` | `rnu` | row numbers relative to the cursor |
| `scrolllock` | | keep the cursor on the middle row (same as `zl`) |
| `depth` | | how far the document is open at the start (`-d` on the command line wins) |

`:set nu` · `:set nonu` · `:set nu!` (toggle) · `:set nu?` (show) · `:set depth=2` ·
`:set nu rnu` · `:set` (what you've changed) · `:set all`

The config file is `%APPDATA%\jqv\config.toml` on Windows, `~/.config/jqv/config.toml` on
Linux and macOS, or wherever `JQV_CONFIG` points. Use the long names:

```toml
[set]
number = true
depth  = 2
```

A missing file is fine. A mistake in it never stops jqv: the bad line is skipped and the problem
shows on the status line when jqv starts. `--print` ignores the config.

Tests: `./scripts/test.ps1` (what CI runs).
