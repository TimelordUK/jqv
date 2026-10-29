# jqv

A fast terminal workbench for JSON: a vim-keyed tree of the document, with jq (gojq,
in-process) for taking it apart. Working name.

Early days: the tree viewer works; the query bench is next. See [docs/DESIGN.md](docs/DESIGN.md)
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
```

| Key | Action |
|---|---|
| `j` `k` `h` `l` / arrows | move; `h`/`l` close/open or go to parent/child |
| `enter` / space | toggle |
| `gg` `G` · `ctrl+f` `ctrl+b` · `ctrl+d` `ctrl+u` | top/bottom · page · half page |
| `ctrl+g` | where am I |
| `:123` · `:set nu` / `rnu` · `:q` | go to row · row numbers · quit |
| `L` / `H` | open whole subtree / fold everything below it (at the top: whole doc) |
| `zo` `zc` · `zR` `zM` · `z1`–`z9` | open/close subtree · everything · to depth |
| `y` / `Y` | copy jq path / value |
| `/` `?` `n` `N` | search forward / back, next / previous; lower-case ignores case |
| `z/` | after a search: fold everything except the paths to the matches |
| `zz` `zt` `zb` | re-centre |
| `F1` | help: every key |
| `q` | quit |

Tests: `./scripts/test.ps1` (what CI runs).
