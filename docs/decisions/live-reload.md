# weft notices outside changes and reloads every 2 seconds

When pages or journals change outside weft (an agent appending to today's
journal, nvim, Obsidian, `git pull` in a shell), weft notices within about two
seconds and shows the new content without `R`. The page on screen updates in
place. An open editor is never changed: it shows a notice, and `Ctrl+S` merges
or asks as before.

Why: agents and other editors write this graph all day. Before this, weft
showed stale pages, links and todos until a manual reindex.

Out of scope: operating-system file-change events (`fsnotify`), a configurable
interval, turning polling off, refreshing open panels, reloading the editor
buffer, reloading the config, incremental indexing.

## Decisions

- **Polling, not file-change events.** Measured on the author's graph at
  `40e7f8c`: a full index build takes about 27 ms and a stat-only walk about
  7 ms. No new dependency. A later measurement of one poll gave about 0.9 ms.
- **The signature.** `graph.Signature` hashes each indexed file's
  graph-relative path, size and modification time, in walk order. Equal
  signatures mean no file was added, removed or changed, as far as `stat` can
  tell. Values are comparable only within one process. A failed `stat` counts as
  size -1, mtime 0.
- **One walk.** `walkGraph` is the single definition of which files are
  indexed (configured roots, dot-folders skipped, `.md` only).
  `BuildIndex` and `ScanSignature` both use it, so the poll can never disagree
  with the build about what exists.
- **The build takes its own signature,** in its walk, before any body is read.
  So the stored signature is never newer than the content: a write during the
  build shows up as a difference on the next poll.
- **Poll loop.** One tick chain per `App`, armed by the first successful
  index load, in `internal/views/reload.go`. A tick only re-arms the clock when
  there is no index, a sync is running, or a build is in flight. Otherwise it
  walks off the UI goroutine. The result is discarded if a build started
  meanwhile or the index changed (generation and index are compared). A
  difference starts one rebuild through the normal async reindex. `diskSig`
  advances only when an index load succeeds, or on the poll that started a
  build, so a failed build is reported once and not retried every 2 seconds.
- **Own writes do not rebuild twice.** Every index weft builds after its own
  write (leaving the editor with a save, capture, linkify, `.`, task toggles,
  sync, `R`) sets `diskSig` from that build, so the next poll sees no
  difference.
- **Refresh is like `R`, without the hint.** The page on screen is redrawn from
  the new index keeping scroll, row cursor and folds where the text allows;
  history is kept. New or changed index warnings still show.
- **Overlays are untouched.** Picker, search, backlinks, todos, agenda,
  calendar, on-this-day, tree and help keep their rows until reopened, as after
  `R`.
- **Editor.** On each index load the open editor gets the new index, so `[[`
  and `#` completion sees pages created meanwhile, and its file is read again:
  if it differs from what the editor last read or wrote, the status line shows
  "changed outside weft — ^S merges or asks". The notice clears on save, on
  reloading the other version in the clash prompt, or when the disk matches the
  baseline again. The buffer and its baseline are never changed.
- **Config is read once.** `.weft/config.toml` changes still need a restart.
- **Read-only.** Polling and reloading write nothing.

## Traps

- A same-size write inside the filesystem's modification-time granularity,
  after the walk's `stat`, is missed until the next change or `R`. On
  nanosecond-timestamp filesystems this is negligible.
- Edits made through a symlinked `.md` file's target do not change the link's
  own `stat`, so they are not noticed.
- `E` (`$EDITOR`): a poll result queued behind the external editor can start
  one build that the build after the editor exits then supersedes. One extra
  rebuild, correct result.
- The synchronous `reindex()` racing an in-flight async build is unchanged: a
  stale async result may land. The next poll repairs it within one interval.
- A plain `Ctrl+S` that stays in the editor does not rebuild the index, so the
  next poll rebuilds it once, by design.
- A failed rebuild started by `R` or a sync pull leaves the last-seen disk state
  alone, so the next poll retries it once and the failure is reported twice.
  A failed rebuild the poll itself started is reported once.
