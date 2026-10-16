# weft — a daily-journal-driven knowledge base for the terminal

Journals are the warp — the continuous daily timeline you lay down. `[[wiki-links]]` are the weft — the cross-threads you weave across into pages. The fabric is your knowledge base, and weft is the terminal tool for keeping it: a recency-sorted page picker (with page-creation), ripgrep-backed full-text search, backlinks (linked and unlinked), and a cross-graph dashboard of your open tasks (`TODO/DOING/LATER/WAITING`) — because a journal tool that can't surface your open loops is incomplete.

Two kinds of link, both pointing at a page — `[[wiki-links]]` and `#tag` / `#[[Multi Word]]`. Pages are markdown files, flat or in folders, and journals are named by a filename pattern you can set. A Logseq graph opens with no config; an Obsidian vault opens with a short one (see Configuration). weft is a navigator, not an outliner: it renders your bullets, workflow markers, and links, but it never makes you tend the tree — no saved folds, no block refs, no zoom. Folding a heading or bullet is a view-only convenience, and `[[Page#Heading]]` links open a page at a heading.

Press `e` to edit the current page in a full-screen in-app editor (`Ctrl+S` saves, `Esc` exits with an unsaved-changes prompt). It opens on the cursor row's line, at the same screen row, and `Esc` brings you back the same way. The editor is a live preview: the page is drawn exactly as the read view draws it, and only the line or block under the cursor (a table, a fence, a quote, a joined paragraph, a `:LOGBOOK:` block) turns back into source while you edit it, then renders again when you move away. `Ctrl+R` switches to the plain source look and back; the choice lasts until weft quits and is never saved. The editor is coloured like the read view (same styles, `WEFT_STYLE` and `NO_COLOR` apply) and shows your terminal's own cursor. Any markdown file opens — CRLF, tabs, any length — and saves byte-for-byte, with the file's own line endings and final-newline state kept. If the file changed on disk since you opened or last saved it, a save merges changes on separate lines (lines both sides added at the same spot are all kept, yours first), and otherwise asks whether to overwrite, reload or keep editing; `E` hands the file to your `$EDITOR`. The editor has undo/redo, selection, copy/cut/paste (copies also go to the system clipboard through OSC 52), find and replace, and bullet-aware editing: `Enter` continues a bullet, `Tab`/`Shift+Tab` indent a bullet with its children, `Alt+↑/↓` move it, `Ctrl+T` cycles its workflow marker. Typing `[[` opens a live, fuzzy-filtered page-name completion list — `↑`/`↓` to choose, `Enter` or `Tab` to insert `[[Page Name]]`, `Esc` to dismiss; an unmatched name shows a `＋ Create` row to insert a link to the not-yet-created page in one keystroke. Press `.` to jump to today's journal.

Prefer modal editing? Set `editor = "vim"` in your user config (see User config under Configuration) and the editor opens in vim-style normal mode; see Vim mode under Keys. Nothing changes unless you set it.

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

Or install with Go 1.26+ (v3.0.0 and later; for v2 use
`go install github.com/fiatcode-gh/weft/v2/cmd/weft@v2.5.1`):

```bash
go install github.com/fiatcode-gh/weft/v3/cmd/weft@latest
```

Or build from source (Go 1.26+):

```bash
git clone https://github.com/fiatcode-gh/weft
cd weft
go build ./cmd/weft
```

## Getting started

1. Run `weft`. It asks `Notes folder [~/notes]:`. Press Enter for `~/notes`, or type any folder. An existing Logseq graph or Obsidian vault works (an Obsidian vault needs the layout file, see Configuration). If the folder does not exist, weft asks `Create …? [Y/n]`. Release builds then ask `Send crash reports to the weft author? [y/N]`; Enter means no (see Crash reports under Configuration). Builds from source never ask. weft saves your choice as `graph = "…"` in the user config, and your answer as `crash_reports`, next to it (it prints the path), and later runs open that folder straight away. `Ctrl+C` or `Ctrl+D` at a prompt quits and saves nothing.
2. weft opens on today's journal (`.` brings you back to it from anywhere). Press `e` to write, `Ctrl+S` to save and `Esc` to leave the editor. The journal file is created on the first save.
3. To link a page, type `[[` in the editor, pick or type a page name, and save. In the read view, `n` moves to the link and `Enter` follows it. A new page has no file yet: press `e` there, write and save to create it. `Ctrl+P` finds any page.
4. Press `?` to see every key. Press `:` to list every action and run one.
5. Optional: make the folder a git repository with a remote. `S` then commits, pulls and pushes.

## Configuration

### Choosing the graph

weft takes the graph folder from the first of these that is set:

1. `--graph PATH`
2. the `WEFT_GRAPH` environment variable
3. `graph` in the user config (see User config)

The first one set is used. If its folder is missing or is not a folder, weft stops with exit status 2 and names where the path came from, for example `weft: graph path "/nope" from --graph is not accessible: stat /nope: no such file or directory`. It never falls back to another source and never asks.

If none is set and both stdin and stdout are a terminal, weft runs the first-run prompt from Getting started. Otherwise it stops with exit status 2 and `weft: no graph folder: pass --graph PATH, set $WEFT_GRAPH, or add graph = "PATH" to <user config path>`.

### User config

Your own settings, apart from any graph's layout, live in `$XDG_CONFIG_HOME/weft/config.toml` (`~/.config/weft/config.toml` when `XDG_CONFIG_HOME` is unset or not an absolute path). The file is optional. weft reads it at start. It is separate from the graph's `<graph>/.weft/config.toml`, which travels with the graph.

| Key | Default | Meaning |
|-----|---------|---------|
| `editor` | `"modeless"` | `"modeless"` is the editor as it has always been. `"vim"` turns on vim mode in the in-app editor (see Vim mode). |
| `graph` | none | The graph folder: an absolute path or one that starts with `~/`. Anything else is an error. |
| `crash_reports` | `false` | `true` sends a crash report when weft crashes (release builds only; see Crash reports). |

```toml
editor = "vim"
graph = "~/notes"
crash_reports = true
```

weft writes this file only on first run: it appends the `graph` line and, in release builds, `crash_reports`, and keeps everything else in the file. If the file changed while weft was asking, weft saves nothing and exits with status 2. There is no key to switch modes while weft runs; edit the file and start weft again.

A problem stops weft with exit status 2 and a message that names the file, for example `weft: /home/you/.config/weft/config.toml: editor: "emacs" must be "modeless" or "vim"`. The problems are: the file cannot be read, a TOML syntax error, an unknown key, an unknown `editor` value, a bad `graph` value (for example `graph: "notes" must be an absolute path or start with ~/`), and a `crash_reports` value that is not `true` or `false`. A missing file is the defaults.

`weft doctor` reads this file only to find the graph, when no flag or environment variable names one. It ignores `editor` and `crash_reports`.

### Crash reports

weft can tell its author when it crashes. It is off unless you turn it on.

- Only release binaries from GitHub Releases can report. A build from `go install` or `go build` never sends anything and never asks.
- Only with `crash_reports = true` in the user config. First run asks `Send crash reports to the weft author? [y/N]` when the key is absent. To change your mind, edit the key.
- One report holds the panic message, the stack trace, the weft version, the OS, the architecture and the Go version. weft adds nothing else: no host name, no user name, no environment, no breadcrumbs.
- In the panic message weft replaces the graph folder with `<graph>` and your home folder with `~`. A panic message can still quote text, so it may hold a line or a name from your notes.
- The report goes to the author's Bugsink at `errors.fiatcode.dev`. The server sees your IP address, as with any connection.
- weft sends once, after the terminal is restored, and waits at most 2 seconds. It never retries. If the send fails, weft prints one line, `weft: crash report not sent: <reason>`.
- Nothing is sent from `weft doctor` or during first run.

### Graph config (.weft/config.toml)

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

#### Recipes

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

#### Names and links

- A page's name is its filename stem. `___` reads as `/`, so `A___B.md` is `A/B`. Folders are not part of the name. A journal's name is its filename stem as it is: `2026_10_12` with the default layout, `2026-10-12` in the Obsidian recipe.
- `[[a/Note]]` picks a page by its path, relative to the graph, without `.md`. It matches whole folders and ignores case, so `[[pages/2026_05_24]]` reaches a page that a journal of the same name hides.
- A link resolves in this order: an exact name, a name that differs only in case, then a path.
- When several pages share a name, the bare name opens a journal first, then the page with the shallowest path, then the one whose path comes first alphabetically.
- The picker and every other list (backlinks, tasks, agenda, search) show a page that loses a name clash by its shortest path that opens it, such as `b/Note`, which is also what `[[b/Note]]` opens.
- `weft doctor` warns about each clash.
- Dot-folders (`.obsidian`, `.git`, `.trash`, `.weft`) are skipped. Only `.md` files are indexed, and search and unlinked mentions read only those files.
- A new page goes to `pages_dir`. A new journal goes to `journals_dir` through the pattern. weft creates a missing folder when the file is first saved.

### Environment variables

| Variable        | Effect                                                                            |
|-----------------|-----------------------------------------------------------------------------------|
| `WEFT_GRAPH`   | Graph path. `--graph` overrides it, and it overrides the `graph` key in the user config. |
| `WEFT_STYLE`   | Force a Glamour markdown style (`dark`, `light`, `ascii`, `notty`) instead of weft's default terminal-palette style, which follows your terminal's own colors. |
| `NO_COLOR`      | Honoured: drops every colour; bold, italic, underline, strikethrough, faint and reverse video stay. Use `TERM=dumb` for fully unstyled output. |
| `WEFT_DEBUG=1` | Mirror Bubble Tea events to `weft.log` under the user cache dir (`$XDG_CACHE_HOME/weft/weft.log`, or the OS equivalent via `os.UserCacheDir`; falls back to `./weft.log` only if the cache dir is unavailable). Useful when reporting bugs. |

### Command line

```bash
weft [--graph PATH]                # open the graph
weft -version                      # print version and exit
weft doctor [--graph PATH]         # health report
weft --graph PATH doctor           # the same, with the flag before the word
```

`weft doctor` walks the graph once and prints a health report. It is read-only and never writes to the graph.

- **unresolved links** — `[[wiki-links]]` and tags whose target page does not exist (a `[[Page#Heading]]` link to a page and heading that exist is not unresolved)
- **orphan pages** — pages no other page links to (journals excluded)
- **unlinked mentions** — bare-text mentions that could become links
- **index warnings** — problems the index walk noticed (unreadable files, name clashes, pages no link can open)

Two warnings come from page names. A name clash: several pages share a name, and the warning lists the files and says which one `[[name]]` opens. An unreachable page: every path link to it opens another page, so no link can reach it (rare). See Names and links under Configuration.

### Exit codes

- `weft`: `0` quit normally. `1` the terminal UI failed or crashed. `2` weft could not start: no graph, a bad graph path, a bad graph or user config, ripgrep missing, the debug log cannot open, or first run ended without a choice or could not save. A crash outside the UI exits `2` like any Go panic. `Ctrl+C` at a first-run prompt ends weft by the interrupt (shells show 130).
- `weft doctor`: `0` the graph is clean, `1` findings exist, `2` operational error (bad graph path, no graph set, bad config file, missing ripgrep, failed scan).

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
| `1`-`5`    | sidebar: page tree, backlinks, todos, agenda, calendar; focuses it |
| `\|`       | show / hide the sidebar             |
| `?`        | toggle the help overlay             |
| `:`        | command palette: every action with its key; type to filter, `Enter` runs it |
| `m`        | mouse on / off for the session (off: the terminal selects text) |
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
| `F2`       | rename the page: preview, then rewrite every link and tag to it (`y`) |
| `R`        | rebuild the index                   |
| `S`        | sync the graph with git (commit → pull → push) |
| `e`        | edit current page in-app at the cursor row |
| `E`        | edit current page in `$EDITOR`      |
| `Esc`      | close an overlay / leave editor / sidebar: focus back to the page |
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
| `Ctrl-R` | switch between live preview and source look (redo in vim mode, where `:preview` switches) |
| `Ctrl-F` | find; `Enter`/`↓` next, `↑` previous, `Tab` find ↔ replace, `Enter` replace, `Ctrl-A` replace all (replace field), `Esc` close |

### Vim mode

With `editor = "vim"` in the user config the in-app editor is modal. It opens in normal mode with the cursor where it opens today. Without the setting nothing here applies. The table above still holds for insert mode, except as noted below. This is a subset of vim, not a copy.

- **Modes and status line.** The status line shows `-- INSERT --`, `-- VISUAL --` or `-- VISUAL LINE --` (nothing in normal mode), and the keys typed so far of a pending command, such as `2d3`. The hint on the right is `:w save · :q quit · :preview source` (or `preview`), or `esc normal · ^S save` in insert mode. The terminal cursor is a bar in insert mode and a blinking block in the other modes. In normal and visual mode the cursor never rests past the last character of a line.
- **Enter insert mode.** `i`, `a`, `I` (first non-blank), `A` (line end), `o`/`O` (a new line below / above; on a `- ` bullet it continues the bullet), and `s`, `S`, `c` and `C` (they delete first). A count on `i a I A o O` repeats what you type. `Esc` leaves insert mode and moves the cursor one left.
- **Motions** (all take a count). `h j k l` (the arrow keys, `Home` and `End` work too), `w b e W B E`, `0 ^ $`, `gg` and `G` (`5G` or `5gg` go to line 5), `f F t T` with `;` and `,`, `%` for `() [] {}`, `{` and `}` for blank-line paragraphs, and `Ctrl-D`/`Ctrl-U` for half a page. `j` and `k` move as the arrow keys do, row by row as the editor wraps; after an operator they are whole lines.
- **Operators.** `d`, `c`, `y` with any motion or text object, and `>` and `<` (shift the lines the motion touches). Doubled (`dd`, `cc`, `yy`, `>>`, `<<`) they act on the current line, or on a count of lines; `>>` and `<<` on one line indent and de-indent a bullet with its children, as `Tab` and `Shift-Tab` do. Shortcuts: `x X s S D C`, `r{char}`, `J` (join), `p` and `P` (below / above for whole lines, after / before the cursor otherwise). Counts multiply: `2d3w` deletes six words.
- **Text objects**, after an operator: `iw aw iW aW`, `i" a" i' a'`, `i( a( i[ a[ i{ a{` (`ib` and `ab` mean the parentheses; the closing bracket works too), `ip ap`.
- **One register.** Yanks, deletes and changes fill the register `Ctrl-C`/`Ctrl-X`/`Ctrl-V` use, and also copy to the system clipboard through OSC 52. `p` pastes it. There are no named registers.
- **Undo, redo, repeat.** `u` undoes and `Ctrl-R` redoes (`Ctrl-R` no longer switches the look in vim mode: use `:preview`). Each normal-mode change is one undo step, and so is an insert session from the key that entered it to `Esc`. `Ctrl-Z`/`Ctrl-Y` also work. `.` repeats the last change; a count before `.` replaces the old count.
- **Search.** `/` and `?` open the find bar to search forward or backward; `Enter` jumps and closes it, `Esc` returns to where you were. `n` and `N` repeat in the same and the opposite direction. With an empty field `Enter` uses the last search. `Ctrl-F` still opens the find and replace bar.
- **Visual modes.** `v` starts visual, `V` visual line. Motions extend the selection, `o` swaps its ends, `v` or `V` again leaves or switches, `Esc` leaves. `d x c y > < J` act on it, and `~ u U` change its case. A count with `>` or `<` shifts that many levels. Objects, `p`, `r`, `:` and `/` do nothing in visual mode. `.` repeats a visual change on the same size of text from the cursor.
- **Ex commands.** `:` opens a line on the status row: `:w` saves, `:q` leaves (asks when there are unsaved changes), `:wq` and `:x` save and leave, `:q!` leaves and discards, `:12` goes to line 12, `:preview` switches live preview and source, and anything else says `not an editor command`. `Esc` cancels, `Backspace` on an empty line closes it. `ZZ` is `:x` and `ZQ` is `:q!`. Saving goes through the same guarded save as `Ctrl-S` (merge, clash prompt).
- **`Esc`.** In insert mode it returns to normal mode. In visual mode it leaves visual. With a command pending it clears it. In normal mode with nothing pending it leaves the editor as `Esc` always has (the unsaved prompt when dirty).
- **weft's own keys keep working** in normal and insert mode: `Ctrl-S`, `Ctrl-F`, `Ctrl-Z`/`Ctrl-Y`, `Ctrl-T`, `Alt-S`/`Alt-E`, `Alt-P`, `Alt-↑`/`Alt-↓`, `Ctrl-A`, `Ctrl-C`/`Ctrl-X`/`Ctrl-V`, `Shift` plus arrows, `PgUp`/`PgDn`. In insert mode `[[` and `#` completion, `Tab`/`Shift-Tab` and `Enter` work as in the table above, and `Ctrl-D`/`Ctrl-U` stay the delete keys. In normal and visual mode `Ctrl-D` and `Ctrl-U` scroll. In visual mode only `Ctrl-S` of these works.
- **Mouse.** The wheel scrolls as before. A click in normal mode places the cursor (never past the last character) and drops a pending command. A click in visual mode leaves visual and places the cursor; the wheel keeps visual mode. A click in insert mode places the cursor and stays in insert mode. The mouse does nothing while the `:` line is open.
- **Paste.** A terminal paste in normal mode inserts at the cursor as one undo step; in visual mode it leaves visual mode first; while the `:` line is open it is ignored.
- **Not supported.** Named registers, marks, macros, `*` and `#`, `:s` and any ex command beyond those above, ranges and counts before `:`, `:set`, folds in the editor, several windows, `R` replace mode, block visual (`Ctrl-V`), `gq` and `=`, `~` in normal mode, `Y`, `H M L`, `Ctrl-F`/`Ctrl-B` as page keys, space, `Enter` and `Backspace` as motions, an operator with a search or `Ctrl-D`/`Ctrl-U`, `r` followed by `Enter`, objects, `p`, `r` and `/` in visual mode, vim's insert-mode `Ctrl-W`, `Ctrl-O` and `Ctrl-R` (register paste), vim keys in the read view, switching modes while weft runs, and setting `editor` from weft (weft writes the user config only on first run, and only the `graph` line).

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

### Sidebar

- **What it is.** A left column with one of five lists next to the page: page tree, backlinks, todos, agenda or calendar. It is hidden by default.
- **Keys.** `1`-`5` open a section and focus it; with the sidebar open they switch section. `|` hides the sidebar, or shows the last section again (the tree the first time).
- **Focus.** The first row of the column shows which side has focus: `▶ 1 Page tree · esc: page` when the sidebar has it, `1 Page tree · 1: focus` when the page has. `Esc` (or the panel's own close key: `q`, `F` in the tree, `b` in backlinks) moves focus back to the page; the sidebar stays.
- **Keys in the sidebar.** `1`-`5`, `|` and `Ctrl-C` work as on the page. Every other key goes to the panel, so `R`, `?` and `:` do not reach the page: press `Esc` first. The panels keep their own footers.
- **Opening pages.** `Enter` opens the page (the task, the reference, the journal) on the right and keeps focus in the sidebar. `c` captures, with the result shown in the panel.
- **Backlinks.** Linked references of the page now shown (`[[links]]` and tags), following the page however you got there. Unlinked references and linkify stay in `b`.
- **Tasks.** `x` marks done or undoes, as in `T` and `A`, with the same refusals. Struck rows clear when you switch section or hide the sidebar.
- **Refresh.** The panels follow every reindex (live reload, `R`, a write by weft) and keep the selected tree node, task, reference or date; if it is gone the selection moves to the nearest row. The agenda uses today's date.
- **Width.** It needs 111 columns. On a narrower terminal `1`-`5` and `|` show `sidebar needs 111+ columns` and do nothing. If the terminal gets narrower than that the sidebar hides, and widening does not bring it back.
- **Where it shows.** Not in the editor (it comes back when you leave, with the page focused). Overlays and prompts cover the whole screen as before.
- **State.** Nothing is saved: visibility, section, focus and selections last for the session. The sidebar tree has its own expanded groups, apart from `F`.

### Renaming a page

`F2` renames the page on screen and rewrites every link and tag that points to it. It asks for the new name in a prompt on the bottom line, filled with the current name (`enter` previews, `esc` cancels). A preview lists the new name, the new file and how many links change in which files. `y` writes, `n` or `Esc` cancels.

- **Rewritten.** `[[Page]]` links in any case, `[[Page|alias]]` (the alias stays), `[[Page#Heading]]` (the heading stays), path links such as `[[pages/Note]]`, `#tag` and `#[[tag]]`, in every file of the graph, including the page's own links to itself. Every link gets the page's new name. A simple tag stays `#name` when the new name allows it, else it becomes `#[[New Name]]`; a `#[[tag]]` stays in bracket form.
- **Not rewritten.** Code spans and fences, bare mentions of the name, and the page's own title and plain text.
- **Where the file goes.** Into the same folder under the new name. A `/` in the name follows `namespace_files`: `A___B.md` beside the old file with `triple-lowbar`, a subfolder of the current folder with `folders`.
- **Refused.** Journals (`journals cannot be renamed`), a page with no file yet (`no file to rename yet`), a rename while a sync runs (`sync in progress — retry when it finishes`), an empty name, the same name (`name unchanged`), a name that is already a page in any case (`"X" is already a page: Key`), a name that would be a journal, a name weft cannot place in the page's folder, a name no link could open, and a name with `]`, `|`, a backtick or a control character. Changing only the case (`alpha` to `Alpha`) is allowed. A refusal shows in the prompt and nothing is written.
- **Order of writes.** weft rewrites the linking files one by one, then renames the page's file last. Each write checks the file has not changed since the preview, and a rename never overwrites an existing file. `y` is refused while the index is updating.
- **A file changed on disk.** The rename stops at the first file that changed, overwrites nothing, and shows what was rewritten and what was not. Links already rewritten point to the new name and stay unresolved until the page is renamed, so press `F2` and type the same new name again to finish.
- **Undo.** There is none. Git is the undo: sync with `S` or commit first, then restore with git. Back/forward history and folds follow the renamed page.

### Command palette

`:` in the read view opens a list of every action with its key, in help order. Movement keys (`j`/`k`, the arrows, `Ctrl-D`/`Ctrl-U`, `g`/`G`, `n`/`N`) and the palette itself are left out.
Typing filters on the name and the key, the same fuzzy match as the picker. `↑`/`↓` (`Ctrl-K`/`Ctrl-J`) move.
`Enter` closes the palette and runs the action exactly as its key does, so a refusal shows its usual message and `Enter`/`Tab` act on the row the cursor was on. `Esc` closes it.
`:` in the editor, a prompt or a panel is plain text.

### Mouse

The mouse is on by default. Press `m` to turn it off until weft quits (the status bar shows `mouse off`), and again to turn it on.

- **Wheel.** On the page, one step scrolls 3 rows; the row cursor stays where it is unless it leaves the screen. In a list the wheel moves the selection one row; in the calendar, one week. Over the sidebar it moves the sidebar's selection and leaves the focus alone.
- **Click on the page.** A link or tag follows, as `n` then `Enter` would. Any other row puts the cursor there. The rule row and the status bar do nothing.
- **Click in the sidebar.** The click focuses the sidebar. A click on a row selects it, and a click on the selected row opens it, as `Enter` does; focus stays in the sidebar. A click on the header row only focuses.
- **Click in lists and overlays.** The same rule applies in the picker, search, palette, backlinks, todos, agenda, tree and on this day: click a row to select it, click it again to open or run it. In the tree a second click on a group opens or closes it. In the calendar a click picks a day and a second click opens it. A click never scrolls a list, so the clicked row stays under the pointer; keys and the wheel move the selection and keep it centred as before.
- **Does nothing.** Clicks outside an overlay (it stays open), on its title, on headers, on the footer and on the status bar. Help, the rename preview and the prompts ignore clicks.
- **Editor.** The wheel scrolls 3 rows, and the cursor moves only if it would leave the screen. A click places the cursor (in live preview, on that line) and clears a selection.
- **Editor in vim mode.** The click rules depend on the mode; see Vim mode under Keys.
- **Select terminal text.** Hold `Shift` while dragging (`Option` in iTerm2), or press `m` first.
- **Nothing is saved.** The on/off state lasts until weft quits. A click never writes a file; it only does what the matching key does.
- **Not supported.** Drag to select, double click, right click and middle click.

## What gets rendered

- `[[wiki-links]]` are styled inline, navigable with `n`/`N`, and follow with `Enter`. Aliased links (`[[Target|alias]]`) show the alias.
- `[[Page#Heading]]` (and `#[[Page#Heading]]`) opens the page with that heading near the top (one row below it). The heading is matched by its text, ignoring case and emphasis marks; the first match wins, and a missing heading leaves you at the top with a hint. The link counts as a backlink of the page. A page literally named `Page#Heading` still wins over the heading reading.
- Folding: `Tab` on a heading hides its section, on a bullet or numbered item hides its children; a folded row ends with `▸ N lines`. `z` cycles the whole page through top-level bullets folded, an outline (every heading stays visible with its text folded, and `Tab` on a heading then folds or unfolds just its text), and everything shown; on a page with nothing foldable it says so. Folds are view-only, last for the session and are dropped when the page's text changes; nothing is saved to the file. Following a link, backlink or task to a hidden line unfolds it, and `n`/`N` skip hidden links.
- A tag is a link to the page of that name: `#kitchen` or `#[[Book Club]]`. It starts at the line start or after whitespace or `(`, the name begins with a letter and continues with letters, digits, combining marks (so `#café` and `#हिन्दी` stay whole), `-`, `_` and `/`; any other name needs the bracket form. The `#` stays visible, and tags are navigable with `n`/`N` and `Enter` like wiki-links. Never a tag: issue numbers (`#18`, `PR #5`), hex colours (`#FAF3E7`), headings, `#+…` and `#!` lines, text inside `[[…]]`, URL fragments, code spans and fences.
- Workflow markers at the start of a bullet are colour-coded (`TODO` red, `DOING` yellow, `LATER` blue, `WAITING` dim, `DONE` green, `CANCELED`/`CANCELLED` strikethrough, `NOW` magenta).
- `:LOGBOOK: ... :END:` blocks are hidden — they're metadata, not content.
- Long bullets wrap with hanging indent, so continuation lines align with the text after the bullet rather than under the bullet glyph.
- Backlinks (`b`) shows two sections: **Linked references** (`[[…]]` links and tags) followed by **Unlinked references** — bare-text mentions of the page name that aren't yet wiki-linked. `Enter` on either section jumps to the mention's page and lands on the reference itself: a linked backlink focuses the back-reference `[[link]]` or tag (cursor on it, scrolled into view), and an unlinked reference highlights the bare-text mention and scrolls it into view. Self-references are always excluded.

## Scope

Writes graph files only via the in-app editor (`e`, saves on `Ctrl+S`) and the `$EDITOR` handoff (`E`); the `.` key creates today's journal (from the `Journal Template` page) if it doesn't exist, `c` appends one line to today's journal (creating it the same way when missing), and the picker can create a new page by name. Linkifying a mention in the backlinks panel also writes the mentioning file. Renaming a page (`F2`) rewrites the files that link to it and then renames its file. The editor, linkify and rename never overwrite outside changes unseen. Outside the graph, weft writes the user config file once, on first run (the `graph` line), and creates the notes folder you confirm there. Press `S` to sync the whole graph with git (commit → `pull --rebase` → push); it runs against the graph directory and degrades to a status-bar hint if that directory isn't a git repository. Folding is view-only and lasts for the session; nothing is saved. weft checks the graph's folders every 2 seconds (file names, sizes and modification times; nothing is read) and rebuilds the index when a page or journal changed outside weft. The page on screen updates in place, open panels keep their rows until reopened, and an open editor is never changed: it shows that the file changed, and `Ctrl+S` merges or asks.

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
