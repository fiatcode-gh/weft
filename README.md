# weft — a daily-journal-driven knowledge base for the terminal

Journals are the warp — the continuous daily timeline you lay down. `[[wiki-links]]` are the weft — the cross-threads you weave across into pages. The fabric is your knowledge base, and weft is the terminal tool for keeping it: a recency-sorted page picker (with page-creation), ripgrep-backed full-text search, backlinks (linked and unlinked), and a cross-graph dashboard of your open tasks (`TODO/DOING/LATER/WAITING`) — because a journal tool that can't surface your open loops is incomplete.

Two kinds of link, both pointing at a page — `[[wiki-links]]` and `#tag` / `#[[Multi Word]]`. Pages are markdown files, flat or in folders, and journals are named by a filename pattern you can set. A Logseq graph opens with no config; an Obsidian vault opens with a short one (see Configuration). weft is a navigator, not an outliner: it renders your bullets, workflow markers, and links, but it never makes you tend the tree — no saved folds, no block refs, no zoom. Folding a heading or bullet is a view-only convenience, and `[[Page#Heading]]` links open a page at a heading.

Press `e` to edit the current page in a full-screen in-app editor (`Ctrl+S` saves, `Esc` exits with an unsaved-changes prompt). It opens on the cursor row's line, at the same screen row, and `Esc` brings you back the same way. The editor is a live preview: the page is drawn exactly as the read view draws it, and only the line or block under the cursor (a table, a fence, a quote, a joined paragraph, a `:LOGBOOK:` block) turns back into source while you edit it, then renders again when you move away. `Ctrl+R` switches to the plain source look and back; the choice lasts until weft quits and is never saved. The editor is coloured like the read view (same styles, `WEFT_STYLE` and `NO_COLOR` apply) and shows your terminal's own cursor. Any markdown file opens — CRLF, tabs, any length — and saves byte-for-byte, with the file's own line endings and final-newline state kept. If the file changed on disk since you opened or last saved it, a save merges changes on separate lines (lines both sides added at the same spot are all kept, yours first), and otherwise asks whether to overwrite, reload or keep editing; `E` hands the file to your `$EDITOR`. The editor has undo/redo, selection, copy/cut/paste (copies also go to the system clipboard through OSC 52), find and replace, and bullet-aware editing: `Enter` continues a bullet, `Tab`/`Shift+Tab` indent a bullet with its children, `Alt+↑/↓` move it, `Ctrl+T` cycles its workflow marker. Typing `[[` opens a live, fuzzy-filtered page-name completion list — `↑`/`↓` to choose, `Enter` or `Tab` to insert `[[Page Name]]`, `Esc` to dismiss; an unmatched name shows a `＋ Create` row to insert a link to the not-yet-created page in one keystroke. Press `.` to jump to today's journal.

Press `S` to sync the graph with git without leaving weft — it commits any changes, `pull --rebase`s, then pushes, reporting progress in the status bar (`⟳ syncing…` → `✓ synced`). A `●` shows in the status bar whenever the graph has local changes that aren't committed or pushed yet, so you always know when a sync is due. Conflicts are left for you to resolve in a shell — weft never touches a conflicted tree.

*Already keep a Logseq graph? weft reads it as-is — the on-disk format (`.md` pages under `pages/`, `YYYY_MM_DD` journals, `[[wiki-links]]`, `TODO`-style bullets) is adapted from Logseq's. But weft is its own tool, not a Logseq client.*

## Install

Requires [ripgrep](https://github.com/BurntSushi/ripgrep) on PATH (the search
view consumes rg's `--json` output).

Download the archive for your platform (Linux or macOS, amd64 or arm64) from
[GitHub Releases](https://github.com/fiatcode-gh/weft/releases), check it
against `checksums.txt`, and put the `weft` binary on your PATH:

```bash
tar -xzf weft_<version>_<os>_<arch>.tar.gz weft
install weft ~/.local/bin/
```

Or install with Go 1.26+ (works from v2.5.1 on; earlier versions use the old
`git.fiatcode.dev` module path):

```bash
go install github.com/fiatcode-gh/weft/v2/cmd/weft@latest
```

Or build from source (Go 1.26+):

```bash
git clone https://github.com/fiatcode-gh/weft
cd weft
go build ./cmd/weft
```

## Usage

```bash
weft --graph /path/to/graph    # explicit path
WEFT_GRAPH=/path/to/graph weft # via env var
weft -version                  # print version and exit
```

A graph path is required — either pass `--graph` or set `$WEFT_GRAPH`. The flag wins when both are set. Drop the env var into your shell config for the zero-arg invocation.

## Configuration

weft needs no config for a Logseq-style graph. To change where pages and journals live, add `<graph>/.weft/config.toml`. weft reads it at start and in `weft doctor`, never writes it, and it travels with the graph when you sync with git. With no file, every key has its default.

| Key | Default | Meaning |
|-----|---------|---------|
| `pages_dir` | `"pages"` | Folder holding pages, relative to the graph. `""` is the graph root. |
| `journals_dir` | `"journals"` | Folder holding journals. It may be `""` or inside `pages_dir`. |
| `journal_file_pattern` | `"YYYY_MM_DD"` | Journal path inside `journals_dir`, without `.md`. |
| `namespace_files` | `"triple-lowbar"` | Where a new page named `A/B` goes: `triple-lowbar` gives `A___B.md`, `folders` gives `A/B.md`. |

Folder values are relative to the graph and cannot leave it (no absolute path, no `..`). A folder that does not exist indexes nothing.

With `journals_dir = ""` and a non-empty `pages_dir`, the whole graph is walked, so every `.md` outside dot-folders is indexed.

The pattern uses three tokens: `YYYY` (four digits), `MM` and `DD` (two digits, zero-padded). Every other character is literal, and `/` makes subfolders, as in `YYYY/MM/YYYY-MM-DD`. The last segment must contain all three tokens, so each date has one name. A segment cannot be empty, `.` or `..`, or start with `.`. A file counts as a journal only when its path matches the pattern and the date is a real calendar date. Any other `.md` file under `journals_dir` is a page.

A config problem stops weft and `weft doctor` with exit status 2. The message names the file and the problem, for example `weft: <graph>/.weft/config.toml: unknown key "bogus"`. The problems are: the file cannot be read, a TOML syntax error, a wrong type or repeated key, an unknown key, a folder outside the graph, a `namespace_files` value other than the two above, and an invalid pattern. weft never falls back to the defaults.

### Recipes

A Logseq graph needs no file. The explicit equivalent of the defaults is:

```toml
pages_dir = "pages"
journals_dir = "journals"
journal_file_pattern = "YYYY_MM_DD"
namespace_files = "triple-lowbar"
```

An Obsidian vault with the daily notes in a `Daily` folder (use your own daily-notes folder name):

```toml
pages_dir = ""
journals_dir = "Daily"
journal_file_pattern = "YYYY-MM-DD"
namespace_files = "folders"
```

If your daily notes sit in year and month folders, use `journal_file_pattern = "YYYY/MM/YYYY-MM-DD"`. weft does not read `.obsidian/` or `logseq/config.edn`.

### Names and links

- A page's name is its filename stem. `___` reads as `/`, so `A___B.md` is `A/B`. Folders are not part of the name. A journal's name is its filename stem as it is: `2026_10_12` with the default layout, `2026-10-12` in the Obsidian recipe.
- `[[a/Note]]` picks a page by its path, relative to the graph, without `.md`. It matches whole folders and ignores case, so `[[pages/2026_05_24]]` reaches a page that a journal of the same name hides.
- A link resolves in this order: an exact name, a name that differs only in case, then a path.
- When several pages share a name, the bare name opens a journal first, then the page with the shallowest path, then the one whose path comes first alphabetically.
- The picker and every other list (backlinks, tasks, agenda, search) show a page that loses a name clash by its shortest path that opens it, such as `b/Note`, which is also what `[[b/Note]]` opens.
- `weft doctor` warns about each clash.
- Dot-folders (`.obsidian`, `.git`, `.trash`, `.weft`) are skipped. Only `.md` files are indexed, and search and unlinked mentions read only those files.
- A new page goes to `pages_dir`. A new journal goes to `journals_dir` through the pattern. weft creates a missing folder when the file is first saved.

## Doctor

```bash
weft doctor                  # check the graph from --graph / $WEFT_GRAPH
weft doctor --graph PATH     # explicit path
```

`weft doctor` walks the graph once and prints a health report. It is read-only and never writes to the graph.

- **unresolved links** — `[[wiki-links]]` and tags whose target page does not exist (a `[[Page#Heading]]` link to a page and heading that exist is not unresolved)
- **orphan pages** — pages no other page links to (journals excluded)
- **unlinked mentions** — bare-text mentions that could become links
- **index warnings** — problems the index walk noticed (unreadable files, name clashes, pages no link can open)

Two warnings come from page names. A name clash: several pages share a name, and the warning lists the files and says which one `[[name]]` opens. An unreachable page: every path link to it opens another page, so no link can reach it (rare). See Names and links under Configuration.

Exit codes: `0` graph is clean, `1` findings exist, `2` operational error (bad graph path, bad config file, missing ripgrep, failed scan).

## Keys

Press `?` from the page view at any time to see a grouped keymap inside the app.

| Key        | Action                              |
|------------|-------------------------------------|
| `Ctrl-P`   | open picker (recent + fuzzy)        |
| `F`        | page tree: folders and namespaces (`→`/`←` expand/collapse, `Enter` open) |
| `/`        | open full-text search (ripgrep)     |
| `T`        | open TODO dashboard (`x` marks done / undoes) |
| `A`        | open the agenda (overdue, today, next 7 days) |
| `b`        | open backlinks for the current page (linked + unlinked refs) |
| `?`        | toggle the help overlay             |
| `n` / `N`  | cycle the link cursor (wiki-links and tags) |
| `Enter`    | follow link (`[[Page#Heading]]` opens at the heading) / open selection |
| `[` / `]`  | back / forward in page history      |
| `.`        | jump to today's journal (creates it from the `Journal Template` page if missing) |
| `<` / `>`  | previous / next journal (on a journal page) |
| `C`        | calendar of journals (arrows day/week, `PgUp`/`PgDn` month, `t` today, `Enter` open) |
| `O`        | on this day: a week, a month and years ago |
| `c`        | capture a line into today's journal (`Tab` toggles `TODO`, `[[`/`#` complete, `Esc` cancels) |
| `j` / `k`  | move the row cursor down / up (the page scrolls at the window edge) |
| `Ctrl-d/u` | half-screen cursor move             |
| `g` / `G`  | cursor to top / bottom of page      |
| `Tab`      | fold / unfold the heading or bullet under the cursor |
| `z`        | fold the page: top-level bullets → outline of headings → all shown |
| `R`        | rebuild the index                   |
| `S`        | sync the graph with git (commit → pull → push) |
| `e`        | edit current page in-app at the cursor row |
| `E`        | edit current page in `$EDITOR`      |
| `Esc`      | close an overlay / leave editor     |
| `q`        | quit (from page view)               |

### In the editor

| Key | Action |
|-----|--------|
| `Ctrl-S` | save (merges outside edits, asks on a clash) |
| `Esc` | leave (prompts if unsaved; closes completion or find first) |
| `Ctrl-Z` / `Ctrl-Y` | undo / redo |
| `Shift`+arrows | select (also `Home`/`End`, `PgUp`/`PgDn`, word moves) |
| `Ctrl-A` | select all |
| `Ctrl-C` / `Ctrl-X` | copy / cut (the line when nothing is selected) |
| `Ctrl-V` | paste what weft copied (use the terminal's paste for the system clipboard) |
| `Alt-←` / `Alt-→` | word left / right (also `Ctrl-←/→`) |
| `Home` / `End` | line start / end |
| `PgUp` / `PgDn` | page up / down |
| `Enter` | continue the bullet (an empty bullet ends it) |
| `Tab` / `Shift-Tab` | indent / de-indent a bullet with its children (selected lines: only those) |
| `Alt-↑` / `Alt-↓` | move the bullet with its children |
| `Ctrl-T` | cycle the marker: open → `DONE` → plain → `TODO` (keeps `[#A]`) |
| `Alt-S` / `Alt-E` | set the task's scheduled / deadline date (`YYYY-MM-DD`, `today`, `tomorrow`, `+Nd`, `+Nw`, a weekday; empty clears) |
| `Alt-P` | cycle priority `[#A]` → `[#B]` → `[#C]` → none |
| `[[` | page-name completion (`↑`/`↓`, `Enter`/`Tab`, `Esc`) |
| `#` + letter | page-name completion inserting `#name`, or `#[[Name]]` when the name has spaces or other characters a tag cannot hold |
| `Ctrl-R` | switch between live preview and source look |
| `Ctrl-F` | find; `Enter`/`↓` next, `↑` previous, `Tab` find ↔ replace, `Enter` replace, `Ctrl-A` replace all (replace field), `Esc` close |

### Tasks

A task is a bullet that starts with a marker (`TODO`, `LATER`, `DOING`, `WAITING`, `NOW`, `DONE`, `CANCELED`). An optional priority `[#A]`–`[#C]` follows the marker. `SCHEDULED: <2026-10-12 Mon>` and `DEADLINE: <…>` lines go under the task and are styled in all three looks.

- A time (`<2026-10-12 Mon 09:30>`) and a repeater (`.+1w`, `++1d`, `+1m`) are read and kept, but a repeater has no effect: weft never advances dates.
- Dates belong to the task they sit under, not to its children, and a child's date never counts for its parent. Dates inside code fences and `:LOGBOOK:` blocks are ignored.
- The agenda (`A`) lists open tasks with a date in three sections: Overdue (before today), Today, and Upcoming (the next 7 days, inclusive). A task with both dates uses the earlier. Rows are ordered by date, timed before untimed, then priority. Undated tasks and tasks beyond the window are left out.
- `x` in the dashboard or the agenda writes only the marker: open → `DONE`, and `x` again restores the earlier marker. It is refused while a sync runs, or when the line changed on disk.

### Journals

- **Template.** If a page named `Journal Template` exists (in any folder), its file is copied as-is into every journal weft creates: with `.`, `E`, capture (`c`), or the first save in the in-app editor. A journal that already exists is never changed. The page is read each time it is used, so edits apply to the next new journal. Without the page, a new journal starts empty.
- **Capture.** `c` opens a one-line prompt over the current screen (also over the dashboard, agenda, backlinks, calendar, on-this-day and page tree overlays). `Enter` appends `- <text>` as the last line of today's journal, creating the file from the template when missing. `Tab` writes `- TODO <text>` instead; `[[` and `#` complete page names; `Esc` cancels. The write is append-only and guarded: if the file changes between read and write, weft re-reads and tries once more, then writes nothing and keeps your text in the prompt. It is refused while a sync runs. The day is fixed when the prompt opens.
- **Calendar.** `C` shows a month grid, Monday first, a `•` after each day that has a journal, `[ ]` around the cursor day. `←`/`→` move a day, `↑`/`↓` a week, `PgUp`/`PgDn` a month (the day clamps to the month's end), `t` goes to today, `Enter` opens that day's journal; a day without one opens as an empty page and nothing is written until you save.
- **On this day.** `O` lists the journals from one week ago, one month ago (the day clamps to the month's end) and the same month and day in earlier years, newest year first, each with its first lines. Dates without a journal are left out; a February 29 has no entry in years without one.

### Page tree

- **What it shows.** `F` lists non-journal pages grouped by folder (relative to `pages_dir`, or to the graph root for files outside it), then by `/` namespace. `kb___notes.md` and `kb/notes.md` both sit under `kb`. A page that is also a group is one row.
- **Rows.** `▸` marks a collapsed group, `▾` an expanded one, with the number of pages below it.
- **Keys.** `↑`/`↓`/`j`/`k` move. `→`/`l` expands a group, or steps into it when open. `←`/`h` collapses it, or steps to the parent. `Enter` opens a page (on a group with no page it expands or collapses). `c` captures. `Esc`/`q`/`F` close.
- **State.** It opens on the page you are reading. Expanded groups last for the session and are never saved.
- **Journals** are left out: use the calendar, `<`/`>` and `O`.

## What gets rendered

- `[[wiki-links]]` are styled inline, navigable with `n`/`N`, and follow with `Enter`. Aliased links (`[[Target|alias]]`) show the alias.
- `[[Page#Heading]]` (and `#[[Page#Heading]]`) opens the page with that heading near the top (one row below it). The heading is matched by its text, ignoring case and emphasis marks; the first match wins, and a missing heading leaves you at the top with a hint. The link counts as a backlink of the page. A page literally named `Page#Heading` still wins over the heading reading.
- Folding: `Tab` on a heading hides its section, on a bullet or numbered item hides its children; a folded row ends with `▸ N lines`. `z` cycles the whole page through top-level bullets folded, an outline (every heading stays visible with its text folded, and `Tab` on a heading then folds or unfolds just its text), and everything shown; on a page with nothing foldable it says so. Folds are view-only, last for the session and are dropped when the page's text changes; nothing is saved to the file. Following a link, backlink or task to a hidden line unfolds it, and `n`/`N` skip hidden links.
- A tag is a link to the page of that name: `#kitchen` or `#[[Book Club]]`. It starts at the line start or after whitespace or `(`, the name begins with a letter and continues with letters, digits, combining marks (so `#café` and `#हिन्दी` stay whole), `-`, `_` and `/`; any other name needs the bracket form. The `#` stays visible, and tags are navigable with `n`/`N` and `Enter` like wiki-links. Never a tag: issue numbers (`#18`, `PR #5`), hex colours (`#FAF3E7`), headings, `#+…` and `#!` lines, text inside `[[…]]`, URL fragments, code spans and fences.
- Workflow markers at the start of a bullet are colour-coded (`TODO` red, `DOING` yellow, `LATER` blue, `WAITING` dim, `DONE` green, `CANCELED`/`CANCELLED` strikethrough, `NOW` magenta).
- `:LOGBOOK: ... :END:` blocks are hidden — they're metadata, not content.
- Long bullets wrap with hanging indent, so continuation lines align with the text after the bullet rather than under the bullet glyph.
- Backlinks (`b`) shows two sections: **Linked references** (`[[…]]` links and tags) followed by **Unlinked references** — bare-text mentions of the page name that aren't yet wiki-linked. `Enter` on either section jumps to the mention's page and lands on the reference itself: a linked backlink focuses the back-reference `[[link]]` or tag (cursor on it, scrolled into view), and an unlinked reference highlights the bare-text mention and scrolls it into view. Self-references are always excluded.

## Environment

| Variable        | Effect                                                                            |
|-----------------|-----------------------------------------------------------------------------------|
| `WEFT_GRAPH`   | Default graph path (overridden by `--graph`).                                     |
| `WEFT_STYLE`   | Force a Glamour markdown style (`dark`, `light`, `ascii`, `notty`) instead of weft's default terminal-palette style, which follows your terminal's own colors. |
| `NO_COLOR`      | Honoured: drops every colour; bold, italic, underline, strikethrough, faint and reverse video stay. Use `TERM=dumb` for fully unstyled output. |
| `WEFT_DEBUG=1` | Mirror Bubble Tea events to `weft.log` under the user cache dir (`$XDG_CACHE_HOME/weft/weft.log`, or the OS equivalent via `os.UserCacheDir`; falls back to `./weft.log` only if the cache dir is unavailable). Useful when reporting bugs. |

## Scope

Writes graph files only via the in-app editor (`e`, saves on `Ctrl+S`) and the `$EDITOR` handoff (`E`); the `.` key creates today's journal (from the `Journal Template` page) if it doesn't exist, `c` appends one line to today's journal (creating it the same way when missing), and the picker can create a new page by name. Linkifying a mention in the backlinks panel also writes the mentioning file. The editor and linkify never overwrite outside changes unseen. Press `S` to sync the whole graph with git (commit → `pull --rebase` → push); it runs against the graph directory and degrades to a status-bar hint if that directory isn't a git repository. Folding is view-only and lasts for the session; nothing is saved. weft checks the graph's folders every 2 seconds (file names, sizes and modification times; nothing is read) and rebuilds the index when a page or journal changed outside weft. The page on screen updates in place, open panels keep their rows until reopened, and an open editor is never changed: it shows that the file changed, and `Ctrl+S` merges or asks.

### What is not supported

A few Logseq features are intentionally out of scope for v1. None of them
crash weft — they degrade to plain text or a silent no-op.

- **`{{query …}}` and `{{embed …}}` blocks** are stripped from the rendered
  page (Glamour can't render them usefully).
- **Block references** don't exist — `#` inside `[[…]]` is part of the page
  name, so `[[page#block]]` links to a page literally named `page#block`. If
  no such page exists and the text before the last `#` names a page with that
  heading, it is a heading link instead (see above).
- **`alias::` / `title::` / `tags::` properties** are not extracted — they
  appear as plain text in the page body; `tags::` is not a tag.
- **Case-insensitive page-name uniqueness** is not enforced — `Alpha.md` and
  `alpha.md` can coexist (weft records a warning in `weft doctor` and resolves
  links deterministically, see Names and links). Link resolution itself is
  case-insensitive: `[[alpha]]` finds `Alpha`. Backlinks are kept per page,
  so the two pages do not share them.
- **Native Obsidian and Logseq config** is not read: set the layout in
  `.weft/config.toml`.
- **Frontmatter and aliases** are not read.
- **Date tokens** other than `YYYY`, `MM` and `DD` are not supported in
  `journal_file_pattern`.
- **The TODO dashboard shows only open markers** (`TODO` / `LATER` /
  `DOING` / `WAITING`). `DONE` / `CANCELED` / `NOW` bullets are styled on
  the page but never appear in the dashboard. Rows marked done with `x` in
  the open panel stay struck through until it closes.
