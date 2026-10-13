# `F2` renames the page on screen and rewrites every link and tag to it

`F2` asks for a new name, shows a preview, and on `y` rewrites every link and
tag that resolves to the page in every file of the graph, then renames the
page's file. Alias and `#Heading` stay. If a file changed on disk the rename
stops without overwriting anything; `F2` with the same name finishes it.

Why: renaming a page by hand leaves every `[[Old]]` unresolved. weft already
resolves links, counts tags and writes files through guarded surfaces, so it
can do the whole rename with no new kind of data. Files stay plain markdown and
nothing is saved beside them.

Out of scope: rename from the tree, picker or backlinks; moving a page to
another existing folder; renaming journals or pages with no file; rewriting
bare mentions, the page's own title, frontmatter or `alias::`; undo (git is the
undo); batch or namespace rename; refreshing other open overlays; changing the
read view's link regex.

## Decisions

- **One link finder, the index's own.** `parseBody` yields the link spans
  (byte span, kind, target as indexed). `BuildIndex`, `ExtractLinks` and the
  rewriter all use it, so the rewriter finds exactly what the index counts.
  `TestLinksToPageMatchIndex` pins this on both fixtures.
- **Which links belong to the page.** A counted link whose `ResolveLink`
  lands on a page with the same `Path`, on the index the preview was built from.
  Case, alias and path spelling do not matter.
- **New link text is the page's `Key` after the rename.** weft rebuilds names
  and keys over a copy of the page list with the page moved, so the key is exact,
  including when the clash rule changes. Every rewritten link gets it. The
  fragment (`#Heading`) is kept, the alias is kept, and spaces just inside
  `[[ … ]]` are dropped.
- **Tags.** `#x` becomes `#key` when the key is a simple tag name
  (`graph.IsSimpleTagName`), else `#[[key]]`. A `#[[x]]` tag keeps its bracket
  form even when the key is a simple name.
- **Placement.** The file stays in its folder (`layout.RenameRel`, which shares
  `NewPageRel`'s encoding and validation). With `triple-lowbar` a `/` in the name
  writes `A___B.md` beside the old file; with `folders` it makes a subfolder of
  the current folder. The result is checked to be clean, relative and inside the
  page's folder.
- **Refusals, in this order.** The page is a journal (`journals cannot be
  renamed`); the name is empty (`new name is empty`); the name holds `]`, `|`,
  a backtick or a control character, which a link could not hold; the layout
  cannot place it; the name or file is unchanged (`name unchanged`); the file
  would be a journal; another page already has the name in any case
  (`"X" is already a page: Key`); or the new key would not open the page. A
  case-only change passes. Refusing a name equal to another page's ignoring case
  is what keeps every other link on the same page: no clash steal and no
  path-resolution steal.
- **Start refusals.** `F2` does nothing but show a hint during sync, on a page
  with no file (`no file to rename yet`) and on a journal.
- **The plan reads disk.** Every indexed file is read once, so the preview shows
  what is on disk, not what the index remembers. A missing file is skipped; a
  read error refuses with `cannot read <rel>: <err>`.
- **Write order and guards.** At `y`, before any write: refuse if the index is
  building, if the page file changed since the preview, or if the destination
  exists (`edit.CheckRenameDest`). Then each linking file is written with
  `edit.WriteFileIfUnchanged` in walk order, the page's own file at its place;
  the first failure stops. The page's file is renamed last with
  `edit.RenameIfUnchanged`, expecting its rewritten content when it links to
  itself. Nothing is retried or rolled back, and every file write is atomic.
- **After a stop.** weft reindexes, the page keeps its old key, and the panel
  lists the files rewritten and not rewritten (the failing one first) with the
  reason. `F2` with the same name rewrites the rest.
- **After success.** History entries that opened the page now name its new key,
  its fold entry moves to the new path, the index is rebuilt on the new key
  with the scroll position kept, and the hint reads `renamed Old → New · N
  links in M files`.
- **`edit` surface.** `CheckRenameDest` and `RenameIfUnchanged`, with the
  sentinel `ErrExists`. A destination that is the same file is allowed only for a
  case-only rename (same folder, names equal ignoring case, `os.SameFile`).
- **UI.** `F2` works in the read view only. A one-line prompt at the bottom
  (`enter preview · esc cancel`) leads to one centered panel that is the preview
  (`y` rename, `n`/`Esc` cancel, `↑`/`↓`/`j`/`k` scroll) and, after a stop, the
  report.

## Traps

- **Heading keys can change.** A heading on another page whose text holds an
  unaliased link to the renamed page changes its `HeadingKey` when that link is
  rewritten. A link `[[X#That heading]]` then no longer matches and becomes
  unresolved. Not handled; `weft doctor` shows it.
- **Check, then rename.** `CheckRenameDest` and `os.Rename` are two steps. A
  program creating the destination between them is not detected, the same
  window `WriteFileIfUnchanged` has.
- **A stop leaves `[[New]]` unresolved.** Links already rewritten point to the new
  name, which no file has until the rename finishes. `F2` with the same name
  completes it.
- **A symlinked page file is renamed as a link.** The link moves, not its
  target, as for writes.
- **A clash winner's rename hands its bare name to the loser.** When two pages
  share a name and the winner is renamed, `[[Name]]` then resolves to the other
  page. All links to the winner are rewritten, and the loser keeps its path
  links.
