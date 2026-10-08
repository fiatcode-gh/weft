package render

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"charm.land/glamour/v2"
)

// previewDoc is a mutable PreviewLines: lines without their terminators, and
// whether each was ended by "\r\n". The last line has no terminator.
type previewDoc struct {
	lines []string
	crlf  []bool
}

func newPreviewDoc(text string) *previewDoc {
	d := &previewDoc{}
	parts := strings.Split(text, "\n")
	for i, p := range parts {
		crlf := false
		if i < len(parts)-1 && strings.HasSuffix(p, "\r") {
			p, crlf = strings.TrimSuffix(p, "\r"), true
		}
		d.lines = append(d.lines, p)
		d.crlf = append(d.crlf, crlf)
	}
	return d
}

func (d *previewDoc) Len() int          { return len(d.lines) }
func (d *previewDoc) Line(i int) string { return d.lines[i] }
func (d *previewDoc) Terminator(i int) string {
	switch {
	case i == len(d.lines)-1:
		return ""
	case d.crlf[i]:
		return "\r\n"
	}
	return "\n"
}

func (d *previewDoc) text() string {
	var b strings.Builder
	for i, l := range d.lines {
		b.WriteString(l)
		b.WriteString(d.Terminator(i))
	}
	return b.String()
}

func (d *previewDoc) insert(i int, line string, crlf bool) {
	d.lines = append(d.lines[:i], append([]string{line}, d.lines[i:]...)...)
	d.crlf = append(d.crlf[:i], append([]bool{crlf}, d.crlf[i:]...)...)
}

func (d *previewDoc) remove(i int) {
	d.lines = append(d.lines[:i], d.lines[i+1:]...)
	d.crlf = append(d.crlf[:i], d.crlf[i+1:]...)
}

// previewChunkRows concatenates the rows of every chunk in document order. ok
// is false when the document has no chunk at all.
func previewChunkRows(t *testing.T, d *previewDoc, sc *Scanner, p *Preview) (rows []string, ok bool) {
	t.Helper()
	for i := 0; i < d.Len(); {
		c, in := p.chunkAt(d, sc, i)
		if !in {
			i++
			continue
		}
		if c.from != i && ok {
			t.Fatalf("chunk %+v does not continue at line %d", c, i)
		}
		r := p.render(d, sc, c)
		if r.err != nil {
			t.Fatalf("chunk %+v: %v", c, r.err)
		}
		rows = append(rows, r.rows...)
		ok = true
		i = c.to
	}
	return rows, ok
}

// checkPreviewMatchesRender is rule 2: the chunks' rows after the head rows
// are the read view's rows for the same document.
func checkPreviewMatchesRender(t *testing.T, raw string, width int) bool {
	t.Helper()
	d := newPreviewDoc(raw)
	p := NewPreview(mustTheme(t), width)
	rows, ok := previewChunkRows(t, d, NewScanner(), p)
	if !ok {
		return false
	}
	res, err := Render(strings.TrimSpace(raw)+"\n", width)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(res.Styled, "\n")
	got := append(append([]string{}, want[:min(p.Head(), len(want))]...), rows...)
	if !reflect.DeepEqual(got, want) {
		k := 0
		for k < len(got) && k < len(want) && got[k] == want[k] {
			k++
		}
		t.Errorf("width %d: chunked rows differ from Render at row %d (got %d rows, want %d)\ndoc: %q\n got: %q\nwant: %q",
			width, k, len(got), len(want), clip(raw), clip(rowAt(got, k)), clip(rowAt(want, k)))
		return false
	}
	return true
}

// clip keeps a failure message readable: a styled row repeats its SGR per cell.
func clip(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

func rowAt(rows []string, k int) string {
	if k < len(rows) {
		return rows[k]
	}
	return "<missing>"
}

func mustTheme(t *testing.T) Theme {
	t.Helper()
	th, err := CurrentTheme()
	if err != nil {
		t.Fatal(err)
	}
	return th
}

const previewConstructPage = "\n\n  # Heading with [[Link]] and [[pi]]\n\n" +
	"Setext title\n============\n\nSub heading\n-----------\n\n" +
	"Paragraph with **bold**, a [markdown link](http://example.com/x) and [[Target|an alias]].\n" +
	"Second line of the paragraph [[pi]] [[a]] [[b]] [[c]] [[d]] [[e]] [[f]] [[g]] [[h]] [[i]] [[j]] [[k]] [[l]] [[m]].\n\n" +
	"- bullet one\n- bullet two with [[Wiki]]\n  - nested a\n    - deeper\n  - nested b\n- TODO open task\n- DONE finished\n- LATER [#A] prioritised\n" +
	"- a very long bullet that has to wrap because it is much longer than the narrow widths used by the differential test, wrap wrap wrap wrap\n\n" +
	"* star item\n* star two\n\n+ plus item\n\n" +
	"1. first\n2. second\n\n   paragraph in item\n3. third\n\n" +
	"- [ ] gfm task\n- [x] gfm done\n\n" +
	"> quote line\n> second quote line\nlazy quote continuation\n\n" +
	"| Name | Link |\n|------|------|\n| a | [[Table]] |\n| b | [x](http://y.z) |\n\n" +
	"---\n\n***\n\n" +
	"```go\npackage main\n\nfunc main() { x := 1 }\n```\n\n" +
	"~~~\ntilde fence\n\nwith blank\n~~~\n\n" +
	"````\n```\ninner fence\n```\n````\n\n" +
	"- item before logbook\n  :LOGBOOK:\n  CLOCK: [2026-01-01 Thu 10:00]\n  :END:\n\n" +
	":LOGBOOK:\nCLOCK: x\n:END:\n\n" +
	"{{query (todo)}}\n\n{{embed [[Embedded]]}}\n\n{{query\n(and [[x]])\n}}\n\n" +
	"See [ref] and [other][ref], also [r2].\n\n" +
	"[ref]: http://example.com/ref\n\n" +
	"[r2]: <http://a b> \"Title\"\n\n" +
	"Term\n: definition\n\n" +
	"<!--\ncomment\n\nstill comment\n-->\n\n<div>\nhtml block\n</div>\n\n<pre>\n\nverbatim\n</pre>\n\n" +
	"After html.\n\ntext with trailing two  \nhard break\n\n![img](x.png) <http://auto.link> https://bare.link/x `[[code]]` span\n\n" +
	"Last paragraph with [[Wrap Edge]] and [[pi]] and some filler words to cross the wrap edge   \n\n\n"

func fixtureDocs(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob("../../testdata/fixture-graph/*/*.md")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixture pages: %v", err)
	}
	out := make(map[string]string, len(paths))
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[p] = string(b)
	}
	return out
}

var previewPieces = []string{
	"# Heading [[Link]]", "## Sub", "Title\n===", "Sub\n---",
	"para one with [[Alpha]] and **bold**", "para line\ncontinued line",
	"lazy para\n- item after para", "- a",
	"- b with [[Beta|alias]] and a [link](http://x.y)",
	"- TODO task item", "- DONE done thing", "  - nested", "    - deeper",
	"- [ ] gfm task", "- [x] gfm done", "* star item", "+ plus item",
	"1. one", "2. two", "1. one again", "3) paren",
	"  continuation of item", "lazy continuation", "> quote line",
	"> quote two\nlazy quote", "```go\nx := 1\n```", "````\n```\ninner\n```\n````",
	"~~~\ncode\n~~~", "- ```\n  code in item\n  ```",
	"| a | b |\n|---|---|\n| 1 | [[T]] |", "---", "***",
	"  :LOGBOOK:\n  CLOCK: x\n  :END:", ":LOGBOOK:\n:END:", "{{query (todo)}}",
	"{{embed [[X]]}}", "{{query\n(and)\n}}", "[ref]: http://example.com/ref",
	"see [ref] and [other][ref]", "[r2]: <http://a b> \"T\"", "use [r2]",
	"Term\n: definition", ": stray colon", "<!--\ncomment\n\nstill\n-->",
	"<div>\nhtml\n</div>", "<pre>\n\nx\n</pre>", "    indented code", "\tTabbed",
	"word " + strings.Repeat("long ", 30), "- " + strings.Repeat("wrap me ", 25),
	"[[A]] [[B]] [[C]] [[D]] [[E]] [[F]] [[G]] [[H]] [[I]] [[J]] [[K]] pi",
	"- LATER [#A] prio", "text with trailing two  \nhard break",
	"- item\n\n  para in item", "1. a\n\n   para", "\\- escaped", "`[[code]]` span",
	"![img](x.png)", "<http://auto.link>", "https://bare.link/x",
	"-", "- ", "foo\n-", "foo\n+ ", "+ ", "\u00a0", "\u00a0- x", "a\rb", "para\r- x",
	"bar\n--", "x\n\n=", "\f", " \t ",
}

// generatedDocs returns n seeded documents: 2-15 pieces joined by newlines, a
// blank line after a piece with probability 1/3, every 4th as CRLF, every 5th
// padded with whitespace.
func generatedDocs(n int) []string {
	rng := rand.New(rand.NewPCG(7, 9))
	docs := make([]string, n)
	for i := range docs {
		var b strings.Builder
		for k, pieces := 0, 2+rng.IntN(14); k < pieces; k++ {
			b.WriteString(previewPieces[rng.IntN(len(previewPieces))])
			b.WriteString("\n")
			if rng.IntN(3) == 0 {
				b.WriteString("\n")
			}
		}
		doc := strings.TrimSuffix(b.String(), "\n")
		if i%4 == 3 {
			doc = strings.ReplaceAll(doc, "\n", "\r\n")
		}
		if i%5 == 4 {
			doc = "\n  " + doc + "  \n\n"
		}
		docs[i] = doc
	}
	return docs
}

var previewWidths = []int{100, 40, 23}

func runPreviewDifferential(t *testing.T) {
	t.Helper()
	compared := 0
	for _, w := range previewWidths {
		for _, doc := range []string{previewConstructPage, strings.ReplaceAll(previewConstructPage, "\n", "\r\n")} {
			if checkPreviewMatchesRender(t, doc, w) {
				compared++
			}
		}
	}
	for name, doc := range fixtureDocs(t) {
		for _, w := range []int{100, 57} {
			if !checkPreviewMatchesRender(t, doc, w) {
				t.Logf("fixture %s", name)
			}
			compared++
		}
	}
	// Every sampleStride-th generated document: the fixed-seed corpus stays
	// the same, every case kind recurs, and only the count shrinks.
	generated := generatedDocs(2000)
	for i := 0; i < len(generated); i += sampleStride {
		if checkPreviewMatchesRender(t, generated[i], previewWidths[i%len(previewWidths)]) {
			compared++
		}
		if t.Failed() {
			t.Fatalf("stopping at generated document %d", i)
		}
	}
	t.Logf("compared %d document renders", compared)
}

func TestPreviewChunksMatchRenderTerminal(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	runPreviewDifferential(t)
}

func TestPreviewChunksMatchRenderDark(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": "dark"}) {
		return
	}
	runPreviewDifferential(t)
}

func TestPreviewChunksMatchRenderNoColor(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "1", "WEFT_STYLE": ""}) {
		return
	}
	runPreviewDifferential(t)
}

func TestPreviewHeadIsOneRow(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"terminal": {"NO_COLOR": "", "WEFT_STYLE": ""},
		"dark":     {"NO_COLOR": "", "WEFT_STYLE": "dark"},
		"nocolor":  {"NO_COLOR": "1", "WEFT_STYLE": ""},
	} {
		t.Run(name, func(t *testing.T) {
			if !inFreshProcess(t, env) {
				return
			}
			if got := NewPreview(mustTheme(t), 80).Head(); got != 1 {
				t.Errorf("Head() = %d, want 1", got)
			}
		})
	}
}

func TestChunkStartRules(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		starts  []int
		bullets []byte // per start; nil = all zero
	}{
		{"bullets after each other", "- a\n- b", []int{0, 1}, []byte{'-', '-'}},
		{"heading after blank", "para\n\n# H", []int{0, 2}, nil},
		{"bullet after blank keeps its marker", "para\n\n* a\n* b", []int{0, 2, 3}, []byte{0, '*', '*'}},
		{"leading blank lines", "\n\n  first\n\nsecond", []int{2, 4}, nil},
		{"empty document", "", nil, nil},
		{"whitespace-only document", "  \n\t\n \u00a0", nil, nil},
		{"no blank line", "a\nb", []int{0}, nil},
		{"indented", "a\n\n  b", []int{0}, nil},
		{"setext underline", "foo\n-", []int{0}, nil},
		{"empty bullet item", "a\n+ ", []int{0}, nil},
		{"task item", "- a\n- [ ] t\n- c", []int{0, 2}, []byte{'-', '-'}},
		{"ordered continues", "1. a\n\n2. b", []int{0}, nil},
		{"ordered after empty item", "-\n\n1. b", []int{0}, []byte{'-'}},
		{"paragraph after an ordered list", "1. a\n\nb", []int{0, 2}, nil},
		{"html adjacency", "<div>\nx\n</div>\n\npara", []int{0}, nil},
		{"html comment block", "<!--\nc\n\nstill\n-->\n\npara", []int{0}, nil},
		{"html type 1 block", "<pre>\n\nx\n</pre>\n\nafter", []int{0}, nil},
		{"definition list continuation", "Term\n: def\n\nnext", []int{0}, nil},
		{"colon line", "a\n\n: x", []int{0}, nil},
		{"reference definition", "a\n\n[r]: http://x\n\nb", []int{0, 4}, nil},
		{"logbook opener", "a\n\n:LOGBOOK:\n:END:\n\nb", []int{0, 5}, nil},
		{"query opener", "a\n\n{{query (x)}}\n\nb", []int{0, 4}, nil},
		{"lone carriage return", "a\rb\n\nc", []int{0}, nil},
		{"commonmark fence", "```\na\n\nb\n```\n\nc", []int{0, 6}, nil},
		{"read-view fence tracker only", "- ```\nx\n\ny\n- ```\n\nz", []int{0, 6}, []byte{'-', 0}},
		{"commonmark fence opened by the closer", "- ```\nx\n\ny\n```\n\nz", []int{0}, []byte{'-'}},
		{"after a rule", "a\n\n***\n\nb", []int{0, 2}, nil},
		{"after a rule and a reference definition", "***\n\n[r]: http://x\n\nb", []int{0}, nil},
		{"after a heading", "# H\n\nb\n\nc", []int{0, 4}, nil},
		{"after a setext heading", "T\n===\n\nb\n\nc", []int{0, 5}, nil},
		{"inside a logbook", ":LOGBOOK:\nx\n\ny\n:END:\n\nz", []int{0, 6}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newPreviewDoc(tc.doc)
			sc := NewScanner()
			var starts []int
			var bullets []byte
			for i := range d.Len() {
				if info := sc.Info(d, i); info.Start {
					starts = append(starts, i)
					bullets = append(bullets, info.Bullet)
				}
			}
			if !reflect.DeepEqual(starts, tc.starts) {
				t.Fatalf("starts = %v, want %v", starts, tc.starts)
			}
			want := tc.bullets
			if want == nil {
				want = make([]byte, len(tc.starts))
			}
			if len(starts) > 0 && !reflect.DeepEqual(bullets, want) {
				t.Errorf("bullets = %q, want %q", bullets, want)
			}
			p := NewPreview(mustTheme(t), 80)
			if tc.starts == nil {
				for i := range d.Len() {
					if c, ok := p.chunkAt(d, sc, i); ok {
						t.Errorf("chunkAt(%d) = %+v, want none", i, c)
					}
				}
			}
		})
	}
}

func TestPreviewSubstMatchesPreprocess(t *testing.T) {
	for n, raw := range generatedDocs(300) {
		d := newPreviewDoc(raw)
		sc := NewScanner()
		p := NewPreview(mustTheme(t), 80)
		first, last := -1, -1
		for i := range d.Len() {
			if strings.TrimSpace(d.Line(i)) != "" {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		for i := range d.Len() {
			c, ok := p.chunkAt(d, sc, i)
			if !ok || c.from != i {
				continue
			}
			var before strings.Builder
			for j := first; j < c.from; j++ {
				l := d.Line(j)
				if j == first {
					l = strings.TrimLeftFunc(l, unicode.IsSpace)
				}
				before.WriteString(l + d.Terminator(j))
			}
			f := preprocess(before.String(), "", idBase{})
			want := idBase{len(f.wikiSubs), len(f.taskMarkers)}
			if got := p.base(d, sc, c.from); got != want {
				t.Fatalf("doc %d (first %d last %d) chunk at %d: base = %+v, want %+v\ndoc: %q", n, first, last, c.from, got, want, raw)
			}
		}
	}
}

// previewEdit applies one seeded edit to d and returns the first line it may
// have changed.
func previewEdit(rng *rand.Rand, d *previewDoc) int {
	i := rng.IntN(d.Len())
	line := func() string {
		for {
			piece := previewPieces[rng.IntN(len(previewPieces))]
			if !strings.Contains(piece, "\n") {
				return piece
			}
		}
	}
	switch rng.IntN(7) {
	case 0:
		d.lines[i] = line()
	case 1:
		d.insert(i, line(), d.crlf[i])
	case 2:
		if d.Len() > 1 {
			d.remove(i)
			if i == d.Len() {
				d.crlf[i-1] = false
			}
		}
	case 3:
		d.lines[i] += " [[x]]"
	case 4:
		d.lines[i] = strings.ReplaceAll(strings.ReplaceAll(d.lines[i], "[[", ""), "]]", "")
	case 5:
		if d.lines[i] == "```" {
			d.remove(i)
		} else {
			d.insert(i, "```", d.crlf[i])
		}
	case 6:
		d.insert(i, "[ref]: http://example.com/ref", d.crlf[i])
	}
	return min(i, d.Len()-1)
}

func TestPreviewInvalidateMatchesFresh(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 13))
	docs := generatedDocs(50)
	for n := 0; n < len(docs); n += invalidateStride {
		d := newPreviewDoc(docs[n])
		sc, p := NewScanner(), NewPreview(mustTheme(t), 60)
		previewChunkRows(t, d, sc, p)
		for e := range 20 {
			from := previewEdit(rng, d)
			sc.Invalidate(from)
			p.Invalidate(from)
			got, gotOK := previewChunkRows(t, d, sc, p)
			want, wantOK := previewChunkRows(t, d, NewScanner(), NewPreview(mustTheme(t), 60))
			if gotOK != wantOK || !reflect.DeepEqual(got, want) {
				t.Fatalf("doc %d edit %d (from line %d): incremental rows differ from fresh\ndoc: %q", n, e, from, d.text())
			}
		}
	}
}

func TestPreviewRenderFailureIsNotCached(t *testing.T) {
	d := newPreviewDoc("# Title\n\npara")
	sc, p := NewScanner(), NewPreview(mustTheme(t), 80)
	c, ok := p.chunkAt(d, sc, 0)
	if !ok {
		t.Fatal("no chunk at line 0")
	}

	calls := 0
	real := glamourRender
	glamourRender = func(r *glamour.TermRenderer, in string) (string, error) {
		calls++
		return "", fmt.Errorf("glamour failed")
	}
	defer func() { glamourRender = real }()

	if r := p.render(d, sc, c); r.err == nil || len(r.rows) != 0 {
		t.Fatalf("render under failure = %+v, want an error and no rows", r)
	}
	first := calls
	if r := p.render(d, sc, c); r.err == nil {
		t.Fatal("second render under failure succeeded from a cache")
	}
	if calls <= first {
		t.Errorf("second render did not call Glamour again (calls %d, then %d)", first, calls)
	}

	glamourRender = real
	if r := p.render(d, sc, c); r.err != nil || len(r.rows) == 0 {
		t.Fatalf("render after recovery = %+v", r)
	}
	before := p.Stats()
	p.render(d, sc, c)
	if p.Stats() != before {
		t.Errorf("a successful render was not cached: %+v then %+v", before, p.Stats())
	}
}
