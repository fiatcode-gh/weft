# Terminal-palette markdown body

## Problem

weft has two coloring systems:

- **Chrome** (links, task markers, selection bar, status bar, todos dashboard,
  editor tinting) already uses ANSI palette indices 0–15, so it honors the
  user's terminal theme today.
- **Markdown body** is rendered by Glamour with the built-in `"dark"` style
  (`render/page.go`, `glamour.WithStandardStyle(styleName())`). That style
  paints elements in fixed ANSI-256 indices (`"39"`, `"228"`, `"63"`, …) and
  truecolor hex (`#00AAFF`, …) in the Chroma syntax highlighter. Those colors
  ignore the terminal theme — this is the "weft's own color scheme" the user
  wants gone.

Goal: make the markdown body draw its colors from the terminal's own 16-color
palette, the same way the chrome already does.

## Approach

Replace Glamour's `"dark"` `StyleConfig` with a custom `ansi.StyleConfig` that
uses **only ANSI 0–15 index strings**. Indexed colors 0–15 render as the
terminal's own palette slots (termenv keeps them as ANSI-16 SGR escapes rather
than converting to RGB), so the body follows whatever theme the user has tuned.

Using indexed colors also sidesteps light-vs-dark detection: the user's theme
already defines those 16 slots to be legible on its own background, and it is a
palette the user has tuned for their (deuteranomalous) color vision — strictly
better than Glamour's fixed hues.

Glamour v0.9.1 exposes `glamour.WithStyles(ansi.StyleConfig)` for exactly this.

## Style-selection logic (`render/page.go`)

Both existing escape hatches are preserved; only the default path changes.

- `NO_COLOR` set → `notty` standard style (unchanged — no color).
- `WEFT_STYLE` set → that named standard style (unchanged — lets anyone force
  `dark`/`dracula`/`light`/etc. back).
- **Default (neither set) → the custom terminal-palette `StyleConfig`** via
  `glamour.WithStyles(...)`. This is the behavior change.

`WEFT_STYLE` pointing at a JSON style *file* is deliberately out of scope
(YAGNI); it stays a named-style selector.

The IO-free `styleName()` rationale (avoid Glamour's OSC 11 background query,
which leaves stray bytes in the input buffer) is retained — style selection
still reads env vars only, never queries the terminal.

## Palette mapping (`internal/render/style.go`, new file)

Elements are distinguished by *type*, each mapped to an ANSI 0–15 slot:

| Element | ANSI slot |
|---|---|
| Document / body text | unset → terminal default fg |
| Heading (H2–H5) | `6` cyan, bold |
| H1 | fg `0` / bg `6`, bold (inverse accent) |
| H6 | `8` dim, bold |
| Inline code | `3` yellow |
| Code-block base text | `7`, dim |
| Blockquote indent | `8` dim |
| Horizontal rule | `8` dim |
| Link | `4` blue, underline |
| LinkText | `4` blue, bold |
| Image / ImageText | `5` magenta |
| Emph / Strong / Strikethrough | attribute only (italic/bold/crossed), no color |

Headings are cyan and links are blue so the two stay visually distinct in the
body.

### Code-fence syntax highlighting (Chroma) — hex anchors + `terminal16`

Code fences do **not** go through termenv; Glamour hands them to the `chroma`
library, whose color parser is `strconv.ParseUint(colour, 16, 32)` — it reads
every color string as **hex RGB**. So a bare ANSI index like `"5"` would be
interpreted as `#000005` (near-black), not palette-5. Chroma has no concept of
the terminal's themed 16 colors *at the style level*.

The terminal-native result is instead achieved at the *formatter* level:
chroma's `terminal16` formatter downsamples a style's RGB colors to the nearest
of the terminal's 16 ANSI palette slots. So the Chroma block uses **pure hex
anchor colors** (chosen to land on the intended palette slot), and the renderer
selects the `terminal16` formatter via `glamour.WithChromaFormatter("terminal16")`.
Net user-visible result: code fences colored from the terminal's own 16 palette.

| Token group | hex anchor | lands on |
|---|---|---|
| Keyword / KeywordReserved / KeywordNamespace / KeywordType | `#0000ff` | blue |
| LiteralString / LiteralStringEscape / GenericInserted | `#00ff00` | green |
| LiteralNumber | `#ff00ff` | magenta |
| Comment / GenericSubheading | `#808080` | dim/grey |
| NameFunction | `#00ffff` | cyan |
| NameBuiltin / NameClass / NameTag / NameDecorator | `#ff00ff` | magenta |
| Operator / Punctuation | `#c0c0c0` | light grey |
| Error | fg `#ffffff` / bg `#ff0000` | white on red |
| GenericDeleted | `#ff0000` | red |
| Text / Name / Background | unset → default |

The `terminal16` formatter is applied **only on the default (terminal-palette)
path** — a `WEFT_STYLE=dracula` user keeps chroma's default `terminal256`
formatter so their chosen theme renders at full fidelity. Prefixes/suffixes and
block structure (indent tokens, `##` heading prefixes, list bullets) carry over
from Glamour's dark config unchanged — only colors change.

## Testing (TDD)

Two robust, non-flaky tests (no dependency on the runtime terminal profile):

1. **Palette guard (split invariant).** Walk every `Color`/`BackgroundColor`
   pointer in the custom `StyleConfig`. Colors **outside** the `CodeBlock.Chroma`
   subtree (termenv-rendered) must each be nil or an ANSI `0..15` index string —
   fails if a hex or 256-index color is reintroduced. Colors **inside** the
   `Chroma` subtree must each be nil or a valid `#rrggbb` hex anchor — fails if a
   bare index (which chroma would misparse as hex) sneaks in. This encodes both
   halves of the mechanism.
2. **Style selection.** Table test over `(NO_COLOR, WEFT_STYLE)` env
   combinations asserting the selector returns the correct path: notty /
   named-standard-style / terminal-custom.

## Scope

- `internal/render/page.go` — style-selection logic.
- `internal/render/style.go` — new file, the custom `StyleConfig`.
- `internal/render/*_test.go` — the two tests above.

No chrome changes. `Warmup` and the per-width renderer cache are unaffected.
