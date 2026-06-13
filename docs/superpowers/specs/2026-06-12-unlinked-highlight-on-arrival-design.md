# Slice 3a.1 — Highlight unlinked mentions on arrival

> Design spec. Written 2026-06-12. Folds into the slice-3a branch
> (`feat/unlinked-references`, PR #6). Follows the unlinked-references panel
> (3a) and the linked-backlink focus-on-jump (T6).

## Goal

When you `enter` on an **unlinked reference** in the backlinks panel, the
destination page should land with the bare-text mention **highlighted and
scrolled into view** — so you can spot where the page is mentioned, even though
it's plain text with no link to anchor a cursor on.

Linked backlinks already do this via the wiki-link cursor (T6). Unlinked refs
have no link element, so this slice adds a **render-level emphasis highlight**:
the referenced page name is highlighted as text on the destination page, and
the viewport scrolls to the first occurrence.

## Approach (chosen in discussion)

Reuse the existing **sentinel substitution machinery** that already powers
wiki-link and task-marker styling (`internal/render/page.go`). The renderer
already: (1) replaces `[[X]]` / task markers with private-use-area (PUA)
sentinels *before* Glamour, (2) renders, (3) walks `sentinelRe` over the styled
output restoring each sentinel to its styled form while recording byte offsets
(`Result.Links` / `Result.Tasks`). We add a **third sentinel type** for an
"emphasis term", styled as a highlight and recorded in a new `Result.Finds`.

This is robust against Glamour's reflow (the same reason wiki-links use
sentinels — no fragile source-line→rendered-row mapping), and consistent with
code that already exists.

## Scope

### In

- `render.RenderWithEmphasis(body, width, term)` — the existing pipeline plus a
  `preprocessEmphasis` pass that wraps whole-word, case-insensitive occurrences
  of `term` (outside fences, inline code, and existing `[[…]]` links) in a new
  emphasis sentinel; restored as a highlighted span; offsets recorded in
  `Result.Finds []int`. `Render(body, width)` becomes
  `RenderWithEmphasis(body, width, "")` (unchanged behavior, no Finds).
- `PageView` highlights + scrolls: a transient `emphasis` term that, when set,
  re-renders the page with the highlight (bypassing the per-page cache) and
  scrolls the viewport to the first `Find`.
- Navigation: `enter` on an **unlinked** backlink row navigates to the
  referencing page with the viewed page's name as the emphasis term.

### Out (non-goals)

- Persisting the highlight across `[`/`]` history navigation — it's **one-shot
  on arrival** (mirrors the todo deep-link ordinal). History nav clears it.
- A general in-page `/` find mode (a future, larger feature this could seed).
- Changing linked-backlink behavior (T6 link-cursor focus stays as-is).
- Highlighting occurrences inside code/fences or inside `[[…]]` links.

## Design details

### Render (`internal/render/page.go`)

- **New PUA sentinel pair** `emphSentinelStart` / `emphSentinelEnd`, in a PUA
  range **distinct** from the wiki and task sentinels (so the three passes can't
  collide). Extend `sentinelRe` with a third alternation capturing the emphasis
  id (a new submatch group).
- **`preprocessEmphasis(body, term string) (string, []string)`** — returns the
  rewritten body and `emphSubs` (the **original matched substring** per id, to
  preserve the user's casing on restore). When `term == ""`, returns
  `(body, nil)`. Otherwise, per non-fenced line, split on backtick and scan only
  even-indexed (outside-inline-code) segments — mirroring
  `replaceWikiLinksOutsideInlineCode` — replacing each whole-word,
  case-insensitive match of `term` with `emphSentinelStart+id+emphSentinelEnd`
  and appending the matched text to `emphSubs`. Whole-word + case-insensitive
  matches the unlinked-ref detection (`rg -w --ignore-case`): build the matcher
  as `regexp.MustCompile("(?i)\\b" + regexp.QuoteMeta(term) + "\\b")`.
- **Ordering:** run `preprocessEmphasis` **after** `preprocessWikiLinks` and
  `preprocessTaskMarkers`. By then `[[term]]` occurrences are already wiki
  sentinels (not literal text), so emphasis only matches the **bare** mentions —
  exactly the unlinked ones. (No extra "skip inside `[[…]]`" logic needed; the
  earlier pass already removed them.)
- **Restore loop:** add a case for the emphasis sentinel — decode id, emit
  `emphasisStyle.Render(emphSubs[id])`, and record the output byte offset
  (`out.Len()` before writing) into `finds`. Return `Result{..., Finds: finds}`.
- **`emphasisStyle`** — a distinct highlight (e.g. reverse-video:
  `lipgloss.NewStyle().Reverse(true)`), visually different from `linkStyle`
  (blue underline). Under `NO_COLOR`/`notty` it renders as plain text (so
  goldens stay stable); the scroll-to-find still works via the recorded offset.
- **`Result` gains `Finds []int`** — byte offsets in `Styled` of each
  highlighted occurrence, document order (mirrors `Tasks`).

### Page view (`internal/views/page.go`)

- **New field `emphasis string`** — the transient term to highlight.
- **`SetPage(name)`** also resets `p.emphasis = ""` (so a plain navigation /
  history Restore clears any prior highlight).
- **`SetPageEmphasizing(name, term string)`** — sets `page`, `cursor=-1`,
  `emphasis=term`, calls `load()`, then `scrollToFirstFind()`.
- **`load()`** — when `p.emphasis != ""`, render via
  `render.RenderWithEmphasis(body, p.width, p.emphasis)` and **bypass the cache
  entirely** (don't read or write `p.cache`, since the highlighted render is
  transient and must not be served later when emphasis is cleared). When
  `p.emphasis == ""`, the existing cached `render.Render` path is unchanged.
- **`scrollToFirstFind()`** — if `len(p.result.Finds) > 0`, scroll the viewport
  to `Finds[0]` using the same byte-offset→row→centered-`SetYOffset` math as
  `ScrollToTask`.

### Overlay + App (`internal/views/overlay.go`, `backlinks.go`, `app.go`)

- **`OverlayResult` gains `HighlightText string`** — when Accept is true and
  non-empty, the App navigates and emphasizes this term on the destination.
- **Backlinks `Update` `keyEnter`:** the **unlinked** row now returns
  `OverlayResult{Selected: r.unl.PageName, Accept: true, HighlightText: b.target}`.
  The **linked** row is unchanged (`FocusLinkTo: b.target`).
- **App Accept handler:** order is `FocusLinkTo` → `HighlightText` → `DeepLink`
  → plain `navigate`. (No overlay sets more than one; backlinks sets exactly one
  of FocusLinkTo / HighlightText per row.)
- **`App.navigateHighlighting(name, term string)`** — mirrors
  `navigateToTask`'s history bookkeeping, then `a.page.SetPageEmphasizing(name,
  term)`. One-shot: the new history entry stores no emphasis, so `[`/`]` restore
  lands without the highlight.

## Testing (TDD)

- **`render` unit tests:**
  - `RenderWithEmphasis` highlights a bare term and populates `Finds` (offset
    within `Styled`); `Render` (no term) leaves `Finds` empty.
  - emphasis does **not** match inside `[[term]]` (already a wiki sentinel),
    inside inline code, or inside a fence.
  - whole-word: `term="Alpha"` does not highlight `Alphabet`.
  - case-insensitive but **casing preserved**: `term="Alpha"` highlights a bare
    `alpha` and the restored text is still `alpha` (the user's bytes), not
    `Alpha`.
- **`PageView` tests:** `SetPageEmphasizing` populates `Finds` and moves the
  viewport (`Offset()` changes) toward the first find on a long page; `SetPage`
  afterwards clears `emphasis` and the next render has empty `Finds`. Cache is
  not polluted (a subsequent plain `SetPage` to the same page renders without
  the highlight).
- **Backlinks:** unlinked-row `enter` returns `HighlightText == b.target` and no
  `FocusLinkTo`; linked-row `enter` unchanged.
- **App:** `navigateHighlighting` (or driving the Accept path) lands on the page
  with `Finds` non-empty and the viewport scrolled.
- **Gate:** `go vet ./... && go test ./...`. Existing page/render goldens must
  stay byte-identical for the no-emphasis path (the cached `Render` path is
  unchanged).

## Risks / notes

- **Cache correctness** is the main risk: emphasis renders must never be cached
  or served when emphasis is later cleared. The bypass-cache-when-emphasis rule
  handles it; a test pins it.
- **PUA collision:** the emphasis sentinel must use codepoints distinct from the
  existing wiki/task sentinels; `sentinelRe` must match all three without
  ambiguity.
- **Offset alignment:** `Finds` offsets are recorded at `out.Len()` in the same
  restore pass as `Links`/`Tasks`, so they index the final `Styled` exactly like
  the proven existing mechanism.
