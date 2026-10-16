# weft v3: a complete terminal tool for journals, tasks and notes

weft v3 turns weft from a reader of a graph that other tools write into the
author's daily tool for journaling, tasks and a personal knowledge base, and
makes it usable by people who are not the author. It was built as 14 units
(some split into parts), each with its own record in this folder.

Why: before v3 the editor was the Bubbles `textarea` (no undo, selection or
find; it refused tabs, CRLF and files over 10000 lines), there was no
date-aware task view, and only one Logseq-shaped layout opened, which shut out
most public users.

Out of scope for the whole epic: block references and embeds, zoom into a
block, folding saved to disk, YAML frontmatter, aliases, recurring tasks, any
sync other than git, several graphs open at once, plugins, a GUI or mobile app,
collaboration and encryption.

## The governing rule: navigator, not outliner

The file is the source of truth and stays plain markdown that any editor can
read. weft never writes structure metadata into a file: no block IDs, no saved
fold state. Anything that only changes how weft shows or edits text is allowed:
view-only folding, `[[Page#Heading]]` links, the live-preview editor, moving a
bullet with its children, task dates written as readable text.

This replaced the older "no fold, no block refs, no zoom" line, which had
ruled out view-only folding and heading links that never touch the file.

## Decisions shared by every unit

1. **Audience.** Public users and the author's daily use. Every unit ships its
   user-facing docs (README and in-app help).
2. **Breaking changes are allowed in v3.** Keys, config and the module path
   (now `github.com/fiatcode-gh/weft/v3`) could break. Each break is listed in
   `CHANGELOG.md` with a migration note.
3. **Modeless editing by default.** Common desktop keys. Vim mode is opt-in
   (`editor = "vim"`) on the same buffer.
4. **One linking primitive.** `[[Page]]`, `#tag` and `#[[Multi Word]]` all link
   to a page; a tag is not a separate kind of object. Text that only looks like
   a tag is never one: issue numbers (`#18`), hex colours, headings, org-style
   lines, shebangs, URL fragments, code. The author's graph had 278 `#<number>`
   references, and none became tags.
5. **Configurable layout.** The journal filename pattern and the folders are
   set in `<graph>/.weft/config.toml`, so Logseq-style and Obsidian-style notes
   open without migration.
6. **The author's graph opens unchanged** with the default config, and task
   syntax stays Logseq-compatible (`TODO`/`DOING`/`DONE`, `SCHEDULED: <…>`,
   `DEADLINE: <…>`, `[#A]`), so agents and other editors that write the graph
   keep working.
7. **No silent overwrite.** No weft write replaces on-disk content it has not
   seen. Every write path detects an outside change since load and lets the
   user decide.
8. **Narrow write surfaces.** File writes go through `internal/edit`, git
   through `internal/sync`. Task toggles, capture, rename and the first-run
   config save use these, not new ones.
9. **Unchanged foundations.** One Go binary, ripgrep at runtime, colours from
   the terminal's own palette, no OSC 11 background detection.

## Units

Order as shipped. Each unit's record holds its own decisions.

1. Save conflict guard (`save-conflict-guard.md`)
2. Bubble Tea v2 (`bubbletea-v2.md`)
3. Editor core (`editor-core.md`)
4. Live preview (`live-preview.md`)
5. Tags (`tags.md`)
6. Tasks and agenda (`tasks-agenda.md`)
7. Journaling and capture (`journaling-capture.md`)
8. Folding and heading links (`folding-heading-links.md`)
9. Graph layout config (`graph-layout-config.md`)
10. Page tree, live reload, page rename (`page-tree.md`, `live-reload.md`,
    `page-rename.md`)
11. Command palette, sidebar, mouse (`command-palette.md`, `sidebar.md`,
    `mouse.md`)
12. Vim mode (`vim-mode.md`)
13. Public readiness (`public-readiness.md`)
14. Crash reports (`crash-reports.md`)

## Traps

- A unit that touches parsing, rendering or writing must be checked against
  the author's real graph read-only, besides the fixture graph: it must open
  and read unchanged, and for tags `weft doctor` must show no new links from
  existing `#` text.
- Every write path that exists when a unit lands must keep decision 7.
