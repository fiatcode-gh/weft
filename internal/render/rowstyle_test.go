package render

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// rowCells decodes a one-line styled row into cells.
func rowCells(row string) []uv.Cell {
	w := ansi.StringWidth(row)
	buf := uv.NewScreenBuffer(w, 1)
	uv.NewStyledString(row).Draw(buf, buf.Bounds())
	out := make([]uv.Cell, w)
	for x := range out {
		out[x] = *buf.CellAt(x, 0)
	}
	return out
}

func matchWord(word string) func(string) [][2]int {
	return func(p string) [][2]int {
		var out [][2]int
		for from := 0; ; {
			j := strings.Index(p[from:], word)
			if j < 0 {
				return out
			}
			out = append(out, [2]int{from + j, from + j + len(word)})
			from += j + len(word)
		}
	}
}

var matchSt = uv.Style{Bg: ansi.IndexedColor(8), Underline: uv.UnderlineSingle}

func TestRestyleRowUnchangedWithoutMatch(t *testing.T) {
	row := styleRed.Styled("hello ") + styleBold.Styled("world")
	if got := RestyleRow(row, matchWord("zzz"), matchSt); got != row {
		t.Errorf("RestyleRow = %q, want the row unchanged %q", got, row)
	}
}

func TestRestyleRowStylesOnlyMatchedCells(t *testing.T) {
	row := "  " + styleRed.Styled("see ab") + styleBold.Styled("cd ab") + " end"
	before := rowCells(row)
	got := RestyleRow(row, matchWord("ab"), matchSt)
	after := rowCells(got)
	if len(after) != len(before) {
		t.Fatalf("width %d, want %d", len(after), len(before))
	}
	plain := ansi.Strip(row)
	covered := make([]bool, len(plain))
	for _, r := range matchWord("ab")(plain) {
		for i := r[0]; i < r[1]; i++ {
			covered[i] = true
		}
	}
	for x := range before {
		if before[x].Content == "" {
			continue
		}
		if after[x].Content != before[x].Content {
			t.Errorf("cell %d text %q, want %q", x, after[x].Content, before[x].Content)
		}
		want := before[x].Style
		if covered[x] {
			want = matchSt
		}
		if !after[x].Style.Equal(&want) {
			t.Errorf("cell %d %q style %q, want %q", x, after[x].Content, after[x].Style.String(), want.String())
		}
	}
}

func TestRestyleRowWideRunes(t *testing.T) {
	row := styleRed.Styled("日本語 日本")
	got := RestyleRow(row, matchWord("日本"), matchSt)
	cells := rowCells(got)
	before := rowCells(row)
	if len(cells) != len(before) {
		t.Fatalf("width %d, want %d", len(cells), len(before))
	}
	// 日本語 日本: cells 0-3 and 7-10 are the matches; 語 and the gap keep red.
	for x := range cells {
		if cells[x].Content == "" { // trailing half of a wide rune
			continue
		}
		match := x < 4 || (x >= 7 && x < 11)
		want := styleRed
		if match {
			want = matchSt
		}
		if !cells[x].Style.Equal(&want) {
			t.Errorf("cell %d %q style %q, want %q", x, cells[x].Content, cells[x].Style.String(), want.String())
		}
	}
	if ansi.Strip(got) != "日本語 日本" {
		t.Errorf("text %q", ansi.Strip(got))
	}
}

func TestRestyleRowKeepsAttributesWithoutColour(t *testing.T) {
	row := styleBold.Styled("bold ab") + " " + styleUl.Styled("ab")
	got := RestyleRow(row, matchWord("ab"), uv.Style{Underline: uv.UnderlineSingle})
	cells := rowCells(got)
	if cells[0].Style.Attrs&uv.AttrBold == 0 {
		t.Errorf("unmatched bold cell lost its attribute: %q", cells[0].Style.String())
	}
	if cells[5].Style.Underline == uv.UnderlineNone {
		t.Errorf("matched cell has no underline: %q", cells[5].Style.String())
	}
}

func TestRestyleRowEmitsOnlySGR(t *testing.T) {
	row := styleRed.Styled("x ab y")
	got := RestyleRow(row, matchWord("ab"), matchSt)
	for i := 0; i < len(got); i++ {
		if got[i] == 0x1b {
			if i+1 >= len(got) || got[i+1] != '[' {
				t.Fatalf("non-CSI escape in %q", got)
			}
			j := i + 2
			for j < len(got) && (got[j] == ';' || got[j] == ':' || (got[j] >= '0' && got[j] <= '9')) {
				j++
			}
			if j >= len(got) || got[j] != 'm' {
				t.Fatalf("non-SGR sequence in %q", got)
			}
			i = j
			continue
		}
		if got[i] < 0x20 || got[i] == 0x7f {
			t.Fatalf("control byte %#x in %q", got[i], got)
		}
	}
}
