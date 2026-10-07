package views

import (
	"image/color"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
)

const findFixture = "foo 1\nbar\nfoo 2\nfoo 3\n"

func openFind(e *EditorView, query string) {
	press(e, ctrl('f'))
	typeRunes(e, query)
}

// viewLines is the frame as plain text rows.
func viewLines(e *EditorView) []string { return strings.Split(plain(e.View()), "\n") }

// barRow is the find bar's row: just above the status line.
func barRow(e *EditorView) string {
	l := viewLines(e)
	return l[len(l)-2]
}

func statusRow(e *EditorView) string {
	l := viewLines(e)
	return l[len(l)-1]
}

func TestFindNoMatch(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", findFixture, false, 60, 12, 0)
	setCursor(e, 1, 2)
	openFind(e, "zzz")
	if got := barRow(e); !strings.Contains(got, "no matches") {
		t.Errorf("bar = %q, want no matches", got)
	}
	if got := cursorPos(e); got != (buffer.Pos{Line: 1, Col: 2}) {
		t.Errorf("cursor = %v, want the origin (1,2)", got)
	}
}

func TestFindWrapAround(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", findFixture, false, 60, 12, 0)
	openFind(e, "foo")
	press(e, named(tea.KeyEnter), named(tea.KeyEnter))
	if got := cursorPos(e); got != (buffer.Pos{Line: 3}) {
		t.Fatalf("cursor = %v, want the last match (3,0)", got)
	}
	if strings.Contains(statusRow(e), "search wrapped") {
		t.Error("wrapped notice before wrapping")
	}
	press(e, named(tea.KeyEnter))
	if got := cursorPos(e); got != (buffer.Pos{}) {
		t.Errorf("next from the last match went to %v, want (0,0)", got)
	}
	if !strings.Contains(statusRow(e), "search wrapped") {
		t.Errorf("status = %q, want search wrapped", statusRow(e))
	}
	press(e, named(tea.KeyUp))
	if got := cursorPos(e); got != (buffer.Pos{Line: 3}) {
		t.Errorf("previous from the first match went to %v, want (3,0)", got)
	}
	if !strings.Contains(statusRow(e), "search wrapped") {
		t.Errorf("status = %q, want search wrapped", statusRow(e))
	}
}

func TestFindSmartCase(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "Alpha alpha\nbeta\n", false, 60, 12, 0)
	openFind(e, "alpha")
	if got := len(e.find.matches); got != 2 {
		t.Errorf("alpha found %d matches, want 2 (Alpha and alpha)", got)
	}
	press(e, named(tea.KeyBackspace), named(tea.KeyBackspace), named(tea.KeyBackspace), named(tea.KeyBackspace), named(tea.KeyBackspace))
	typeRunes(e, "Alpha")
	if got := len(e.find.matches); got != 1 {
		t.Errorf("Alpha found %d matches, want 1 (exact case)", got)
	}
}

func TestFindJumpsFromOrigin(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", findFixture, false, 60, 12, 0)
	setCursor(e, 1, 0)
	openFind(e, "foo")
	if got := cursorPos(e); got != (buffer.Pos{Line: 2}) {
		t.Errorf("cursor = %v, want the first match after the origin (2,0)", got)
	}
	if got := barRow(e); !strings.Contains(got, "2/3") {
		t.Errorf("bar = %q, want 2/3", got)
	}
}

func TestFindHighlightsVisibleMatches(t *testing.T) {
	for _, env := range []struct {
		name, noColor string
		profile       colorprofile.Profile
	}{
		{"colour", "", colorprofile.TrueColor},
		{"NO_COLOR", "1", colorprofile.Ascii},
	} {
		t.Run(env.name, func(t *testing.T) {
			if !inFreshProcess(t, map[string]string{"NO_COLOR": env.noColor}) {
				return
			}
			e := editorAt(nil, "P", "/tmp/p.md", "foo foo\n", false, 60, 8, 0)
			openFind(e, "foo")
			rows := frameCells(e.View(), 60, env.profile)
			cells := cellsShowing(t, rows, "foo foo")
			under := func(c uv.Cell) bool { return c.Style.Underline != uv.UnderlineNone }
			bold := func(c uv.Cell) bool { return c.Style.Attrs&uv.AttrBold != 0 }
			for i, c := range cells {
				switch {
				case i < 3: // the current match
					if !under(c) || !bold(c) {
						t.Errorf("current cell %d %q: style %q, want bold+underline", i, c.Content, c.Style.String())
					}
					if env.noColor == "" && (!sameColour(c.Style.Bg, 11) || !sameColour(c.Style.Fg, 0)) {
						t.Errorf("current cell %d: style %q, want fg 0 bg 11", i, c.Style.String())
					}
				case i == 3: // the gap
					if under(c) || bold(c) {
						t.Errorf("gap cell is styled: %q", c.Style.String())
					}
				default:
					if !under(c) || bold(c) {
						t.Errorf("other cell %d %q: style %q, want underline only", i, c.Content, c.Style.String())
					}
					if env.noColor == "" && !sameColour(c.Style.Bg, 8) {
						t.Errorf("other cell %d: style %q, want bg 8", i, c.Style.String())
					}
				}
			}
			if env.noColor != "" {
				assertNoColour(t, rows, "matches under NO_COLOR")
			}
		})
	}
}

func sameColour(c color.Color, idx int) bool {
	if c == nil {
		return false
	}
	want := uv.ConvertStyle(uv.Style{Fg: ansi.IndexedColor(idx)}, colorprofile.TrueColor).Fg
	r, g, b, _ := c.RGBA()
	wr, wg, wb, _ := want.RGBA()
	return r == wr && g == wg && b == wb
}

func TestReplaceOne(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", findFixture, false, 60, 12, 0)
	openFind(e, "foo")
	press(e, named(tea.KeyTab))
	typeRunes(e, "X")
	press(e, named(tea.KeyEnter))
	if got, want := text(e), "X 1\nbar\nfoo 2\nfoo 3\n"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got := cursorPos(e); got != (buffer.Pos{Line: 2}) {
		t.Errorf("cursor = %v, want the next match (2,0)", got)
	}
	if got := barRow(e); !strings.Contains(got, "1/2") {
		t.Errorf("bar = %q, want 1/2", got)
	}
}

func TestReplaceAllIsOneUndoStep(t *testing.T) {
	quietTerm(t)
	const src = "foo bar foo\nfoo\n"
	e := editorAt(nil, "P", "/tmp/p.md", src, false, 60, 12, 0)
	openFind(e, "foo")
	press(e, named(tea.KeyTab))
	typeRunes(e, "Q")
	press(e, ctrl('a'))
	if got, want := text(e), "Q bar Q\nQ\n"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if !strings.Contains(statusRow(e), "replaced 3") {
		t.Errorf("status = %q, want replaced 3", statusRow(e))
	}
	press(e, ctrl('z'))
	if got := text(e); got != src {
		t.Errorf("one undo gave %q, want %q", got, src)
	}
}

func TestFindBarEscClosesBarNotEditor(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", findFixture, false, 60, 12, 0)
	setCursor(e, 1, 0)
	openFind(e, "foo")
	res, _ := e.Update(named(tea.KeyEsc))
	if res.Exit || e.mode != editing {
		t.Fatalf("esc with the bar open = %+v mode %v, want the bar closed only", res, e.mode)
	}
	if e.find != nil {
		t.Error("bar still open")
	}
	if got := cursorPos(e); got != (buffer.Pos{Line: 2}) {
		t.Errorf("cursor = %v, want the match start (2,0)", got)
	}
	if _, ok := selected(e); ok {
		t.Error("closing the bar left a selection")
	}
	if len(viewLines(e)) != 12 {
		t.Errorf("frame has %d rows, want 12", len(viewLines(e)))
	}
	if res, _ := e.Update(named(tea.KeyEsc)); !res.Exit {
		t.Error("second esc on a clean buffer should leave")
	}
}

func TestFindSeedsFromSelection(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "xx alpha\nalpha beta\n", false, 60, 12, 0)
	setCursor(e, 0, 3)
	for range 5 {
		press(e, shiftKey(tea.KeyRight))
	}
	press(e, ctrl('f'))
	if e.find.query != "alpha" {
		t.Errorf("query = %q, want the selection", e.find.query)
	}
	if e.find.origin != (buffer.Pos{Col: 3}) {
		t.Errorf("origin = %v, want the selection start", e.find.origin)
	}
	if _, ok := selected(e); ok {
		t.Error("the selection survived opening the bar")
	}
	if got := barRow(e); !strings.Contains(got, "1/2") {
		t.Errorf("bar = %q, want 1/2", got)
	}

	// A multi-line selection seeds nothing: the last query stays.
	press(e, named(tea.KeyEsc))
	setCursor(e, 0, 0)
	press(e, shiftKey(tea.KeyDown))
	press(e, ctrl('f'))
	if e.find.query != "alpha" {
		t.Errorf("query after a multi-line selection = %q, want the last query", e.find.query)
	}
}

func TestFindKeepsLastQuery(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", findFixture, false, 60, 12, 0)
	openFind(e, "bar")
	press(e, named(tea.KeyEsc), ctrl('f'))
	if e.find == nil || e.find.query != "bar" {
		t.Errorf("reopened bar = %+v, want query bar", e.find)
	}
}

func TestFindBarPasteGoesToField(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", findFixture, false, 60, 12, 0)
	press(e, ctrl('f'))
	e.Paste(tea.PasteMsg{Content: "fo\x01o\nzz"})
	if e.find.query != "foo" {
		t.Errorf("query = %q, want foo (first line, C0 dropped)", e.find.query)
	}
	if got := text(e); got != findFixture {
		t.Errorf("paste reached the buffer: %q", got)
	}
	if len(e.find.matches) != 3 {
		t.Errorf("matches = %d, want 3", len(e.find.matches))
	}
}

func TestFindRecomputesAfterUndo(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", findFixture, false, 60, 12, 0)
	openFind(e, "foo")
	press(e, named(tea.KeyTab))
	typeRunes(e, "X")
	press(e, named(tea.KeyEnter))
	if got := len(e.find.matches); got != 2 {
		t.Fatalf("after replace %d matches, want 2", got)
	}
	press(e, ctrl('z'))
	if got := len(e.find.matches); got != 3 {
		t.Errorf("after undo %d matches, want 3", got)
	}
	press(e, ctrl('y'))
	if got := len(e.find.matches); got != 2 {
		t.Errorf("after redo %d matches, want 2", got)
	}
}

func TestFindCursorInBar(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", findFixture, false, 60, 12, 0)
	openFind(e, "foo")
	lines := viewLines(e)
	row := len(lines) - 2
	c := e.Cursor()
	if c == nil || c.Y != row {
		t.Fatalf("cursor = %+v, want on the bar row %d", c, row)
	}
	if want := strings.Index(lines[row], "Find: foo") + len("Find: foo"); c.X != want {
		t.Errorf("cursor x = %d, want %d (end of the find field)", c.X, want)
	}
	press(e, named(tea.KeyTab))
	typeRunes(e, "ab")
	lines = viewLines(e)
	c = e.Cursor()
	if want := strings.Index(lines[row], "Replace: ab") + len("Replace: ab"); c == nil || c.X != want || c.Y != row {
		t.Errorf("cursor = %+v, want (%d,%d) at the end of the replace field", c, want, row)
	}
}

func TestFindBarTakesOneTextRow(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", plainLines(30), false, 60, 12, 0)
	before := e.textHeight()
	press(e, ctrl('f'))
	if got := e.textHeight(); got != before-1 {
		t.Errorf("textHeight = %d, want %d", got, before-1)
	}
	if got := len(viewLines(e)); got != 12 {
		t.Errorf("frame has %d rows, want 12", got)
	}
}

func TestFindKeepsMatchOnScreen(t *testing.T) {
	quietTerm(t)
	src := plainLines(60) + "needle\n"
	e := editorAt(nil, "P", "/tmp/p.md", src, false, 60, 12, 0)
	openFind(e, "needle")
	if text(e) != src {
		t.Fatal("typing into the bar edited the buffer")
	}
	rows := viewLines(e)[:e.textHeight()]
	if !strings.Contains(strings.Join(rows, "\n"), "needle") {
		t.Errorf("the match is not in the text window:\n%s", strings.Join(rows, "\n"))
	}
}

// A selection seeds the query with the file's own bytes; ESC, other control
// characters and invalid UTF-8 must not get into the bar, as typed and pasted
// text can't.
func TestFindSeedFromSelectionDropsControlBytes(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "a\x1b[31m\tb\xffc\n", false, 60, 12, 0)
	e.buf.MoveTo(buffer.Pos{}, false)
	e.buf.MoveTo(e.buf.LineEnd(buffer.Pos{}), true)
	press(e, ctrl('f'))
	if e.find == nil {
		t.Fatal("find bar did not open")
	}
	if want := "a[31mbc"; e.find.query != want {
		t.Errorf("query = %q, want %q", e.find.query, want)
	}
	if bar := barRow(e); strings.ContainsFunc(bar, unicode.IsControl) || !utf8.ValidString(bar) {
		t.Errorf("bar row has raw control bytes or invalid UTF-8: %q", bar)
	}
}

// The bar draws its fields as the editor draws text: tabs as spaces, control
// and invalid bytes as carets and U+FFFD, and the terminal cursor after them.
func TestFindBarDisplaysFieldsLikeTheEditor(t *testing.T) {
	quietTerm(t)
	f := &findBar{query: "a\tb\xff", repl: "c\x1bd", replaceShown: true, field: replaceField, cur: -1}
	bar, x := f.barView(80)
	got := ansi.Strip(bar)
	if want := "Find: a   b\uFFFD  no matches  Replace: c^[d"; got != want {
		t.Errorf("bar = %q, want %q", got, want)
	}
	if want := ansi.StringWidth(got); x != want {
		t.Errorf("cursor column = %d, want %d (the end of the replace field)", x, want)
	}
	f.field = findField
	if _, x := f.barView(80); x != ansi.StringWidth("Find: a   b\uFFFD") {
		t.Errorf("find-field cursor column = %d, want %d", x, ansi.StringWidth("Find: a   b\uFFFD"))
	}
}
