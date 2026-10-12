# A graph declares its layout in `.weft/config.toml`

Where pages and journals live is set by an optional `<graph>/.weft/config.toml`.
With no file, weft uses the Logseq layout (`pages/`, `journals/`,
`YYYY_MM_DD`). Pages can sit in folders, journals are found by a filename
pattern, and `[[folder/Page]]` picks a page by its path. An Obsidian vault opens
with a four-line config. The README, Configuration, lists the keys.

Why: the layout was hard-coded in about eight places, folders were skipped with
a warning, and journals were named `YYYY-MM-DD` while the author's graph and the
`weft-worklog` skill link them by filename stem (105 of the 153 unresolved links
on the real graph at the time). weft still writes no structure metadata into
files, and never writes the config.

Out of scope: reading `.obsidian/` or `logseq/config.edn`, a user-level config,
an ignore list, frontmatter and aliases, date tokens other than `YYYY`/`MM`/`DD`,
rewriting existing links, live reload, a folder browser.

## Decisions

- **One layout definition.** `internal/layout` owns the config, the journal
  pattern and new-page placement. `graph.BuildIndex`, `views.New`, `doctor.Run`
  and `cmd/weft` all take a `layout.Layout`; the index keeps it as `Index.Layout`.
- **Strict config.** `layout.Load` returns `<config path>: <problem>` for an
  unreadable file, a TOML error, a wrong type, a repeated key, an unknown key
  (`unknown key "x"`), a folder outside the graph, a bad `namespace_files` and a
  bad pattern. `cmd/weft` prints `weft: <err>` and exits 2, in the TUI and in
  `weft doctor`. There is no fallback to defaults. A missing file is the default;
  a missing configured folder indexes nothing and is not an error. The TOML
  library is `github.com/BurntSushi/toml`, chosen for `MetaData.Undecoded`
  (unknown keys) and line numbers in errors.
- **Pattern rules.** `YYYY`, `MM`, `DD` are tokens, everything else is literal,
  `/` separates segments, tokens may repeat (repeats must agree), and the last
  segment must hold all three tokens so a name maps to one date. No segment may
  be empty, `.`, `..` or start with `.`. The date must be a real calendar day.
- **Journal identity is pattern plus date; the name is the filename stem.** A
  file is a journal when it lies under `journals_dir`, its remainder matches the
  pattern and the date is real. Any other `.md` under `journals_dir` is a page.
  The name is the stem as is, so the default layout names a journal
  `2026_10_12`. This is the breaking change: `[[2026-10-12]]` no longer opens it.
  Everything by date (calendar, on-this-day, `<`/`>`, `.`, capture, template)
  goes through `Index.Journals`, `JournalOn`, `JournalDateOf` and `JournalPath`.
- **Walk rules.** The roots are the outermost of `pages_dir` and `journals_dir`
  (one root when one contains the other; pages first otherwise). Dot-folders are
  skipped, except the pages dir, the journals dir and their ancestors. Only `.md`
  files are read. A journals folder inside the pages folder is walked once.
  Dot-files such as `.x.md` are indexed. A symlinked pages or journals root is
  followed; symlinked subfolders are not.
- **Resolution order.** A link target resolves by exact name, then by name
  ignoring case, then by path: two or more segments, matched against the page's
  graph-relative path without `.md`, on whole segments, ignoring case. So
  `[[pages/2026_05_24]]` reaches a page that a journal of the same name hides.
  Path candidates come from a map keyed by the lower-cased stem, not a scan.
- **Winner rule.** In every group (same name, same folded name, several path
  matches): a journal first, then fewer `/` in the path, then the smaller path.
  Each clash is an index warning, so `weft doctor` shows it.
- **`Key` is the in-app page identity.** `PageMeta.Key` is the link target that
  opens exactly that page: its `Name`, or for a page that lost a name clash, its
  shortest path suffix that resolves to it (`b/Note`). History, `PageView`,
  backlink sources, task rows, search hits and overlays all hold the key, so
  `Resolve(key)` returns that page by construction. Without it, two pages with
  one name would open the same file from every list.
- **Backlinks by page path.** A link that resolves is filed under the target
  page's `Path`; an unresolved one under its lower-cased text. `Alpha` and
  `alpha` in different files therefore have separate backlinks.
- **Unlinked mentions only for `Key == Name`.** A bare mention names the clash
  winner, so a clash loser has none. Phantom names (linked, not indexed) are
  still scanned.
- **Search by explicit file list.** `search.Run` and `search.Mentions` pass the
  indexed paths (`idx.Files()`) to `rg` in batches under an argument budget
  (`argBudget`, 128 KiB), and a hit maps to its page by path, never by basename.
  This is why non-`.md` files and dot-folders are no longer searched.
- **New pages and journals.** `Layout.NewPageRel` places a page: `triple-lowbar`
  joins `/` with `___` in `pages_dir`; `folders` keeps the folders and rejects
  empty, `.` and `..` segments and folders starting with `.` (so a link cannot
  write outside the graph). A new journal goes to `JournalRel(day)`.
  `edit.EnsureFile` and `WriteFileIfUnchanged` create the parent folder.

## Traps

- A page can be unreachable: when every path suffix of its path is taken by a
  page name (a file named `pages___x.md` is the page `pages/x`), no link opens
  it. The index warns (`… cannot be opened by a link`) and its `Key` is the full
  path.
- The zero `Layout` is invalid. Use `layout.Default()` or `layout.Load`.
- Task dates (`SCHEDULED`, `DEADLINE`) keep `YYYY-MM-DD`; the journal pattern
  does not apply to them.
- Tests that build an `Index` by hand need `Layout` and each page's `Key` set.
- `Alpha.md` and `alpha.md` still produce a case warning, but now keep separate
  backlinks.
