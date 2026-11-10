# jqv

A fast terminal workbench for JSON: a vim-keyed tree of the document, with jq (gojq,
in-process) for taking it apart. Working name.

Early days: the tree viewer works, and the REPL (`jqv repl`, below) is where the query
bench is being built: lines of jq on the document, jq's manual inside, its own line editor.
Learning jq alongside it: [learn/](learn/README.md). See [docs/DESIGN.md](docs/DESIGN.md)
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
jqv repl testdata/people.json   # the REPL: lines of jq on the document (below)
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

## The REPL

`jqv repl file.json` (YAML too) runs lines of jq on the document. Each line runs on the
current `.` and is forgotten; `:push` keeps it as a stage, so `.` becomes what it gives.
`$root` is always the whole document, `$_` the last line's result. Errors come with a caret
and a hint for the usual mistakes (`. age`, `\(` in a regex, `.` after `X as $v |`).

| Command | What |
|---|---|
| `:push [expr]` (`:p`) · `:pop` · `:reset` | make the last line (or expr) a stage · take the last back · drop them all |
| `:stages` · `:program` | the stages with what each gives · as one jq program |
| `:tree [expr]` | the last line step by step, with what each step gives, and where it breaks |
| `:lex [expr]` · `:fmt [expr]` | how the last line is read into tokens · laid out over lines, a stage a line, brackets indented, in colour |
| `:pp [expr]` · `:! cmd` · `:!` | pretty-print the last result · pipe it into a program · into `[repl] pipe` |
| `:help` · `:help trim` · `:help jq` · `:help functions` | this list · jq's manual on a name, operator (`:help \|`) or section · its contents · every function |
| `:try N` · `:try N .` | run example N from the last `:help` on its own input · on yours |
| `:wq [expr]` · `:q` | quit, printing the program as a `jq` command (below) · quit |
| `:cls` | clear the screen |

On a terminal the line is jqv's own editor:

| Key | Action |
|---|---|
| `up` `down` | history, kept across sessions: this document's first, lines that failed left out |
| `ctrl+a` `ctrl+e` / `home` `end` · `ctrl+left` `ctrl+right` | start / end of line · word back / forward |
| `ctrl+w` · `ctrl+k` · `ctrl+u` · `ctrl+delete` · `ctrl+y` | cut word back · to end · to start · word forward · paste the cut back |
| `esc` · `ctrl+c` | empty the line · abandon it |
| `ctrl+l` | clear the screen |
| `F1` | `:help` for the word under the cursor; the line stays |
| `F2` | the last result into `[repl] pipe` (`bat -l json`), as a bare `:!`; the line you were typing comes back after |
| `F5` · `ctrl+up` `ctrl+down` | run the line up to the cursor · step back / on along the pipeline, running each stage |
| `F8` | the line laid out over lines, as `:fmt`; the line stays |
| `ctrl+d` | on an empty line: quit |

Flags: `--plain` uses the console's own line editing instead (piped lines always are);
`--show-keys` prints each key the editor gets, to see what a terminal or Zellij sends;
`--emit FILE` writes `:wq`'s command to a file too. History is
in `%LOCALAPPDATA%\jqv\history.jsonl` (`~/.local/state/jqv` elsewhere, `JQV_HISTORY` to move it),
in [hit](https://github.com/TimelordUK/hit)'s format.

`:wq` quits and prints what you built as a jq command (`jq '.[] | .people[] | .name'
people.json`), copied to the clipboard. With this in your pwsh profile, `jqr file.json` runs
the REPL, and after `:wq` the command is one up-arrow away:

```powershell
function jqr {
    $f = New-TemporaryFile
    try {
        jqv repl --emit $f.FullName @args
        $cmd = (Get-Content $f.FullName -Raw)
        if ($cmd) { [Microsoft.PowerShell.PSConsoleReadLine]::AddToHistory($cmd.Trim()) }
    } finally { Remove-Item $f.FullName -ErrorAction Ignore }
}
```

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

[repl]
pipe = "bat -l json"   # jqv repl: a bare :! sends the last result here
```

A missing file is fine. A mistake in it never stops jqv: the bad line is skipped and the problem
shows on the status line when jqv starts. `--print` ignores the config.

Tests: `./scripts/test.ps1` (what CI runs).
