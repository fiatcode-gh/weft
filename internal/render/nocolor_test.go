package render

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// styledRows parses SGR output into screen cells the way Bubble Tea's
// renderer sees them, then converts each style to profile p.
func styledRows(s string, width int, p colorprofile.Profile) [][]uv.Cell {
	ss := uv.NewStyledString(s)
	h := ss.Height()
	buf := uv.NewScreenBuffer(width, h)
	ss.Draw(buf, uv.Rect(0, 0, width, h))
	rows := make([][]uv.Cell, h)
	for y := range h {
		for x := range width {
			c := buf.CellAt(x, y)
			if c == nil {
				c = &uv.EmptyCell
			}
			cell := *c
			cell.Style = uv.ConvertStyle(cell.Style, p)
			rows[y] = append(rows[y], cell)
		}
	}
	return rows
}

// cellsOf returns the cells that display text, found on one row.
func cellsOf(t *testing.T, rows [][]uv.Cell, text string) []uv.Cell {
	t.Helper()
	for _, row := range rows {
		var sb strings.Builder
		for _, c := range row {
			sb.WriteString(c.Content)
		}
		line := sb.String()
		if i := strings.Index(line, text); i >= 0 {
			start := utf8.RuneCountInString(line[:i])
			return row[start : start+utf8.RuneCountInString(text)]
		}
	}
	t.Fatalf("no row contains %q", text)
	return nil
}

func requireAttr(t *testing.T, cells []uv.Cell, attr uint8, what string) {
	t.Helper()
	for _, c := range cells {
		if c.Style.Attrs&attr == 0 {
			t.Errorf("%s: cell %q lacks the attribute (style %q)", what, c.Content, c.Style.String())
		}
	}
}

func TestNoColorAddsOnlyAttributes(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "1", "WEFT_STYLE": ""}) {
		return
	}
	const width = 60
	body := strings.TrimSpace(goldenCorpus) + "\n"

	got, err := Render(body, width)
	if err != nil {
		t.Fatal(err)
	}

	// The same render through plain ASCII style: the reference layout.
	ascii, err := glamour.NewTermRenderer(
		glamour.WithStyles(*styles.DefaultStyles["notty"]),
		glamour.WithChromaFormatter("terminal256"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		t.Fatal(err)
	}
	rendererMu.Lock()
	rendererCache = map[rendererKey]*glamour.TermRenderer{{"notty", width}: ascii}
	rendererMu.Unlock()
	ref, err := Render(body, width)
	if err != nil {
		t.Fatal(err)
	}

	if g, w := ansi.Strip(got.Styled), ansi.Strip(ref.Styled); g != w {
		t.Fatalf("attributes changed text or layout\ngot:\n%s\nwant:\n%s", g, w)
	}

	rows := styledRows(got.Styled, 80, colorprofile.Ascii)
	requireAttr(t, cellsOf(t, rows, "Heading one"), uv.AttrBold, "heading")
	requireAttr(t, cellsOf(t, rows, "italic"), uv.AttrItalic, "emphasis")
	requireAttr(t, cellsOf(t, rows, "struck"), uv.AttrStrikethrough, "strikethrough")
	for y, row := range rows {
		for x, c := range row {
			if c.Style.Fg != nil || c.Style.Bg != nil || c.Style.UnderlineColor != nil {
				t.Fatalf("cell (%d,%d) %q keeps a colour after Ascii conversion: %q", x, y, c.Content, c.Style.String())
			}
		}
	}
}
