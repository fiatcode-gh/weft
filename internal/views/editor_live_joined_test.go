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

// The text of the resmoke's Lazy and Lazy2 pages. At width 100 the read view
// wraps the first (with a period) in two stages, so the rows 'lazy' and
// 'third' stand alone without the quote's bar and no row holds the end of one
// line and the start of the next. In the second the quote's last line starts on
// the row that ends the lazy line.
var lazyOrphanPages = map[string][3]string{
	"lazy period": {
		"> lazy one quote first line has words and padding words to make it long enough to wrap here.",
		"lazy continuation line zzfindlazy of the quote with more padding words to wrap here too.",
		"> quote third line closes the quote here."},
	"lazy no period": {
		"> lazy one quote first line has words and padding words to make it long enough to wrap here",
		"lazy continuation line zzfindlazy of the quote with more padding words to wrap here too",
		"> quote third line closes the quote here"},
}

// A lazy quote whose rendered form has orphan rows is revealed whole, from any
// of its lines and from a find jump: no rendered row of it stays on screen.
func TestLiveRevealsLazyQuoteWithOrphanRows(t *testing.T) {
	skipInSourceRun(t)
	quietTerm(t)
	for name, lines := range lazyOrphanPages {
		t.Run(name, func(t *testing.T) {
			page := strings.Join(lines[:], "\n") + "\n\nafter lazy\n"
			want := slices.Concat(lines[:], []string{"after lazy"})
			e := liveEditor(page, 100, 40)
			goTo(e, 4)
			if rendered := shownText(e); slices.Contains(rendered, lines[0]) || (name == "lazy period" && !slices.Contains(rendered, "lazy")) {
				t.Fatalf("setup: the quote is not rendered, or has no orphan 'lazy' row at this width:\n%s", strings.Join(plainFrame(e), "\n"))
			}
			for l := range lines {
				goTo(e, l)
				if got := shownText(e); !slices.Equal(got, want) {
					t.Errorf("cursor on line %d:\n got %q\nwant %q", l, got, want)
				}
			}
			goTo(e, 4)
			openFind(e, "zzfindlazy")
			if got := e.buf.Cursor().Line; got != 1 {
				t.Fatalf("find jumped to line %d, want 1", got)
			}
			if got := shownText(e); !slices.Equal(got, want) {
				t.Errorf("find jump into the quote:\n got %q\nwant %q", got, want)
			}
		})
	}
}
