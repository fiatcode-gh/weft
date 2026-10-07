package views

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
)

// newApp boots an App on a throwaway graph without touching the environment,
// so a test controls NO_COLOR and WEFT_STYLE itself.
func newApp(t *testing.T, files map[string]string, width, height int) *App {
	t.Helper()
	dir, _ := writeGraph(t, files)
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	if a.page == nil {
		t.Fatal("PageView not constructed after boot")
	}
	return a
}

func frameLines(a *App) []string { return strings.Split(plain(a.View().Content), "\n") }

var esc = tea.KeyPressMsg{Code: tea.KeyEsc}

// glyphless is a screen row's text without its list bullet, so an editor row
// ("- x") and a read row ("• x") compare equal.
func glyphless(s string) string {
	s = strings.TrimSpace(s)
	for _, g := range []string{"- ", "• ", "* ", "+ "} {
		if strings.HasPrefix(s, g) {
			return strings.TrimSpace(strings.TrimPrefix(s, g))
		}
	}
	return s
}

// escPage is 80 one-row bullets "- L<n>", line 14 wrapping over two rows at 80 columns.
func escPage() string {
	var b strings.Builder
	for n := range 80 {
		if n == 14 {
			b.WriteString("- L14 wrapped alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau upsilon\n")
			continue
		}
		fmt.Fprintf(&b, "- L%d\n", n)
	}
	return b.String()
}

// TestEscKeepsScreenRow: whichever way the editor closes, the cursor's line is
// on the read view's screen row where the editor drew it.
func TestEscKeepsScreenRow(t *testing.T) {
	quietTerm(t)
	down := tea.KeyPressMsg{Code: tea.KeyDown}
	tests := []struct {
		name      string
		leading   string
		toWrapped bool // move down onto the wrapped bullet's continuation row, else downs rows
		downs     int
		ups       int // moves back up afterwards, so the cursor ends off the window's last row
		exit      func(t *testing.T, a *App)
	}{
		{"clean after 7 rows down", "", false, 7, 0, func(t *testing.T, a *App) { a.Update(esc) }},
		{"clean after 40 rows down, the editor scrolled", "", false, 40, 5, func(t *testing.T, a *App) { a.Update(esc) }},
		{"clean on a continuation row", "", true, 0, 0, func(t *testing.T, a *App) { a.Update(esc) }},
		{"save then esc", "", true, 0, 0, func(t *testing.T, a *App) {
			a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
			if !a.editor.saved {
				t.Fatal("setup: ctrl+s did not save")
			}
			a.Update(esc)
			a.Update(a.buildIndexCmd()())
		}},
		{"dirty esc then discard, 3 lines inserted above", "", true, 0, 0, func(t *testing.T, a *App) {
			at := a.editor.buf.Cursor()
			a.editor.replaceBuffer("x\ny\nz\n"+a.editor.Content(), at.Line+3, at.Col)
			a.Update(esc)
			a.Update(key("d"))
		}},
		{"leading blank lines", "\n\n", true, 0, 0, func(t *testing.T, a *App) { a.Update(esc) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newApp(t, map[string]string{"pages/Long.md": tt.leading + escPage()}, 80, 30)
			a.navigate("Long")
			for range 5 {
				a.page.LineDown()
			}
			a.Update(key("e"))
			if a.editor == nil {
				t.Fatal("editor did not open")
			}
			if tt.toWrapped {
				for i := 0; i < 30 && a.editor.ExitAnchor().RowInLine != 1; i++ {
					a.Update(down)
				}
				if a.editor.ExitAnchor().RowInLine != 1 {
					t.Fatal("setup: the cursor never reached the continuation row")
				}
			} else {
				for range tt.downs {
					a.Update(down)
				}
				for range tt.ups {
					a.Update(tea.KeyPressMsg{Code: tea.KeyUp})
				}
			}
			sr := a.editor.ExitAnchor().ScreenRow
			want := glyphless(frameLines(a)[sr])
			if want == "" {
				t.Fatal("setup: the cursor row is blank")
			}

			tt.exit(t, a)

			if a.editor != nil {
				t.Fatal("editor did not close")
			}
			if got := glyphless(frameLines(a)[sr]); got != want {
				t.Errorf("read view screen row %d = %q, want the editor's cursor row %q", sr, got, want)
			}
		})
	}
}

// TestEditorWindowMatchesReadWindow: the editor draws the read view's rule row
// above its status line, so its text window is the read view's. A cursor on
// the editor's last text row lands on the read view's last text row.
func TestEditorWindowMatchesReadWindow(t *testing.T) {
	quietTerm(t)
	a := newApp(t, map[string]string{"pages/Long.md": escPage()}, 80, 30)
	a.navigate("Long")
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	lastEdit := a.editor.textHeight() - 1
	for i := 0; i < 80 && a.editor.ExitAnchor().ScreenRow != lastEdit; i++ {
		a.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if sr := a.editor.ExitAnchor().ScreenRow; sr != lastEdit {
		t.Fatalf("setup: cursor on screen row %d, want the editor's last text row %d", sr, lastEdit)
	}
	lastRead := a.height - 3 // H-2 text rows, then the rule and the status line
	if lastEdit != lastRead {
		t.Errorf("editor's last text row is %d, the read view's is %d", lastEdit, lastRead)
	}
	frame := frameLines(a)
	if len(frame) != a.height {
		t.Fatalf("editor frame has %d rows, want %d", len(frame), a.height)
	}
	if rule := frame[a.height-2]; rule != strings.Repeat("─", a.width) {
		t.Errorf("editor's second-to-last row = %q, want the rule row", rule)
	}
	if status := frame[a.height-1]; !strings.Contains(status, "[edit]") {
		t.Errorf("editor's last row = %q, want the status line", status)
	}
	want := glyphless(frame[lastEdit])
	if want == "" {
		t.Fatal("setup: the cursor row is blank")
	}

	a.Update(esc)

	if a.editor != nil {
		t.Fatal("editor did not close")
	}
	if got := glyphless(frameLines(a)[lastRead]); got != want {
		t.Errorf("read view's last text row = %q, want the editor's cursor row %q", got, want)
	}
}

// roundTripPage has headings, wrapped nested bullets and paragraphs.
func roundTripPage() string {
	var b strings.Builder
	long := "wrapped alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau"
	for n := range 12 {
		fmt.Fprintf(&b, "## Section %d\n\n", n)
		fmt.Fprintf(&b, "- top %d %s\n  - nested %d %s\n    - deep %d %s\n\n", n, long, n, long, n, long)
		fmt.Fprintf(&b, "A paragraph %d %s\n\n", n, long)
	}
	return b.String()
}

func TestEditorRoundTripIsStable(t *testing.T) {
	quietTerm(t)
	a := newApp(t, map[string]string{"pages/Long.md": roundTripPage()}, 80, 30)
	a.navigate("Long")
	for _, off := range []int{0, 1, 17, 38, 61} {
		a.page.Restore(off, -1)
		if a.page.Offset() != off {
			t.Fatalf("setup: offset %d not reachable, got %d", off, a.page.Offset())
		}
		a.Update(key("e"))
		a.Update(esc)
		if a.editor != nil {
			t.Fatalf("offset %d: editor did not close", off)
		}
		if got := a.page.Offset(); got != off {
			t.Errorf("offset %d: e then Esc left the page at %d", off, got)
		}
	}
}

// Esc on an unchanged page neither re-renders it nor recomputes the row map
// that e already built; an outside change forces a rebuild and the line still
// lands on its row.
func TestEscReusesPageAndMap(t *testing.T) {
	quietTerm(t)
	orig := sourceRowsFor
	t.Cleanup(func() { sourceRowsFor = orig })
	calls := 0
	sourceRowsFor = func(body string, width int, emphasis string) ([]int, []bool) {
		calls++
		return orig(body, width, emphasis)
	}
	a := newApp(t, map[string]string{"pages/Long.md": escPage()}, 80, 30)
	a.navigate("Long")
	for range 20 {
		a.page.LineDown()
	}
	a.Update(key("e"))
	if calls != 1 {
		t.Fatalf("e computed the row map %d times, want 1", calls)
	}
	page, renders := a.page, RenderCount()

	a.Update(esc)
	if a.page != page || calls != 1 || RenderCount() != renders {
		t.Fatalf("clean Esc: same page %v, map calls %d, renders +%d; want true, 1, +0", a.page == page, calls, RenderCount()-renders)
	}

	a.Update(key("e"))
	at := a.editor.buf.Cursor()
	a.editor.replaceBuffer("x\ny\nz\n"+a.editor.Content(), at.Line+3, at.Col)
	a.Update(esc)
	a.Update(key("d"))
	if a.editor != nil || a.page != page || calls != 1 || RenderCount() != renders {
		t.Fatalf("discard Esc: closed %v, same page %v, map calls %d, renders +%d; want true, true, 1, +0",
			a.editor == nil, a.page == page, calls, RenderCount()-renders)
	}

	a.Update(key("e"))
	sr := a.editor.ExitAnchor().ScreenRow
	want := glyphless(frameLines(a)[sr])
	path := a.editor.path
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("- appended outside\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	a.Update(esc)
	if a.page == page {
		t.Fatal("an outside change must rebuild the page")
	}
	if got := glyphless(frameLines(a)[sr]); got != want {
		t.Errorf("after the rebuild, screen row %d = %q, want %q", sr, got, want)
	}
}

// layoutDoc has wrapped bullets at depth 0-2, H2/H3 headings and paragraphs,
// with no syntax the read view hides.
const layoutDoc = "## Second heading\n" +
	"\n" +
	"A plain paragraph that is long enough to wrap across more than one row at eighty columns of terminal width.\n" +
	"\n" +
	"### Third heading\n" +
	"\n" +
	"- top bullet short\n" +
	"- top bullet that is long enough to wrap across more than one row at eighty columns of terminal width ok\n" +
	"  - nested bullet that is long enough to wrap across more than one row at eighty columns of terminal width ok\n" +
	"    - deepest bullet that is long enough to wrap across more than one row at eighty columns of terminal width ok\n" +
	"  - nested short\n" +
	"\n" +
	"Closing paragraph.\n"

func layoutParity(t *testing.T, textOnlyNested bool) {
	a := newApp(t, map[string]string{"pages/Doc.md": layoutDoc}, 80, 30)
	a.navigate("Doc")
	read := frameLines(a)
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	edit := frameLines(a)
	// The read view adds blank rows the source spells differently (two after
	// a nested list), so lines are compared in order, blank rows dropped. Rows
	// at and below the page window's end (status bar) are not part of the page.
	rows := func(frame []string) (out []string) {
		for _, r := range frame[:a.height-2] {
			if strings.TrimSpace(r) != "" {
				out = append(out, strings.TrimRight(r, " "))
			}
		}
		return out
	}
	rr, er := rows(read), rows(edit)
	if len(rr) != len(er) || len(rr) < 12 {
		t.Fatalf("read view shows %d text rows, editor %d:\n%s\n---\n%s", len(rr), len(er), strings.Join(rr, "\n"), strings.Join(er, "\n"))
	}
	for i := range rr {
		// Nested rows (indented bullets and their wrapped rows) under NO_COLOR:
		// the text must match, not the column (PLAN §3.2).
		if textOnlyNested && strings.HasPrefix(rr[i], "    ") {
			if glyphless(rr[i]) != glyphless(er[i]) {
				t.Errorf("text row %d: editor %q, read view %q", i, er[i], rr[i])
			}
			continue
		}
		if col := func(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }; col(rr[i]) != col(er[i]) || glyphless(rr[i]) != glyphless(er[i]) {
			t.Errorf("text row %d: editor %q, read view %q", i, er[i], rr[i])
		}
	}
}

func TestLayoutParityRowsTerminal(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	layoutParity(t, false)
}

func TestLayoutParityRowsDark(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": "dark"}) {
		return
	}
	layoutParity(t, false)
}

func TestLayoutParityRowsNoColor(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "1", "WEFT_STYLE": ""}) {
		return
	}
	layoutParity(t, true)
}

// parityDoc is the painter's colour document: every element it colours, each in
// its own block.
const parityDoc = "# Title\n" +
	"\n" +
	"## Second level\n" +
	"\n" +
	"### Third level\n" +
	"\n" +
	"plain words **strong** and *slanted* and ~~struck~~ and `code` and [label](http://x.y) end\n" +
	"\n" +
	"- TODO a\n" +
	"- DOING b\n" +
	"- LATER c\n" +
	"- WAITING d\n" +
	"- DONE e\n" +
	"- CANCELED f\n" +
	"- NOW g\n" +
	"- see [[Target]] and [[Real|Alias]] here\n" +
	"  - child words\n" +
	"\n" +
	"> quoted words\n" +
	"\n" +
	"| ca | cb |\n" +
	"|---|---|\n" +
	"| c1 | c2 |\n" +
	"\n" +
	"---\n" +
	"\n" +
	"```go\n" +
	"func main() { s := \"lit\" }\n" +
	"```\n"

type paritySample struct{ name, needle string }

var paritySamples = []paritySample{
	{"h1", "Title"}, {"h2", "Second level"}, {"h3", "Third level"},
	{"body", "plain words"}, {"strong", "strong"}, {"emph", "slanted"},
	{"strike", "struck"}, {"code", "code"}, {"link", "label"},
	{"TODO", "TODO"}, {"DOING", "DOING"}, {"LATER", "LATER"}, {"WAITING", "WAITING"},
	{"DONE", "DONE"}, {"CANCELED", "CANCELED"}, {"NOW", "NOW"},
	{"wiki target", "Target"}, {"wiki alias", "Alias"},
	{"child", "child words"}, {"quote", "quoted words"},
	{"table header", "ca"}, {"table cell", "c1"},
	{"fence func", "func"}, {"fence name", "main"}, {"fence string", "\"lit\""},
}

// bodyOnlySamples drops the samples that sit after a wiki-link or workflow
// marker in the same text run, where a named style loses the read view's body
// colour (PLAN §12 item 2).
func bodyOnlySamples() []paritySample {
	var out []paritySample
	for _, s := range paritySamples {
		switch s.name {
		case "body", "child", "quote", "h1", "h2", "h3", "strong", "emph", "strike", "code", "link",
			"table header", "table cell", "fence func", "fence name", "fence string", "wiki target":
			out = append(out, s)
		}
	}
	return out
}

func cellText(c uv.Cell) string {
	if c.Content == "" {
		return " "
	}
	return c.Content
}

func cellRowText(row []uv.Cell) string {
	var b strings.Builder
	for _, c := range row {
		b.WriteString(cellText(c))
	}
	return b.String()
}

// styleRunIn returns the styles of the cells spelling needle, which must occur
// once on the screen, and the row it is on.
func styleRunIn(t *testing.T, scr [][]uv.Cell, side, needle string) ([]uv.Style, int) {
	t.Helper()
	var found []uv.Style
	n, at := 0, 0
	rs := []rune(needle)
	for y, row := range scr {
		for x := 0; x+len(rs) <= len(row); x++ {
			var styles []uv.Style
			ok := true
			for i, r := range rs {
				if cellText(row[x+i]) != string(r) {
					ok = false
					break
				}
				styles = append(styles, row[x+i].Style)
			}
			if ok {
				n++
				found, at = styles, y
			}
		}
	}
	if n != 1 {
		t.Fatalf("%s: %q found %d times, want once", side, needle, n)
	}
	return found, at
}

func firstGlyphOf(t *testing.T, scr [][]uv.Cell, side, needle string) uv.Cell {
	t.Helper()
	for _, row := range scr {
		if strings.Contains(cellRowText(row), needle) {
			for _, c := range row {
				if cellText(c) != " " {
					return c
				}
			}
		}
	}
	t.Fatalf("%s: no row contains %q", side, needle)
	return uv.Cell{}
}

func ruleCellOf(t *testing.T, scr [][]uv.Cell, side string) uv.Cell {
	t.Helper()
	for _, row := range scr {
		s := strings.TrimSpace(cellRowText(row))
		if len(s) >= 3 && strings.Trim(s, "-") == "" {
			for _, c := range row {
				if cellText(c) != " " {
					return c
				}
			}
		}
	}
	t.Fatalf("%s: no rule row", side)
	return uv.Cell{}
}

// parityScreens boots the App on the colour document and returns the raw read
// frame and the raw editor frame (opened at the top, cursor on line 0) as
// cells under profile p. It also checks the cursor sits on the Title row.
func parityScreens(t *testing.T, p colorprofile.Profile) (read, edit [][]uv.Cell) {
	t.Helper()
	a := newApp(t, map[string]string{"pages/Doc.md": parityDoc}, 100, 60)
	a.navigate("Doc")
	read = frameCells(a.View().Content, 100, p)
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	edit = frameCells(a.View().Content, 100, p)
	_, titleRow := styleRunIn(t, edit, "editor", "Title")
	if c := a.View().Cursor; c == nil || c.Y != titleRow {
		t.Fatalf("cursor = %+v, want it on the Title row %d", c, titleRow)
	}
	return read, edit
}

func compareParity(t *testing.T, read, edit [][]uv.Cell, samples []paritySample) {
	t.Helper()
	for _, sm := range samples {
		r, _ := styleRunIn(t, read, "read view", sm.needle)
		e, _ := styleRunIn(t, edit, "editor", sm.needle)
		for i := range r {
			if !r[i].Equal(&e[i]) {
				t.Errorf("%s: %q cell %d: editor %+v, read view %+v", sm.name, sm.needle, i, e[i], r[i])
				break
			}
		}
	}
	cmp := func(name string, r, e uv.Cell) {
		if !r.Style.Equal(&e.Style) {
			t.Errorf("%s: editor %+v (%q), read view %+v (%q)", name, e.Style, e.Content, r.Style, r.Content)
		}
	}
	cmp("bullet", firstGlyphOf(t, read, "read view", "Target"), firstGlyphOf(t, edit, "editor", "Target"))
	cmp("rule", ruleCellOf(t, read, "read view"), ruleCellOf(t, edit, "editor"))
}

func TestColourParityTerminal(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	read, edit := parityScreens(t, colorprofile.TrueColor)
	compareParity(t, read, edit, paritySamples)
}

func TestColourParityDark(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": "dark"}) {
		return
	}
	read, edit := parityScreens(t, colorprofile.TrueColor)
	compareParity(t, read, edit, bodyOnlySamples())
}

func TestColourParityNoColor(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "1", "WEFT_STYLE": ""}) {
		return
	}
	// What weft displays under NO_COLOR: Bubble Tea converts every cell to Ascii.
	read, edit := parityScreens(t, colorprofile.Ascii)
	assertNoColour(t, read, "read view")
	assertNoColour(t, edit, "editor")
	compareParity(t, read, edit, paritySamples)

	wantAttrs := map[string]struct {
		attrs uint8
		ul    bool
	}{
		"h1": {uv.AttrBold, false}, "h2": {uv.AttrBold, false}, "h3": {uv.AttrBold, false},
		"strong": {uv.AttrBold, false}, "emph": {uv.AttrItalic, false},
		"strike": {uv.AttrStrikethrough, false},
		"TODO":   {uv.AttrBold, false}, "CANCELED": {uv.AttrStrikethrough, false},
		"wiki target": {0, true}, "wiki alias": {0, true},
	}
	for _, sm := range paritySamples {
		want, ok := wantAttrs[sm.name]
		if !ok {
			continue
		}
		for _, side := range []struct {
			name string
			scr  [][]uv.Cell
		}{{"read view", read}, {"editor", edit}} {
			styles, _ := styleRunIn(t, side.scr, side.name, sm.needle)
			for i, st := range styles {
				if st.Attrs&want.attrs != want.attrs {
					t.Errorf("%s %s cell %d attrs %#x, want %#x set", side.name, sm.name, i, st.Attrs, want.attrs)
				}
				if want.ul && st.Underline == uv.UnderlineNone {
					t.Errorf("%s %s cell %d is not underlined", side.name, sm.name, i)
				}
			}
		}
	}
}
