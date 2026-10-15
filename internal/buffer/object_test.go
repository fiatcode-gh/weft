package buffer

import "testing"

func rg(l1, c1, l2, c2 int) Range { return Range{Pos{l1, c1}, Pos{l2, c2}} }

type objRow struct {
	name   string
	text   string
	p      Pos
	n      int
	around bool
	big    bool
	quote  byte
	open   byte
	want   Range
	ok     bool
}

func runObjRows(t *testing.T, rows []objRow, f func(b *Buffer, r objRow) (Range, bool)) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			b := New(r.text)
			before, cur := b.String(), b.Cursor()
			got, ok := f(b, r)
			if ok != r.ok || (ok && got != r.want) {
				t.Fatalf("%q at %v n=%d around=%v = %v, %v; want %v, %v", r.text, r.p, r.n, r.around, got, ok, r.want, r.ok)
			}
			if b.String() != before || b.Cursor() != cur {
				t.Fatalf("object mutated the buffer or moved the cursor")
			}
		})
	}
}

func TestWordObject(t *testing.T) {
	fam := len(famEmoji)
	rows := []objRow{
		{name: "iw mid", text: "foo bar baz", p: mp(0, 5), n: 1, want: rg(0, 4, 0, 7), ok: true},
		{name: "aw mid", text: "foo bar baz", p: mp(0, 5), n: 1, around: true, want: rg(0, 4, 0, 8), ok: true},
		{name: "iw start", text: "foo bar", p: mp(0, 0), n: 1, want: rg(0, 0, 0, 3), ok: true},
		{name: "aw start", text: "foo bar", p: mp(0, 0), n: 1, around: true, want: rg(0, 0, 0, 4), ok: true},
		{name: "iw end", text: "foo bar", p: mp(0, 5), n: 1, want: rg(0, 4, 0, 7), ok: true},
		{name: "aw end takes leading blanks", text: "foo  bar", p: mp(0, 6), n: 1, around: true, want: rg(0, 3, 0, 8), ok: true},
		{name: "aw sole word", text: "foo", p: mp(0, 1), n: 1, around: true, want: rg(0, 0, 0, 3), ok: true},
		{name: "iw on blanks", text: "foo   bar", p: mp(0, 4), n: 1, want: rg(0, 3, 0, 6), ok: true},
		{name: "aw on blanks takes next word", text: "foo   bar", p: mp(0, 4), n: 1, around: true, want: rg(0, 3, 0, 9), ok: true},
		{name: "aw on trailing blanks", text: "foo  ", p: mp(0, 4), n: 1, around: true, want: rg(0, 3, 0, 5), ok: true},
		{name: "iw punct", text: "foo.bar", p: mp(0, 3), n: 1, want: rg(0, 3, 0, 4), ok: true},
		{name: "iw word before punct", text: "foo.bar", p: mp(0, 1), n: 1, want: rg(0, 0, 0, 3), ok: true},
		{name: "aw punct no blanks", text: "foo.bar", p: mp(0, 3), n: 1, around: true, want: rg(0, 3, 0, 4), ok: true},
		{name: "iW over foo.bar", text: "foo.bar baz", p: mp(0, 1), n: 1, big: true, want: rg(0, 0, 0, 7), ok: true},
		{name: "aW over foo.bar", text: "foo.bar baz", p: mp(0, 5), n: 1, big: true, around: true, want: rg(0, 0, 0, 8), ok: true},
		{name: "iw count 2", text: "foo bar baz qux", p: mp(0, 0), n: 2, want: rg(0, 0, 0, 4), ok: true},
		{name: "iw count 3", text: "foo bar baz qux", p: mp(0, 0), n: 3, want: rg(0, 0, 0, 7), ok: true},
		{name: "aw count 2", text: "foo bar baz qux", p: mp(0, 0), n: 2, around: true, want: rg(0, 0, 0, 8), ok: true},
		{name: "aw count 3", text: "foo bar baz qux", p: mp(0, 0), n: 3, around: true, want: rg(0, 0, 0, 12), ok: true},
		{name: "aw count stops at line end", text: "foo bar", p: mp(0, 0), n: 2, around: true, want: rg(0, 0, 0, 7), ok: true},
		{name: "aw count at line end takes leading blank", text: "foo bar", p: mp(0, 4), n: 2, around: true, want: rg(0, 3, 0, 7), ok: true},
		{name: "iw count clipped", text: "foo bar", p: mp(0, 0), n: 9, want: rg(0, 0, 0, 7), ok: true},
		{name: "n below 1 counts as 1", text: "foo bar", p: mp(0, 0), n: 0, want: rg(0, 0, 0, 3), ok: true},
		{name: "empty line", text: "", p: mp(0, 0), n: 1, ok: false},
		{name: "empty middle line", text: "a\n\nb", p: mp(1, 0), n: 1, around: true, ok: false},
		{name: "second line", text: "x\nfoo bar", p: mp(1, 5), n: 1, want: rg(1, 4, 1, 7), ok: true},
		{name: "multibyte word", text: "café bar", p: mp(0, 2), n: 1, want: rg(0, 0, 0, 5), ok: true},
		{name: "multibyte aw", text: "café bar", p: mp(0, 2), n: 1, around: true, want: rg(0, 0, 0, 6), ok: true},
		{name: "combining mark word", text: "e\u0301x y", p: mp(0, 0), n: 1, want: rg(0, 0, 0, 4), ok: true},
		{name: "combining mark aw", text: "e\u0301x y", p: mp(0, 0), n: 1, around: true, want: rg(0, 0, 0, 5), ok: true},
		{name: "emoji run", text: famEmoji + "a b", p: mp(0, 0), n: 1, want: rg(0, 0, 0, fam), ok: true},
		{name: "word after emoji", text: famEmoji + "a b", p: mp(0, fam), n: 1, want: rg(0, fam, 0, fam+1), ok: true},
		{name: "crlf iw", text: "foo bar\r\nbaz", p: mp(0, 4), n: 1, want: rg(0, 4, 0, 7), ok: true},
		{name: "crlf aw", text: "foo bar\r\nbaz", p: mp(0, 4), n: 1, around: true, want: rg(0, 3, 0, 7), ok: true},
		{name: "crlf second line", text: "foo bar\r\nbaz", p: mp(1, 1), n: 1, want: rg(1, 0, 1, 3), ok: true},
	}
	runObjRows(t, rows, func(b *Buffer, r objRow) (Range, bool) {
		return b.WordObject(r.p, r.n, r.around, r.big)
	})
}

func TestQuoteObject(t *testing.T) {
	const say = `say "hi there" ok`
	rows := []objRow{
		{name: "i before pair", text: say, p: mp(0, 0), want: rg(0, 5, 0, 13), ok: true},
		{name: "a before pair", text: say, p: mp(0, 0), around: true, want: rg(0, 4, 0, 15), ok: true},
		{name: "i on opening", text: say, p: mp(0, 4), want: rg(0, 5, 0, 13), ok: true},
		{name: "i inside", text: say, p: mp(0, 8), want: rg(0, 5, 0, 13), ok: true},
		{name: "i on closing", text: say, p: mp(0, 13), want: rg(0, 5, 0, 13), ok: true},
		{name: "after the last pair", text: say, p: mp(0, 15), ok: false},
		{name: "a trailing blanks win", text: `x "a"  y`, p: mp(0, 3), around: true, want: rg(0, 2, 0, 7), ok: true},
		{name: "a leading blanks when none follow", text: `say "hi"`, p: mp(0, 5), around: true, want: rg(0, 3, 0, 8), ok: true},
		{name: "a no blanks", text: `"hi"`, p: mp(0, 1), around: true, want: rg(0, 0, 0, 4), ok: true},
		{name: "escaped quote", text: `"a\"b"`, p: mp(0, 0), want: rg(0, 1, 0, 5), ok: true},
		{name: "escaped quote a", text: `"a\"b"`, p: mp(0, 2), around: true, want: rg(0, 0, 0, 6), ok: true},
		{name: "escaped backslash does not escape", text: `"a\\" x`, p: mp(0, 1), want: rg(0, 1, 0, 4), ok: true},
		{name: "empty pair i", text: `""`, p: mp(0, 0), want: rg(0, 1, 0, 1), ok: true},
		{name: "empty pair a", text: `""`, p: mp(0, 1), around: true, want: rg(0, 0, 0, 2), ok: true},
		{name: "single quotes i", text: `it 'is' ok`, p: mp(0, 4), quote: '\'', want: rg(0, 4, 0, 6), ok: true},
		{name: "single quotes a", text: `it 'is' ok`, p: mp(0, 4), quote: '\'', around: true, want: rg(0, 3, 0, 8), ok: true},
		{name: "double quote ignores single", text: `it 'is' ok`, p: mp(0, 4), ok: false},
		{name: "between pairs takes the next", text: `"a" b "c"`, p: mp(0, 4), want: rg(0, 7, 0, 8), ok: true},
		{name: "pairs left to right", text: `"a" "b`, p: mp(0, 5), ok: false},
		{name: "no quotes", text: "abc", p: mp(0, 1), ok: false},
		{name: "line local", text: "\"x\"\nabc", p: mp(1, 1), ok: false},
		{name: "multibyte", text: `"café" x`, p: mp(0, 2), want: rg(0, 1, 0, 6), ok: true},
		{name: "multibyte a", text: `"café" x`, p: mp(0, 2), around: true, want: rg(0, 0, 0, 7+1), ok: true},
		{name: "crlf", text: "say \"hi\"\r\nnext", p: mp(0, 5), want: rg(0, 5, 0, 7), ok: true},
		{name: "crlf a", text: "say \"hi\"\r\nnext", p: mp(0, 5), around: true, want: rg(0, 3, 0, 8), ok: true},
	}
	runObjRows(t, rows, func(b *Buffer, r objRow) (Range, bool) {
		q := r.quote
		if q == 0 {
			q = '"'
		}
		return b.QuoteObject(r.p, q, r.around)
	})
}

func TestBracketObject(t *testing.T) {
	const call = "f(a, (b), c)"
	rows := []objRow{
		{name: "inside level 1 a", text: call, p: mp(0, 2), n: 1, open: '(', around: true, want: rg(0, 1, 0, 12), ok: true},
		{name: "inside level 1 i", text: call, p: mp(0, 2), n: 1, open: '(', want: rg(0, 2, 0, 11), ok: true},
		{name: "on opener", text: call, p: mp(0, 1), n: 1, open: '(', want: rg(0, 2, 0, 11), ok: true},
		{name: "on closer", text: call, p: mp(0, 11), n: 1, open: '(', want: rg(0, 2, 0, 11), ok: true},
		{name: "nested inner i", text: call, p: mp(0, 6), n: 1, open: '(', want: rg(0, 6, 0, 7), ok: true},
		{name: "nested inner a", text: call, p: mp(0, 6), n: 1, open: '(', around: true, want: rg(0, 5, 0, 8), ok: true},
		{name: "nested level 2 i", text: call, p: mp(0, 6), n: 2, open: '(', want: rg(0, 2, 0, 11), ok: true},
		{name: "nested level 2 a", text: call, p: mp(0, 6), n: 2, open: '(', around: true, want: rg(0, 1, 0, 12), ok: true},
		{name: "level 3 missing", text: call, p: mp(0, 6), n: 3, open: '(', ok: false},
		{name: "inner opener level 2", text: call, p: mp(0, 5), n: 2, open: '(', around: true, want: rg(0, 1, 0, 12), ok: true},
		{name: "inner closer level 1", text: call, p: mp(0, 7), n: 1, open: '(', around: true, want: rg(0, 5, 0, 8), ok: true},
		{name: "after inner pair is outer", text: call, p: mp(0, 8), n: 1, open: '(', want: rg(0, 2, 0, 11), ok: true},
		{name: "balanced pair skipped going back", text: "(a (b) c)", p: mp(0, 7), n: 1, open: '(', around: true, want: rg(0, 0, 0, 9), ok: true},
		{name: "double nested", text: "((x))", p: mp(0, 2), n: 2, open: '(', want: rg(0, 1, 0, 4), ok: true},
		{name: "square", text: "x[1,2]", p: mp(0, 3), n: 1, open: '[', around: true, want: rg(0, 1, 0, 6), ok: true},
		{name: "square i", text: "x[1,2]", p: mp(0, 3), n: 1, open: '[', want: rg(0, 2, 0, 5), ok: true},
		{name: "curly", text: "a{bc}", p: mp(0, 2), n: 1, open: '{', want: rg(0, 2, 0, 4), ok: true},
		{name: "other kinds ignored", text: "(a [b] c)", p: mp(0, 4), n: 1, open: '(', around: true, want: rg(0, 0, 0, 9), ok: true},
		{name: "square inside round", text: "(a [b] c)", p: mp(0, 4), n: 1, open: '[', want: rg(0, 4, 0, 5), ok: true},
		{name: "square not enclosing", text: "(a [b] c)", p: mp(0, 7), n: 1, open: '[', ok: false},
		{name: "empty pair", text: "()", p: mp(0, 0), n: 1, open: '(', want: rg(0, 1, 0, 1), ok: true},
		{name: "unmatched opener", text: "(a", p: mp(0, 1), n: 1, open: '(', ok: false},
		{name: "unmatched closer", text: "a)", p: mp(0, 0), n: 1, open: '(', ok: false},
		{name: "outside any pair after", text: "f(a) g", p: mp(0, 5), n: 1, open: '(', ok: false},
		{name: "outside any pair before", text: "f(a) g", p: mp(0, 0), n: 1, open: '(', ok: false},
		{name: "n below 1 counts as 1", text: call, p: mp(0, 6), n: 0, open: '(', want: rg(0, 6, 0, 7), ok: true},
		{name: "multiline a", text: "{\n  x\n}", p: mp(1, 2), n: 1, open: '{', around: true, want: rg(0, 0, 2, 1), ok: true},
		{name: "multiline i is the middle text", text: "{\n  x\n}", p: mp(1, 2), n: 1, open: '{', want: rg(1, 0, 1, 3), ok: true},
		{name: "multiline indented closer", text: "{\n  x\n  }", p: mp(1, 2), n: 1, open: '{', want: rg(1, 0, 1, 3), ok: true},
		{name: "multiline closer after text", text: "{\n x }", p: mp(1, 1), n: 1, open: '{', want: rg(1, 0, 1, 3), ok: true},
		{name: "multiline opener not last", text: "{ x\n}", p: mp(0, 2), n: 1, open: '{', want: rg(0, 1, 0, 3), ok: true},
		{name: "multiline empty is empty at opener+1", text: "{\n}", p: mp(0, 0), n: 1, open: '{', want: rg(0, 1, 0, 1), ok: true},
		{name: "multiline from the closer line", text: "(\n  a\n  b\n)", p: mp(3, 0), n: 1, open: '(', want: rg(1, 0, 2, 3), ok: true},
		{name: "multibyte", text: "(café)", p: mp(0, 3), n: 1, open: '(', want: rg(0, 1, 0, 6), ok: true},
		{name: "multibyte a", text: "(café)", p: mp(0, 3), n: 1, open: '(', around: true, want: rg(0, 0, 0, 7), ok: true},
		{name: "combining mark", text: "(e\u0301)", p: mp(0, 1), n: 1, open: '(', want: rg(0, 1, 0, 4), ok: true},
		{name: "crlf multiline i", text: "{\r\n  x\r\n}", p: mp(1, 2), n: 1, open: '{', want: rg(1, 0, 1, 3), ok: true},
		{name: "crlf multiline a", text: "{\r\n  x\r\n}", p: mp(1, 2), n: 1, open: '{', around: true, want: rg(0, 0, 2, 1), ok: true},
	}
	runObjRows(t, rows, func(b *Buffer, r objRow) (Range, bool) {
		return b.BracketObject(r.p, r.n, r.open, r.around)
	})
}

func TestParagraphObject(t *testing.T) {
	const doc = "a\nb\n\nc\nd\n\n\ne" // 0 a, 1 b, 2 "", 3 c, 4 d, 5 "", 6 "", 7 e
	rows := []struct {
		name        string
		text        string
		line, n     int
		around      bool
		first, last int
		ok          bool
	}{
		{"ip on text", doc, 0, 1, false, 0, 1, true},
		{"ip second text line", doc, 1, 1, false, 0, 1, true},
		{"ip on blank", doc, 2, 1, false, 2, 2, true},
		{"ip on blank run", doc, 5, 1, false, 5, 6, true},
		{"ip count 2", doc, 0, 2, false, 0, 2, true},
		{"ip count 3", doc, 0, 3, false, 0, 4, true},
		{"ip count from second run", doc, 3, 2, false, 3, 6, true},
		{"ip count clipped", doc, 0, 10, false, 0, 7, true},
		{"n below 1 counts as 1", doc, 0, 0, false, 0, 1, true},
		{"ip last line", doc, 7, 1, false, 7, 7, true},
		{"ap with following blank run", doc, 0, 1, true, 0, 2, true},
		{"ap second paragraph", doc, 3, 1, true, 3, 6, true},
		{"ap count 2", doc, 0, 2, true, 0, 6, true},
		{"ap count past the end", doc, 0, 3, true, 0, 7, true},
		{"ap without following run extends back", doc, 7, 1, true, 5, 7, true},
		{"ap on blank takes next text", doc, 2, 1, true, 2, 4, true},
		{"ap on blank run takes last text", doc, 5, 1, true, 5, 7, true},
		{"ap at buffer start no blank", "a\nb", 0, 1, true, 0, 1, true},
		{"ap blank only after", "a\nb\n\n", 0, 1, true, 0, 2, true},
		{"final empty line never included ip", "a\n\nb\n", 2, 1, false, 2, 2, true},
		{"final empty line clamps", "a\n\nb\n", 3, 1, false, 2, 2, true},
		{"final empty line never included ap", "a\n\nb\n", 2, 1, true, 1, 2, true},
		{"trailing blank line then final", "a\n\n", 0, 1, true, 0, 1, true},
		{"trailing blank ip", "a\n\n", 1, 1, false, 1, 1, true},
		{"whitespace line is blank", "a\n  \nb", 1, 1, false, 1, 1, true},
		{"whitespace line separates", "a\n \t\nb", 0, 1, true, 0, 1, true},
		{"empty buffer", "", 0, 1, false, 0, 0, true},
		{"crlf ip", "a\r\nb\r\n\r\nc", 0, 1, false, 0, 1, true},
		{"crlf ap", "a\r\nb\r\n\r\nc", 0, 1, true, 0, 2, true},
		{"multibyte", "é\n🙂\n\nx", 1, 1, false, 0, 1, true},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			b := New(r.text)
			before, cur := b.String(), b.Cursor()
			first, last, ok := b.ParagraphObject(Pos{r.line, 0}, r.n, r.around)
			if ok != r.ok || (ok && (first != r.first || last != r.last)) {
				t.Fatalf("%q line %d n=%d around=%v = %d, %d, %v; want %d, %d, %v", r.text, r.line, r.n, r.around, first, last, ok, r.first, r.last, r.ok)
			}
			if b.String() != before || b.Cursor() != cur {
				t.Fatalf("object mutated the buffer or moved the cursor")
			}
		})
	}
}
