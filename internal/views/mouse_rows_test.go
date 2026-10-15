package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v3/internal/layout"
)

// drawBox is a rounded box of w columns and h lines.
func drawBox(w, h int) string {
	lines := make([]string, h)
	for i := range lines {
		switch i {
		case 0:
			lines[i] = "╭" + strings.Repeat("─", w-2) + "╮"
		case h - 1:
			lines[i] = "╰" + strings.Repeat("─", w-2) + "╯"
		default:
			lines[i] = "│" + strings.Repeat(" ", w-2) + "│"
		}
	}
	return strings.Join(lines, "\n")
}

func TestBoxOriginMatchesCenterOverlay(t *testing.T) {
	a := New("/no/such/path", layout.Default(), "test")
	screens := [][2]int{{80, 24}, {81, 25}, {80, 25}, {81, 24}, {100, 40}, {101, 41}, {30, 10}, {31, 11}}
	boxes := [][2]int{{20, 6}, {21, 7}, {20, 7}, {21, 6}, {200, 50}, {200, 6}, {20, 50}, {3, 3}}
	for _, s := range screens {
		for _, b := range boxes {
			a.width, a.height = s[0], s[1]
			out := strings.Split(ansi.Strip(a.centerOverlay(drawBox(b[0], b[1]))), "\n")
			wx, wy := -1, -1
			for y, line := range out {
				if i := strings.Index(line, "╭"); i >= 0 {
					wx, wy = ansi.StringWidth(line[:i]), y
					break
				}
			}
			if x, y := a.boxOrigin(b[0], b[1]); x != wx || y != wy {
				t.Errorf("screen %v box %v: boxOrigin (%d,%d), ╭ at (%d,%d)", s, b, x, y, wx, wy)
			}
		}
	}
	a.width, a.height = 0, 0
	if x, y := a.boxOrigin(20, 6); x != 0 || y != 0 {
		t.Errorf("unknown size: boxOrigin (%d,%d), want (0,0)", x, y)
	}
}

func TestRowMapMultiLineSpans(t *testing.T) {
	var m rowMap
	var b strings.Builder
	for range 2 { // reuse after reset must not carry state
		m.reset()
		b.Reset()
		b.WriteString("title\n\n")
		from := b.Len()
		b.WriteString("head\n  p1\n  p2\n")
		m.span(&b, from, 7)
		b.WriteString("gap\n")
		from = b.Len()
		b.WriteString("head2\n")
		m.span(&b, from, 8)
		want := map[int]int{2: 7, 3: 7, 4: 7, 6: 8}
		for line := -1; line < 9; line++ {
			item, ok := m.itemAt(line)
			w, wok := want[line]
			if ok != wok || (ok && item != w) {
				t.Errorf("line %d: itemAt = (%d,%v), want (%d,%v)", line, item, ok, w, wok)
			}
		}
	}
}

func TestRowPanelsMapDrawnRows(t *testing.T) {
	for _, c := range listCases() {
		t.Run(c.name, func(t *testing.T) {
			a := c.overlay(t, 16)
			p, ok := a.active.(rowPanel)
			if !ok {
				t.Fatalf("%T is not a rowPanel", a.active)
			}
			body := func() []string {
				lines := strings.Split(ansi.Strip(p.View()), "\n")
				var out []string
				for _, l := range lines[2 : len(lines)-2] {
					out = append(out, strings.TrimRight(strings.TrimSuffix(strings.TrimPrefix(l, "│  "), "│"), " "))
				}
				return out
			}
			// A panel scrolled to the middle shows both "more" lines.
			if c.more {
				for range 9 {
					a.Update(listDown)
				}
			}
			var mapped []int
			for i, line := range body() {
				trim := strings.TrimSpace(line)
				chrome := i == 0 || trim == "" || strings.HasPrefix(trim, "──") ||
					((strings.HasPrefix(trim, "↑") || strings.HasPrefix(trim, "↓")) && strings.Contains(trim, "more")) ||
					!(strings.HasPrefix(line, "   ") || strings.HasPrefix(line, " ▶ "))
				item, got := p.drawnRows().itemAt(i)
				if got == chrome {
					t.Errorf("body line %d %q: mapped %v (item %d), want mapped %v", i, trim, got, item, !chrome)
				}
				if strings.HasPrefix(line, " ▶ ") && (!got || item != p.selected()) {
					t.Errorf("selected line %d %q maps to (%d,%v), selected() is %d", i, trim, item, got, p.selected())
				}
				if got && (len(mapped) == 0 || mapped[len(mapped)-1] != item) {
					mapped = append(mapped, item)
				}
			}
			if len(mapped) < 2 {
				t.Fatalf("only %d items on screen", len(mapped))
			}
			for _, item := range mapped {
				if p.selected() == item {
					continue
				}
				if res := pickRow(p, item); res.kind != overlayResultNone {
					t.Errorf("pickRow(%d) returned %+v, want the zero result", item, res)
				}
				if p.selected() != item {
					t.Errorf("pickRow(%d) left %d selected", item, p.selected())
				}
			}
			if _, ok := p.drawnRows().itemAt(-1); ok {
				t.Error("itemAt(-1) maps")
			}
			if _, ok := p.drawnRows().itemAt(1 << 20); ok {
				t.Error("itemAt past the box maps")
			}
		})
	}
}

func TestRowPanelsBacklinksConfirmMapsNothing(t *testing.T) {
	var a *App
	for _, c := range listCases() {
		if c.name == "backlinks" {
			a = c.overlay(t, 24)
		}
	}
	bl := a.active.(*Backlinks)
	for i, r := range bl.rows {
		if r.unlinkedRow() {
			bl.sel = i
		}
	}
	if !bl.rows[bl.sel].unlinkedRow() {
		t.Fatal("setup: no unlinked row")
	}
	bl.confirming = true
	n := len(strings.Split(bl.View(), "\n"))
	for i := range n {
		if item, ok := bl.drawnRows().itemAt(i); ok {
			t.Errorf("confirming: body line %d maps to item %d", i, item)
		}
	}
}
