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
// line the cursor is on, and a jump to a match in it reveals the same rows.
func TestLiveRevealsWrappedJoinedUnits(t *testing.T) {
	skipInSourceRun(t)
	quietTerm(t)
	for _, tc := range []struct {
		name  string
		lines []string
	}{
		{"paragraph", []string{
			"Joined paragraph first line has alpha text",
			"second line has the uniquezebra word here",
			"third line closes the paragraph"}},
		{"lazy quote", []string{
			"> quoted first line has alpha text",
			"lazy second line has the uniquezebra word",
			"third lazy line closes the quote"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := "- before\n\n" + strings.Join(tc.lines, "\n") + "\n\n- after\n"
			want := slices.Concat([]string{"• before"}, tc.lines, []string{"• after"})
			e := liveEditor(page, 60, 24)
			if n := len(shownText(e)); n < 4 {
				t.Fatalf("setup: the rendered page shows %d rows, want the joined text on 2+ rows:\n%s", n, strings.Join(plainFrame(e), "\n"))
			}
			for l := 2; l < 2+len(tc.lines); l++ {
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
