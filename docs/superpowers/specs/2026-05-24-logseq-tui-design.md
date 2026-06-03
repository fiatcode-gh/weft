# Logseq TUI — Design (v1)

**Status:** draft, awaiting user review
**Date:** 2026-05-24
**Repo:** `~/Development/Projects/fiatcode/logseq-tui`
**Working name:** `lstui` (final binary name TBD before first commit of code)

## Goal

A read-only terminal UI for browsing a local Logseq graph (default: `~/Documents/fiat-codex`). The tool serves three roughly equal use cases:

1. **Quick lookups** — open a page or today's journal fast from the shell.
2. **Sit-and-read browsing** — read pages, follow `[[links]]`, inspect backlinks.
3. **TODO surfacing** — see open `TODO`/`LATER`/`DOING` bullets across the graph.

The tool never writes to the graph. Editing is explicitly out of scope for v1.

## Scope

**In scope:**

- Fuzzy command-palette navigation across page names + journal dates
- Page viewer rendering markdown with indentation preserved (flat — no fold/unfold)
- Full-text search via `ripgrep`
- Backlinks panel for the current page
- TODO/LATER/DOING dashboard, grouped by page, filterable by marker
- Manual index refresh (`R`)

**Out of scope (v1):**

- Any form of editing or write-back to the graph
- Block expand/collapse (outliner folding)
- Filesystem watch / live reload (manual `R` refresh only)
- Datalog query support
- Multi-graph switching (single graph path per invocation)
- Persistent index cache on disk (rebuild every launch)

**Deferred to v2 candidates:** filesystem watch, block fold/unfold, quick-capture append (the only write feature that would still feel safe), multi-graph profiles.

## Stack

- **Language:** Go (single static binary, fast startup matters for quick-lookup use case)
- **TUI framework:** Bubble Tea + Bubbles + Lip Gloss
- **Markdown rendering:** Glamour
- **Fuzzy matching:** `sahilm/fuzzy`
- **Search:** shell out to `rg` (ripgrep) — checked on PATH at startup, hard-fail with a useful message if missing
- **Testing:** standard `testing` + `teatest` for view snapshots

## Architecture

### Data model

Three structures built in-memory by the indexer at startup:

```
PageMeta { Name string; Path string; IsJournal bool; Date *time.Time }
Ref      { FromPage string; LineNumber int; Context string }
TodoBullet { Page string; LineNumber int; Marker string; Text string; Priority string }
```

- `pages []PageMeta` — every `.md` file under `pages/` and `journals/`.
- `backlinks map[string][]Ref` — reverse index of `[[wiki-links]]`.
- `todos []TodoBullet` — every bullet whose text starts with `TODO`/`LATER`/`DOING`/`WAITING` (case-sensitive, matching Logseq conventions).

Page *content* is not held in memory — it's read on demand when the user opens a page. The resident set is just the index.

### Indexing

- Single pass at startup over `<graph>/pages/*.md` and `<graph>/journals/*.md`.
- For each file: parse filename → page name (Logseq uses `___` to encode `/` in namespaces; e.g., `foo___bar.md` → page `foo/bar`). Walk lines, extract `[[wiki-links]]` and TODO bullets.
- Expected runtime: <100ms for graphs up to a few thousand files. Verified against `~/Documents/fiat-codex` during implementation.
- No persistence. Rebuild every launch. Manual `R` keybind to rebuild mid-session.

### Package layout

```
cmd/lstui/main.go           # entry, flag parsing, graph path resolution
internal/graph/
  index.go                  # walk + build PageMeta/backlinks/todos
  parse.go                  # extract [[links]], todo bullets, properties
  resolve.go                # page name ↔ filename (namespace ___ handling)
internal/views/
  app.go                    # top-level Bubble Tea model, view switcher
  page.go                   # page viewer
  palette.go                # fuzzy palette overlay
  search.go                 # ripgrep results overlay
  backlinks.go              # backlinks side panel
  todos.go                  # TODO dashboard
internal/render/
  page.go                   # glamour wrapper + wiki-link highlight
testdata/fixture-graph/     # ~10 pages, 3 journals, mix of TODOs + links
```

### Why not DDD

The "domain" is data records (pages, blocks, links, todos) with no behavior, no invariants worth defending in code, no transactional boundaries. DDD ceremony (aggregates, repositories, value objects, domain services) would multiply code volume without preventing any class of bug that's actually possible at this scale. The package layout above already gives appropriate-weight separation: `graph` owns the model, `render` owns presentation, `views` owns interaction. Revisit if v2 introduces write/edit features with real outliner invariants (parent/child consistency, transitive ref updates).

## UX

### Views (one active at a time, `App` model swaps them)

**Page view** (default — opens to today's journal):
- Rendered via Glamour with indentation preserved.
- `[[wiki-links]]` post-processed to be highlighted and navigable.
- `n` / `N` cycles cursor between links; `Enter` follows.
- `b` toggles backlinks side panel.
- Status bar: page name + key hints.

**Palette overlay** (`Ctrl-P`):
- Fuzzy match against page names + journal dates.
- Journal dates auto-included as virtual entries even if the file doesn't exist (`2026-05-24` → opens read-only with "no entry yet" placeholder).
- `Enter` opens; `Esc` cancels.

**Search overlay** (`/`):
- Spawns `rg --json <query> <graph>` as a `tea.Cmd`; streams results into a scrollable list.
- Each result: page name, line number, surrounding context.
- `Enter` opens the page scrolled to the matching line.

**Backlinks panel** (`b` toggles from page view):
- Right-side panel listing pages that link to the current one, with the surrounding bullet as context.
- `j`/`k` to navigate, `Enter` to follow.

**TODO dashboard** (`T`):
- Full-screen list of all `TODO`/`LATER`/`DOING`/`WAITING` bullets.
- Grouped by page (collapsible group headers via `space`).
- `t` cycles marker filter: TODO → LATER → DOING → WAITING → all.
- `Enter` opens the source page at the bullet's line.

### Key bindings (summary)

| Key | Context | Action |
|-----|---------|--------|
| `Ctrl-P` | any | open palette |
| `/` | any | open search |
| `T` | any | open TODO dashboard |
| `b` | page view | toggle backlinks panel |
| `n` / `N` | page view | cycle wiki-link cursor |
| `Enter` | any list / link cursor | open / follow |
| `Esc` | overlay | drop back to page |
| `R` | any | rebuild index |
| `q` | page view | quit |

### Long pages

With block fold/unfold out of scope, a journal with 200 bullets is one long scroll. Accepted tradeoff for v1. `Ctrl-d`/`Ctrl-u` half-page scroll standard.

## Data flow

```
launch
  → resolve graph path (flag > env > default ~/Documents/fiat-codex)
  → indexer goroutine builds PageMeta / backlinks / todos
  → App model boots into page view at today's journal
  → keypress
      → view-specific handler
      → may dispatch tea.Cmd (rg search, file read)
      → results return via tea.Msg
      → view rerenders
```

All file reads are on demand. Only the index lives in memory.

## Error handling

- **Missing `rg` on PATH:** hard-fail at startup with a clear message naming the binary and a hint to install it. Don't pretend search works.
- **Missing graph path:** hard-fail with the resolved path and where it came from (flag/env/default).
- **Unreadable file during indexing:** log to stderr, skip the file, continue. Don't abort the index.
- **Dangling `[[wiki-link]]`:** render as a distinct style (e.g., dim) — following it opens an empty placeholder view with the would-be page name.
- **Malformed markdown:** Glamour handles it; fall back to plain text rendering if Glamour errors.

## Testing strategy

Per global TDD instructions: failing test first for every feature.

- **`internal/graph/parse.go`** — table-driven unit tests. Cover: namespace filenames (`foo___bar.md`), wiki-link extraction including aliases (`[[Page|alias]]`), TODO markers with priorities (`TODO [#A]`), property blocks (`key:: value`), code-block bodies (links inside fenced code must NOT be extracted).
- **`internal/graph/index.go`** — integration test against `testdata/fixture-graph/`. Assert: page count, backlink map (including dangling refs), TODO list contents and ordering.
- **`internal/render/page.go`** — golden-file tests for wiki-link highlighting and Glamour fallback on malformed input.
- **Search** — integration test shelling out to real `rg` against the fixture. Skip with a clear message if `rg` isn't on PATH (not a silent pass).
- **Views** — `teatest` snapshot tests for each view's rendered output given known model state.

## Dependencies (v1, pinned in `go.mod`)

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles`
- `github.com/charmbracelet/lipgloss`
- `github.com/charmbracelet/glamour`
- `github.com/charmbracelet/x/exp/teatest` (test-only)
- `github.com/sahilm/fuzzy`

External runtime dependency: `rg` (ripgrep).

## Configuration

- Graph path resolution order: `--graph <path>` flag > `LSTUI_GRAPH` env var > `~/Documents/fiat-codex` default.
- No config file in v1. Add only when a second config knob materializes.

## Open questions

None blocking. Final binary name (`lstui` is a placeholder) to be chosen at first commit.
