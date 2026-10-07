# Move to the Charm v2 libraries

weft now runs on Bubble Tea v2, Lip Gloss v2, Bubbles v2 and Glamour v2
(`charm.land/*/v2`), with teatest v2 for tests. Keys, views, colours and the
terminal traffic stay as they were; the few visible differences are listed in
the changelog.

Why: the next units need v2. The editor rewrite needs reliable modifier keys
(kitty keyboard protocol), and the UI shell needs mouse support. The v1 line
gets no new features. The move was done alone, with no new behaviour, so any
regression traces back to the migration.

Out of scope: turning on kitty keyboard or mouse input, replacing the editor's
textarea, a `/v3` module path, a release.

## Decisions

- **The whole stack moves together**, Glamour included, so weft depends on one
  Charm generation and no longer imports `termenv`.
- **weft filters seven sequences out of its own output** (`quietStdout` in
  `cmd/weft/terminal.go`). Bubble Tea v2 always sends two DECRQM probes and a
  kitty keyboard query, and switches on kitty keyboard and modifyOtherKeys
  mode, with no option to stop it. When weft quit before the replies arrived,
  they landed in the shell: bash once tried to run `2026` as a command.
  Dropping the sequences keeps the terminal traffic weft had under v1. Rejected:
  `WithInput(nil)` (no keyboard at all) and forking Bubble Tea.
- **Colour policy lives in `render.ColorProfile`.** Any non-empty `NO_COLOR`
  turns all styling off; a colour terminal renders in TrueColor, since weft's
  own colours are ANSI 0-15 and pass through unchanged. `colorprofile` alone
  was rejected because it only honours a boolean `NO_COLOR`.
- **The textarea was migrated, not replaced.** editor-core replaces it next, so
  this unit only restores v1 behaviour: v1 key bindings, a plain reverse cursor,
  the lifted line cap, bracketed paste into the editor only.
- **Cursor placement is one pass.** In Bubbles v2 each one-line cursor move
  costs time proportional to the cursor's row, and weft walked the cursor line
  by line, so opening a 4000-line page took 7 s. `moveCursorTo` reloads the
  buffer from the target line and inserts the lines above it, which leaves the
  cursor in place in linear time. Paging steps the cursor directly instead of
  sending one textarea update per row, because every update redraws the whole
  buffer.
- **Accepted slowdown:** typing and paging on very long pages stay slower than
  v1 (about 0.03 s per key at 1000 lines), because the v2 textarea redraws the
  whole buffer per update. editor-core removes the textarea, so a workaround
  here would be wasted. *Superseded by `editor-core.md`: the textarea, its
  cursor-placement workaround and this slowdown are gone; a 10000-line page now
  opens in about 7 ms and takes about 2 ms per key.*
- **Glamour v2 hyperlinks are stripped**: clickable links would be a new
  feature.

## Traps

- Lip Gloss v2 `Style.Width` includes the border; bordered panels go through
  `renderBordered` to keep v1 widths.
- Lip Gloss v2 `Render` always emits ANSI; downsampling happens in the Bubble
  Tea renderer. Tests strip escapes with `plain()` and keep `NO_COLOR=1`.
- Glamour v2 hard-wraps over-wide words, which split weft's padded link
  sentinels; leftover pad runes are deleted so row counts stay aligned.
- Bubble Tea v2 reads one key per character. v1 merged characters that arrived
  together into one key and dropped it, so a fast-typed or `tmux send-keys`
  picker query stayed empty; v2 fixes that.
- `TestProgramOptionsSilenceTerminalProbes` pins the filter to Bubble Tea
  v2.0.10. Re-run it on every Bubble Tea upgrade.
- Smoke-run the TUI under `env -i`: the agent harness sets `NO_COLOR`, `CI` and
  `TERM=dumb`.
