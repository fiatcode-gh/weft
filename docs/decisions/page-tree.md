# `F` opens a read-only tree of pages by folder and namespace

`F` shows the graph's pages as a tree. Folders and `/` namespaces form one
hierarchy, so a Logseq graph (`kb___notes.md` in a flat `pages/`) and an
Obsidian vault (`kb/notes.md`) look the same. `Enter` opens a page.

Why: the picker finds a page you can name, but nothing showed how the graph is
organised. Pages can sit in folders, and namespaces like `kb/notes` already
exist, with no way to browse them. weft stays a navigator: the tree writes
nothing and saves no structure.

Out of scope: filtering or searching in the tree (the picker does that),
rename, move, create or delete from the tree, a sidebar or split pane, mouse,
showing journals or non-`.md` files, saving expansion, refreshing an open tree
when the index changes (unit 10b).

## Decisions

- **One tree path per page.** The folders of `Rel`, relative to `pages_dir`
  when the file is inside it and to the graph root otherwise, then the segments
  of `Name` split on `/`. Empty segments are dropped. `(*Index).PageTree` builds
  it in `internal/graph/tree.go`, with no UI imports.
- **Journals are left out.** The calendar, `<`/`>` and on-this-day cover them.
  A non-journal file in the journals folder is a page and appears.
- **A node is a page, a group or both.** A page `a` next to pages under `a/` is
  one row. A node is a group when it has children. A group row shows `▸`
  (collapsed) or `▾` (expanded) and the number of pages below it.
- **Identity is the tree path, case-sensitive.** `kb` and `Kb` are different
  siblings.
- **Sort.** Siblings sort by label ignoring case, then by label bytes, then by
  `Rel`. Rows show only their own segment, indented by depth.
- **`Enter` opens by `Key`**, so a page that lost a name clash opens its own
  file. `Enter` on a group-only row toggles it.
- **Keys.** `↑`/`↓`/`j`/`k` move and stop at the ends. `→`/`l` expands a
  collapsed group, or moves to the first child when expanded; on a page-only row
  it does nothing. `←`/`h` collapses an expanded group, or moves to the parent;
  at top level it does nothing. `c` opens capture. `Esc`, `q` and `F` close.
  Other keys do nothing.
- **Starting state.** Top-level rows start collapsed on the first opening, and
  later openings keep what was expanded. On each opening
  the ancestors of the page on screen are added to the session's expanded set
  and the cursor sits on that page's row. For a journal, or a page not in the
  tree, the cursor starts on the first row.
- **Expansion is session-only.** `treeOpen` on `App`, keyed by tree path, like
  `foldStore`. It is never written to disk. Collapsing a group removes only
  that group, so its expanded descendants stay expanded when it is opened again.
- **Built per opening.** No cache on `Index`.
- **Read-only.** `c` opens the capture prompt over the tree, and the result
  shows in the panel through `SetError`. That is the only write path, and it is
  capture's own.

## Traps

- Two pages with the same tree path (for example `pages/a___b.md` and
  `pages/a/b.md` in a folders graph) show as two rows with the same label. The
  page with the smaller `Rel` holds the group row. Nothing tells them apart yet.
- A `Name` made only of `/` has no segment, so the filename stem is the label.
- An open tree does not refresh on reindex or live reload; reopen it.
- Paths left in `treeOpen` after a reindex are ignored: they are only looked up.
