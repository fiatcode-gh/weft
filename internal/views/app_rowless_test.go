package views

import (
	"fmt"
	"strings"
	"testing"
)

// rowlessPage has every kind of read row that carries no letters or digits
// (a rule, a table's delimiter border, a punctuation-only bullet) among wrapped
// bullets, a LOGBOOK, quotes, a fence and headings, repeated so that scrolling
// it takes well over a hundred offsets at either width.
func rowlessPage() string {
	var b strings.Builder
	long := "wrapped alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau"
	for n := range 6 {
		fmt.Fprintf(&b, "## Section %d\n\n", n)
		fmt.Fprintf(&b, "- TODO top %d %s\n", n, long)
		b.WriteString("  :LOGBOOK:\n  CLOCK: [2026-10-01 Thu 09:00]\n  :END:\n")
		fmt.Fprintf(&b, "  - nested %d %s\n\n", n, long)
		b.WriteString("---\n\n")
		b.WriteString("| Name | Value |\n|------|-------|\n| a | 1 |\n| b | 2 |\n\n")
		b.WriteString("- ...\n- after the dots\n\n")
		fmt.Fprintf(&b, "> quoted %d %s\n\n", n, long)
		b.WriteString("```go\nx := 1\n```\n\n")
		b.WriteString("---\n\n")
		fmt.Fprintf(&b, "A paragraph %d %s\n\n", n, long)
	}
	return b.String()
}

// roundTripSweep: with no edit, e then Esc leaves the read frame byte for byte
// as it was, at every scroll offset of the first 120 (sampled under the race
// detector, see sweepOffsets).
func roundTripSweep(t *testing.T) {
	const offsets = 120
	for _, width := range []int{40, 80} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			a := newApp(t, map[string]string{"pages/Doc.md": rowlessPage()}, width, 20)
			a.navigate("Doc")
			a.page.Restore(offsets-1, offsets-1, -1)
			if a.page.Offset() != offsets-1 {
				t.Fatalf("setup: offset %d not reachable, got %d", offsets-1, a.page.Offset())
			}
			checked := 0
			for _, off := range sweepOffsets(offsets-1, uint64(width)) {
				a.page.Restore(off, off, -1)
				if a.page.Offset() != off {
					t.Fatalf("setup: offset %d not reachable, got %d", off, a.page.Offset())
				}
				want := a.View().Content
				a.Update(key("e"))
				if a.editor == nil {
					t.Fatalf("offset %d: editor did not open", off)
				}
				a.Update(esc)
				if a.editor != nil {
					t.Fatalf("offset %d: editor did not close", off)
				}
				if got := a.View().Content; got != want {
					t.Errorf("offset %d: a clean e then Esc changed the read frame (offset now %d):\nbefore:\n%s\nafter:\n%s",
						off, a.page.Offset(), plain(want), plain(got))
				}
				checked++
			}
			if !raceDetector && checked != offsets {
				t.Fatalf("checked %d offsets, want %d", checked, offsets)
			}
			if checked < 2 {
				t.Fatalf("checked %d offsets", checked)
			}
		})
	}
}

func TestEditorRoundTripSweepTerminal(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	roundTripSweep(t)
}

func TestEditorRoundTripSweepDark(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": "dark"}) {
		return
	}
	roundTripSweep(t)
}

func TestEditorRoundTripSweepNoColor(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "1", "WEFT_STYLE": ""}) {
		return
	}
	roundTripSweep(t)
}

// TestEditorOpensOnRowlessRows: with a row that has no letters at the top of the read view,
// the editor's first text row is that row's own source line; with the row
// under it at the top, the same holds for the text below.
func TestEditorOpensOnRowlessRows(t *testing.T) {
	quietTerm(t)
	a := newApp(t, map[string]string{"pages/Doc.md": rowlessPage()}, 80, 20)
	a.navigate("Doc")
	tests := []struct {
		name     string
		topRow   string // the read view's first row, as it reads in the plain frame
		wantEdit string // the editor's first text row
	}{
		{"rule", "--------", "---"},
		{"table delimiter border", "------------------|-", "|------|-------|"},
		{"table header under the rule", "Name", "| Name | Value |"},
		{"punctuation-only bullet", "• ...", "..."},
		{"bullet after it", "after the dots", "after the dots"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			off := -1
			for o := range 120 {
				a.page.Restore(o, o, -1)
				if strings.Contains(strings.TrimSpace(frameLines(a)[0]), tt.topRow) {
					off = o
					break
				}
			}
			if off < 0 {
				t.Fatalf("setup: no offset puts %q on the first row", tt.topRow)
			}
			a.Update(key("e"))
			if a.editor == nil {
				t.Fatal("editor did not open")
			}
			defer a.Update(esc)
			if got := glyphless(frameLines(a)[0]); got != tt.wantEdit {
				t.Errorf("editor's first row = %q, want %q (read offset %d)", got, tt.wantEdit, off)
			}
		})
	}
}

// A row no source line spells (a table's outer border) lends its text to the
// line below it, but is never what the editor anchors on: the line below is
// anchored on the screen row it really has, and placing that anchor restores
// the offset.
func TestReadingAnchorSkipsBorrowedRows(t *testing.T) {
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{"pages/Doc.md": rowlessPage()})
	p := NewPageView(idx, "Doc", 80, 20)
	orig := sourceRowsFor
	t.Cleanup(func() { sourceRowsFor = orig })
	var rule, header int // the first rule, treated as a border above the header
	sourceRowsFor = func(body string, width int, emphasis string) ([]int, []bool) {
		lines, own := orig(body, width, emphasis)
		rows := p.full[:min(len(lines), p.rowCount())]
		for rule = range rows {
			if strings.TrimSpace(plain(rows[rule])) == "--------" {
				break
			}
		}
		header = rule + 1
		for !nonBlank(rows[header]) {
			header++
		}
		lines[rule], own[rule] = lines[header], false
		return lines, own
	}
	p.sourceRows()
	p.Restore(rule, rule, -1)
	if p.Offset() != rule {
		t.Fatalf("setup: offset %d not reachable, got %d", rule, p.Offset())
	}
	at, ok := p.ReadingAnchor()
	want := Anchor{Line: p.sourceRows()[header], RowInLine: 0, ScreenRow: header - rule}
	if !ok || at != want {
		t.Fatalf("ReadingAnchor = (%+v, %v), want (%+v, true)", at, ok, want)
	}
	p.Restore(rule+5, rule+5, -1)
	p.PlaceAnchor(at)
	if got := p.Offset(); got != rule {
		t.Errorf("PlaceAnchor(%+v) left offset %d, want %d", at, got, rule)
	}
}
