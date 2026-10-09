# Tasks have dates and priorities, and `A` is an agenda of them

weft reads Logseq `SCHEDULED:` / `DEADLINE:` lines and `[#A]`–`[#C]`
priorities and styles them in the read view, live preview and source look. `A`
opens an agenda overlay (Overdue, Today, Upcoming within 7 days). `x` in `T` and
`A` marks a task done or undoes it. In the editor, `Alt+S` / `Alt+E` set a date,
`Alt+P` cycles the priority, and `Ctrl+T` follows one marker cycle.

Why: tasks written in Logseq carried dates weft showed as plain text, and the
dashboard could not say what was due. Files stay plain markdown: weft writes
only a marker, a priority or a stamp line the user asked for, never metadata.

Out of scope: recurring tasks (a repeater is kept and has no effect), LOGBOOK
writes, rescheduling from the agenda, marker changes other than done/undo from
`T`/`A`, tag grouping, a configurable window, a calendar, dates on non-task
bullets, inline dates, and `SCHEDULED:` on a page's first line.

## Decisions

- **D1 One grammar in `internal/graph`.** `task.go` (`ParseTaskPrefix`,
  `NextMarker`, `SetTaskMarker`, `NextPriority`, `SetTaskPriority`, `MarkTask`,
  `TaskBlockAt`), `taskdate.go` (`ParseStampLine`, `ParseDateInput`,
  `StampText`) and `agenda.go` (`BuildAgenda`) are called by the index, the
  render package, the painter, the buffer and the views. `LogbookStartRe` and
  `LogbookEndRe` moved to `logbook.go` as the one definition.
- **D2 Index semantics stay put.** `todoRe` and `IsOpenTask` are unchanged, so
  ordinals and `Result.Tasks` stay aligned. Relocation compares the
  whitespace-normalised tail after the marker (`TaskTail`).
- **D3 One ownership walker.** `parseBody` and `TaskBlockAt` share it. Every
  bullet ends the current owner, so a date never leaks to a child or a parent;
  fenced lines and LOGBOOK lines are never stamps; the first stamp of each
  kind wins.
- **D4 Stamp grammar.** A whole line, `SCHEDULED:` or `DEADLINE:` in upper case,
  a real calendar date, an optional weekday (ignored), a time 00:00–23:59 and a
  repeater. Anything else is plain text.
- **D5 `Ctrl+T` cycle.** Any open marker → `DONE`; `DONE`, `CANCELED`,
  `CANCELLED` → plain; plain → `TODO`. The priority is kept. A cancelled task
  is closed like a done one, so one press clears it rather than reopening it.
- **D6 Priority.** `NextPriority` runs "" → A → B → C → "". Adding inserts
  ` [#X]` after the marker; removing deletes the gap and the tag.
- **D7 Sentinels only.** Priority and stamps reuse the task-marker sentinel
  family, so no new id kind or id base. Theme values are `Scheduled` (cyan,
  italic), `Deadline` (bright red, italic) and `Priority` A/B/C (bright red,
  bright yellow, bright blue; bold).
- **D8 Agenda model.** `BuildAgenda` uses the earlier of the two dates (earliest
  time when equal). Overdue is before today, Today is today, Upcoming is up to
  `AgendaDays` (7) days ahead, inclusive. Order: section, date, timed first,
  time, priority, page, line.
- **D9 `x` write path.** `App.markTask` refuses during sync, reads a snapshot,
  relocates with `graph.MarkTask`, writes with `edit.WriteFileIfUnchanged`,
  reindexes and rebuilds the panel. Rows marked done stay struck in `doneRows`
  until the overlay closes; `x` on one restores the earlier marker, `Enter`
  opens the page.
- **D10 Editor keys.** `Alt+S`, `Alt+E`, `Alt+P`: unbound, and they use the same
  ESC-prefix encoding as the existing `Alt+b/f/c/l/u/d`. `Alt+D` is delete-word.
- **D11 Date prompt.** A modal one-row bar in the find bar's slot. Accepts
  `YYYY-MM-DD`, `today`, `tomorrow`, `+Nd`, `+Nw` (N is 1–4 digits) and a
  weekday (the next one after today); empty clears. An invalid entry keeps the
  prompt open. Each change is one undo group.
- **D12 Clock.** `App.nowFunc` feeds the agenda, and `EditorView.now` feeds the
  prompt, so tests inject a fixed day.
- **D13 Agenda width.** The agenda's inner width is capped at 92 cells
  (`agendaInnerWidthMax`), the dashboard's at 80. The agenda row carries a
  `scheduled … · deadline … · page` label beside the task text; at 80 cells the
  label was truncated on wide terminals.
- **Perf.** Medians over 5 runs of 20 iterations, ns/op, against `f4c1511` on a
  10000-line page, final tree: Open +3.3%, Keystroke −3.9%, PageDown −2.0%,
  Reveal −1.6%, LiveOpen −3.0%, LiveKeystroke −3.8%, LivePageDown −6.6%,
  LiveReveal −10.4%. All within the +10% bound. Source-look Reveal is noisy on
  single runs (one early run read +14.6%; alternating reruns put both trees
  around 260k).

## Traps

- A stamp sentinel is one token, so below its width it is cut at the edge like
  a long wiki link.
- Stamps are styled anywhere outside fences and LOGBOOK, but only task-owned
  ones are indexed.
- The painter leaves stamps in top-level indented code unstyled while the read
  view styles them.
- A fence inside a LOGBOOK block still toggles the index's fence state, as
  before.
- A degenerate `- TODO [#A]` line: the index text `[#A]` and the transform's
  priority A disagree, so relocation fails safe and nothing is written.
