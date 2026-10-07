package render

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const rowsLong = "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike november " +
	"oscar papa quebec romeo sierra tango uniform victor whiskey xray yankee zulu"

// rowsCorpus: each corpus line is its own block; the bullets form one list.
var rowsCorpus = []string{
	rowsLong,
	"## H2 " + rowsLong,
	"### H3 " + rowsLong,
	"- b0 " + rowsLong,
	"  - b1 " + rowsLong,
	"    - b2 " + rowsLong,
	"1. " + rowsLong,
	"> " + rowsLong,
	"```",
	"code " + rowsLong + " " + rowsLong,
	"```",
	// Quotes whose first wrapped row is exactly W-QI wide at W = 40, 60, 80:
	// the read view re-wraps such a row and strands its last word on an
	// orphan row (see Geometry.Rows). Without that rule these rows differ.
	orphanQuote(40),
	orphanQuote(60),
	orphanQuote(80),
}

// orphanQuote is a quote whose first wrapped row is exactly width-5 (Text minus
// the quote indent) cells wide: four-letter words, the last one sized to fit.
func orphanQuote(width int) string {
	target := width - 5
	var words []string
	n := 0
	for target-n > 9 {
		words = append(words, "word")
		n += 5
	}
	words = append(words, strings.Repeat("q", target-n))
	return "> " + strings.Join(words, " ") + " zzz tail"
}

// rowsCorpusBody separates the blocks by blank lines, except inside the
// bullet list (lines 3-5) and the fence (lines 8-10).
func rowsCorpusBody() string {
	var b strings.Builder
	for i, l := range rowsCorpus {
		b.WriteString(l)
		b.WriteByte('\n')
		if i != 3 && i != 4 && i != 8 && i != 9 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func checkRowsMatchReadView(t *testing.T, textOnlyForNested bool) {
	t.Helper()
	th, err := CurrentTheme()
	if err != nil {
		t.Fatal(err)
	}
	body := rowsCorpusBody()
	srcLines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	for _, width := range []int{40, 60, 80} {
		res, err := RenderWithEmphasis(body, width, "")
		if err != nil {
			t.Fatal(err)
		}
		rowMap := SourceRows(body, width, "")
		if rowMap == nil {
			t.Fatal("SourceRows is nil")
		}
		readRows := strings.Split(res.Styled, "\n")
		g := NewGeometry(th, width)
		sc := NewScanner()
		src := strLines(srcLines)
		for li, line := range srcLines {
			info := sc.Info(src, li)
			if info.Kind == KindBlank || info.Kind == KindFence {
				continue
			}
			var want []string
			for r, l := range rowMap {
				if l != li {
					continue
				}
				plain := strings.TrimRight(ansi.Strip(readRows[r]), " ")
				if plain == "" {
					continue
				}
				want = append(want, plain)
			}
			want = normaliseReadRows(want, info)
			cells := displayCells(line)
			var got []string
			for _, row := range g.Rows(line, info) {
				var disp strings.Builder
				for _, c := range cells {
					if c.src >= row.Start && c.src < row.End {
						disp.WriteString(c.text)
					}
				}
				got = append(got, strings.TrimRight(strings.Repeat(" ", row.Col)+disp.String(), " "))
			}
			if len(got) != len(want) {
				t.Errorf("w=%d line %d (%v): %d editor rows, %d read rows\neditor: %q\nread:   %q",
					width, li, info.Kind, len(got), len(want), got, want)
				continue
			}
			for i := range got {
				a, b := got[i], want[i]
				if textOnlyForNested && (info.Kind == KindBullet || info.Kind == KindOrdered) {
					a, b = strings.TrimLeft(a, " "), strings.TrimLeft(b, " ")
				}
				if a != b {
					t.Errorf("w=%d line %d (%v) row %d\neditor: %q\nread:   %q", width, li, info.Kind, i, a, b)
				}
			}
		}
	}
}

// normaliseReadRows rewrites read-view glyphs to the source characters they
// stand for: the first bullet dot becomes "-", a quote bar becomes ">" on the
// first row and a space on continuation rows.
func normaliseReadRows(rows []string, info LineInfo) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		switch {
		case info.Kind == KindBullet && i == 0:
			r = strings.Replace(r, "•", "-", 1)
		case info.Kind == KindQuote:
			bar := "│"
			if !strings.Contains(r, bar) {
				bar = "|"
			}
			if i == 0 {
				r = strings.Replace(r, bar, ">", 1)
			} else {
				r = strings.Replace(r, bar, " ", 1)
			}
		}
		out[i] = r
	}
	return out
}

func TestRowsMatchReadViewTerminal(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	checkRowsMatchReadView(t, false)
}

func TestRowsMatchReadViewDark(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": "dark"}) {
		return
	}
	checkRowsMatchReadView(t, false)
}

func TestRowsMatchReadViewNoColor(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "1", "WEFT_STYLE": ""}) {
		return
	}
	// LevelIndent is 4 here, so nested bullets sit at different columns; their
	// text must still agree.
	checkRowsMatchReadView(t, true)
}

func TestRowsWrapProperty(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	pieces := []string{"a", "bc", "def", "ghij", "-", "--", "中", "文字", "é", "x-y"}
	for range 500 {
		var b strings.Builder
		b.WriteString("w")
		for range rng.IntN(25) {
			if rng.IntN(3) == 0 {
				b.WriteString(strings.Repeat(" ", 1+rng.IntN(3)))
			}
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}
		line := b.String()
		budget := 1 + rng.IntN(30)
		g := Geometry{Width: budget, Text: budget}
		rows := g.Rows(line, LineInfo{Kind: KindText})
		want := strings.Split(ansi.Wrap(line, budget, ""), "\n")
		if len(rows) != len(want) {
			t.Fatalf("%q at %d: %d rows, ansi.Wrap gives %d: %q", line, budget, len(rows), len(want), want)
		}
		var joined strings.Builder
		pos := 0
		for i, row := range rows {
			if row.Start != pos || row.End < row.Start {
				t.Fatalf("%q at %d: row %d = %+v does not continue at %d", line, budget, i, row, pos)
			}
			pos = row.End
			joined.WriteString(line[row.Start:row.End])
			if got := strings.TrimRight(line[row.Start:row.End], " "); got != strings.TrimRight(want[i], " ") {
				t.Errorf("%q at %d: row %d = %q, want %q", line, budget, i, got, want[i])
			}
		}
		if joined.String() != line {
			t.Errorf("%q at %d: rows join to %q", line, budget, joined.String())
		}
	}
}

func TestDisplayCells(t *testing.T) {
	cases := []struct {
		name, line, display string
		widths              []int
	}{
		{"tab to next multiple of four", "a\tb", "a   b", []int{1, 3, 1}},
		{"tab at column zero", "\tb", "    b", []int{4, 1}},
		{"escape", "\x1b[", "^[[", []int{2, 1}},
		{"delete", "\x7f", "^?", []int{2}},
		{"c1 code point", "\u009b", "�", []int{1}},
		{"invalid byte", "a\xffb", "a�b", []int{1, 1, 1}},
		{"emoji is two wide", "😀x", "😀x", []int{2, 1}},
		{"cjk is two wide", "中", "中", []int{2}},
		{"combining mark joins its base", "e\u0301x", "e\u0301x", []int{1, 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cells := displayCells(tc.line)
			var disp strings.Builder
			var widths []int
			pos := 0
			for _, c := range cells {
				if c.src != pos {
					t.Errorf("cell starts at %d, previous ended at %d", c.src, pos)
				}
				pos = c.end
				disp.WriteString(c.text)
				widths = append(widths, c.w)
			}
			if pos != len(tc.line) {
				t.Errorf("cells cover %d of %d bytes", pos, len(tc.line))
			}
			if disp.String() != tc.display {
				t.Errorf("display = %q, want %q", disp.String(), tc.display)
			}
			if len(widths) != len(tc.widths) {
				t.Fatalf("widths = %v, want %v", widths, tc.widths)
			}
			for i := range widths {
				if widths[i] != tc.widths[i] {
					t.Errorf("widths = %v, want %v", widths, tc.widths)
					break
				}
			}
		})
	}
}

func TestCellXOffsetAt(t *testing.T) {
	g := Geometry{Width: 10, Text: 10}
	bullet := LineInfo{Kind: KindBullet, Marker: 2}
	line := "- aaaa bbbb cccc"

	rows := g.Rows(line, bullet)
	wantRows := []Row{{0, 7, 0}, {7, 16, 2}}
	if len(rows) != 2 || rows[0] != wantRows[0] || rows[1] != wantRows[1] {
		t.Fatalf("rows = %+v, want %+v", rows, wantRows)
	}
	cx := []struct{ off, row, x int }{
		{0, 0, 0},
		{6, 0, 6},   // the space dropped at the break stays on the earlier row
		{7, 1, 2},   // a row boundary belongs to the next row; hanging column
		{10, 1, 5},  //
		{16, 1, 11}, // the line end belongs to the last row
	}
	for _, c := range cx {
		if r, x := g.CellX(line, bullet, rows, c.off); r != c.row || x != c.x {
			t.Errorf("CellX(%d) = (%d,%d), want (%d,%d)", c.off, r, x, c.row, c.x)
		}
	}
	at := []struct{ row, x, off int }{
		{0, 0, 0},
		{0, 100, 6}, // never the End of a non-last row
		{1, 0, 7},   // left of the row's column
		{1, 5, 10},
		{1, 100, 16},
	}
	for _, c := range at {
		if got := g.OffsetAt(line, bullet, rows, c.row, c.x); got != c.off {
			t.Errorf("OffsetAt(%d,%d) = %d, want %d", c.row, c.x, got, c.off)
		}
	}

	// A wide cell: x inside it resolves to the cell's start.
	wide := "中中"
	wrows := g.Rows(wide, LineInfo{Kind: KindText})
	if got := g.OffsetAt(wide, LineInfo{Kind: KindText}, wrows, 0, 1); got != 0 {
		t.Errorf("OffsetAt inside a wide cell = %d, want 0", got)
	}
	if got := g.OffsetAt(wide, LineInfo{Kind: KindText}, wrows, 0, 2); got != 3 {
		t.Errorf("OffsetAt at the second wide cell = %d, want 3", got)
	}

	// Overflow spaces: a long run of spaces keeps counting columns on its row.
	sp := "ab" + strings.Repeat(" ", 14) + "cd"
	srows := g.Rows(sp, LineInfo{Kind: KindText})
	if r, x := g.CellX(sp, LineInfo{Kind: KindText}, srows, 12); r != 0 || x != 12 {
		t.Errorf("CellX inside overflow spaces = (%d,%d), want (0,12)", r, x)
	}
}

func TestGeometryFromTheme(t *testing.T) {
	var th Theme
	g := NewGeometry(th, 3)
	if g.Width != 3 || g.Margin != 0 || g.Text != 3 || g.QuoteIndent != 0 || g.CodePad != 0 {
		t.Errorf("zero theme geometry = %+v", g)
	}
	one := uint(2)
	th.Config.Document.Margin = &one
	th.Config.List.LevelIndent = 3
	th.Config.BlockQuote.Indent = &one
	th.Config.CodeBlock.Margin = &one
	th.Config.CodeBlock.Indent = &one
	g = NewGeometry(th, 3)
	if g.Margin != 2 || g.Text != 1 || g.LevelIndent != 3 || g.QuoteIndent != 2 || g.CodePad != 4 {
		t.Errorf("geometry = %+v", g)
	}
}
