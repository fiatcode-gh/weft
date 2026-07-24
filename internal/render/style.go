package render

import "github.com/charmbracelet/glamour/ansi"

// terminalStyleConfig is weft's default Glamour style, drawing the rendered
// markdown body from the user's own terminal palette instead of Glamour's
// hardcoded hues — the same way the TUI chrome (internal/views/theme.go) does.
// Structurally mirrors Glamour's styles.DarkStyleConfig; only the colors differ.
// Headings are cyan and links are blue so the two stay visually distinct.
//
// Colors come in two flavors, guarded by TestTerminalStyleConfigColors:
//   - termenv-rendered colors (everything outside CodeBlock.Chroma) are bare
//     ANSI 0-15 index strings, which termenv maps to the terminal palette.
//   - Chroma (code-fence) colors are #rrggbb hex anchors: chroma parses colors
//     as hex RGB, and the terminal16 formatter (see page.go) downsamples them
//     to the terminal's 16-color palette.
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
		// Chroma colors are hex anchors, NOT ANSI indices: chroma parses color
		// strings as hex RGB (strconv.ParseUint base 16). The terminal16
		// formatter (see page.go) downsamples these anchors to the terminal's
		// 16-color palette, so syntax highlighting honors the terminal theme.
		Chroma: &ansi.Chroma{
			Text:                ansi.StylePrimitive{},
			Error:               ansi.StylePrimitive{Color: strPtr("#ffffff"), BackgroundColor: strPtr("#ff0000")},
			Comment:             ansi.StylePrimitive{Color: strPtr("#808080")},
			CommentPreproc:      ansi.StylePrimitive{Color: strPtr("#0000ff")},
			Keyword:             ansi.StylePrimitive{Color: strPtr("#0000ff")},
			KeywordReserved:     ansi.StylePrimitive{Color: strPtr("#0000ff")},
			KeywordNamespace:    ansi.StylePrimitive{Color: strPtr("#0000ff")},
			KeywordType:         ansi.StylePrimitive{Color: strPtr("#0000ff")},
			Operator:            ansi.StylePrimitive{Color: strPtr("#c0c0c0")},
			Punctuation:         ansi.StylePrimitive{Color: strPtr("#c0c0c0")},
			Name:                ansi.StylePrimitive{},
			NameBuiltin:         ansi.StylePrimitive{Color: strPtr("#ff00ff")},
			NameTag:             ansi.StylePrimitive{Color: strPtr("#ff00ff")},
			NameAttribute:       ansi.StylePrimitive{Color: strPtr("#00ffff")},
			NameClass:           ansi.StylePrimitive{Color: strPtr("#ff00ff"), Underline: boolPtr(true), Bold: boolPtr(true)},
			NameDecorator:       ansi.StylePrimitive{Color: strPtr("#ff00ff")},
			NameFunction:        ansi.StylePrimitive{Color: strPtr("#00ffff")},
			LiteralNumber:       ansi.StylePrimitive{Color: strPtr("#ff00ff")},
			LiteralString:       ansi.StylePrimitive{Color: strPtr("#00ff00")},
			LiteralStringEscape: ansi.StylePrimitive{Color: strPtr("#00ff00")},
			GenericDeleted:      ansi.StylePrimitive{Color: strPtr("#ff0000")},
			GenericEmph:         ansi.StylePrimitive{Italic: boolPtr(true)},
			GenericInserted:     ansi.StylePrimitive{Color: strPtr("#00ff00")},
			GenericStrong:       ansi.StylePrimitive{Bold: boolPtr(true)},
			GenericSubheading:   ansi.StylePrimitive{Color: strPtr("#808080")},
			Background:          ansi.StylePrimitive{},
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
