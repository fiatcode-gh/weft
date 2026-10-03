# weft — a daily-journal-driven knowledge base for the terminal

Journals are the warp — the continuous daily timeline you lay down. `[[wiki-links]]` are the weft — the cross-threads you weave across into pages. The fabric is your knowledge base, and weft is the terminal tool for keeping it: a recency-sorted page picker (with page-creation), ripgrep-backed full-text search, backlinks (linked and unlinked), and a cross-graph dashboard of your open tasks (`TODO/DOING/LATER/WAITING`) — because a journal tool that can't surface your open loops is incomplete.

One linking primitive — `[[wiki-links]]`, no tags. Pages are flat markdown under `pages/` and `journals/`, with `___` for namespaces and no nested directories to manage. weft is a navigator, not an outliner: it renders your bullets, workflow markers, and links, but it never makes you tend the tree — no fold/unfold, no block refs, no zoom.

Press `e` to edit the current page in a full-screen in-app editor (`Ctrl+S` saves, `Esc` exits with an unsaved-changes prompt); `E` hands the file to your `$EDITOR`. While editing, typing `[[` opens a live, fuzzy-filtered page-name completion list — `↑`/`↓` to choose, `Enter` or `Tab` to insert `[[Page Name]]`, `Esc` to dismiss; an unmatched name shows a `＋ Create` row to insert a link to the not-yet-created page in one keystroke. `e` refuses to open a file whose content the textarea would alter on load (CRLF line endings, tabs, or more than 10000 lines), pointing you at `E`/`$EDITOR` instead via a status-bar hint. Press `.` to jump to today's journal.

Press `S` to sync the graph with git without leaving weft — it commits any changes, `pull --rebase`s, then pushes, reporting progress in the status bar (`⟳ syncing…` → `✓ synced`). A `●` shows in the status bar whenever the graph has local changes that aren't committed or pushed yet, so you always know when a sync is due. Conflicts are left for you to resolve in a shell — weft never touches a conflicted tree.

*Already keep a Logseq graph? weft reads it as-is — the on-disk format (flat `.md`, `YYYY_MM_DD` journals, `[[wiki-links]]`, `TODO`-style bullets) is adapted from Logseq's. But weft is its own tool, not a Logseq client.*

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

## Doctor

```bash
weft doctor                  # check the graph from --graph / $WEFT_GRAPH
weft doctor --graph PATH     # explicit path
```

`weft doctor` walks the graph once and prints a health report. It is read-only and never writes to the graph.

- **unresolved links** — `[[wiki-links]]` whose target page does not exist
- **orphan pages** — pages no other page links to (journals excluded)
- **unlinked mentions** — bare-text mentions that could become links
- **index warnings** — problems the index walk noticed (unreadable files, name collisions, subdirectories)

Exit codes: `0` graph is clean, `1` findings exist, `2` operational error (bad graph path, missing ripgrep, failed scan).

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
| `S`        | sync the graph with git (commit → pull → push) |
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
| `WEFT_STYLE`   | Force a Glamour markdown style (`dark`, `light`, `ascii`, `notty`) instead of weft's default terminal-palette style, which follows your terminal's own colors. |
| `NO_COLOR`      | Honoured: forces `notty` rendering, no ANSI styling anywhere.                     |
| `WEFT_DEBUG=1` | Mirror Bubble Tea events to `weft.log` under the user cache dir (`$XDG_CACHE_HOME/weft/weft.log`, or the OS equivalent via `os.UserCacheDir`; falls back to `./weft.log` only if the cache dir is unavailable). Useful when reporting bugs. |

## Scope

Writes graph files only via the in-app editor (`e`, saves on `Ctrl+S`) and the `$EDITOR` handoff (`E`); the `.` key creates today's journal if it doesn't exist, and the picker can create a new page by name. Press `S` to sync the whole graph with git (commit → `pull --rebase` → push); it runs against the graph directory and degrades to a status-bar hint if that directory isn't a git repository. No fold/unfold, no filesystem-watch live reload (use `R`).

### What is not supported

A few Logseq features are intentionally out of scope for v1. None of them
crash weft — they degrade to plain text or a silent no-op.

- **`{{query …}}` and `{{embed …}}` blocks** are stripped from the rendered
  page (Glamour can't render them usefully).
- **Block references** don't exist — `#` is an ordinary character in page
  names, so `[[page#block]]` links to a page literally named `page#block`.
- **`alias::` / `title::` / `tags::` properties** are not extracted — they
  appear as plain text in the page body.
- **Case-insensitive page-name uniqueness** is not enforced — `Alpha.md` and
  `alpha.md` can coexist (weft records a warning — status bar points at
  `weft.log` — and resolves links deterministically to the first). Link
  resolution itself is case-insensitive: `[[alpha]]` finds `Alpha`.
- **`pages/` or `journals/` subdirectories** are skipped with a warning
  surfaced in-app. Namespace pages must use the `___` filename convention.
- **The TODO dashboard shows only open markers** (`TODO` / `LATER` /
  `DOING` / `WAITING`). `DONE` / `CANCELED` / `NOW` bullets are styled on
  the page but never appear in the dashboard.
