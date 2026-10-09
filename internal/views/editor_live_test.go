package views

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// liveEditor opens a live-preview editor on content with line 0 under the top
// margin row.
func liveEditor(content string, width, height int) *EditorView {
	return NewEditorView(nil, "P", "/tmp/p.md", content, false, width, height, Anchor{Line: 0, ScreenRow: 1}, nil, false)
}

// frameText is the text window of e's frame, one string per screen row.
func frameText(e *EditorView) []string {
	return strings.Split(e.View(), "\n")[:e.textHeight()]
}

func plainFrame(e *EditorView) []string {
	rows := frameText(e)
	for i, r := range rows {
		rows[i] = plain(r)
	}
	return rows
}

func tap(e *EditorView, code rune) { e.Update(tea.KeyPressMsg{Code: code}) }

// goTo puts the cursor at the start of line l and scrolls the frame to the
// top margin, as a tall window shows it.
func goTo(e *EditorView, l int) {
	setCursor(e, l, 0)
	e.syncReveal()
	e.top = viewPos{0, -1}
}

// liveConstructPage has every construct rule 2 names: the render package's
// construct page plus wrapped bullets and a line of 12 links.
const liveConstructPage = "\n\n  # Heading with [[Link]] and [[pi]]\n\n" +
	"Setext title\n============\n\nSub heading\n-----------\n\n" +
	"Paragraph with **bold**, a [markdown link](http://example.com/x) and [[Target|an alias]].\n" +
	"Second line of the paragraph [[pi]] [[a]] [[b]] [[c]] [[d]] [[e]] [[f]] [[g]] [[h]] [[i]] [[j]] [[k]] [[l]] [[m]].\n\n" +
	"- bullet one\n- bullet two with [[Wiki]]\n  - nested a\n    - deeper\n  - nested b\n- TODO open task\n- DONE finished\n- LATER [#A] prioritised\n" +
	"- a very long bullet that has to wrap because it is much longer than the narrow widths used by the differential test, wrap wrap wrap wrap\n\n" +
	"- wrapped bullet one that is long enough to wrap across a few rows of a forty column window\n  - nested wrapped bullet that also needs more than one row at this narrow width\n\n" +
	"Links: [[l1]] [[l2]] [[l3]] [[l4]] [[l5]] [[l6]] [[l7]] [[l8]] [[l9]] [[l10]] [[l11]] [[l12]] and words to wrap\n\n" +
	"* star item\n* star two\n\n+ plus item\n\n" +
	"1. first\n2. second\n\n   paragraph in item\n3. third\n\n" +
	"- [ ] gfm task\n- [x] gfm done\n\n" +
	"> quote line\n> second quote line\nlazy quote continuation\n\n" +
	"| Name | Link |\n|------|------|\n| a | [[Table]] |\n| b | [x](http://y.z) |\n\n" +
	"---\n\n***\n\n" +
	"```go\npackage main\n\nfunc main() { x := 1 }\n```\n\n" +
	"~~~\ntilde fence\n\nwith blank\n~~~\n\n" +
	"````\n```\ninner fence\n```\n````\n\n" +
	"- item before logbook\n  :LOGBOOK:\n  CLOCK: [2026-01-01 Thu 10:00]\n  :END:\n\n" +
	":LOGBOOK:\nCLOCK: x\n:END:\n\n" +
	"{{query (todo)}}\n\n{{embed [[Embedded]]}}\n\n{{query\n(and [[x]])\n}}\n\n" +
	"See [ref] and [other][ref], also [r2].\n\n" +
	"[ref]: http://example.com/ref\n\n" +
	"[r2]: <http://a b> \"Title\"\n\n" +
	"Term\n: definition\n\n" +
	"<!--\ncomment\n\nstill comment\n-->\n\n<div>\nhtml block\n</div>\n\n<pre>\n\nverbatim\n</pre>\n\n" +
	"After html.\n\ntext with trailing two  \nhard break\n\n![img](x.png) <http://auto.link> https://bare.link/x `[[code]]` span\n\n" +
	"Last paragraph with [[Wrap Edge]] and [[pi]] and some filler words to cross the wrap edge   \n\n\n"

// rowsOf is the rows of every line the preview draws, in order, per line.
func rowsOf(t *testing.T, p *render.Preview, sc *render.Scanner, e *EditorView, l int) render.LineRows {
	t.Helper()
	lr, ok := p.Rows(e.buf, sc, l)
	if !ok {
		t.Fatalf("preview refused line %d", l)
	}
	return lr
}

func rowCountOf(lr render.LineRows) int { return len(lr.Lead) + len(lr.Body) + len(lr.Trail) }

// checkLiveMatchesRead is rule 2: with the cursor on each line of page in
// turn, the rows before the line's unit and the rows after it are the read
// view's rows, byte for byte.
func checkLiveMatchesRead(t *testing.T, page string, width int) {
	t.Helper()
	res, err := render.Render(strings.TrimSpace(page)+"\n", width)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(res.Styled, "\n")
	e := liveEditor(page, width, 3*len(want)+40)
	ref, sc := render.NewPreview(e.theme, width), render.NewScanner()
	if ref.Head() != 1 {
		t.Fatalf("head = %d rows, want 1", ref.Head())
	}
	want = want[ref.Head():]
	for l := range e.buf.Len() {
		goTo(e, l)
		got := frameText(e)[1:]
		from, to := ref.Unit(e.buf, sc, l)
		before, after, unit := 0, 0, 0
		for i := range e.buf.Len() {
			lr := rowsOf(t, ref, sc, e, i)
			switch {
			case i < from:
				before += rowCountOf(lr)
			case i >= to:
				after += rowCountOf(lr)
			default:
				unit += len(lr.Lead) + len(e.line(i).rows) + len(lr.Trail)
			}
		}
		if before+after > len(want) {
			t.Fatalf("line %d: rendered rows %d+%d exceed the read view's %d", l, before, after, len(want))
		}
		if !slices.Equal(got[:before], want[:before]) {
			k := 0
			for got[k] == want[k] {
				k++
			}
			t.Fatalf("cursor on line %d (unit %d..%d): row %d before the unit differs\n got: %q\nwant: %q", l, from, to, k, got[k], want[k])
		}
		tailGot, tailWant := got[before+unit:before+unit+after], want[len(want)-after:]
		if !slices.Equal(tailGot, tailWant) {
			k := 0
			for tailGot[k] == tailWant[k] {
				k++
			}
			t.Fatalf("cursor on line %d (unit %d..%d): row %d after the unit differs\n got: %q\nwant: %q", l, from, to, k, tailGot[k], tailWant[k])
		}
	}
}

// liveTaskDatesPage has priorities and stamp lines in the shapes the render
// package's previewTaskDatesPage covers.
const liveTaskDatesPage = "- TODO [#A] open with priority\n  SCHEDULED: <2026-05-25 Mon 09:30 .+1w>\n  DEADLINE: <2026-05-27 Wed>\n" +
	"- DONE [#B] closed\n  DEADLINE: <2026-05-27 Wed>\n" +
	"- NOW [#C] running with [[link]]\n  - TODO child\n    SCHEDULED: <2026-06-01 Mon>\n" +
	"- LATER [#A] a very long task text that has to wrap because it is much longer than the narrow widths used by the test, wrap wrap wrap\n  SCHEDULED: <2026-05-25 Mon>\n\n" +
	"```\nSCHEDULED: <2026-05-25 Mon>\n```\n\n" +
	"- TODO logged\n  :LOGBOOK:\n  SCHEDULED: <2026-05-25 Mon>\n  :END:\n" +
	"- TODO malformed\n  SCHEDULED: <2026-02-30>\n\n" +
	"SCHEDULED: <2026-05-25 Mon>\n\nplain tail\n"

func runLiveMatchesRead(t *testing.T) {
	for _, w := range []int{100, 40} {
		checkLiveMatchesRead(t, liveConstructPage, w)
		checkLiveMatchesRead(t, liveTaskDatesPage, w)
	}
}

func TestLiveRowsMatchReadViewTerminal(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	runLiveMatchesRead(t)
}

func TestLiveRowsMatchReadViewDark(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": "dark"}) {
		return
	}
	runLiveMatchesRead(t)
}

func TestLiveRowsMatchReadViewNoColor(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "1", "WEFT_STYLE": ""}) {
		return
	}
	runLiveMatchesRead(t)
}

// rawShown reports whether some row of rows is line drawn raw.
func rawShown(rows []string, line string) bool {
	want := strings.TrimSpace(render.DisplayText(line))
	return slices.ContainsFunc(rows, func(r string) bool { return strings.TrimSpace(plain(r)) == want })
}

func TestLiveRevealsUnits(t *testing.T) {
	quietTerm(t)
	tests := []struct {
		name  string
		page  string
		units [][2]int // [from, to) of every multi-line unit
	}{
		{"table", "- before\n\n| H1 | H2 |\n|----|----|\n| a | b |\n| c | d |\n\n- after\n", [][2]int{{2, 6}}},
		{"fence", "- before\n\n```go\nx := 1\n\ny := 2\n```\n\n- after\n", [][2]int{{2, 7}}},
		{"quote with lazy line", "- before\n\n> quote one\n> quote two\nlazy tail\n\n- after\n", [][2]int{{2, 5}}},
		{"top-level logbook", "- before\n\n:LOGBOOK:\nCLOCK: [2026-01-01 Thu 10:00]\n:END:\n\n- after\n", [][2]int{{2, 5}}},
		{"nested logbook", "- before\n- item\n  :LOGBOOK:\n  CLOCK: [2026-01-01 Thu 10:00]\n  :END:\n- after\n", [][2]int{{2, 5}}},
		{"query", "- before\n\n{{query\n(and [[x]])\n}}\n\n- after\n", [][2]int{{2, 5}}},
		{"joined paragraph", "- before\n\nfirst part of the thought\nsecond part of the thought\n\n- after\n", [][2]int{{2, 4}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := liveEditor(tc.page, 80, 40)
			lines := strings.Split(tc.page, "\n")
			// What a rendered line looks like: some render the same as their source.
			rendered := plainFrame(liveEditor(tc.page, 80, 40))
			for l, line := range lines {
				if strings.TrimSpace(line) == "" {
					continue
				}
				from, to := l, l+1
				for _, u := range tc.units {
					if u[0] <= l && l < u[1] {
						from, to = u[0], u[1]
					}
				}
				goTo(e, l)
				if f, z := e.preview.Unit(e.buf, e.scanner, l); f != from || z != to {
					t.Errorf("cursor on line %d: unit %d..%d, want %d..%d", l, f, z, from, to)
				}
				rows := plainFrame(e)
				for i, other := range lines {
					if strings.TrimSpace(other) == "" {
						continue
					}
					in := from <= i && i < to
					switch got := rawShown(rows, other); {
					case in && !got:
						t.Errorf("cursor on line %d: unit line %d %q is not raw\n%s", l, i, other, strings.Join(rows, "\n"))
					case !in && got && (i == 0 || !rawShown(rendered, other)):
						t.Errorf("cursor on line %d: line %d %q is raw outside the unit", l, i, other)
					}
				}
			}
		})
	}

	t.Run("selection from a list item into a table", func(t *testing.T) {
		page := "- item\n\n| a | b |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |\n\n- after\n"
		e := liveEditor(page, 80, 40)
		goTo(e, 0)
		e.buf.MoveTo(buffer.Pos{Line: 3, Col: 2}, true)
		rows := plainFrame(e)
		for i, line := range strings.Split(page, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if want := i <= 5; rawShown(rows, line) != want {
				t.Errorf("line %d %q raw = %v, want %v\n%s", i, line, !want, want, strings.Join(rows, "\n"))
			}
		}
	})

	t.Run("wrapped bullet is revealed whole", func(t *testing.T) {
		page := "- first\n\n" + liveWrapped + "\n\n- last\n"
		e := liveEditor(page, 40, 40)
		goTo(e, 2)
		if n := len(e.line(2).rows); n < 2 {
			t.Fatalf("setup: the bullet wraps to %d raw rows", n)
		}
		text := strings.Join(plainFrame(e), "\n")
		if !strings.Contains(text, "- wrapped") || strings.Contains(text, "• wrapped") {
			t.Errorf("wrapped bullet is not raw:\n%s", text)
		}
		goTo(e, 0)
		if text := strings.Join(plainFrame(e), "\n"); strings.Contains(text, "- wrapped") || !strings.Contains(text, "• wrapped") {
			t.Errorf("wrapped bullet is not rendered once the cursor leaves:\n%s", text)
		}
	})
}

const liveWrapped = "- wrapped bullet that is long enough to run across several rows of a forty column window"

func TestLiveDownVisitsEveryLine(t *testing.T) {
	quietTerm(t)
	e := liveEditor(liveWalkPage, 40, 14)
	flat := flatRawRows(e)
	if e.buf.Cursor().Line != 0 {
		t.Fatal("setup: cursor not on line 0")
	}
	for k := 1; k < len(flat); k++ {
		tap(e, tea.KeyDown)
		_ = e.View()
		if got := e.buf.Cursor().Line; got != flat[k] {
			t.Fatalf("Down %d reached line %d, want %d", k, got, flat[k])
		}
	}
	tap(e, tea.KeyDown)
	if got := e.buf.Cursor(); got != e.buf.End() {
		t.Errorf("Down off the last row: cursor %v, want the buffer end %v", got, e.buf.End())
	}
}

func TestLiveUpVisitsEveryLine(t *testing.T) {
	quietTerm(t)
	e := liveEditor(liveWalkPage, 40, 14)
	flat := flatRawRows(e)
	e.Update(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl})
	_ = e.View()
	for k := len(flat) - 1; k >= 1; k-- {
		if got := e.buf.Cursor().Line; got != flat[k] {
			t.Fatalf("before Up %d the cursor is on line %d, want %d", k, got, flat[k])
		}
		tap(e, tea.KeyUp)
		_ = e.View()
	}
	if got := e.buf.Cursor().Line; got != 0 {
		t.Errorf("Up reached line %d, want 0", got)
	}
	tap(e, tea.KeyUp)
	if got := e.buf.Cursor(); got != (buffer.Pos{}) {
		t.Errorf("Up off the first row: cursor %v, want the buffer start", got)
	}
}

func TestLiveDownEntersHiddenBlock(t *testing.T) {
	quietTerm(t)
	page := "- item\n:LOGBOOK:\nCLOCK: [2026-01-01 Thu 10:00]\n:END:\n- next\n"
	e := liveEditor(page, 60, 12)
	goTo(e, 0)
	if text := strings.Join(plainFrame(e), "\n"); strings.Contains(text, ":LOGBOOK:") {
		t.Fatalf("the hidden block shows before the cursor enters it:\n%s", text)
	}
	tap(e, tea.KeyDown)
	if got := e.buf.Cursor().Line; got != 1 {
		t.Fatalf("Down from the line above reached line %d, want the block's opener 1", got)
	}
	text := strings.Join(plainFrame(e), "\n")
	for _, want := range []string{":LOGBOOK:", "CLOCK:", ":END:"} {
		if !strings.Contains(text, want) {
			t.Errorf("entering the block did not reveal %q:\n%s", want, text)
		}
	}
}

// flatRawRows lists the line of each raw row of the buffer, in order.
func flatRawRows(e *EditorView) []int {
	var flat []int
	for l := range e.buf.Len() {
		for range e.line(l).rows {
			flat = append(flat, l)
		}
	}
	return flat
}

const liveWalkPage = "\n\n# Title\n\n- item\n  :LOGBOOK:\n  CLOCK: [2026-01-01 Thu 10:00]\n  :END:\n\n[ref]: http://example.com\n\n" +
	"```go\nx := 1\n```\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n" +
	liveWrapped + "\n\nlast paragraph\n"

// A page that scrolls: every line has a unique token the old frame shows.
func liveTokenPage() string {
	lines := []string{
		"# Heading #", "", "- item #", "- item # with [[Link]]", "", "> quote #", "> quote #", "",
		"| a # | b |", "|----------|---|", "| c # | d |", "",
		"```go", "code := #", "```", "",
		"- tail #", "  :LOGBOOK:", "  CLOCK: [2026-01-01 Thu 10:00]", "  :END:", "- last #", "",
		"1. one #", "2. two #", "", "- more #", "- more # again", "", "final # words", "",
		"- extra #", "- extra #", "- extra #", "- extra #", "- extra #", "- extra #", "- extra #", "- extra #", "- extra #", "- extra #",
	}
	for i, l := range lines {
		lines[i] = strings.ReplaceAll(l, "#", fmt.Sprintf("Q%03d", i))
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestLiveCursorKeepsScreenRow(t *testing.T) {
	quietTerm(t)
	page := liveTokenPage()
	checked := 0
	check := func(t *testing.T, e *EditorView, name string, forward bool, key func()) {
		t.Helper()
		old := plainFrame(e)
		oldShown := slices.Clone(e.shown)
		from := e.buf.Cursor().Line
		oldCount := e.rowCount(from)
		oldCur, _ := e.cursorRow()
		oldScreen := e.cursorScreenRow()
		key()
		_ = e.View()
		l := e.buf.Cursor().Line
		if l == from || slices.ContainsFunc(oldShown, func(s lineSpan) bool { return s.from <= l && l < s.to }) {
			return
		}
		// A line that shares its row with the line above has no Body of its
		// own: it keeps the row of the line after it (rule 4).
		if lr, ok := e.preview.Rows(e.buf, e.scanner, l); ok && len(lr.Body) == 0 {
			return
		}
		tok := fmt.Sprintf("Q%03d", l)
		s := -1
		for i, r := range old {
			if strings.Contains(r, tok) && (s < 0 || !forward) {
				s = i
			}
		}
		if s < 0 {
			return
		}
		checked++
		got := e.cursorScreenRow()
		if got != s && !(got < s && e.top == (viewPos{0, -1})) {
			t.Fatalf("%s: line %d moved to screen row %d, want its old row %d\nold:\n%s\nnew:\n%s",
				name, l, got, s, strings.Join(old, "\n"), strings.Join(plainFrame(e), "\n"))
		}
		if got == s && forward {
			if sl := oldScreen - oldCur.row; sl > 0 && e.rowCount(from) == oldCount {
				if now := plainFrame(e); !slices.Equal(now[:sl], old[:sl]) {
					t.Fatalf("%s: rows above the line just left moved although its height did not change\nold:\n%s\nnew:\n%s",
						name, strings.Join(old, "\n"), strings.Join(now, "\n"))
				}
			}
		}
	}
	// A window that scrolls, and one that shows the whole page so every move
	// has its target on screen.
	run := func(name string, start func(*EditorView), forward bool, keys ...func(*EditorView)) {
		for _, h := range []int{12, 80} {
			t.Run(fmt.Sprintf("%s/h%d", name, h), func(t *testing.T) {
				checked = 0
				e := liveEditor(page, 60, h)
				start(e)
				for range 45 {
					for _, k := range keys {
						check(t, e, name, forward, func() { k(e) })
					}
				}
				if want := map[int]int{12: 4, 80: 12}[h]; checked < want {
					t.Errorf("only %d moves were checked, want at least %d", checked, want)
				}
			})
		}
	}
	key := func(code rune) func(*EditorView) { return func(e *EditorView) { tap(e, code) } }
	top := func(e *EditorView) { _ = e.View() }
	bottom := func(e *EditorView) {
		e.Update(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl})
		_ = e.View()
	}
	run("down", top, true, key(tea.KeyDown))
	run("up", bottom, false, key(tea.KeyUp))
	run("right at line end", top, true, key(tea.KeyEnd), key(tea.KeyRight))
	run("left at line start", bottom, false, key(tea.KeyHome), key(tea.KeyLeft))
}

func TestLiveCursorClampsAtTopMargin(t *testing.T) {
	quietTerm(t)
	e := liveEditor(liveTokenPage(), 60, 12)
	for range 6 {
		tap(e, tea.KeyDown)
		_ = e.View()
	}
	for range 6 {
		tap(e, tea.KeyUp)
		_ = e.View()
	}
	if e.buf.Cursor().Line != 0 {
		t.Fatalf("cursor on line %d, want 0", e.buf.Cursor().Line)
	}
	if e.top != (viewPos{0, -1}) {
		t.Errorf("top = %+v, want the top margin row", e.top)
	}
	if got := e.cursorScreenRow(); got != 1 {
		t.Errorf("cursor on screen row %d, want 1 below the margin", got)
	}
}

// The editors here are built live explicitly, so the source-mode run would
// repeat the default run's work. Under the race detector every screen row is
// too slow for the whole page: a third of them, seeded per line, still pins
// each line, row and both ends.
func TestLivePlaceAndExitAnchor(t *testing.T) {
	skipInSourceRun(t)
	quietTerm(t)
	const h = 9
	page := liveTokenPage() + "- " + strings.Repeat("wrapping words ", 8) + "\n"
	probe := NewEditorView(nil, "P", "/tmp/p.md", page, false, 40, h+2, Anchor{ScreenRow: 1}, nil, false)
	for l := range probe.buf.Len() {
		rows := len(probe.line(l).rows)
		srs := make([]int, h)
		for sr := range srs {
			srs[sr] = sr
		}
		if raceDetector {
			srs = sampleRange(h-1, uint64(l), 3)
		}
		for _, k := range []int{0, rows - 1, rows + 3} {
			for _, sr := range srs {
				at := Anchor{Line: l, RowInLine: k, ScreenRow: sr}
				e := NewEditorView(nil, "P", "/tmp/p.md", page, false, 40, h+2, at, nil, false)
				want := Anchor{Line: l, RowInLine: min(k, rows-1), ScreenRow: sr}
				if got := e.ExitAnchor(); got != want {
					t.Fatalf("place(%+v): ExitAnchor = %+v, want %+v", at, got, want)
				}
				_ = e.View()
				if got := e.ExitAnchor(); got != want {
					t.Fatalf("place(%+v) then View: ExitAnchor = %+v, want %+v", at, got, want)
				}
			}
		}
	}
}

// D6: a page down walks display rows, and the trailing empty line of a file
// that ends in a newline has none while the cursor is elsewhere, so PgDn at
// the end of the buffer stops on the last line that does. Down is by source
// line and still reaches the empty line.
func TestLivePgDownStopsOnLastDrawnLine(t *testing.T) {
	skipInSourceRun(t)
	quietTerm(t)
	e := liveEditor("alpha BeTa gamma\nsecond line\n- item\n", 40, 10)
	if e.buf.Len() != 4 {
		t.Fatalf("setup: %d lines, want 3 and the empty one after the final newline", e.buf.Len())
	}
	setCursor(e, 0, 3)
	e.syncReveal()
	tap(e, tea.KeyPgDown)
	if l, _ := cursorRowCol(e); l != 2 {
		t.Fatalf("PgDn from line 0 left the cursor on line %d, want 2, the last line with a display row", l)
	}
	tap(e, tea.KeyPgDown)
	if l, _ := cursorRowCol(e); l != 2 {
		t.Errorf("PgDn at the end moved the cursor to line %d, want it to stay on line 2", l)
	}
	tap(e, tea.KeyDown)
	if l, c := cursorRowCol(e); l != 3 || c != 0 {
		t.Errorf("Down from line 2 = (%d,%d), want the trailing empty line (3,0)", l, c)
	}
}

func TestLiveRowlessLines(t *testing.T) {
	quietTerm(t)
	t.Run("have no rows until the cursor is on them", func(t *testing.T) {
		page := "\n\n# Title\n\n[ref]: http://example.com\n\n- a\n  :LOGBOOK:\n  CLOCK: [2026-01-01 Thu 10:00]\n  :END:\n- b\n"
		e := liveEditor(page, 60, 30)
		goTo(e, 2)
		for _, l := range []int{0, 1, 4, 7, 8, 9} {
			if n := e.rowCount(l); n != 0 {
				t.Errorf("line %d has %d rows with the cursor elsewhere, want 0", l, n)
			}
		}
		for _, l := range []int{0, 1, 4, 7, 8, 9} {
			goTo(e, l)
			if n := e.rowCount(l); n == 0 {
				t.Errorf("line %d has no rows with the cursor on it", l)
			}
		}
	})
	t.Run("a blank line swaps its margin row for a raw row", func(t *testing.T) {
		e := liveEditor("first paragraph here\n\nsecond paragraph here\n", 60, 30)
		total := func() (n int) {
			for l := range e.buf.Len() {
				n += e.rowCount(l)
			}
			return n
		}
		goTo(e, 0)
		before := total()
		goTo(e, 1)
		if after := total(); after != before {
			t.Errorf("rows with the cursor on the blank line = %d, want %d", after, before)
		}
		if e.rowCount(1) != 1 {
			t.Errorf("the blank line has %d rows, want 1", e.rowCount(1))
		}
	})
	t.Run("empty and blank buffers show only the cursor line", func(t *testing.T) {
		for _, content := range []string{"", "  \n \n\n"} {
			e := liveEditor(content, 60, 10)
			_ = e.View()
			n := 0
			for l := range e.buf.Len() {
				n += e.rowCount(l)
			}
			if n != 1 {
				t.Errorf("%q: %d display rows, want only the cursor's raw line", content, n)
			}
		}
		e := liveEditor("", 60, 10)
		for _, s := range []string{"-", " ", "h", "i"} {
			e.Update(key(s))
		}
		tap(e, tea.KeyEnter)
		if text := strings.Join(plainFrame(e), "\n"); !strings.Contains(text, "• hi") {
			t.Errorf("typed text is not rendered once the cursor left it:\n%s", text)
		}
	})
}

func TestLiveEditingKeepsRendering(t *testing.T) {
	quietTerm(t)
	t.Run("typing re-renders the line once the cursor leaves", func(t *testing.T) {
		e := liveEditor("- one\n- two\n- three\n", 60, 20)
		goTo(e, 1)
		tap(e, tea.KeyEnd)
		for _, r := range " edited **bold**" {
			e.Update(key(string(r)))
		}
		tap(e, tea.KeyDown)
		_ = e.View()
		checkLiveEdited(t, e)
		if text := strings.Join(plainFrame(e), "\n"); !strings.Contains(text, "• two edited ") {
			t.Errorf("edited line not rendered:\n%s", text)
		}
	})
	t.Run("a new wiki-link shifts later chunks' sentinel ids", func(t *testing.T) {
		page := "- a [[p1]]\n- b [[p2]]\n\npara [[p3]] and [[p4]]\n\n- c [[p5]]\n\n- TODO d [[p6]]\n"
		e := liveEditor(page, 24, 40)
		goTo(e, 0)
		tap(e, tea.KeyEnd)
		for _, r := range " [[x]]" {
			e.Update(key(string(r)))
		}
		for range 3 {
			tap(e, tea.KeyDown)
		}
		_ = e.View()
		checkLiveEdited(t, e)
	})
}

// checkLiveEdited compares e's frame, with the cursor where it is, to the read
// view of the buffer's text, outside the cursor's unit.
func checkLiveEdited(t *testing.T, e *EditorView) {
	t.Helper()
	res, err := render.Render(strings.TrimSpace(e.Content())+"\n", e.width)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(res.Styled, "\n")[1:]
	l := e.buf.Cursor().Line
	ref, sc := render.NewPreview(e.theme, e.width), render.NewScanner()
	from, to := ref.Unit(e.buf, sc, l)
	e.top = viewPos{0, -1}
	got := frameText(e)[1:]
	before, after, unit := 0, 0, 0
	for i := range e.buf.Len() {
		lr := rowsOf(t, ref, sc, e, i)
		switch {
		case i < from:
			before += rowCountOf(lr)
		case i >= to:
			after += rowCountOf(lr)
		default:
			unit += len(lr.Lead) + len(e.line(i).rows) + len(lr.Trail)
		}
	}
	if !slices.Equal(got[:before], want[:before]) {
		t.Errorf("rows before the cursor's unit differ from the read view\n got: %q\nwant: %q", got[:before], want[:before])
	}
	if tail := got[before+unit : before+unit+after]; !slices.Equal(tail, want[len(want)-after:]) {
		t.Errorf("rows after the cursor's unit differ from the read view\n got: %q\nwant: %q", tail, want[len(want)-after:])
	}
}

func TestLiveSourceModeDrawsSourceRows(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "P", "/tmp/p.md", "# Title\n\n- item **bold**\n", false, 60, 10, Anchor{ScreenRow: 1}, nil, true)
	setCursor(e, 2, 0)
	text := strings.Join(plainFrame(e), "\n")
	for _, want := range []string{"# Title", "- item **bold**"} {
		if !strings.Contains(text, want) {
			t.Errorf("source mode lost %q:\n%s", want, text)
		}
	}
	if st := e.preview.Stats(); st.Lines != 0 {
		t.Errorf("source mode rendered %d preview lines", st.Lines)
	}
}
