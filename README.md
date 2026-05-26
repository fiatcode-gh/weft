# peekseq — a read-only Logseq TUI

A terminal browser for a local Logseq graph. Recency-sorted page picker, ripgrep-backed full-text search, backlinks, and a TODO dashboard. Renders pages with hanging-indent bullets, coloured workflow markers (TODO/DOING/LATER/WAITING/DONE/CANCELED/NOW), and highlighted wiki-links you can step through. Never writes to the graph.

## Install

Requires Go 1.26+ and [ripgrep](https://github.com/BurntSushi/ripgrep) on PATH.

```bash
go install git.fiatcode.dev/fiatcode/peekseq/cmd/peekseq@latest
```

Or from source:

```bash
git clone https://git.fiatcode.dev/fiatcode/peekseq
cd peekseq
go build ./cmd/peekseq
```

## Usage

```bash
peekseq --graph /path/to/graph       # explicit path
PEEKSEQ_GRAPH=/path/to/graph peekseq # via env var
peekseq -version                     # print version and exit
```

A graph path is required — either pass `--graph` or set `$PEEKSEQ_GRAPH`. The flag wins when both are set. Drop the env var into your shell config for the zero-arg invocation.

## Keys

Press `?` from the page view at any time to see a grouped keymap inside the app.

| Key        | Action                              |
|------------|-------------------------------------|
| `Ctrl-P`   | open picker (recent + fuzzy)        |
| `/`        | open full-text search (ripgrep)     |
| `T`        | open TODO dashboard                 |
| `b`        | open backlinks for the current page |
| `?`        | toggle the help overlay             |
| `n` / `N`  | cycle the wiki-link cursor          |
| `Enter`    | follow link / open selection        |
| `[` / `]`  | back / forward in page history      |
| `j` / `k`  | scroll one line                     |
| `Ctrl-d/u` | half-page scroll                    |
| `g` / `G`  | jump to top / bottom of page        |
| `R`        | rebuild the index                   |
| `Esc`      | close an overlay                    |
| `q`        | quit (from page view)               |

## What gets rendered

- `[[wiki-links]]` are styled inline, navigable with `n`/`N`, and follow with `Enter`. Aliased links (`[[Target|alias]]`) show the alias.
- Workflow markers at the start of a bullet are colour-coded (`TODO` red, `DOING` yellow, `LATER` blue, `WAITING` dim, `DONE` green, `CANCELED`/`CANCELLED` strikethrough, `NOW` magenta).
- `:LOGBOOK: ... :END:` blocks are hidden — they're metadata, not content.
- Long bullets wrap with hanging indent, so continuation lines align with the text after the bullet rather than under the bullet glyph.
- Backlinks filters self-references — viewing a page never lists that page's own mentions of itself.

## Environment

| Variable        | Effect                                                                            |
|-----------------|-----------------------------------------------------------------------------------|
| `PEEKSEQ_GRAPH`   | Default graph path (overridden by `--graph`).                                     |
| `PEEKSEQ_STYLE`   | Force a Glamour markdown style (`dark`, `light`, `ascii`, `notty`). Default `dark`. |
| `NO_COLOR`      | Honoured: forces `notty` rendering, no ANSI styling anywhere.                     |
| `PEEKSEQ_DEBUG=1` | Mirror Bubble Tea events to `./peekseq.log`. Useful when reporting bugs.            |

## Scope

Read-only. No editing, no fold/unfold, no filesystem-watch live reload (use `R`).
