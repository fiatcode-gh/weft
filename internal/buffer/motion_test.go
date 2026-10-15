package buffer

import "testing"

const famEmoji = "👨‍👩‍👧"

func mp(l, c int) Pos { return Pos{l, c} }

type motionRow struct {
	name string
	text string
	from Pos
	n    int
	big  bool
	want Pos
}

func runMotionRows(t *testing.T, rows []motionRow, f func(b *Buffer, r motionRow) Pos) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			b := New(r.text)
			before := b.String()
			cur := b.Cursor()
			if got := f(b, r); got != r.want {
				t.Fatalf("%q from %v n=%d big=%v = %v, want %v", r.text, r.from, r.n, r.big, got, r.want)
			}
			if b.String() != before || b.Cursor() != cur {
				t.Fatalf("motion mutated the buffer or moved the cursor")
			}
		})
	}
}

func TestLastLine(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0}, {"a", 0}, {"a\n", 0}, {"\n", 0}, {"a\nb", 1}, {"a\nb\n", 1},
		{"a\n\n", 1}, {"ab\r\ncd\r\n", 1}, {"a\n\nb", 2},
	}
	for _, c := range cases {
		if got := New(c.text).LastLine(); got != c.want {
			t.Errorf("LastLine(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

func TestLineLast(t *testing.T) {
	cases := []struct {
		name string
		text string
		from Pos
		want Pos
	}{
		{"empty buffer", "", mp(0, 0), mp(0, 0)},
		{"ascii", "abc", mp(0, 0), mp(0, 2)},
		{"composed", "é", mp(0, 0), mp(0, 0)},
		{"combining", "ae\u0301", mp(0, 0), mp(0, 1)},
		{"zwj emoji", "x" + famEmoji, mp(0, 0), mp(0, 1)},
		{"cjk", "日本", mp(0, 0), mp(0, 3)},
		{"crlf", "ab\r\ncd\r\n", mp(1, 0), mp(1, 1)},
		{"empty middle line", "a\n\nb", mp(1, 0), mp(1, 0)},
		{"from line end", "abc", mp(0, 3), mp(0, 2)},
		{"first line", "ab\ncd", mp(0, 1), mp(0, 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := New(c.text).LineLast(c.from); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestFirstNonBlank(t *testing.T) {
	cases := []struct {
		name string
		text string
		from Pos
		want Pos
	}{
		{"only blanks", "   ", mp(0, 0), mp(0, 2)},
		{"tab", "\tx", mp(0, 0), mp(0, 1)},
		{"empty", "", mp(0, 0), mp(0, 0)},
		{"mixed then multibyte", "  \té", mp(0, 0), mp(0, 3)},
		{"no indent", "abc", mp(0, 2), mp(0, 0)},
		{"from the middle", "  ab", mp(0, 3), mp(0, 2)},
		{"second line crlf", "a\r\n  b\r\n", mp(1, 0), mp(1, 2)},
		{"tab only", "\t", mp(0, 0), mp(0, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := New(c.text).FirstNonBlank(c.from); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

type stepRow struct {
	name    string
	text    string
	from    Pos
	n       int
	want    Pos
	wantGot int
}

func TestStepRight(t *testing.T) {
	rows := []stepRow{
		{"two", "abc", mp(0, 0), 2, mp(0, 2), 2},
		{"past the end", "abc", mp(0, 0), 5, mp(0, 3), 3},
		{"at the end", "abc", mp(0, 3), 1, mp(0, 3), 0},
		{"zero counts as one", "abc", mp(0, 0), 0, mp(0, 1), 1},
		{"combining", "e\u0301x", mp(0, 0), 1, mp(0, 3), 1},
		{"combining then x", "e\u0301x", mp(0, 0), 2, mp(0, 4), 2},
		{"zwj emoji", "a" + famEmoji + "b", mp(0, 1), 1, mp(0, 1+len(famEmoji)), 1},
		{"cjk past end", "日本", mp(0, 0), 3, mp(0, 6), 2},
		{"never leaves the line", "ab\ncd", mp(0, 1), 3, mp(0, 2), 1},
		{"crlf", "ab\r\ncd\r\n", mp(1, 0), 9, mp(1, 2), 2},
		{"empty buffer", "", mp(0, 0), 3, mp(0, 0), 0},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			q, got := New(r.text).StepRight(r.from, r.n)
			if q != r.want || got != r.wantGot {
				t.Fatalf("got %v,%d want %v,%d", q, got, r.want, r.wantGot)
			}
		})
	}
}

func TestStepLeft(t *testing.T) {
	rows := []stepRow{
		{"two", "abc", mp(0, 3), 2, mp(0, 1), 2},
		{"past the start", "abc", mp(0, 3), 5, mp(0, 0), 3},
		{"at the start", "abc", mp(0, 0), 1, mp(0, 0), 0},
		{"zero counts as one", "abc", mp(0, 3), 0, mp(0, 2), 1},
		{"combining", "e\u0301x", mp(0, 4), 2, mp(0, 0), 2},
		{"zwj emoji", "a" + famEmoji + "b", mp(0, 1+len(famEmoji)), 1, mp(0, 1), 1},
		{"cjk", "日本", mp(0, 6), 1, mp(0, 3), 1},
		{"never leaves the line", "ab\ncd", mp(1, 1), 3, mp(1, 0), 1},
		{"crlf", "ab\r\ncd\r\n", mp(1, 2), 1, mp(1, 1), 1},
		{"first line start", "ab\ncd", mp(0, 0), 2, mp(0, 0), 0},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			q, got := New(r.text).StepLeft(r.from, r.n)
			if q != r.want || got != r.wantGot {
				t.Fatalf("got %v,%d want %v,%d", q, got, r.want, r.wantGot)
			}
		})
	}
}

func TestWordStart(t *testing.T) {
	rows := []motionRow{
		{"w stops at punctuation", "foo.bar baz", mp(0, 0), 1, false, mp(0, 3)},
		{"w stops at bar", "foo.bar baz", mp(0, 0), 2, false, mp(0, 4)},
		{"w stops at baz", "foo.bar baz", mp(0, 0), 3, false, mp(0, 8)},
		{"w past the end", "foo.bar baz", mp(0, 0), 4, false, mp(0, 11)},
		{"count far past the end", "a b", mp(0, 0), 9, false, mp(0, 3)},
		{"W skips punctuation", "foo.bar baz", mp(0, 0), 1, true, mp(0, 8)},
		{"W past the end", "foo.bar baz", mp(0, 0), 2, true, mp(0, 11)},
		{"underscore and digit are keyword", "foo_bar1 x", mp(0, 0), 1, false, mp(0, 9)},
		{"leading spaces", "  foo", mp(0, 0), 1, false, mp(0, 2)},
		{"tab", "a\tb", mp(0, 0), 1, false, mp(0, 2)},
		{"from a blank", "a  b", mp(0, 1), 1, false, mp(0, 3)},
		{"mid word", "foo.bar", mp(0, 1), 1, false, mp(0, 3)},
		{"punctuation run", "a  ..b", mp(0, 0), 1, false, mp(0, 3)},
		{"punctuation run then word", "a  ..b", mp(0, 0), 2, false, mp(0, 5)},
		{"next line", "ab\ncd", mp(0, 0), 1, false, mp(1, 0)},
		{"empty line is a stop", "ab\n\ncd", mp(0, 0), 1, false, mp(1, 0)},
		{"past the empty line", "ab\n\ncd", mp(0, 0), 2, false, mp(2, 0)},
		{"from an empty line", "ab\n\ncd", mp(1, 0), 1, false, mp(2, 0)},
		{"last word", "ab cd", mp(0, 3), 1, false, mp(0, 5)},
		{"trailing newline never reached", "a b\n", mp(0, 2), 1, false, mp(0, 3)},
		{"empty buffer", "", mp(0, 0), 1, false, mp(0, 0)},
		{"crlf next line", "ab\r\ncd\r\n", mp(0, 0), 1, false, mp(1, 0)},
		{"crlf end", "ab\r\ncd\r\n", mp(0, 0), 3, false, mp(1, 2)},
		{"cjk is keyword", "日本 x", mp(0, 0), 1, false, mp(0, 7)},
		{"combining grapheme stays in the word", "e\u0301x y", mp(0, 0), 1, false, mp(0, 5)},
		{"emoji is punctuation", famEmoji + " a", mp(0, 0), 1, false, mp(0, len(famEmoji)+1)},
		{"zero counts as one", "a b", mp(0, 0), 0, false, mp(0, 2)},
		{"line clamp", "a b", mp(7, 0), 1, false, mp(0, 2)},
	}
	runMotionRows(t, rows, func(b *Buffer, r motionRow) Pos { return b.WordStart(r.from, r.n, r.big) })
}

func TestWordEnd(t *testing.T) {
	rows := []motionRow{
		{"e from the start", "foo.bar baz", mp(0, 0), 1, false, mp(0, 2)},
		{"e punctuation", "foo.bar baz", mp(0, 0), 2, false, mp(0, 3)},
		{"e bar", "foo.bar baz", mp(0, 0), 3, false, mp(0, 6)},
		{"e baz", "foo.bar baz", mp(0, 0), 4, false, mp(0, 10)},
		{"e past the end", "foo.bar baz", mp(0, 0), 5, false, mp(0, 10)},
		{"E skips punctuation", "foo.bar baz", mp(0, 0), 1, true, mp(0, 6)},
		{"E second", "foo.bar baz", mp(0, 0), 2, true, mp(0, 10)},
		{"from the end of a word", "foo bar", mp(0, 2), 1, false, mp(0, 6)},
		{"from inside a word", "foo bar", mp(0, 1), 1, false, mp(0, 2)},
		{"underscore digit", "foo_bar1", mp(0, 0), 1, false, mp(0, 7)},
		{"leading spaces", "  foo", mp(0, 0), 1, false, mp(0, 4)},
		{"tab", "a\tbc", mp(0, 0), 1, false, mp(0, 3)},
		{"across an empty line", "ab\n\ncd", mp(0, 1), 1, false, mp(2, 1)},
		{"nothing follows", "ab", mp(0, 1), 1, false, mp(0, 1)},
		{"nothing follows trailing newline", "a\n", mp(0, 0), 1, false, mp(0, 0)},
		{"empty buffer", "", mp(0, 0), 1, false, mp(0, 0)},
		{"count past the end", "a b", mp(0, 0), 9, false, mp(0, 2)},
		{"crlf", "ab\r\ncd\r\n", mp(0, 0), 1, false, mp(0, 1)},
		{"crlf next line", "ab\r\ncd\r\n", mp(0, 0), 2, false, mp(1, 1)},
		{"combining then x", "e\u0301x y", mp(0, 0), 1, false, mp(0, 3)},
		{"cjk", "日本 x", mp(0, 0), 1, false, mp(0, 3)},
		{"zero counts as one", "ab cd", mp(0, 0), 0, false, mp(0, 1)},
	}
	runMotionRows(t, rows, func(b *Buffer, r motionRow) Pos { return b.WordEnd(r.from, r.n, r.big) })
}

func TestWordBack(t *testing.T) {
	rows := []motionRow{
		{"b from the end", "foo.bar baz", mp(0, 11), 1, false, mp(0, 8)},
		{"b bar", "foo.bar baz", mp(0, 11), 2, false, mp(0, 4)},
		{"b punctuation", "foo.bar baz", mp(0, 11), 3, false, mp(0, 3)},
		{"b foo", "foo.bar baz", mp(0, 11), 4, false, mp(0, 0)},
		{"b past the start", "foo.bar baz", mp(0, 11), 5, false, mp(0, 0)},
		{"B skips punctuation", "foo.bar baz", mp(0, 11), 1, true, mp(0, 8)},
		{"B second", "foo.bar baz", mp(0, 11), 2, true, mp(0, 0)},
		{"mid word", "foo.bar baz", mp(0, 6), 1, false, mp(0, 4)},
		{"from a word start", "foo.bar baz", mp(0, 4), 1, false, mp(0, 3)},
		{"across an empty line", "ab\n\ncd", mp(2, 0), 1, false, mp(1, 0)},
		{"past the empty line", "ab\n\ncd", mp(2, 0), 2, false, mp(0, 0)},
		{"leading spaces", "  foo", mp(0, 2), 1, false, mp(0, 0)},
		{"at the buffer start", "ab", mp(0, 0), 1, false, mp(0, 0)},
		{"previous line", "ab\ncd", mp(1, 0), 1, false, mp(0, 0)},
		{"previous line word", "x ab\ncd", mp(1, 0), 1, false, mp(0, 2)},
		{"crlf", "ab\r\ncd", mp(1, 2), 1, false, mp(1, 0)},
		{"crlf across", "ab\r\ncd", mp(1, 0), 1, false, mp(0, 0)},
		{"combining", "x e\u0301y", mp(0, 6), 1, false, mp(0, 2)},
		{"cjk", "日本 語", mp(0, 10), 1, false, mp(0, 7)},
		{"cjk again", "日本 語", mp(0, 10), 2, false, mp(0, 0)},
		{"tab", "a\tb", mp(0, 2), 1, false, mp(0, 0)},
		{"tab from the end", "a\tb", mp(0, 3), 1, false, mp(0, 2)},
		{"zero counts as one", "ab cd", mp(0, 5), 0, false, mp(0, 3)},
	}
	runMotionRows(t, rows, func(b *Buffer, r motionRow) Pos { return b.WordBack(r.from, r.n, r.big) })
}

func TestFindInLine(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		from   Pos
		n      int
		g      string
		kind   byte
		repeat bool
		want   Pos
		ok     bool
	}{
		{"f first", "a,b,c,d", mp(0, 0), 1, ",", 'f', false, mp(0, 1), true},
		{"f second", "a,b,c,d", mp(0, 0), 2, ",", 'f', false, mp(0, 3), true},
		{"f third", "a,b,c,d", mp(0, 0), 3, ",", 'f', false, mp(0, 5), true},
		{"f count too large", "a,b,c,d", mp(0, 0), 4, ",", 'f', false, mp(0, 0), false},
		{"f skips the grapheme under the cursor", "aa", mp(0, 0), 1, "a", 'f', false, mp(0, 1), true},
		{"t adjacent without repeat stays", "a,b,c,d", mp(0, 0), 1, ",", 't', false, mp(0, 0), true},
		{"t adjacent with repeat skips it", "a,b,c,d", mp(0, 0), 1, ",", 't', true, mp(0, 2), true},
		{"t one before", "a,b,c,d", mp(0, 2), 1, ",", 't', false, mp(0, 2), true},
		{"t repeat next", "a,b,c,d", mp(0, 2), 1, ",", 't', true, mp(0, 4), true},
		{"t count 2", "a,b,c,d", mp(0, 0), 2, ",", 't', false, mp(0, 2), true},
		{"t far", "abc,d", mp(0, 0), 1, ",", 't', false, mp(0, 2), true},
		{"F first", "a,b,c,d", mp(0, 6), 1, ",", 'F', false, mp(0, 5), true},
		{"F second", "a,b,c,d", mp(0, 6), 2, ",", 'F', false, mp(0, 3), true},
		{"F third", "a,b,c,d", mp(0, 6), 3, ",", 'F', false, mp(0, 1), true},
		{"F count too large", "a,b,c,d", mp(0, 6), 4, ",", 'F', false, mp(0, 6), false},
		{"T adjacent without repeat stays", "a,b,c,d", mp(0, 6), 1, ",", 'T', false, mp(0, 6), true},
		{"T adjacent with repeat skips it", "a,b,c,d", mp(0, 6), 1, ",", 'T', true, mp(0, 4), true},
		{"T count 2", "a,b,c,d", mp(0, 6), 2, ",", 'T', false, mp(0, 4), true},
		{"T far", "a,bcd", mp(0, 4), 1, ",", 'T', false, mp(0, 2), true},
		{"not found", "abc", mp(0, 0), 1, "z", 'f', false, mp(0, 0), false},
		{"not found backwards", "abc", mp(0, 2), 1, "z", 'F', false, mp(0, 2), false},
		{"never crosses the line", "a\nb", mp(0, 0), 1, "b", 'f', false, mp(0, 0), false},
		{"empty line", "", mp(0, 0), 1, "a", 'f', false, mp(0, 0), false},
		{"empty target", "abc", mp(0, 0), 1, "", 'f', false, mp(0, 0), false},
		{"composed grapheme", "xéy", mp(0, 0), 1, "é", 'f', false, mp(0, 1), true},
		{"plain e is not e plus accent", "e\u0301e", mp(0, 0), 1, "e", 'f', false, mp(0, 3), true},
		{"accented target backwards", "e\u0301xy", mp(0, 4), 1, "e\u0301", 'F', false, mp(0, 0), true},
		{"cjk", "a日b日", mp(0, 0), 2, "日", 'f', false, mp(0, 5), true},
		{"t before a wide grapheme", "a日,b", mp(0, 0), 1, ",", 't', false, mp(0, 1), true},
		{"T after a wide grapheme", "a,日b", mp(0, 5), 1, ",", 'T', false, mp(0, 2), true},
		{"crlf", "a,b\r\nc,d", mp(1, 0), 1, ",", 'f', false, mp(1, 1), true},
		{"zero counts as one", "a,b", mp(0, 0), 0, ",", 'f', false, mp(0, 1), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := New(c.text)
			got, ok := b.FindInLine(c.from, c.n, c.g, c.kind, c.repeat)
			if got != c.want || ok != c.ok {
				t.Fatalf("got %v,%v want %v,%v", got, ok, c.want, c.ok)
			}
		})
	}
}

func TestMatchBracket(t *testing.T) {
	cases := []struct {
		name string
		text string
		from Pos
		want Pos
		ok   bool
	}{
		{"on open", "(a)", mp(0, 0), mp(0, 2), true},
		{"on close", "(a)", mp(0, 2), mp(0, 0), true},
		{"before a bracket", "x (a)", mp(0, 0), mp(0, 4), true},
		{"nested outer", "((a)[b])", mp(0, 0), mp(0, 7), true},
		{"nested inner", "((a)[b])", mp(0, 1), mp(0, 3), true},
		{"nested other kind", "((a)[b])", mp(0, 4), mp(0, 6), true},
		{"nested outer close", "((a)[b])", mp(0, 7), mp(0, 0), true},
		{"across lines", "{\n  a\n}", mp(0, 0), mp(2, 0), true},
		{"across lines back", "{\n  a\n}", mp(2, 0), mp(0, 0), true},
		{"nested across lines", "(\n(\n)\n)", mp(0, 0), mp(3, 0), true},
		{"other kinds ignored", "([)]", mp(0, 0), mp(0, 2), true},
		{"unmatched open", "(a", mp(0, 0), mp(0, 0), false},
		{"unmatched close", "a)", mp(0, 0), mp(0, 0), false},
		{"no bracket", "abc", mp(0, 0), mp(0, 0), false},
		{"bracket only before the cursor", "(a) b", mp(0, 3), mp(0, 3), false},
		{"quotes are not special", `("(")`, mp(0, 0), mp(0, 0), false},
		{"crlf", "(a\r\n)", mp(0, 0), mp(1, 0), true},
		{"multibyte", "é(日)", mp(0, 0), mp(0, 6), true},
		{"empty buffer", "", mp(0, 0), mp(0, 0), false},
		{"curly on a later line start", "x\n{a}", mp(1, 0), mp(1, 2), true},
		{"first bracket wins", "a ) (b)", mp(0, 0), mp(0, 0), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := New(c.text).MatchBracket(c.from)
			if got != c.want || ok != c.ok {
				t.Fatalf("got %v,%v want %v,%v", got, ok, c.want, c.ok)
			}
		})
	}
}

func TestParagraphForward(t *testing.T) {
	const doc = "a\nb\n\nc\n\n\nd"
	rows := []motionRow{
		{"to the first blank line", doc, mp(0, 0), 1, false, mp(2, 0)},
		{"from the middle of a paragraph", doc, mp(1, 1), 1, false, mp(2, 0)},
		{"second paragraph", doc, mp(0, 0), 2, false, mp(4, 0)},
		{"past the last blank run", doc, mp(0, 0), 3, false, mp(6, 1)},
		{"from a blank line", doc, mp(2, 0), 1, false, mp(4, 0)},
		{"from the last line", doc, mp(6, 0), 1, false, mp(6, 1)},
		{"count far past", doc, mp(0, 0), 99, false, mp(6, 1)},
		{"blank line right after", "a\n\nb", mp(0, 0), 1, false, mp(1, 0)},
		{"whitespace-only line is blank", "a\n  \nb", mp(0, 0), 1, false, mp(1, 0)},
		{"leading blank lines", "\n\na\n\nb", mp(0, 0), 1, false, mp(3, 0)},
		{"trailing newline", "a\n", mp(0, 0), 1, false, mp(0, 1)},
		{"empty buffer", "", mp(0, 0), 1, false, mp(0, 0)},
		{"crlf", "ab\r\n\r\ncd\r\n", mp(0, 0), 1, false, mp(1, 0)},
		{"crlf to the end", "ab\r\n\r\ncd\r\n", mp(0, 0), 2, false, mp(2, 2)},
		{"multibyte last line", "日本\n\n語", mp(0, 0), 2, false, mp(2, 3)},
		{"zero counts as one", doc, mp(0, 0), 0, false, mp(2, 0)},
	}
	runMotionRows(t, rows, func(b *Buffer, r motionRow) Pos { return b.ParagraphForward(r.from, r.n) })
}

func TestParagraphBack(t *testing.T) {
	const doc = "a\nb\n\nc\n\n\nd"
	rows := []motionRow{
		{"to the nearest blank line", doc, mp(6, 0), 1, false, mp(5, 0)},
		{"second", doc, mp(6, 0), 2, false, mp(2, 0)},
		{"to the buffer start", doc, mp(6, 0), 3, false, mp(0, 0)},
		{"from the middle of a paragraph", doc, mp(1, 1), 1, false, mp(0, 0)},
		{"from a blank line", doc, mp(4, 0), 1, false, mp(2, 0)},
		{"at the buffer start", doc, mp(0, 0), 1, false, mp(0, 0)},
		{"count far past", doc, mp(6, 0), 99, false, mp(0, 0)},
		{"whitespace-only line is blank", "a\n \nb", mp(2, 0), 1, false, mp(1, 0)},
		{"blank line right before", "a\n\nb", mp(2, 0), 1, false, mp(1, 0)},
		{"trailing newline", "a\n\nb\n", mp(2, 1), 1, false, mp(1, 0)},
		{"empty buffer", "", mp(0, 0), 1, false, mp(0, 0)},
		{"crlf", "ab\r\n\r\ncd\r\n", mp(2, 2), 1, false, mp(1, 0)},
		{"crlf to the start", "ab\r\n\r\ncd\r\n", mp(2, 2), 2, false, mp(0, 0)},
		{"zero counts as one", doc, mp(6, 0), 0, false, mp(5, 0)},
	}
	runMotionRows(t, rows, func(b *Buffer, r motionRow) Pos { return b.ParagraphBack(r.from, r.n) })
}
