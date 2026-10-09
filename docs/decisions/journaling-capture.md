# Journals start from a template, `c` captures a line, `C` and `O` navigate by date

A page named `Journal Template` seeds every journal weft creates: `.`, `E`,
capture and the editor's first save. `c` opens a one-line prompt that appends a
bullet (or `- TODO` with `Tab`) to today's journal, over the read view or over
the todos, agenda, backlinks, calendar and on-this-day overlays. `C` opens a
Monday-first month grid that marks days with a journal and opens one with
`Enter`. `O` lists the journals from a week ago, a month ago and the same day
in earlier years, with previews.

Why: a day's journal always started blank, getting a thought into it meant `.`, `e`,
typing and saving, and there was no way to see or reach a journal by
date other than `<` / `>`. Files stay plain markdown: the template is copied
as-is and capture writes one line the user typed, never metadata.

Out of scope: template variables or expansion, per-weekday templates,
templates for non-journal pages, capture into a day other than today, capture
from the picker or search, editing or deleting from the calendar, week
numbers, a configurable `O` offset list, and a template watcher.

## Decisions

- **D1 Template resolution.** `journalTemplateName` is `Journal Template`.
  `App.journalTemplate` resolves it with `idx.Resolve` (so `pages/` or any
  indexed location works) and reads the file with `readSnapshot` every time it
  is used, so an edit applies to the next new journal without a reindex. No
  page, or a page whose file has vanished, gives an empty template; a read
  failure is an error and nothing is created.
- **D2 One creation function.** `App.createJournal` is the only function that
  creates a journal file for `.` and `E`; capture and the editor's first save
  create through their own guarded write (D3, D4). `createJournal`
  writes with a template through
  `edit.WriteFileIfUnchanged` against an empty snapshot, so a file that appears
  meanwhile is left alone (`edit.ErrChanged` is swallowed, `created` false).
  With no template it still calls `edit.EnsureFile`, so the empty-file path is
  byte-for-byte what it was. `createJournalAndReindex` wraps it for `.` and `E`.
- **D3 Editor `disk` and `baseline`.** A new journal opens with the template as
  buffer content and as `baseline`, while `disk` stays the empty,
  non-existent snapshot. So an untouched template is not dirty (leaving does
  not prompt, nothing is written), and the first save is still a create-only
  write that detects a file made in between.
- **D4 Capture is one guarded write.** `App.appendToJournal` reads a snapshot,
  builds the content (the file's, or the template's when the file is missing)
  with `graph.AppendLine`, and writes with `edit.WriteFileIfUnchanged`. On
  `edit.ErrChanged` it re-reads and tries exactly once more; a second change
  writes nothing and the prompt stays open with the text and a retry message.
  `AppendLine` keeps the file's line-break style (`\r\n` or `\n`) and adds a
  missing final break first. The target page is fixed when the prompt opens, so
  a prompt left open across midnight still writes to the day it announced.
  Capture is refused while a sync runs (`syncBusyMsg`).
- **D5 Prompt layering and key order.** `CapturePrompt` is not an `Overlay`;
  `App` draws it over whatever is on screen (`spliceBottom`). `App.Update`
  routes keys in this order: editor, capture prompt, open overlay, read-view
  keys. The prompt takes every key but `ctrl+c`, which still quits. Because it
  sits above overlays, closing it returns to the overlay beneath it.
- **D6 `overlayCapture` emitters.** Todos, agenda, backlinks, calendar and
  on-this-day return `overlayCapture()` for `c`, and `App` calls `openCapture`.
  The picker and search keep `c` as typed text, and help has no use for it.
- **D7 In-panel feedback.** A status-bar hint is hidden behind an overlay, so
  `submitCapture` also calls `SetError` on the active overlay when it
  implements `panelFeedback`, in addition to the hint. The message is cleared
  on the overlay's next key.
- **D8 Calendar grid.** `monthGrid` lays weeks out Monday first. Each day is a
  5-cell group: `[` `]` around the cursor day (selection style), a `•` after days
  that exist in `idx.Journals()`, bold underline for today. `PgUp`/`PgDn` use
  `graph.AddMonthsClamped`. `Enter` opens `YYYY-MM-DD` and creates nothing; a
  day without a journal opens as the empty page.
- **D9 On-this-day offsets.** `graph.OnThisDay` yields one week back
  (`AddDate(0, 0, -7)`), one month back (`AddMonthsClamped`, so Mar 31 gives
  Feb 28 or 29) and every earlier year's same month and day, newest first,
  labelled `1 week ago`, `1 month ago` and the year. Missing dates are left
  out. A February 29 has no entry in years without one; `02-28` is not
  substituted.
- **D10 No fixture change.** Existing golden and fixture graphs have no
  `Journal Template` page, so `.` and capture there behave as before. The only
  goldens that changed are `TestHelpGolden`, plus the new `TestCalendarGolden`
  and `TestOnThisDayGolden`.

## Traps

- A template created outside weft needs a reindex (`R`) before `idx.Resolve`
  finds it; once indexed, edits to its content are picked up without one.
- Lists under the prompt are not refreshed after a capture: the reindex runs,
  but an open todos or agenda panel keeps its rows until it is reopened.
- `App.editCurrent`'s branch for a file that vanished after indexing stays
  template-free (`edit.EnsureFile`); it is a narrow race, not a creation path.
- `idx.ByName` prefers `pages/` for a date-named page, so `NewOnThisDay` reads
  `journals/<file>` directly instead of resolving the name.
- The calendar's `journals` slice is a snapshot taken when it opens; a capture
  made from it shows its new mark only after reopening.
