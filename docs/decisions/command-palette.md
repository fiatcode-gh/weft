# `:` opens a command palette of every read-view action

`:` in the read view lists every action with its key, in help order. Typing
filters the list, `Enter` runs the action exactly as its key would, `Esc`
closes. The key handling, the help page and the palette all come from one
action table.

Why: the read view has about 30 keys. Its key handling was a long `switch`, the
help page a separate hand-written list, and a palette would have been a third
list that drifts. New users also had no way to find a feature without reading
the help page.

Out of scope: the palette in the editor, prompts or panels, editor commands,
actions with arguments (the picker opens a page by name), recently used
ordering, custom key bindings, the sidebar (11b), mouse (11c).

## Decisions

- **One table.** `readActions` in `internal/views/actions.go` is a slice of
  `action{keys, key, title, group, palette, run}`. `keys` are the key strings
  that run it, `key` and `title` are what help and the palette show (one string,
  so they cannot differ), `group` is the help group, `palette` says whether the
  palette lists it, `run` does the work on `*App`. `readKeys` maps a key string
  to its action. There is no id field: tests name an action by its key.
- **`init()`, not variable initializers.** `readActions`, `readKeys`,
  `helpGroups`, `paletteActions` and `paletteLabels` are assigned in one
  `init()`, in that order. The `:` action's `run` calls `NewPalette`, which reads
  `paletteActions`, which comes from `readActions`. As package-level
  initializers that is an initialization cycle and does not compile.
- **Dispatch is one lookup.** `App.Update` ends with `readKeys[key]` and calls
  `run`. The keys handled before it are unchanged: the loading state, the
  editor, capture, rename, and overlay routing. Each `run` is the old case body,
  so no key changed behaviour. `.`, `e` and `E` are wrapped in `unlessSyncing`,
  which refuses with `blockIfSyncing`'s hint; `F2` checks inside `openRename`.
- **Shared keys.** One action may own several keys (`j` and `↓`, `q` and
  `ctrl+c`). A key string belongs to exactly one action.
- **Paired help rows.** `j / k`, `g / G` and `n / N` are two actions. The first
  owns the help row; the second has an empty `key` and `title`, follows it in
  the same group, and shares the row. Neither is in the palette.
- **The `esc` row.** `esc  close overlay` is a table entry with no keys, no
  `run` and `palette: false`. It exists only so help keeps the row between `?`
  and `q`. It is the only help-only entry.
- **Help layout.** `buildHelpGroups` builds the read-view groups from the table
  in the order Browse, Links, History, Open, Sync, Edit, Editor, Find, Maintain,
  Sidebar, Help / quit. Editor and Find stay hand-written (`editorHelp`, `findHelp`)
  because the editor owns those keys; they sit between Edit and Maintain. The
  help page gains one row, `:  command palette`, kept short so the help page
  stays two columns at 130 wide.
- **Palette contents.** The 30 actions with `palette: true`, in table order,
  including the six sidebar entries (`1`-`5` and `|`).
  Left out: `j/k`, `ctrl-d`, `ctrl-u`, `g/G`, `n/N` (the palette closes onto the
  same screen, so movement does nothing visible), `:` itself, and the `esc`
  row. Actions on the cursor row (`enter`, `tab`) are listed.
- **Filter.** `fuzzy.Find` over the label `key + " " + title`, the same library
  as the picker, so a query matches either the key or the name. A blank query
  lists everything.
- **Run.** `Enter` returns `overlayResultRun` with the action. The App sets
  `a.active = nil` and then calls the action's `run`. Closing first means a
  refusal (sync running, no file to rename) shows in the status bar as it does
  for the key, a panel action opens its panel on the read view, and `enter` and
  `tab` act on the read-view cursor row, which the palette never moved. `q`
  quits.
- **`:` scope.** The read-view dispatch is the only place `:` is a key. In the
  editor, prompts and panels the key goes to their own `Update` first, so it
  stays typed text or does nothing.
- **Drift tests.** `TestReadViewKeysOnlyThroughTable` fails when a read-view key
  is handled outside the table. `TestActionsHaveHelpRows` fails when an action
  has no help row. `TestActionTableShape` pins the table's structure.

## Traps

- A new read-view key is a table entry. A `case` in `App.Update` is caught by
  the drift test.
- A long help title or key widens the help page's right column and can drop it
  to one column at the golden's width.
