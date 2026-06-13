# Slice 3a — Unlinked references in the backlinks panel

> Design spec. Written 2026-06-12. Follows Slice 1 (in-app editor) and Slice 2
> (`[[`-autocomplete). Target release: **v1.3.0**.
>
> This is the first half of the roadmap's "Slice 3 — unlinked references +
> one-key linkify." It is deliberately split: this slice is **read-only**
> (detect + display); **one-key linkify** — the first writer to files other
> than the current page — is a separate slice (3b → v1.4.0).

## Goal

For the page being viewed, surface *unlinked references* — bare-text mentions
of the page's name elsewhere in the graph that aren't already wrapped in
`[[…]]` — alongside the existing linked backlinks. This is page-level
*connect*: it shows you where a page is talked about but not yet linked, so a
later slice can offer to link them.

## Scope

### In

- **Detection.** When the backlinks panel opens for a page, find bare-text
  mentions of the page's name across `pages/` + `journals/` that are not
  already links.
- **Display.** Fold the results into the existing backlinks view (`b`) as a
  second **Unlinked references** section beneath the existing **Linked
  references**.
- **Navigation.** `enter` on an unlinked-reference row navigates to that file
  (read-only jump), consistent with how linked backlinks navigate.

### Out (explicit non-goals for this slice)

- **One-key linkify** — writing `[[…]]` into other files. That is Slice 3b
  (v1.4.0); it is the first writer to files other than the current
  page/journal and gets its own design.
- **Alias matching.** The index tracks `Name` only; there is no `alias::`
  property. Detection is by page name.
- **Namespaced-leaf matching.** A page named `Proj/Sub` is matched by its full
  name string, not by the leaf `Sub`. Namespaced pages will naturally surface
  few bare-text hits; that is acceptable.
- **Frequency / length caps.** Short or common page names (`Go`, `API`) may
  surface many hits; we show them all. Read-only means noise is harmless, and
  whole-word matching already removes the worst of it. Tune later only if real
  use demands it.

## Matching rules

A ripgrep hit for the page name is an **unlinked reference** unless it is:

1. **Inside a `[[…]]` span** on its line — it's already a linked reference.
2. **Inside a fenced code block** — reuse the existing fence-aware logic
   (`fenceRe`, as `ExtractWikiLinks` already does).
3. **In the page's own file** — a self-mention is not surfaced (mirrors the
   backlinks view filtering self-references).

Match semantics: **whole-word** (`rg -w`), **fixed-string** (`rg -F`, so names
with regex metacharacters are literal), **case-insensitive** (a prose mention
of `alpha` is a valid unlinked reference to page `Alpha`, and peekseq's
`Resolve` is fold-keyed anyway).

## Architecture

The view stays display-only; the **App orchestrates detection** (it holds both
`graphPath` and the index) and injects the result into the backlinks view.

- **`internal/search` gains `Mentions(graphPath, name) ([]Hit, error)`** — a
  thin sibling of `Run` that shells `rg --json -w -F --ignore-case -- <name>`
  over `pages/` + `journals/`, reusing the existing JSON parsing and the
  exit-code-1-is-success handling. Returns the same `search.Hit` type.
- **A pure filter** turns raw hits into displayable unlinked references,
  applying the three matching rules above. It is the unit that holds the logic
  and the tests, and it takes its inputs as plain data (hits + a way to read
  each file's body) so it runs without invoking ripgrep. Proposed home:
  `internal/graph` (it already owns `fenceRe`, `wikiLinkRe`, and link
  semantics) exposing something like
  `FilterUnlinked(hits []search.Hit, target, targetPath string, read func(path string) (string, error)) []UnlinkedRef`.
  - If importing `internal/search` from `internal/graph` would create a
    dependency cycle, the filter takes a minimal local hit struct instead of
    `search.Hit`, and the App adapts — decided at plan time. The filter staying
    pure is the load-bearing constraint, not its package.
- **`UnlinkedRef`** is the display record: `{FilePath string; PageName string;
  Line int; Context string; Match search.Span}` (page name derived from the
  filename via `PageNameFromFilename`, for the row label and navigation
  target).
- **`NewBacklinks` grows an `unlinked []UnlinkedRef` parameter** (current
  signature: `NewBacklinks(idx, target, width, height)`). The App builds the
  list before constructing the view:
  `internal/views/app.go` line ~476 changes from
  `NewBacklinks(a.idx, a.page.Page(), a.width, a.height)` to compute the
  unlinked refs (run `search.Mentions`, read each hit's file, filter) and pass
  them in. Detection errors degrade gracefully — on a ripgrep failure the
  panel still shows linked backlinks with an empty unlinked section (the App
  may set a transient hint, matching the existing reindex-failure pattern).

## Display

The `Backlinks` view renders two sections within its existing scroll/list
chrome:

- **Linked references** — the current in-memory backlinks, unchanged (instant,
  no I/O).
- **Unlinked references** — one row per `UnlinkedRef`: the referencing page
  name plus the matched line as context (the match emphasised, reusing the
  search view's `Span`-based highlighting). Omitted entirely when there are
  none.

One scroll position spans both sections; the existing `↑/↓ N more` window chrome
covers the combined row count. `enter` on a linked row navigates as today; on an
unlinked row it navigates to the referencing page (line-targeting via the
existing todo/deep-link mechanism is a nice-to-have, not required this slice).

## Timing

Detection runs **synchronously when the panel opens** (the App computes it
before constructing the view). `rg` on a single name scoped to two directories
is fast; if it ever drags on a large graph, move it to a `tea.Cmd` and fill the
section in on completion. Not pre-optimized.

## Testing

- **Pure-filter unit tests** (no ripgrep) with synthetic hits + file bodies:
  - excludes an occurrence inside `[[Target]]` (already linked),
  - excludes an occurrence inside a fenced code block,
  - excludes hits in the target's own file,
  - keeps a plain-prose mention, and surfaces a case-variant (`alpha` →
    `Alpha`),
  - respects whole-word (a substring like `Alphabet` is not a hit for `Alpha`).
- **`search.Mentions`** — a focused test against the existing test fixture
  graph, asserting it returns hits for a known bare mention.
- **teatest golden** for the combined linked + unlinked backlinks panel;
  regenerate with `-update` and visually inspect.

## Out-of-scope follow-ups (noted, not built)

- **Slice 3b — one-key linkify (v1.4.0):** a key on an unlinked-reference row
  wraps that occurrence as `[[…]]` in its file (via `internal/edit`), with
  careful per-occurrence confirmation, then reindex. First writer to files
  other than the current page — design the write-confirmation UX then.
- Alias matching; namespaced-leaf matching; frequency/length caps — revisit
  only if real use calls for them.
