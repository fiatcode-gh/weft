# Slice 3b — One-key linkify

> Design spec. Written 2026-06-14. Follows Slice 3a (unlinked references,
> read-only — v1.3.0). Target release: **v1.4.0**.
>
> This is the **write half** of the roadmap's "Slice 3 — unlinked references +
> one-key linkify," and peekseq's **first writer to files other than the
> current page/journal** — the biggest expansion of the write-invariant so
> far. The confirmation UX is designed accordingly.

## Goal

From the backlinks panel's **Unlinked references** section, a single key turns
the selected bare-text mention into a `[[Target]]` link **in its source file**,
behind an explicit preview-and-confirm step. After the write the index is
rebuilt and the panel refreshes in place: the just-linkified reference drops
out of *Unlinked* and reappears under *Linked*.

## Scope

### In

- **Linkify a single occurrence.** `l` on a selected unlinked-reference row
  opens a confirm sub-state; `y`/`enter` wraps that one mention as
  `[[<matched>]]` in its source file.
- **Preview + confirm.** The confirm sub-state shows the affected line
  before→after and asks `y`/`n`. Writing to another file is never silent.
- **Authoritative re-match at write time.** The write re-reads the file fresh
  and re-finds the mention by whole-word/Unicode-boundary match, rather than
  trusting the offset captured when the panel opened.
- **In-place refresh.** A successful write triggers a synchronous reindex and
  rebuilds the backlinks overlay for the same target; the panel stays open.
- **Feedback inside the overlay.** Failures (file unreadable, mention gone,
  write error) render a status line *in the panel* — not a status-bar hint
  that would be invisible behind the overlay.

### Out (explicit non-goals for this slice)

- **Bulk linkify** — "linkify all occurrences in this file" or "everywhere."
  MVP is one occurrence per confirm; the next occurrence on a line surfaces on
  refresh.
- **Page-view linkify.** Offered only from the backlinks panel, where the
  `UnlinkedRef` data already lives.
- **In-app undo.** Git is the undo. No restore key for the MVP.
- **Alias / namespaced-leaf matching.** Unchanged from Slice 3a: matching is by
  full page name.
- **Casing normalization.** We preserve the author's bytes (`[[<matched>]]`)
  rather than rewriting to `[[Target]]`.

## What gets written

The matched text is wrapped **verbatim**: a mention `alpha` becomes
`[[alpha]]`, `Alpha` becomes `[[Alpha]]`. `(*Index).Resolve` is fold-keyed, so
`[[alpha]]` still resolves to the page `Alpha`; rewriting the author's casing
would be a needless edit to their prose. Only the matched span is touched —
the surrounding line, its indentation, line ending, and the file's trailing
newline are untouched.

## The byte-offset problem

`UnlinkedRef.Match` is a span into the **trimmed** `Context` (ripgrep's
`parseJSON` strips leading indentation and shifts offsets), not the raw source
line. The file may also have changed since the panel opened. We therefore do
**not** trust the stored offset for the write. Instead, at confirm time:

1. Read the source file fresh.
2. Locate the 1-based target line's byte range `[lineStart, lineEnd)` by
   walking the raw `body` (no `strings.Split`/rejoin — splicing by absolute
   byte offset preserves CRLF and the trailing newline for free).
3. On that raw line, run a case-insensitive, whole-word, literal match of the
   target name; drop any occurrence that falls inside a `[[…]]` span; take the
   **first** remaining occurrence (the same rule `firstUnlinkedMatch` uses, so
   it corresponds to the row the user selected).
4. Splice: `body[:abs] + "[[" + body[abs:absEnd] + "]]" + body[absEnd:]`.

If the line is out of range or no unlinked occurrence remains (file changed, or
it was already linked), return `ErrMentionNotFound`. This single re-match is
simultaneously the file-changed guard and the don't-double-wrap guard.

The **preview** shown in the confirm sub-state is computed separately and
cheaply from the data the overlay already holds (`Context` + `Match`) — no IO
until the user actually confirms. A preview/write race (file changed in the
sub-second window) simply falls through to the `ErrMentionNotFound` feedback;
the write is always authoritative.

## Components

### 1. `graph.LinkifyMention` (pure)

```go
// ErrMentionNotFound is returned when the target line is out of range or holds
// no unlinked, whole-word occurrence of the name to wrap.
var ErrMentionNotFound = errors.New("mention not found")

// LinkifyMention wraps the first unlinked, whole-word, case-insensitive
// occurrence of target on the 1-based line in body as [[<matched>]], preserving
// the matched bytes. It splices by absolute byte offset, so line endings and
// the trailing newline are preserved. span is the [[…]]-free match location in
// the original body (for callers that want it). Returns ErrMentionNotFound when
// no such occurrence exists.
func LinkifyMention(body string, line int, target string) (newBody string, span search.Span, err error)
```

Matching: `regexp.MustCompile("(?i)" + regexp.QuoteMeta(target))` to find
candidate spans, post-filtered by a Unicode-aware whole-word check (mirrors
`rg -w`; Go's `\b` is ASCII-only). Occurrences inside `[[…]]` are excluded via
the existing `wikiLinkRe`.

The whole-word helpers `wholeWordAt`/`isWordRune` are currently unexported in
`internal/render`. A **local copy** (~10 lines) is added to `graph` rather than
introducing a `render`→`graph` dependency or a shared package — keeping the
slice tight. (The two copies are noted as a known small duplication.)

### 2. `Backlinks` overlay — confirm sub-state

New state on the model:

```go
confirming bool   // l pressed on an unlinked row; showing the preview
errMsg     string // failure feedback rendered in the panel
```

Behavior:

- `l` on a selected **unlinked** row → `confirming = true`. `l` on a linked row
  or header is ignored.
- While `confirming`:
  - View renders a block: `── Linkify in <PageName>:<Line> ──`, a `before:`
    line (the trimmed `Context`) and an `after:` line (the same with the
    matched span wrapped in `[[…]]`), then `y confirm · n/esc cancel`.
  - `y`/`enter` → returns `OverlayResult{Linkify: &ref, LinkifyTarget: target}`.
  - `n`/`esc` → `confirming = false`, back to the list.
- `SetLinkifyError(msg string)` sets `errMsg` **and clears `confirming`** —
  on a failed write the user returns to the list with the error shown rather
  than being stuck in a confirm for a mention that is gone. `errMsg` is cleared
  when a fresh confirm starts; on a *successful* write the App rebuilds the
  whole overlay, so the new instance has no error.
- The hint line gains `· l linkify` when the selected row is an unlinked one.

The preview after-line is built purely from `Context` + `Match`:
`Context[:Match.Start] + "[[" + Context[Match.Start:Match.End] + "]]" +
Context[Match.End:]`.

### 3. `OverlayResult` + App wiring

`OverlayResult` gains:

```go
Linkify       *graph.UnlinkedRef // when set, the App linkifies this ref
LinkifyTarget string             // the page name to wrap (the panel's target)
```

A new branch in the App's overlay dispatch, ahead of the generic `Accept`
path:

```go
if res.Linkify != nil {
    ref := res.Linkify
    body, err := os.ReadFile(ref.FilePath)
    if err != nil {
        bl.SetLinkifyError("cannot read " + ref.PageName + ": " + err.Error())
        return a, nil
    }
    newBody, _, err := graph.LinkifyMention(body /* string */, ref.Line, res.LinkifyTarget)
    if err != nil {
        bl.SetLinkifyError("mention no longer found in " + ref.PageName)
        return a, nil
    }
    if err := edit.WriteFile(ref.FilePath, []byte(newBody)); err != nil {
        bl.SetLinkifyError("write failed: " + err.Error())
        return a, nil
    }
    if err := a.reindex(); err != nil {
        return a, a.setHint("reindex failed: " + err.Error())
    }
    a.active = NewBacklinks(a.idx, res.LinkifyTarget, a.unlinkedRefs(res.LinkifyTarget), a.width, a.height)
    return a, nil
}
```

(`bl` is the `*Backlinks` type-asserted from `a.active`; `os.ReadFile` returns
`[]byte`, converted to string for `LinkifyMention`.)

### 4. Small refactor — extract `(a *App) reindex()`

`createJournalAndReindex` already does `BuildIndex` + rebind `a.page`. Pull the
idx-rebuild + page-rebuild into:

```go
func (a *App) reindex() error {
    idx, err := graph.BuildIndex(a.graphPath)
    if err != nil {
        return err
    }
    a.idx = idx
    if a.page != nil {
        a.page = NewPageView(a.idx, a.page.Page(), a.width, a.height)
    }
    return nil
}
```

`createJournalAndReindex` calls it after `EnsureFile`. No behavior change to the
journal path; the linkify path reuses it. Reindex is synchronous (as the
journal path already is) so the overlay can be rebuilt immediately while it
stays open — no `indexLoadedMsg` round-trip needed.

## Selection after refresh

`NewBacklinks` resets the selection to the first selectable row. The MVP accepts
this jump-to-top after a linkify rather than threading the prior index through
the rebuild. The linkified reference has moved up into *Linked references*, so
top-of-list is a reasonable landing spot. (Revisit only if it grates in use.)

## Edge cases

- **Multiple occurrences on one line** — the first unlinked one is wrapped; the
  next surfaces on refresh and can be linkified again.
- **File changed since detection** — the fresh read + re-match defends this;
  a vanished mention yields `ErrMentionNotFound` → in-panel feedback.
- **Already linked / double-wrap** — excluded by the `wikiLinkRe` filter in the
  re-match; such a row would report "mention no longer found."
- **CRLF / trailing newline** — preserved by absolute-offset splicing.
- **Unicode words** (`Über`, `C++`, `日本`) — matched by the Unicode-aware
  whole-word post-filter, consistent with `rg -w` and the Slice 3a.1 emphasis
  logic.

## Testing

- **`graph.LinkifyMention`** (pure unit tests): happy path; preserves casing;
  CRLF body; no trailing newline; multi-occurrence (first wins); occurrence
  already inside `[[…]]` is skipped; line out of range → `ErrMentionNotFound`;
  Unicode word boundary (`Über`/`C++`).
- **`Backlinks`** (Update/View): `l` on unlinked row enters confirm; `l` on
  linked row / header is a no-op; `n`/`esc` exits confirm; `y`/`enter` returns
  the `Linkify` result; `SetLinkifyError` renders the status line. A teatest
  golden covers the confirm block.
- **App-level** (teatest): open backlinks, `l`, `y` → the source file gains
  `[[…]]` on the expected line and the panel refreshes (ref moves
  Unlinked→Linked); an unreadable/changed file path shows the in-panel error.
- **Pre-commit gate:** `go vet ./... && go test ./...` — both must pass.

## Safety net

Git is the undo. The preview/confirm step plus the authoritative re-match are
the in-app safety; no further undo machinery for the MVP.

## After 3b

This completes the *connect* roadmap (page-level linked + unlinked, read and
write). Possible later work: bulk linkify, alias matching, a cursor-anchored
completion popup, in-page `/` find (the emphasis machinery is a foundation).
