package views

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

const findLivePage = "# Title\n\n- see [[Alpha]] and [docs](http://x.y/alpha)\n\nOther paragraph here.\n\nAlpha raw line\n\nLast **bold** text.\n"

func findLiveEditor() *EditorView {
	return liveEditor(findLivePage, 60, 24)
}

// rowIndex is the index of the first frame row whose plain text contains s.
func rowIndex(rows []string, s string) int {
	for i, r := range rows {
		if strings.Contains(plain(r), s) {
			return i
		}
	}
	return -1
}

func underlined(c uv.Cell) bool { return c.Style.Underline != uv.UnderlineNone }

func TestFindLiveHighlightsRenderedRows(t *testing.T) {
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
			e := findLiveEditor()
			goTo(e, 6) // the "Alpha raw line" line: a match elsewhere in the page is rendered
			before := frameText(e)
			openFind(e, "alpha")
			// The first match at or after the cursor is on line 6: revealed raw.
			cur := e.buf.Cursor()
			if cur.Line != 6 {
				t.Fatalf("cursor on line %d, want 6", cur.Line)
			}
			rows := frameText(e)
			bullet := rowIndex(rows, "see Alpha and docs")
			if bullet < 0 {
				t.Fatalf("rendered bullet row missing:\n%s", strings.Join(plain2(rows), "\n"))
			}
			cells := frameCells(strings.Join(rows, "\n"), 60, env.profile)
			row := cells[bullet]
			pl := plain(rows[bullet])
			start := ansi.StringWidth(pl[:strings.Index(pl, "Alpha")])
			ref := frameCells(strings.Join(before, "\n"), 60, env.profile)[rowIndex(before, "see Alpha and docs")]
			for x, c := range row {
				if x >= start && x < start+len("Alpha") {
					if !underlined(c) {
						t.Errorf("match cell %d %q has no underline", x, c.Content)
					}
					if env.noColor == "" && !sameColour(c.Style.Bg, 8) {
						t.Errorf("match cell %d %q: style %q, want bg 8", x, c.Content, c.Style.String())
					}
					continue
				}
				if c.Content != ref[x].Content || !c.Style.Equal(&ref[x].Style) {
					t.Errorf("cell %d outside the match changed: %q %q, was %q %q", x, c.Content, c.Style.String(), ref[x].Content, ref[x].Style.String())
				}
			}
			// Rows with no occurrence are byte-identical to the frame without the bar.
			other := rowIndex(rows, "Other paragraph here.")
			if other < 0 || other >= len(before) || rows[other] != before[other] {
				t.Errorf("row without occurrences changed:\n%q\n%q", rows[other], before[other])
			}
		})
	}
}

func plain2(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = plain(r)
	}
	return out
}

func TestFindLiveStepRevealsAndRestyles(t *testing.T) {
	quietTerm(t)
	e := findLiveEditor()
	goTo(e, 6)
	openFind(e, "alpha")
	if e.buf.Cursor().Line != 6 {
		t.Fatalf("cursor on line %d, want 6", e.buf.Cursor().Line)
	}
	if rowIndex(frameText(e), "Alpha raw line") < 0 {
		t.Fatal("current match's line not shown")
	}
	// Step to the next match (wraps to the bullet's line, 2): it is revealed raw and
	// the previous line renders again.
	press(e, named(tea.KeyEnter))
	if got := e.buf.Cursor().Line; got != 2 {
		t.Fatalf("cursor on line %d after step, want 2", got)
	}
	rows := frameText(e)
	if rowIndex(rows, "[[Alpha]]") < 0 {
		t.Errorf("stepped-to line not raw:\n%s", strings.Join(plain2(rows), "\n"))
	}
	if i := rowIndex(rows, "Alpha raw line"); i < 0 {
		t.Fatal("previous match line missing")
	} else {
		cells := frameCells(strings.Join(rows, "\n"), 60, colorprofile.Ascii)[i]
		any := false
		for _, c := range cells {
			any = any || underlined(c)
		}
		if !any {
			t.Error("previous match's line, rendered again, has no highlight")
		}
	}
}

func TestFindLiveCloseRestoresRows(t *testing.T) {
	quietTerm(t)
	e := findLiveEditor()
	goTo(e, 6)
	before := frameText(e)
	openFind(e, "alpha")
	press(e, named(tea.KeyEscape))
	if e.find != nil {
		t.Fatal("bar still open")
	}
	after := frameText(e)
	for i := range before {
		if i < len(after) && rowIndex([]string{before[i]}, "see") >= 0 && before[i] != after[i] {
			t.Errorf("row %d not restored:\n%q\n%q", i, after[i], before[i])
		}
	}
}
