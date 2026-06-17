# weft — a daily-journal-driven knowledge base for the terminal

Journals are the warp — the continuous daily timeline you lay down. `[[wiki-links]]` are the weft — the cross-threads you weave across into pages. The fabric is your knowledge base, and weft is the terminal tool for keeping it: a recency-sorted page picker (with page-creation), ripgrep-backed full-text search, backlinks (linked and unlinked), and a cross-graph dashboard of your open tasks (`TODO/DOING/LATER/WAITING`) — because a journal tool that can't surface your open loops is incomplete.

One linking primitive — `[[wiki-links]]`, no tags. Pages are flat markdown under `pages/` and `journals/`, with `___` for namespaces and no nested directories to manage. weft is a navigator, not an outliner: it renders your bullets, workflow markers, and links, but it never makes you tend the tree — no fold/unfold, no block refs, no zoom.

Press `e` to edit the current page in a full-screen in-app editor (`Ctrl+S` saves, `Esc` exits with an unsaved-changes prompt); `E` hands the file to your `$EDITOR`. While editing, typing `[[` opens a live, fuzzy-filtered page-name completion list — `↑`/`↓` to choose, `Enter` or `Tab` to insert `[[Page Name]]`, `Esc` to dismiss; an unmatched name shows a `＋ Create` row to drop a red link in one keystroke. Press `.` to jump to today's journal.

*Already keep a Logseq graph? weft reads it as-is — the on-disk format (flat `.md`, `YYYY_MM_DD` journals, `[[wiki-links]]`, `TODO`-style bullets) is adapted from Logseq's. But weft is its own tool, not a Logseq client.*

## Install

Requires Go 1.26+ and [ripgrep](https://github.com/BurntSushi/ripgrep) 14+ on PATH
(the `--json` output format that the search view consumes was added in rg 14).

```bash
go install git.fiatcode.dev/fiatcode/weft/v2/cmd/weft@latest
```

Or from source:

```bash
git clone https://git.fiatcode.dev/fiatcode/weft
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

## Keys

Press `?` from the page view at any time to see a grouped keymap inside the app.

| Key        | Action                              |
|------------|-------------------------------------|
| `Ctrl-P`   | open picker (recent + fuzzy)        |
| `/`        | open full-text search (ripgrep)     |
| `T`        | open TODO dashboard                 |
| `b`        | open backlinks for the current page (linked + unlinked refs) |
| `?`        | toggle the help overlay             |
| `n` / `N`  | cycle the wiki-link cursor          |
| `Enter`    | follow link / open selection        |
| `[` / `]`  | back / forward in page history      |
| `.`        | jump to today's journal (creates it if missing) |
| `<` / `>`  | previous / next journal (on a journal page) |
| `j` / `k`  | scroll one line                     |
| `Ctrl-d/u` | half-page scroll                    |
| `g` / `G`  | jump to top / bottom of page        |
| `R`        | rebuild the index                   |
| `e`        | edit current page in-app            |
| `E`        | edit current page in `$EDITOR`      |
| `Esc`      | close an overlay / leave editor     |
| `q`        | quit (from page view)               |

## What gets rendered

- `[[wiki-links]]` are styled inline, navigable with `n`/`N`, and follow with `Enter`. Aliased links (`[[Target|alias]]`) show the alias.
- Workflow markers at the start of a bullet are colour-coded (`TODO` red, `DOING` yellow, `LATER` blue, `WAITING` dim, `DONE` green, `CANCELED`/`CANCELLED` strikethrough, `NOW` magenta).
- `:LOGBOOK: ... :END:` blocks are hidden — they're metadata, not content.
- Long bullets wrap with hanging indent, so continuation lines align with the text after the bullet rather than under the bullet glyph.
- Backlinks (`b`) shows two sections: **Linked references** (`[[…]]` mentions) followed by **Unlinked references** — bare-text mentions of the page name that aren't yet wiki-linked. `Enter` on either section jumps to the mention's page and lands on the reference itself: a linked backlink focuses the back-reference `[[link]]` (cursor on it, scrolled into view), and an unlinked reference highlights the bare-text mention and scrolls it into view. Self-references are always excluded.

## Environment

| Variable        | Effect                                                                            |
|-----------------|-----------------------------------------------------------------------------------|
| `WEFT_GRAPH`   | Default graph path (overridden by `--graph`).                                     |
| `WEFT_STYLE`   | Force a Glamour markdown style (`dark`, `light`, `ascii`, `notty`). Default `dark`. |
| `NO_COLOR`      | Honoured: forces `notty` rendering, no ANSI styling anywhere.                     |
| `WEFT_DEBUG=1` | Mirror Bubble Tea events to `./weft.log`. Useful when reporting bugs.            |

## Scope

Writes only via the in-app editor (`e`, saves on `Ctrl+S`) and the `$EDITOR` handoff (`E`); the `.` key creates today's journal if it doesn't exist, and the picker can create a new page by name. No fold/unfold, no filesystem-watch live reload (use `R`).

### What is not supported

A few Logseq features are intentionally out of scope for v1. None of them
crash weft — they degrade to plain text or a silent no-op.

- **`{{query …}}` and `{{embed …}}` blocks** are stripped from the rendered
  page (Glamour can't render them usefully).
- **Block references `[[page#block]]`** resolve to `page`; the `#block`
  fragment is dropped.
- **`alias::` / `title::` / `tags::` properties** are not extracted — they
  appear as plain text in the page body.
- **Case-insensitive linking** is not enforced. `[[alpha]]` on a page called
  `Alpha` resolves correctly.
- **`pages/` or `journals/` subdirectories** are skipped with a stderr
  warning. Namespace pages must use the `___` filename convention.
- **The TODO dashboard shows only open markers** (`TODO` / `LATER` /
  `DOING` / `WAITING`). `DONE` / `CANCELED` / `NOW` bullets are styled on
  the page but never appear in the dashboard.
