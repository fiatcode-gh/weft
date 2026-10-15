# Vim mode is opt-in and sits on the same editor

With `editor = "vim"` in the user config (`$XDG_CONFIG_HOME/weft/config.toml`)
the in-app editor is modal: normal, insert, visual and visual-line modes, counts,
the core motions and operators, text objects, one register, undo and redo, dot
repeat, search, and the ex commands to save and leave. weft's own editor keys
keep working. Without the setting nothing changes.

Why: the goal is an in-app editor good enough to replace nvim for note work. The
modeless editor makes vim users reach for keys that do something else.

Out of scope: named registers, marks, macros, `*` and `#`, `:s` and other ex
commands beyond the list, ranges, `:set`, folds in the editor, multiple windows,
replace mode (`R`), block visual, `gq` and `=`, insert-mode `Ctrl-W`, `Ctrl-O` and
`Ctrl-R`, vim keys in the read view, a key to switch modes while weft runs,
writing the user config from weft. Also left out: objects, `p`, `r` and `/` in
visual mode, `r<Enter>`, normal-mode `~`, `Y`, `H M L`, `Ctrl-F`/`Ctrl-B` as page
keys, space, `Enter` and `Backspace` as normal-mode motions, an operator with a
search or `Ctrl-D`/`Ctrl-U`, vim's exclusive-to-linewise rule, and persisting
anything about the vim session.

*Superseded in part by `public-readiness.md`: weft now writes the user config
once, on first run, to save the `graph` line. It still never writes `editor`.*

## Decisions

1. **Opt-in and plumbing.** `internal/userconfig` (`Path`, `Load`) reads the one
   key `editor`. `cmd/weft` loads it after the layout (`loadUserConfig`) and an
   error prints `weft: <err>` and exits 2; `weft doctor` does not read it. The
   result reaches the editor as `App.SetVimEditor`, called once before `Run`.
   `App.enterEditor` calls `EditorView.enableVim` when it is set. `views.New` and
   `NewEditorView` keep their signatures. `EditorView.vim` is a `*vimState`; nil
   is the modeless editor and every modeless path is as before.
   *Superseded in part by `public-readiness.md`: the user config now has a
   second key, `graph`, and `weft doctor` reads the file to find the graph when
   no flag or env names one.*
2. **Routing.** `EditorView.Update` keeps the clash and exit prompts first. The
   old key body, minus the `Ctrl+R` toggle, is `editKey`. Modeless: the toggle,
   then `editKey`. Vim: `vimUpdate`. The ex line owns the keys while it is open;
   else with a prompt or the find bar open the key goes to `editKey`; else by mode.
3. **Grammar.** `parseVim` (`editor_vim_parse.go`) is a pure parser over key
   tokens: `[count] (motion | command | operator [count] (motion | object | doubled operator))`.
   `0` continues a count but never starts one; counts multiply and saturate at
   99999. A token that cannot continue the command drops it and nothing runs.
   `vimToken` names a key: its text when plain, else `msg.String()`; the arrow,
   `home` and `end` keys alias to `h j k l 0 $` in normal and visual mode.
4. **Clamp and the final line.** After every normal or visual key `vimSettle`
   dismisses completion, clamps the cursor to `LastLine` and `LineLast`, and
   syncs the view and find. The empty line after a final line break is never a
   resting place outside insert mode. A modeless buffer selection skips the clamp.
5. **Undo marks.** `buffer.UndoMark` and `buffer.Squash` make every normal or
   visual change one undo step. Entering insert stores `insMark` before the
   entering change, so `cw` plus the typing is one step. `outsideInsertStep`
   makes undo, redo, a merge on save and a clash reload their own steps inside an
   insert session.
6. **`j` and `k`** move as `↓` and `↑` do (row by row as the editor wraps); as
   operator motions they are logical lines. `Ctrl-D`/`Ctrl-U` move half the
   text height and keep the cursor's screen row. `$` sticks to the line end for
   later vertical moves.
7. **Motion kinds** decide operator ranges: exclusive, inclusive or linewise,
   as vim does. `w` ending across a line break stops at the line end; `cw` on a
   non-blank is `ce`; `l` under an operator may reach the line end; `G`, `gg`
   and `N%` land on the first non-blank. A motion with no target cancels the
   operator and adds no undo step.
8. **Text objects** are pure buffer functions. Words and quotes stay on a line;
   brackets go outward across lines with nesting; `ip`/`ap` are runs of blank or
   non-blank lines.
9. **Operators and shortcuts.** `x X s S D C` are operator plus motion aliases
   (`vimShortcuts`). `>>`/`<<` with count 1 are the bullet-with-children
   `Indent`/`Outdent`; with a larger count they shift lines (`ShiftLines`). `o`
   and `O` go through `buffer.OpenLine`, which continues a `- ` bullet.
10. **Registers.** One unnamed register, the App's clip. Yank, and delete or
    change of a non-empty range, call `toRegister`, which also returns the OSC 52
    clipboard command. `p`/`P` put through `buffer.Put`. `Ctrl+C/X/V` keep their
    modeless behaviour on the same register.
11. **Dot repeat.** `vimChange` stores the command, the inputs of its insert
    session (keys and bracketed pastes) and, for visual changes, a `vimExtent`.
    `.` runs the command through the same executor, feeds the inputs through
    `editKey` with recording off, and leaves insert as `Esc` does, in one undo step.
    A count before `.` replaces the stored one. A count on `i a I A o O` replays
    the session's input at `Esc`.
12. **Visual.** `vimState.anchor` holds the fixed end; the buffer keeps no
    selection in visual mode. `selection()` returns the visual range, else the
    buffer's own. `d x c y > < J ~ u U` act on it (`~ u U` through `CaseRange`).
13. **Ex line.** `:` opens `vimState.ex`; the status row shows `:` plus the text
    and the terminal cursor follows it. `:w`, `:q`, `:wq`, `:x`, `:q!`, `:<n>`,
    `:preview`; anything else is a notice. Saving and leaving are only an
    `EditorResult`, so the guarded save, merge and prompts are unchanged. `ZZ` is
    `x` and `ZQ` is `q!`.
14. **Search.** `/` and `?` open the existing find bar with a direction
    (`findBar.dir`). While typing, the current match is the first strictly after
    (or before) the origin, wrapping. `Enter` lands on it and records the query and
    direction; `Esc` returns to the origin. `n`/`N` use `buffer.Search`.
15. **`Esc`.** Insert: squash the session, one grapheme left, normal. Visual:
    normal. Normal with a pending command: clear it. Normal and idle: `editKey`,
    so a clean editor exits and a dirty one asks, as modeless.
16. **weft keys.** Normal mode sends `vimPassthrough` and every `extendKeys`
    chord to `editKey`, clearing the pending command. Insert mode sends every key
    there except `Esc` and the undo and redo keys. `Ctrl+R` is redo in normal and
    insert mode, nothing in visual; `:preview` is the look switch. `Ctrl-D` and
    `Ctrl-U` are vim scrolls in normal and visual mode and the modeless delete
    keys in insert mode.
17. **Status line.** Left: the existing text plus `-- INSERT --`, `-- VISUAL --`
    or `-- VISUAL LINE --`. Right, first that applies: save error, notice, the
    pending tokens, date prompt, find hints, outside notice, then the vim hints.
    Modeless is unchanged.
18. **Cursor shape.** `Cursor()` asks for a bar in insert mode only; other
    states keep the default block. `quietStdout` does not filter the cursor-style
    sequence (a test pins that).
19. **Mouse.** Not ready while the ex line is open. A click drops a pending
    command and clamps; a visual click leaves visual; the wheel
    keeps visual mode; an insert click places the cursor and stays in insert mode
    (`editor_mouse.go`).
20. **Paste.** Insert: as before, and recorded. Normal: drops a pending command,
    inserts at the cursor, then `vimSettle`. Visual: leaves visual first. Ex
    line: ignored.
21. **Help.** `vimHelp` is a hand-written group after `findHelp` in
    `buildHelpGroups`. Modeless editor and find rows are unchanged. Help stays two
    columns at 130.
22. **Cost.** Per-key passive work (parse, status, clamp) is O(1) or O(cursor
    line). One-shot motions and searches cost in proportion to what they scan.
    `TestEditorSpeed10000`, `TestEditorWorkIsWindowBounded` and
    `TestVimWorkIsWindowBounded` bound it.

## Traps

- A new editor key must be classified for vim normal mode: weft passthrough
  (`vimPassthrough` or `extendKeys`) or a vim command. An unclassified key is
  silently dropped there.
- Any new whole-buffer swap (reload, merge, replace) in an editor that can be
  in insert mode must go through `outsideInsertStep`, or the swap joins the
  insert session's undo step.
- `selection()` is the only selection source for drawing and for the reveal
  set. Do not call `buf.Selection()` there: visual mode keeps no buffer selection.
- A new insert-mode chord that must not replay on `.` goes in `vimUnrecorded`.
- `TestEditorVimStatusGolden` is pinned to the source look so both suite runs
  match; do not switch it to live preview.
- Vim code never writes disk. Saving and leaving are an `EditorResult` for the
  App, which owns the guarded save.
- Help rows follow the help limits: keys at most 15 cells, descriptions at most
  31 characters, or the two-column help outgrows 130 columns.
- Parked: `f`/`t`/`r` take one rune (`vimArgToken`), so a grapheme typed as several runes (a ZWJ emoji, a decomposed accent) is dropped.
