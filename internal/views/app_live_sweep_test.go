package views

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
)

// sameCells reports whether two screen rows hold the same text and styles.
// Cells the read view pads with unstyled spaces equal empty cells.
func sameCells(a, b []uv.Cell) bool {
	norm := func(c uv.Cell) (string, string) {
		if c.Content == "" || c.Content == " " {
			return " ", c.Style.String()
		}
		return c.Content, c.Style.String()
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		at, as := norm(a[i])
		bt, bs := norm(b[i])
		if at != bt || as != bs {
			return false
		}
	}
	return true
}

// liveReadSweep: at every scroll offset of the read view (a sample of them
// under the race detector, where all would cost minutes; see sweepOffsets), e
// opens live preview and every text row above the cursor unit and below it is
// the read view's row, cell for cell, as far as the window shows.
func liveReadSweep(t *testing.T) {
	compared := 0
	pages := []struct{ name, text string }{{"rowless", rowlessPage()}, {"round trip", roundTripPage()}}
	for p, page := range pages {
		for _, width := range []int{40, 80} {
			t.Run(fmt.Sprintf("%s width %d", page.name, width), func(t *testing.T) {
				a := newApp(t, map[string]string{"pages/Doc.md": page.text}, width, 20)
				a.navigate("Doc")
				tally := tallyRows(page.text, width)
				last := -1
				for off := range 400 {
					a.page.Restore(off, -1)
					if a.page.Offset() != off {
						if off == 0 {
							t.Fatal("setup: offset 0 not reachable")
						}
						break
					}
					last = off
				}
				for _, off := range sweepOffsets(last, uint64(p*100+width)) {
					a.page.Restore(off, -1)
					read := frameCells(a.View().Content, width, colorprofile.TrueColor)
					a.Update(key("e"))
					e := a.editor
					if e == nil || e.source {
						t.Fatalf("offset %d: no live editor", off)
					}
					edit := frameCells(a.View().Content, width, colorprofile.TrueColor)
					compared += compareLiveToRead(t, e, tally, off, read, edit)
					a.Update(esc)
					if a.editor != nil {
						t.Fatalf("offset %d: editor did not close", off)
					}
				}
			})
		}
	}
	t.Logf("compared %d rows", compared)
	if compared == 0 {
		t.Fatal("no row was compared")
	}
}

// rowTally is how many display rows each line has in the read view's own
// rendering (Lead, Body, Trail), counted once per page and width: the editor's
// rows away from its cursor unit are those rows.
type rowTally struct{ lead, body, trail []int }

func tallyRows(page string, width int) rowTally {
	ref := NewEditorView(nil, "P", "/tmp/p.md", page, false, width, 20, Anchor{ScreenRow: 1}, nil, false)
	var r rowTally
	for i := range ref.buf.Len() {
		lr, _ := ref.liveRows(i)
		r.lead = append(r.lead, len(lr.Lead))
		r.body = append(r.body, len(lr.Body))
		r.trail = append(r.trail, len(lr.Trail))
	}
	return r
}

// compareLiveToRead checks the editor frame against the read frame whose
// scroll offset was off, and returns how many rows it compared.
func compareLiveToRead(t *testing.T, e *EditorView, tally rowTally, off int, read, edit [][]uv.Cell) int {
	t.Helper()
	if len(e.shown) != 1 {
		t.Fatalf("offset %d: reveal set %v, want the cursor's unit only", off, e.shown)
	}
	from, to := e.shown[0].from, e.shown[0].to
	before := 1 // display index of the unit's first line's first row; 1 is the top margin row
	for i := range from {
		before += tally.lead[i] + tally.body[i] + tally.trail[i]
	}
	cur := e.buf.Cursor()
	absCursor := before
	end := before
	var shift int
	for i := from; i < to; i++ {
		rawRows := len(e.line(i).rows)
		if i < cur.Line {
			absCursor += tally.lead[i] + rawRows + tally.trail[i]
		}
		end += tally.lead[i] + rawRows + tally.trail[i]
		shift += rawRows - tally.body[i]
	}
	rawRow, _ := e.rawRow()
	absCursor += tally.lead[cur.Line] + rawRow
	absTop := absCursor - e.cursorScreenRow()
	start := before + tally.lead[from]

	compared := 0
	for y := range e.textHeight() {
		ae := absTop + y
		var ar int
		switch {
		case ae < start:
			ar = ae
		case ae >= end:
			ar = ae - shift
		default:
			continue
		}
		ry := ar - off
		if ry < 0 || ry >= e.textHeight() {
			continue
		}
		if !sameCells(edit[y], read[ry]) {
			t.Errorf("offset %d: editor row %d (display %d) differs from read row %d\n editor: %q\n   read: %q",
				off, y, ae, ry, cellsText(edit[y]), cellsText(read[ry]))
		}
		compared++
	}
	return compared
}

func cellsText(cells []uv.Cell) string {
	s := ""
	for _, c := range cells {
		if c.Content == "" {
			s += " "
		} else {
			s += c.Content
		}
	}
	return s
}

func TestLiveFrameMatchesReadFrameSweepTerminal(t *testing.T) {
	skipInSourceRun(t)
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	liveReadSweep(t)
}

func TestLiveFrameMatchesReadFrameSweepDark(t *testing.T) {
	skipInSourceRun(t)
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": "dark"}) {
		return
	}
	liveReadSweep(t)
}

func TestLiveFrameMatchesReadFrameSweepNoColor(t *testing.T) {
	skipInSourceRun(t)
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "1", "WEFT_STYLE": ""}) {
		return
	}
	liveReadSweep(t)
}
