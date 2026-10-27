# jqv — notes for Claude

- Read `docs/DESIGN.md` before changing behaviour; keep it in sync when decisions change.
- New ideas go in `docs/WISHLIST.md` with the next free ID in their category (`C-` core,
  `Q-` query, `V-` views/output, `R-` recipes/history, `T-` TUI layout/keys/chrome).
  Never renumber; mark items `dropped` instead of deleting them.
- Reference wishlist IDs in commit messages (e.g. `Q-001: gojq wrapper with timeout`).
- The loaded document is never modified. Queries and views produce output; key order is kept.
- jq runs in-process via gojq. Nothing on the keystroke path spawns a process.
- The owner dogfoods on pwsh 7 / Windows Terminal daily; ergonomics decided by use beat theory.
- Bug or behaviour change from daily use: add a failing test/fixture first, then fix (DESIGN §7.1).
- TUI logic is tested through the pure model (`Update` with key messages), not a terminal.
- Golden files: `go test ./... -update` re-blesses; review the diff before committing.
- `./scripts/test.ps1` runs what CI runs (gofmt, vet, test, short fuzz).
- Default keys must avoid Zellij/tmux/Windows Terminal defaults (DESIGN §5).
