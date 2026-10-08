package views

import (
	"slices"
	"strings"
	"testing"
)

// shownText is the non-blank rows of e's frame, trimmed.
func shownText(e *EditorView) []string {
	var out []string
	for _, r := range plainFrame(e) {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// A paragraph or quote the read view joins and wraps onto more than one row is
// revealed whole: all its source lines replace all its rendered rows, whichever
// line the cursor is on, and a jump to a match in it reveals the same rows. The
// lines end (and, in the last two variants, start) in the punctuation that
// stays between one line's last letter and the next line's first words.
func TestLiveRevealsWrappedJoinedUnits(t *testing.T) {
	skipInSourceRun(t)
	quietTerm(t)
	variants := []struct {
		name  string
		lines [3]string
	}{
		{"plain", [3]string{"Joined paragraph first line has alpha text", "second line has the uniquezebra word here", "third line closes the paragraph"}},
		{"period", [3]string{"Joined paragraph first line has alpha text.", "Second line has the uniquezebra word here.", "Third line closes the paragraph."}},
		{"bold", [3]string{"Joined paragraph first line has **alpha**", "Second line has the uniquezebra **word**", "Third line closes the **paragraph**"}},
		{"link", [3]string{"Joined paragraph first line has [[Alpha]]", "Second line has the uniquezebra [[Word]]", "Third line closes the [[Paragraph]]"}},
		{"paren", [3]string{"Joined paragraph first line has (alpha)", "Second line has the uniquezebra (word)", "Third line closes the (paragraph)"}},
		{"opening", [3]string{"Joined paragraph first line has alpha", "**Second** line has the uniquezebra word", "(third) line closes the paragraph"}},
	}
	for _, v := range variants {
		for _, quote := range []bool{false, true} {
			lines := v.lines[:]
			name := "paragraph " + v.name
			if quote {
				lines = slices.Clone(lines)
				lines[0] = "> " + lines[0]
				name = "lazy quote " + v.name
			}
			t.Run(name, func(t *testing.T) {
				page := "- before\n\n" + strings.Join(lines, "\n") + "\n\n- after\n"
				want := slices.Concat([]string{"• before"}, lines, []string{"• after"})
				e := liveEditor(page, 60, 24)
				if n := len(shownText(e)); n < 4 {
					t.Fatalf("setup: the rendered page shows %d rows, want the joined text on 2+ rows:\n%s", n, strings.Join(plainFrame(e), "\n"))
				}
				for l := 2; l < 2+len(lines); l++ {
					goTo(e, l)
					if got := shownText(e); !slices.Equal(got, want) {
						t.Errorf("cursor on line %d:\n got %q\nwant %q", l, got, want)
					}
				}

				goTo(e, 0)
				openFind(e, "zebra")
				if got := e.buf.Cursor().Line; got != 3 {
					t.Fatalf("find jumped to line %d, want 3", got)
				}
				if got := shownText(e); !slices.Equal(got, want) {
					t.Errorf("find jump into the unit:\n got %q\nwant %q", got, want)
				}
			})
		}
	}
}
