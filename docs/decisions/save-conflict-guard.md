# Save never overwrites outside changes

Saving in the in-app editor compares the file on disk with the content the
editor opened or last saved. If nothing changed, it saves as before. If the
outside changes and the user's edits touch separate lines, it writes the
merged text and the buffer shows it. Anything else stops the save and asks:
overwrite, reload or keep editing. Linkify uses the same content check.

Why: agents append to today's journal while it is open in weft, and `Ctrl+S`
used to write the buffer without looking at the disk, so those entries were
lost.

Out of scope: reloading the editor while typing, a diff view or per-hunk
resolution, the `$EDITOR` hand-off, and git conflicts.

## Decisions

- **Merge first, ask only on a real clash.** Asking on every outside change
  was rejected: the common case (an agent appends while the user edits above)
  would prompt every time, and both of its choices lose one side.
- **Git's adjacency rule, with one exception.** Changes on the same or
  adjacent lines clash. When both sides only inserted lines at the same spot,
  both blocks are kept, the user's first. Without the exception, the user and
  an agent both appending to a journal, the most common case, would always
  clash. Identical changes or insertions are kept once; overlapping but
  different insertions are both kept, so a line can appear twice. Nothing is
  lost.
- **Compare content, not modification time.** A touched mtime alone is not a
  change.
- **Hand-written line merge in `internal/merge`.** `go-udiff` was rejected: its
  diff and merge work on characters and its merge rules are not git's line
  adjacency rule. An edit distance over 1000 lines on either side counts as a
  clash, which bounds memory; the cost is a prompt instead of a merge.
- **Every replacing write is content-checked.** `edit.WriteFile` became the
  private `writeFile`; `WriteFileIfUnchanged` is the only exported way to
  replace a file, so a new write feature cannot bypass the check.
- **Reload is not offered when the disk content cannot be loaded faithfully**
  (CRLF, tabs, over 10000 lines), matching the editor's refusal to open such
  files. From a clash reached through save-and-exit, overwrite saves and
  exits; reload and keep editing stay in the editor. *Superseded in part by
  `editor-core.md`: the editor now opens any file, so reload is offered
  whenever the file exists.*

## Traps

- An empty journal is `""` on disk but the editor's content is `"\n"`. Merge
  inputs go through `mergeInput`, which turns newline-only text into empty
  text; otherwise both sides appending to an empty journal would clash.
  *Superseded by `editor-core.md`: the editor's content is now exact and
  `merge.Text` merges the final newline as its own change, so `mergeInput` is
  gone.*
- The guard re-reads the file just before the rename. A write by another
  program between that read and the rename is not detected; weft cannot lock
  files.
