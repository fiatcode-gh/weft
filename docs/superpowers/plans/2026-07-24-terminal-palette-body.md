# Terminal-palette markdown body Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render the Glamour markdown body using only ANSI 0–15 colors so it follows the user's terminal theme instead of Glamour's hardcoded `"dark"` hues.

**Architecture:** Add a custom `ansi.StyleConfig` (`terminalStyleConfig`) whose every color is a bare ANSI 0–15 index string, and make it the default Glamour style. `NO_COLOR` and `WEFT_STYLE` remain escape hatches that select named standard styles instead.

**Tech Stack:** Go, `github.com/charmbracelet/glamour` v0.9.1 (`glamour/ansi.StyleConfig`, `glamour.WithStyles`), standard `testing` + `reflect`.

## Global Constraints

- Module path is `git.fiatcode.dev/fiatcode/weft/v2/...` (Go v2 suffix).
- `github.com/charmbracelet/x/ansi` is already imported as `ansi` in `page.go`. The Glamour style package `github.com/charmbracelet/glamour/ansi` is a **different** package with the same base name — import it only in files that do not also need `x/ansi` (i.e. the new `style.go`), so no alias is required there.
- Every color in `terminalStyleConfig` MUST be nil or a string parsing to an integer in `0..15`. No hex (`#rrggbb`), no 16–255 indices.
- Style selection reads environment variables only — it must never query the terminal (preserves the existing OSC 11 avoidance documented in `page.go`).
- `NO_COLOR` takes precedence over `WEFT_STYLE`.

---

### Task 1: Custom terminal-palette StyleConfig + palette guard

**Files:**
- Create: `internal/render/style.go`
- Test: `internal/render/style_test.go`

**Interfaces:**
- Consumes: nothing (leaf).
- Produces: package-level `var terminalStyleConfig ansi.StyleConfig` (Glamour style, colors are ANSI 0–15 index strings). Also unexported helpers `strPtr(string) *string`, `boolPtr(bool) *bool`, `uintPtr(uint) *uint` in package `render`.

- [ ] **Step 1: Write the failing palette-guard test**

Create `internal/render/style_test.go`:

```go
package render

import (
	"reflect"
	"strconv"
	"testing"
)

// collectStyleColors walks v recursively and appends the string value of every
// non-nil *string field named "Color" or "BackgroundColor" it finds. This
// reaches into nested StyleBlock/StylePrimitive/Chroma structs and the *Chroma
// pointer without hand-listing every field.
func collectStyleColors(v reflect.Value, out *[]string) {
	switch v.Kind() {
	case reflect.Ptr:
		if !v.IsNil() {
			collectStyleColors(v.Elem(), out)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			fv := v.Field(i)
			if (f.Name == "Color" || f.Name == "BackgroundColor") &&
				fv.Kind() == reflect.Ptr && fv.Type().Elem().Kind() == reflect.String {
				if !fv.IsNil() {
					*out = append(*out, fv.Elem().String())
				}
				continue
			}
			collectStyleColors(fv, out)
		}
	}
}

func TestTerminalStyleConfigUsesOnlyANSI16(t *testing.T) {
	// arrange
	var colors []string
	collectStyleColors(reflect.ValueOf(terminalStyleConfig), &colors)

	// assert
	if len(colors) == 0 {
		t.Fatal("no colors found in terminalStyleConfig; walker or config is wrong")
	}
	for _, c := range colors {
		n, err := strconv.Atoi(c)
		if err != nil || n < 0 || n > 15 {
			t.Errorf("color %q is not an ANSI 0-15 index (must be a bare 0..15)", c)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/ -run TestTerminalStyleConfigUsesOnlyANSI16`
Expected: FAIL to compile — `undefined: terminalStyleConfig`.

- [ ] **Step 3: Write the StyleConfig**

Create `internal/render/style.go`:

```go
package render

import "github.com/charmbracelet/glamour/ansi"

// terminalStyleConfig is weft's default Glamour style. Every color is an ANSI
// 0-15 index string, so the rendered markdown body draws from the user's own
// terminal palette instead of Glamour's hardcoded hues — the same way the TUI
// chrome (internal/views/theme.go) already does. Structurally mirrors Glamour's
// styles.DarkStyleConfig; only the colors differ. Headings are cyan and links
// are blue so the two stay visually distinct in the body.
//
// TestTerminalStyleConfigUsesOnlyANSI16 fails if any color here is not a bare
// ANSI 0-15 index (e.g. a "#rrggbb" hex or a 16-255 index), which would break
// terminal-theme fidelity.
var terminalStyleConfig = ansi.StyleConfig{
	Document: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			BlockPrefix: "\n",
			BlockSuffix: "\n",
			// Color unset: inherit the terminal's default foreground.
		},
		Margin: uintPtr(2),
	},
	BlockQuote: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{Color: strPtr("8")}, // dim
		Indent:         uintPtr(1),
		IndentToken:    strPtr("│ "),
	},
	List: ansi.StyleList{
		LevelIndent: 2,
	},
	Heading: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			BlockSuffix: "\n",
			Color:       strPtr("6"), // cyan
			Bold:        boolPtr(true),
		},
	},
	H1: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix:          " ",
			Suffix:          " ",
			Color:           strPtr("0"),
			BackgroundColor: strPtr("6"),
			Bold:            boolPtr(true),
		},
	},
	H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
	H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
	H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
	H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
	H6: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: "###### ",
			Color:  strPtr("8"), // dim
			Bold:   boolPtr(true),
		},
	},
	Strikethrough:  ansi.StylePrimitive{CrossedOut: boolPtr(true)},
	Emph:           ansi.StylePrimitive{Italic: boolPtr(true)},
	Strong:         ansi.StylePrimitive{Bold: boolPtr(true)},
	HorizontalRule: ansi.StylePrimitive{Color: strPtr("8"), Format: "\n--------\n"},
	Item:           ansi.StylePrimitive{BlockPrefix: "• "},
	Enumeration:    ansi.StylePrimitive{BlockPrefix: ". "},
	Task: ansi.StyleTask{
		Ticked:   "[✓] ",
		Unticked: "[ ] ",
	},
	Link: ansi.StylePrimitive{
		Color:     strPtr("4"), // blue
		Underline: boolPtr(true),
	},
	LinkText: ansi.StylePrimitive{
		Color: strPtr("4"),
		Bold:  boolPtr(true),
	},
	Image: ansi.StylePrimitive{
		Color:     strPtr("5"), // magenta
		Underline: boolPtr(true),
	},
	ImageText: ansi.StylePrimitive{
		Color:  strPtr("5"),
		Format: "Image: {{.text}} →",
	},
	Code: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: " ",
			Suffix: " ",
			Color:  strPtr("3"), // yellow
		},
	},
	CodeBlock: ansi.StyleCodeBlock{
		StyleBlock: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: strPtr("7")},
			Margin:         uintPtr(2),
		},
		Chroma: &ansi.Chroma{
			Text:                ansi.StylePrimitive{},
			Error:               ansi.StylePrimitive{Color: strPtr("15"), BackgroundColor: strPtr("1")},
			Comment:             ansi.StylePrimitive{Color: strPtr("8")},
			CommentPreproc:      ansi.StylePrimitive{Color: strPtr("4")},
			Keyword:             ansi.StylePrimitive{Color: strPtr("4")},
			KeywordReserved:     ansi.StylePrimitive{Color: strPtr("4")},
			KeywordNamespace:    ansi.StylePrimitive{Color: strPtr("4")},
			KeywordType:         ansi.StylePrimitive{Color: strPtr("4")},
			Operator:            ansi.StylePrimitive{Color: strPtr("7")},
			Punctuation:         ansi.StylePrimitive{Color: strPtr("7")},
			Name:                ansi.StylePrimitive{},
			NameBuiltin:         ansi.StylePrimitive{Color: strPtr("5")},
			NameTag:             ansi.StylePrimitive{Color: strPtr("5")},
			NameAttribute:       ansi.StylePrimitive{Color: strPtr("6")},
			NameClass:           ansi.StylePrimitive{Color: strPtr("5"), Underline: boolPtr(true), Bold: boolPtr(true)},
			NameDecorator:       ansi.StylePrimitive{Color: strPtr("5")},
			NameFunction:        ansi.StylePrimitive{Color: strPtr("6")},
			LiteralNumber:       ansi.StylePrimitive{Color: strPtr("5")},
			LiteralString:       ansi.StylePrimitive{Color: strPtr("2")},
			LiteralStringEscape: ansi.StylePrimitive{Color: strPtr("2")},
			GenericDeleted:      ansi.StylePrimitive{Color: strPtr("1")},
			GenericEmph:         ansi.StylePrimitive{Italic: boolPtr(true)},
			GenericInserted:     ansi.StylePrimitive{Color: strPtr("2")},
			GenericStrong:       ansi.StylePrimitive{Bold: boolPtr(true)},
			GenericSubheading:   ansi.StylePrimitive{Color: strPtr("8")},
			Background:          ansi.StylePrimitive{}, // no forced bg; terminal default
		},
	},
	Table: ansi.StyleTable{
		StyleBlock: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{}},
	},
	DefinitionDescription: ansi.StylePrimitive{BlockPrefix: "\n🠶 "},
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func uintPtr(u uint) *uint    { return &u }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/render/ -run TestTerminalStyleConfigUsesOnlyANSI16`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/render/style.go internal/render/style_test.go
git commit -m "feat(render): add terminal-palette Glamour style (ANSI 0-15)"
```

---

### Task 2: Wire style selection into the renderer

**Files:**
- Modify: `internal/render/page.go` (replace `styleName` at lines ~153-167; update `rendererFor` at ~185-188)
- Test: `internal/render/style_test.go` (add selection test)

**Interfaces:**
- Consumes: `terminalStyleConfig` (Task 1); `glamour.WithStyles`, `glamour.WithStandardStyle`.
- Produces: `type styleSelection struct { terminal bool; name string }`; `func selectStyle() styleSelection`; `func styleOption() glamour.TermRendererOption`. Removes `func styleName() string`.

- [ ] **Step 1: Write the failing selection test**

Add to `internal/render/style_test.go`:

```go
func TestSelectStyle(t *testing.T) {
	cases := []struct {
		name     string
		noColor  string
		weftS    string
		want     styleSelection
	}{
		{"defaults to terminal palette", "", "", styleSelection{terminal: true}},
		{"NO_COLOR forces notty", "1", "", styleSelection{name: "notty"}},
		{"WEFT_STYLE selects named", "", "dracula", styleSelection{name: "dracula"}},
		{"NO_COLOR beats WEFT_STYLE", "1", "dracula", styleSelection{name: "notty"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			t.Setenv("NO_COLOR", tc.noColor)
			t.Setenv("WEFT_STYLE", tc.weftS)

			// act
			got := selectStyle()

			// assert
			if got != tc.want {
				t.Errorf("selectStyle() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/ -run TestSelectStyle`
Expected: FAIL to compile — `undefined: styleSelection` / `undefined: selectStyle`.

- [ ] **Step 3: Replace `styleName` with the selection helpers**

In `internal/render/page.go`, replace the whole `styleName` function (the block starting with the `// styleName picks the Glamour style ...` comment through the closing `}`) with:

```go
// styleSelection is which Glamour style weft should render with. selectStyle
// derives it from env vars only — never querying the terminal.
type styleSelection struct {
	terminal bool   // use terminalStyleConfig (weft's default; honors the terminal palette)
	name     string // standard style name, meaningful only when !terminal
}

// selectStyle chooses the render style without any terminal IO. This is
// deliberate: Glamour's WithAutoStyle issues OSC 11 background-colour queries
// over stdin, which can leave stray reply bytes in the terminal's input
// buffer. When weft is quit and immediately re-opened, the next session's
// termenv reads those stale bytes, fails to parse them, and blocks for
// seconds before timing out. Reading env vars sidesteps the problem.
//
// NO_COLOR wins over WEFT_STYLE.
func selectStyle() styleSelection {
	if os.Getenv("NO_COLOR") != "" {
		return styleSelection{name: "notty"}
	}
	if s := os.Getenv("WEFT_STYLE"); s != "" {
		return styleSelection{name: s}
	}
	return styleSelection{terminal: true}
}

// styleOption turns the selection into the Glamour renderer option.
func styleOption() glamour.TermRendererOption {
	sel := selectStyle()
	if sel.terminal {
		return glamour.WithStyles(terminalStyleConfig)
	}
	return glamour.WithStandardStyle(sel.name)
}
```

- [ ] **Step 4: Use `styleOption()` in `rendererFor`**

In `internal/render/page.go`, inside `rendererFor`, change the renderer construction from:

```go
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(styleName()),
		glamour.WithWordWrap(width),
	)
```

to:

```go
	r, err := glamour.NewTermRenderer(
		styleOption(),
		glamour.WithWordWrap(width),
	)
```

- [ ] **Step 5: Run the render suite to verify pass**

Run: `go test ./internal/render/`
Expected: PASS (both new tests and all existing render tests).

- [ ] **Step 6: Verify nothing else referenced `styleName`**

Run: `grep -rn 'styleName' internal/ cmd/`
Expected: no output (the function is fully removed and had no other callers).

- [ ] **Step 7: Commit**

```bash
git add internal/render/page.go internal/render/style_test.go
git commit -m "feat(render): default to terminal-palette style; keep NO_COLOR/WEFT_STYLE hatches"
```

---

## Final verification

- [ ] Run the full suite: `go test ./...` — expected PASS.
- [ ] Run vet/staticcheck as CI does: `go vet ./...` — expected clean.
- [ ] Manual smoke (optional, terminal-dependent): open weft on a page with headings, a code fence, and task markers; confirm colors track the terminal theme and switch when the terminal theme switches.
