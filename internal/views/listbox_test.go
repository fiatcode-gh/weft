package views

import "testing"

// anchoredAt is a listBox over a 30-item list that a click anchored at start:
// it renders the window centred on sel, then anchors it.
func anchoredAt(t *testing.T, sel, rows, start int) *listBox {
	t.Helper()
	b := &listBox{sel: sel}
	if s, _ := b.window(30, rows); s != start {
		t.Fatalf("fixture: sel %d draws start %d, want %d", sel, s, start)
	}
	b.anchor()
	if !b.anchored || b.anchorTop != start {
		t.Fatalf("anchor() pinned %v/%d, want true/%d", b.anchored, b.anchorTop, start)
	}
	return b
}

func TestListBoxWindowAnchor(t *testing.T) {
	type win struct{ start, end int }
	check := func(t *testing.T, b *listBox, count, rows int, want win, anchored bool) {
		t.Helper()
		s, e := b.window(count, rows)
		if (win{s, e}) != want || b.drawnTop != s || b.anchored != anchored {
			t.Errorf("sel %d count %d rows %d: window [%d,%d) drawnTop %d anchored %v, want [%d,%d) drawnTop %d anchored %v",
				b.sel, count, rows, s, e, b.drawnTop, b.anchored, want.start, want.end, want.start, anchored)
		}
	}

	t.Run("unanchored is scrollWindow", func(t *testing.T) {
		for _, rows := range []int{6, 40} {
			for sel := range 30 {
				b := &listBox{sel: sel}
				s, e := scrollWindow(sel, 30, rows)
				check(t, b, 30, rows, win{s, e}, false)
			}
		}
	})
	t.Run("selection in the anchored window keeps it", func(t *testing.T) {
		for sel := 10; sel <= 15; sel++ {
			b := anchoredAt(t, 13, 6, 10)
			b.sel = sel
			check(t, b, 30, 6, win{10, 16}, true)
		}
	})
	t.Run("selection outside centres and drops for good", func(t *testing.T) {
		for _, sel := range []int{9, 16} {
			b := anchoredAt(t, 13, 6, 10)
			b.sel = sel
			s, e := scrollWindow(sel, 30, 6)
			check(t, b, 30, 6, win{s, e}, false)
			b.sel = 12 // inside the old anchored window: stays centred
			s, e = scrollWindow(12, 30, 6)
			check(t, b, 30, 6, win{s, e}, false)
		}
	})
	t.Run("list shrinks", func(t *testing.T) {
		b := anchoredAt(t, 13, 6, 10)
		b.sel = 12
		check(t, b, 13, 6, win{10, 13}, true)
	})
	t.Run("list shrinks past the anchored start", func(t *testing.T) {
		b := anchoredAt(t, 13, 6, 10)
		b.sel = 9
		s, e := scrollWindow(9, 10, 6)
		check(t, b, 10, 6, win{s, e}, false)
	})
	t.Run("taller box", func(t *testing.T) {
		b := anchoredAt(t, 13, 6, 10)
		b.sel = 17
		check(t, b, 30, 8, win{10, 18}, true)
	})
	t.Run("create row past the list", func(t *testing.T) {
		b := anchoredAt(t, 13, 6, 10)
		b.sel = 30
		check(t, b, 30, 6, win{10, 16}, true)
	})
	t.Run("dropAnchor centres", func(t *testing.T) {
		b := anchoredAt(t, 13, 6, 10)
		b.sel = 15
		b.dropAnchor()
		s, e := scrollWindow(15, 30, 6)
		check(t, b, 30, 6, win{s, e}, false)
	})
}

func threes(i int) string { return string(rune('a' + i/3)) }

func TestListBoxGroupWindowAnchor(t *testing.T) {
	const n, budget = 30, 10
	t.Run("unanchored is groupedWindow", func(t *testing.T) {
		for sel := range n {
			b := &listBox{sel: sel}
			s, e := b.groupWindow(n, budget, threes)
			ws, we := groupedWindow(n, sel, budget, threes)
			if s != ws || e != we || b.drawnTop != ws {
				t.Errorf("sel %d: [%d,%d) drawnTop %d, want [%d,%d)", sel, s, e, b.drawnTop, ws, we)
			}
		}
	})

	// Start 6 holds items 6..11 in a budget of 10 (header 2, 1, 1, header+blank 3, 1, 1).
	anchored := func(sel int) *listBox {
		b := &listBox{sel: sel, drawnTop: 6}
		b.anchor()
		return b
	}
	t.Run("selection inside the window keeps the start", func(t *testing.T) {
		for sel := 6; sel <= 11; sel++ {
			b := anchored(sel)
			s, e := b.groupWindow(n, budget, threes)
			if s != 6 || e != groupedEnd(n, 6, budget, threes) || !b.anchored || b.drawnTop != 6 {
				t.Errorf("sel %d: [%d,%d) anchored %v drawnTop %d, want start 6 kept", sel, s, e, b.anchored, b.drawnTop)
			}
		}
	})
	centres := func(t *testing.T, b *listBox, n, budget int) {
		t.Helper()
		s, e := b.groupWindow(n, budget, threes)
		ws, we := groupedWindow(n, b.sel, budget, threes)
		if s != ws || e != we || b.anchored || b.drawnTop != ws {
			t.Errorf("sel %d: [%d,%d) anchored %v drawnTop %d, want centred [%d,%d) and dropped", b.sel, s, e, b.anchored, b.drawnTop, ws, we)
		}
	}
	t.Run("selection outside centres", func(t *testing.T) {
		for _, sel := range []int{5, 12} {
			centres(t, anchored(sel), n, budget)
		}
	})
	t.Run("start past the end centres", func(t *testing.T) {
		b := anchored(5)
		centres(t, b, 6, budget)
	})
	t.Run("budget too small to reach the selection centres", func(t *testing.T) {
		centres(t, anchored(11), n, 3)
	})
}

func TestGroupedEndMatchesWindow(t *testing.T) {
	for _, n := range []int{1, 2, 7, 30} {
		for budget := 1; budget <= 20; budget++ {
			for sel := range n {
				start, end := groupedWindow(n, sel, budget, threes)
				if got := groupedEnd(n, start, budget, threes); got != end {
					t.Errorf("n %d budget %d sel %d: groupedEnd(start %d) = %d, groupedWindow end %d", n, budget, sel, start, got, end)
				}
			}
		}
	}
}
