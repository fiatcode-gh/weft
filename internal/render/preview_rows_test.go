package render

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"
)

// rowsDoc is a document with a Scanner and a Preview over it.
type rowsDoc struct {
	d  *previewDoc
	sc *Scanner
	p  *Preview
}

func newRowsDoc(t *testing.T, text string, width int) rowsDoc {
	t.Helper()
	return rowsDoc{newPreviewDoc(text), NewScanner(), NewPreview(mustTheme(t), width)}
}

func (r rowsDoc) rows(t *testing.T, i int) LineRows {
	t.Helper()
	lr, ok := r.p.Rows(r.d, r.sc, i)
	if !ok {
		t.Fatalf("Rows(%d) = false for %q", i, clip(r.d.text()))
	}
	return lr
}

func plain(rows []string) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = ansi.Strip(row)
	}
	return out
}

func allBlank(rows []string) bool {
	for _, row := range rows {
		if strings.TrimSpace(ansi.Strip(row)) != "" {
			return false
		}
	}
	return true
}

// checkRowsPartition is rule 3 of the contract: Lead ++ Body ++ Trail of every
// line, in order, are the rows the read view draws after its head.
func checkRowsPartition(t *testing.T, raw string, width int) (fallbacks int) {
	t.Helper()
	r := newRowsDoc(t, raw, width)
	if _, ok := r.p.chunkAt(r.d, r.sc, 0); !ok {
		first, _ := r.p.bounds(r.d, r.sc)
		if first < 0 {
			return 0
		}
	}
	res, err := Render(strings.TrimSpace(raw)+"\n", width)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(res.Styled, "\n")[r.p.Head():]
	var got []string
	for i := range r.d.Len() {
		lr, ok := r.p.Rows(r.d, r.sc, i)
		if !ok {
			t.Errorf("Rows(%d) = false\ndoc: %q", i, clip(raw))
			return
		}
		got = append(append(append(got, lr.Lead...), lr.Body...), lr.Trail...)
	}
	if !reflect.DeepEqual(got, want) {
		k := 0
		for k < len(got) && k < len(want) && got[k] == want[k] {
			k++
		}
		t.Errorf("width %d: line rows differ from Render at row %d (got %d rows, want %d)\ndoc: %q\n got: %q\nwant: %q",
			width, k, len(got), len(want), clip(raw), clip(rowAt(got, k)), clip(rowAt(want, k)))
	}
	for i := range r.d.Len() {
		if c, ok := r.p.chunkAt(r.d, r.sc, i); ok && c.from == i {
			if a := r.p.render(r.d, r.sc, c).attribution(); a != nil && a.fallback {
				fallbacks++
			}
		}
	}
	return fallbacks
}

func runRowsPartition(t *testing.T) {
	t.Helper()
	fallbacks, chunks := 0, 0
	for _, w := range previewWidths {
		for _, doc := range []string{previewConstructPage, strings.ReplaceAll(previewConstructPage, "\n", "\r\n")} {
			fallbacks += checkRowsPartition(t, doc, w)
		}
	}
	for name, doc := range fixtureDocs(t) {
		if checkRowsPartition(t, doc, 100); t.Failed() {
			t.Fatalf("stopping at fixture %s", name)
		}
	}
	generated := generatedDocs(rowsDocCount)
	for i := 0; i < len(generated); i += sampleStride {
		fallbacks += checkRowsPartition(t, generated[i], previewWidths[i%len(previewWidths)])
		chunks++
		if t.Failed() {
			t.Fatalf("stopping at generated document %d", i)
		}
	}
	t.Logf("%d generated documents, %d chunks fell back", chunks, fallbacks)
}

func TestPreviewRowsPartitionChunkRowsTerminal(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	runRowsPartition(t)
}

func TestPreviewRowsPartitionChunkRowsDark(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": "dark"}) {
		return
	}
	runRowsPartition(t)
}

func TestPreviewRowsPartitionChunkRowsNoColor(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "1", "WEFT_STYLE": ""}) {
		return
	}
	runRowsPartition(t)
}

func TestPreviewUnits(t *testing.T) {
	cases := []struct {
		name  string
		doc   string
		width int
		want  [][2]int // cursor line → unit, for the lines listed
		lines []int
	}{
		{"table", "| a | b |\n|---|---|\n| 1 | 2 |", 40, [][2]int{{0, 3}, {0, 3}, {0, 3}}, []int{0, 1, 2}},
		{"fence with a blank code line", "intro\n\n```go\nx\n\ny\n```\n\nafter", 40,
			[][2]int{{0, 1}, {1, 2}, {2, 7}, {2, 7}, {2, 7}, {2, 7}, {2, 7}, {7, 8}, {8, 9}}, []int{0, 1, 2, 3, 4, 5, 6, 7, 8}},
		{"fence in a list item", "- item\n  ```\n  code\n  ```\n- next", 40,
			[][2]int{{0, 1}, {1, 4}, {1, 4}, {1, 4}, {4, 5}}, []int{0, 1, 2, 3, 4}},
		{"quote with a lazy line", "> a\nlazy\n\nafter", 40, [][2]int{{0, 2}, {0, 2}, {2, 3}, {3, 4}}, []int{0, 1, 2, 3}},
		{"top-level logbook", "para\n\n:LOGBOOK:\nCLOCK: x\n:END:\n\nnext", 40,
			[][2]int{{0, 1}, {1, 2}, {2, 5}, {2, 5}, {2, 5}, {5, 6}, {6, 7}}, []int{0, 1, 2, 3, 4, 5, 6}},
		{"nested logbook", "- item\n  :LOGBOOK:\n  CLOCK: x\n  :END:\n- next", 40,
			[][2]int{{0, 1}, {1, 4}, {1, 4}, {1, 4}, {4, 5}}, []int{0, 1, 2, 3, 4}},
		{"multi-line query", "a\n\n{{query\n(and)\n}}\n\nb", 40, [][2]int{{2, 5}, {2, 5}, {2, 5}}, []int{2, 3, 4}},
		{"single-line embed", "a\n\n{{embed [[X]]}}\n\nb", 40, [][2]int{{2, 3}}, []int{2}},
		{"setext heading", "Title\n===\n\npara", 40, [][2]int{{0, 2}, {0, 2}, {2, 3}, {3, 4}}, []int{0, 1, 2, 3}},
		{"two-line paragraph", "one\ntwo\n\nx", 40, [][2]int{{0, 2}, {0, 2}, {2, 3}}, []int{0, 1, 2}},
		{"joined paragraph wrapped onto two rows", "Joined paragraph first line\nsecond line has the word\nthird line closes it\n\nx", 40,
			[][2]int{{0, 3}, {0, 3}, {0, 3}, {3, 4}}, []int{0, 1, 2, 3}},
		{"lazy quote wrapped onto two rows", "> quoted first line has text\nlazy second line has the word\nthird lazy line closes it\n\nx", 40,
			[][2]int{{0, 3}, {0, 3}, {0, 3}, {3, 4}}, []int{0, 1, 2, 3}},
		{"image then an adjacent quote", "# Notes\n\n![diagram](assets/diagram.png)\n> a quote right after the image", 100,
			[][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}}, []int{0, 1, 2, 3}},
		{"bullet with an image, then a child bullet", "- parent\n  ![shot](assets/a.png)\n  - child bullet", 100,
			[][2]int{{0, 1}, {1, 2}, {2, 3}}, []int{0, 1, 2}},
		{"autolink line, blank, heading", "See the [[Doc]] page\n<http://example.com>\n\n## Next heading", 100,
			[][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}}, []int{0, 1, 2, 3}},
		{"image whose url words are the next line's first words", "![x](a.png)\n> a png", 100,
			[][2]int{{0, 1}, {1, 2}}, []int{0, 1}},
		{"lazy quote with orphan rows", "> lazy one quote first line has words and padding words to make it long enough to wrap here.\n" +
			"lazy continuation line zzfindlazy of the quote with more padding words to wrap here too.\n" +
			"> quote third line closes the quote here.\n\nafter lazy", 100,
			[][2]int{{0, 3}, {0, 3}, {0, 3}, {3, 4}, {4, 5}}, []int{0, 1, 2, 3, 4}},
		{"lazy quote, two lazy lines", "> quote one\nlazy two\nlazy three\n\nx", 40,
			[][2]int{{0, 3}, {0, 3}, {0, 3}, {3, 4}}, []int{0, 1, 2, 3}},
		{"text after a quote heading is not lazy", "> # Head\ntext\n\nx", 40,
			[][2]int{{0, 1}, {1, 2}, {2, 3}}, []int{0, 1, 2}},
		{"list item after a quote is not lazy", "> quote\n- item\n\nx", 40,
			[][2]int{{0, 1}, {1, 2}, {2, 3}}, []int{0, 1, 2}},
		{"list item continuation", "- item\n  cont\n- next", 40, [][2]int{{0, 1}, {1, 2}, {2, 3}}, []int{0, 1, 2}},
		{"wrapped bullet", "- " + strings.Repeat("wrap me ", 8) + "\n- next", 30, [][2]int{{0, 1}, {1, 2}}, []int{0, 1}},
		{"blank line", "a\n\nb", 40, [][2]int{{0, 1}, {1, 2}, {2, 3}}, []int{0, 1, 2}},
		{"outside the trimmed document", "\n\na\n\n", 40, [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}}, []int{0, 1, 2, 3, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRowsDoc(t, tc.doc, tc.width)
			for k, i := range tc.lines {
				from, to := r.p.Unit(r.d, r.sc, i)
				if got := [2]int{from, to}; got != tc.want[k] {
					t.Errorf("Unit(%d) = %v, want %v\ndoc: %q", i, got, tc.want[k], tc.doc)
				}
			}
		})
	}
}

func TestPreviewAttribution(t *testing.T) {
	t.Run("table rows", func(t *testing.T) {
		// This theme draws no outer border: the delimiter row is the
		// delimiter line's own Body.
		r := newRowsDoc(t, "para\n\n| a | b |\n|---|---|\n| 1 | 2 |", 40)
		header, delim := r.rows(t, 2), r.rows(t, 3)
		if len(header.Body) != 1 || !strings.Contains(ansi.Strip(header.Body[0]), "a") {
			t.Errorf("header Body = %q, want the header row", plain(header.Body))
		}
		if len(delim.Body) != 1 || !strings.ContainsAny(ansi.Strip(delim.Body[0]), "-─") {
			t.Errorf("delimiter Body = %q, want the delimiter row", plain(delim.Body))
		}
	})
	t.Run("letterless rows follow the line above or below", func(t *testing.T) {
		rows := []string{"  a ", "  ───", "  b ", "  ───", "", " c "}
		cr := &chunkRender{rows: rows, lines: []int{0, 0, 1, 1, 2, 2}, own: []bool{true, false, true, false, false, true}}
		if got, want := cr.attribution().line, []int{0, 0, 1, 1, -1, 2}; !reflect.DeepEqual(got, want) {
			t.Errorf("line = %v, want %v", got, want)
		}
		cr = &chunkRender{rows: []string{"  ───", "  a "}, lines: []int{1, 1}, own: []bool{false, true}}
		if got, want := cr.attribution().line, []int{1, 1}; !reflect.DeepEqual(got, want) {
			t.Errorf("line = %v, want %v", got, want)
		}
	})
	t.Run("list margin is the first item's lead", func(t *testing.T) {
		r := newRowsDoc(t, "para\n- a\n- b", 40)
		if lr := r.rows(t, 1); len(lr.Lead) != 1 || !allBlank(lr.Lead) || len(lr.Body) == 0 {
			t.Errorf("Rows(1) = %+v, want one blank Lead row and a Body", plain(lr.Lead))
		}
		if lr := r.rows(t, 0); len(lr.Lead) != 0 || len(lr.Trail) != 0 {
			t.Errorf("Rows(0) lead/trail = %q/%q, want none", plain(lr.Lead), plain(lr.Trail))
		}
	})
	t.Run("blank line between paragraphs", func(t *testing.T) {
		r := newRowsDoc(t, "para\n\npara2", 40)
		if lr := r.rows(t, 1); len(lr.Body) != 1 || !allBlank(lr.Body) || len(lr.Lead) != 0 || len(lr.Trail) != 0 {
			t.Errorf("Rows(1) = %q, want the margin row as Body", plain(lr.Body))
		}
		if lr := r.rows(t, 2); len(lr.Lead) != 0 {
			t.Errorf("Rows(2).Lead = %q, want none (the margin is line 1's)", plain(lr.Lead))
		}
	})
	t.Run("blank code line", func(t *testing.T) {
		r := newRowsDoc(t, "```\na\n\nb\n```", 40)
		if lr := r.rows(t, 2); len(lr.Body) != 1 || !allBlank(lr.Body) {
			t.Errorf("Rows(2).Body = %q, want the blank code row", plain(lr.Body))
		}
		if lr := r.rows(t, 1); len(lr.Body) != 1 || !strings.Contains(ansi.Strip(lr.Body[0]), "a") {
			t.Errorf("Rows(1).Body = %q, want the code row of a", plain(lr.Body))
		}
	})
	t.Run("document tail", func(t *testing.T) {
		r := newRowsDoc(t, "a\n\nb", 40)
		if lr := r.rows(t, 2); len(lr.Trail) == 0 || !allBlank(lr.Trail) {
			t.Errorf("Rows(2).Trail = %q, want the tail margin rows", plain(lr.Trail))
		}
		if lr := r.rows(t, 0); len(lr.Trail) != 0 {
			t.Errorf("Rows(0).Trail = %q, want none", plain(lr.Trail))
		}
	})
	t.Run("hidden block and reference definition", func(t *testing.T) {
		r := newRowsDoc(t, "text\n\n:LOGBOOK:\nCLOCK: x\n:END:\n\n[r]: http://x\n\nmore", 40)
		for _, i := range []int{2, 3, 4, 6} {
			if lr := r.rows(t, i); len(lr.Lead)+len(lr.Body)+len(lr.Trail) != 0 {
				t.Errorf("Rows(%d) = %+v, want no rows", i, lr)
			}
		}
	})
	t.Run("lines outside the trimmed document", func(t *testing.T) {
		r := newRowsDoc(t, "\n\na\n\n", 40)
		for _, i := range []int{0, 1, 3, 4} {
			if lr, ok := r.p.Rows(r.d, r.sc, i); !ok || len(lr.Lead)+len(lr.Body)+len(lr.Trail) != 0 {
				t.Errorf("Rows(%d) = %+v, %v, want empty and true", i, lr, ok)
			}
		}
	})
}

func TestPreviewNonMonotoneFallsBack(t *testing.T) {
	orig := chunkRowMap
	t.Cleanup(func() { chunkRowMap = orig })
	chunkRowMap = func(r *glamour.TermRenderer, styled, pre string, src []int) rowMap {
		m := orig(r, styled, pre, src)
		for i := range m.lines {
			m.lines[i], m.own[i] = len(m.lines)-1-i, true
		}
		return m
	}
	r := newRowsDoc(t, "1. a\n2. b\n3. c\n\nnext", 40)
	if from, to := r.p.Unit(r.d, r.sc, 1); from != 0 || to != 4 {
		t.Errorf("Unit(1) = %d, %d, want 0, 4 (the whole chunk)", from, to)
	}
	body := func(i int) []string { return plain(r.rows(t, i).Body) }
	if got := body(0); len(got) != 3 || !strings.Contains(got[0], "a") || !strings.Contains(got[2], "c") {
		t.Errorf("Rows(0).Body = %q, want the chunk's three content rows", got)
	}
	for _, i := range []int{1, 2} {
		if lr := r.rows(t, i); len(lr.Lead)+len(lr.Body)+len(lr.Trail) != 0 {
			t.Errorf("Rows(%d) = %+v, want no rows (all content is in line 0)", i, lr)
		}
	}
	if lr := r.rows(t, 4); len(lr.Body) != 1 || !strings.Contains(ansi.Strip(lr.Body[0]), "next") {
		t.Errorf("Rows(4).Body = %q, want next", plain(lr.Body))
	}
}

func TestPreviewSingleLineChunkSkipsTagging(t *testing.T) {
	r := newRowsDoc(t, "# Title\n\nfirst\nsecond\n\n- item\n\n- other", 40)
	calls := 0
	real := glamourRender
	t.Cleanup(func() { glamourRender = real })
	glamourRender = func(g *glamour.TermRenderer, in string) (string, error) {
		calls++
		return real(g, in)
	}
	for i := 0; i < r.d.Len(); {
		c, ok := r.p.chunkAt(r.d, r.sc, i)
		if !ok {
			i++
			continue
		}
		nonBlank := 0
		for k := c.from; k < c.to; k++ {
			if strings.TrimSpace(r.d.Line(k)) != "" {
				nonBlank++
			}
		}
		want := 2
		if nonBlank == 1 {
			want = 1
		}
		calls = 0
		r.p.render(r.d, r.sc, c)
		if calls != want {
			t.Errorf("chunk %+v (%d non-blank lines): %d Glamour calls, want %d", c, nonBlank, calls, want)
		}
		i = c.to
	}
}

// rowsDocCount is how many generated documents the partition test covers per
// style: each costs two Glamour renders per chunk.
const rowsDocCount = 400

// words splits s into its runs of letters and numbers.
func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !isTextRune(r) })
}

// sharesRow reports whether some rendered row holds the last word of line a
// right before the first word of line b: a wrap that falls inside the lines
// rather than between them.
func sharesRow(rows []string, a, b string) bool {
	wa, wb := words(a), words(b)
	last, first := wa[len(wa)-1], wb[0]
	for _, row := range rows {
		w := words(row)
		for i := 0; i+1 < len(w); i++ {
			if w[i] == last && w[i+1] == first {
				return true
			}
		}
	}
	return false
}

// A paragraph or lazy quote the read view joins and wraps is one unit from any
// of its lines, whatever punctuation its lines start and end with: the render
// keeps the text after a line's last letter ('.', '**', ']]', ')') between the
// line's end and the next line's first words. Each width in 24..70 at which
// the wrap falls inside lines (so every line shares a row with the next) is a
// case.
func TestPreviewUnitsWrappedJoins(t *testing.T) {
	variants := []struct {
		name  string
		lines [3]string
	}{
		{"period", [3]string{"Joined paragraph first line.", "Second line has the word.", "Third line closes it."}},
		{"bold", [3]string{"Joined paragraph first **alpha**", "Second line has the **word**", "Third line closes **it**"}},
		{"link", [3]string{"Joined paragraph first [[Alpha]]", "Second line has the [[Word]]", "Third line closes [[It]]"}},
		{"paren", [3]string{"Joined paragraph first (alpha)", "Second line has the (word)", "Third line closes (it)"}},
		{"code", [3]string{"Joined paragraph first `alpha`", "Second line has the `word`", "Third line closes `it`"}},
		{"opening", [3]string{"Joined paragraph first line", "**Second** line has the word", "(third) line closes it"}},
		{"link opening", [3]string{"Joined paragraph first line", "[[Second]] line has the word", "`third` line closes it"}},
	}
	for _, v := range variants {
		for _, quote := range []bool{false, true} {
			doc, kind := v.lines[0]+"\n"+v.lines[1]+"\n"+v.lines[2]+"\n\nx", "paragraph"
			if quote {
				doc, kind = "> "+v.lines[0]+"\n"+v.lines[1]+"\n"+v.lines[2]+"\n\nx", "lazy quote"
			}
			cases := 0
			for width := 24; width <= 70; width++ {
				r := newRowsDoc(t, doc, width)
				c, ok := r.p.chunkAt(r.d, r.sc, 0)
				if !ok {
					t.Fatal("no chunk")
				}
				rows := plain(r.p.render(r.d, r.sc, c).rows)
				if !sharesRow(rows, v.lines[0], v.lines[1]) || !sharesRow(rows, v.lines[1], v.lines[2]) {
					continue
				}
				cases++
				t.Run(fmt.Sprintf("%s %s w%d", kind, v.name, width), func(t *testing.T) {
					for i := range 3 {
						if from, to := r.p.Unit(r.d, r.sc, i); from != 0 || to != 3 {
							t.Errorf("Unit(%d) = [%d %d], want [0 3]\ndoc: %q\nrows: %q", i, from, to, doc, rows)
						}
					}
				})
			}
			if cases == 0 {
				t.Errorf("%s %s: no width wraps inside every line", kind, v.name)
			}
		}
	}
}
