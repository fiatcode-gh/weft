# lstui — a read-only Logseq TUI

A terminal browser for a local Logseq graph. Fuzzy palette navigation, ripgrep-backed search, backlinks, and a TODO dashboard. Never writes to the graph.

## Install

Requires Go 1.22+ and [ripgrep](https://github.com/BurntSushi/ripgrep) on PATH.

```bash
go install github.com/fiatcode/logseq-tui/cmd/lstui@latest
```

Or from source:

```bash
git clone https://github.com/fiatcode/logseq-tui
cd logseq-tui
go build ./cmd/lstui
```

## Usage

```bash
lstui                              # uses ~/Documents/fiat-codex by default
lstui --graph /path/to/graph       # explicit path
LSTUI_GRAPH=/path/to/graph lstui   # via env var
```

Graph path resolution: `--graph` flag > `$LSTUI_GRAPH` > `~/Documents/fiat-codex`.

## Keys

| Key       | Action                              |
|-----------|-------------------------------------|
| `Ctrl-P`  | open palette (fuzzy page search)    |
| `/`       | open full-text search               |
| `T`       | open TODO dashboard                 |
| `b`       | toggle backlinks panel              |
| `n` / `N` | cycle wiki-link cursor              |
| `Enter`   | follow link / open selection        |
| `j` / `k` | scroll                              |
| `Ctrl-d/u`| half-page scroll                    |
| `R`       | rebuild index                       |
| `Esc`     | close overlay                       |
| `q`       | quit                                |

## Scope

Read-only. No editing, no fold/unfold, no live reload. Spec at `docs/superpowers/specs/2026-05-24-logseq-tui-design.md`.
