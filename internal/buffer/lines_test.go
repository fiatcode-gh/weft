package buffer

import "testing"

func TestLineText(t *testing.T) {
	cases := []struct {
		name        string
		text        string
		first, last int
		want        string
	}{
		{"single", "a\nb\nc", 1, 1, "b\n"},
		{"multiple", "a\nb\nc", 0, 1, "a\nb\n"},
		{"reversed", "a\nb\nc", 2, 1, "b\nc\n"},
		{"clamped", "a\nb", 0, 9, "a\nb\n"},
		{"crlf", "a\r\nb\r\nc", 0, 1, "a\nb\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := New(c.text)
			if got := b.LineText(c.first, c.last); got != c.want {
				t.Fatalf("LineText = %q, want %q", got, c.want)
			}
			if b.String() != c.text {
				t.Fatalf("LineText mutated the buffer")
			}
		})
	}
}

type lineRow struct {
	name        string
	text        string
	cur         Pos
	first, last int
	above       bool
	want        string
	wantCur     Pos
}

func runLineRows(t *testing.T, rows []lineRow, f func(b *Buffer, r lineRow)) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			b := New(r.text)
			b.MoveTo(r.cur, false)
			f(b, r)
			if got := b.String(); got != r.want {
				t.Fatalf("String = %q, want %q", got, r.want)
			}
			if b.Cursor() != r.wantCur {
				t.Fatalf("Cursor = %v, want %v", b.Cursor(), r.wantCur)
			}
			if _, ok := b.Selection(); ok {
				t.Fatalf("selection left behind")
			}
			if !b.Undo() {
				t.Fatalf("no undo group")
			}
			if got := b.String(); got != r.text {
				t.Fatalf("Undo = %q, want %q", got, r.text)
			}
			if b.Undo() {
				t.Fatalf("more than one undo group")
			}
		})
	}
}

func TestDeleteLines(t *testing.T) {
	runLineRows(t, []lineRow{
		{name: "middle", text: "a\nb\nc\n", first: 1, last: 1, want: "a\nc\n", wantCur: mp(1, 0)},
		{name: "first", text: "a\nb\nc\n", first: 0, last: 0, want: "b\nc\n", wantCur: mp(0, 0)},
		{name: "last with final newline", text: "a\nb\nc\n", first: 2, last: 2, want: "a\nb\n", wantCur: mp(1, 0)},
		{name: "last without final newline", text: "a\nb\nc", first: 2, last: 2, want: "a\nb", wantCur: mp(1, 0)},
		{name: "every line with final newline", text: "a\nb\n", first: 0, last: 1, want: "", wantCur: mp(0, 0)},
		{name: "every line without final newline", text: "a\nb", first: 0, last: 1, want: "", wantCur: mp(0, 0)},
		{name: "several", text: "a\nb\nc\nd", first: 1, last: 2, want: "a\nd", wantCur: mp(1, 0)},
		{name: "reversed and clamped", text: "a\nb\nc", first: 9, last: 1, want: "a", wantCur: mp(0, 0)},
		{name: "crlf middle", text: "a\r\nb\r\nc\r\n", first: 1, last: 1, want: "a\r\nc\r\n", wantCur: mp(1, 0)},
		{name: "crlf last", text: "a\r\nb", first: 1, last: 1, want: "a", wantCur: mp(0, 0)},
		{name: "cursor on first non-blank", text: "x\n  y\n  z", first: 0, last: 0, want: "  y\n  z", wantCur: mp(0, 2)},
	}, func(b *Buffer, r lineRow) { b.DeleteLines(r.first, r.last) })
}

func TestChangeLines(t *testing.T) {
	runLineRows(t, []lineRow{
		{name: "plain", text: "a\nb\nc", first: 1, last: 1, want: "a\n\nc", wantCur: mp(1, 0)},
		{name: "indent", text: "  foo\nbar", first: 0, last: 0, want: "  \nbar", wantCur: mp(0, 2)},
		{name: "tabs", text: "\t\tfoo\nbar", first: 0, last: 0, want: "\t\t\nbar", wantCur: mp(0, 2)},
		{name: "several", text: "x\n  a\n  b\nz", first: 1, last: 2, want: "x\n  \nz", wantCur: mp(1, 2)},
		{name: "last line", text: "a\n  b", first: 1, last: 1, want: "a\n  ", wantCur: mp(1, 2)},
		{name: "indent from first line only", text: "a\n    b", first: 0, last: 1, want: "", wantCur: mp(0, 0)},
		{name: "crlf", text: "a\r\nb\r\nc", first: 0, last: 1, want: "\r\nc", wantCur: mp(0, 0)},
	}, func(b *Buffer, r lineRow) { b.ChangeLines(r.first, r.last) })
}

func TestOpenLine(t *testing.T) {
	runLineRows(t, []lineRow{
		{name: "plain below", text: "a\nb", cur: mp(0, 0), want: "a\n\nb", wantCur: mp(1, 0)},
		{name: "plain above", text: "a\nb", cur: mp(1, 0), above: true, want: "a\n\nb", wantCur: mp(1, 0)},
		{name: "indented below", text: "  a", want: "  a\n  ", wantCur: mp(1, 2)},
		{name: "indented above", text: "  a", above: true, want: "  \n  a", wantCur: mp(0, 2)},
		{name: "bullet below", text: "- x\ny", want: "- x\n- \ny", wantCur: mp(1, 2)},
		{name: "bullet above", text: "- x\ny", above: true, want: "- \n- x\ny", wantCur: mp(0, 2)},
		{name: "todo bullet below", text: "  - TODO x", want: "  - TODO x\n  - ", wantCur: mp(1, 4)},
		{name: "todo bullet above", text: "  - TODO x", above: true, want: "  - \n  - TODO x", wantCur: mp(0, 4)},
		{name: "star is no bullet", text: "* item", want: "* item\n", wantCur: mp(1, 0)},
		{name: "last line below", text: "a\nb", cur: mp(1, 0), want: "a\nb\n", wantCur: mp(2, 0)},
		{name: "tab indent below", text: "\tx", want: "\tx\n\t", wantCur: mp(1, 1)},
		{name: "crlf below", text: "a\r\nb", want: "a\r\n\r\nb", wantCur: mp(1, 0)},
		{name: "crlf below last", text: "a\r\nb", cur: mp(1, 0), want: "a\r\nb\r\n", wantCur: mp(2, 0)},
		{name: "crlf above", text: "a\r\nb", above: true, want: "\r\na\r\nb", wantCur: mp(0, 0)},
	}, func(b *Buffer, r lineRow) { b.OpenLine(r.above) })
}

func TestPut(t *testing.T) {
	type row struct {
		name     string
		text     string
		cur      Pos
		put      string
		linewise bool
		after    bool
		n        int
		want     string
		wantCur  Pos
	}
	rows := []row{
		{name: "linewise after middle", text: "a\nb\nc", cur: mp(1, 0), put: "X\n", linewise: true, after: true, n: 1, want: "a\nb\nX\nc", wantCur: mp(2, 0)},
		{name: "linewise before middle", text: "a\nb\nc", cur: mp(1, 0), put: "X\n", linewise: true, n: 1, want: "a\nX\nb\nc", wantCur: mp(1, 0)},
		{name: "linewise after last with final newline", text: "a\nb\n", cur: mp(1, 0), put: "X\n", linewise: true, after: true, n: 1, want: "a\nb\nX\n", wantCur: mp(2, 0)},
		{name: "linewise after last without final newline", text: "a\nb", cur: mp(1, 0), put: "X\n", linewise: true, after: true, n: 1, want: "a\nb\nX", wantCur: mp(2, 0)},
		{name: "linewise count 3", text: "a\nb", cur: mp(0, 0), put: "X\n", linewise: true, after: true, n: 3, want: "a\nX\nX\nX\nb", wantCur: mp(1, 0)},
		{name: "linewise count 3 after last", text: "a", cur: mp(0, 0), put: "X\n", linewise: true, after: true, n: 3, want: "a\nX\nX\nX", wantCur: mp(1, 0)},
		{name: "linewise no trailing newline", text: "a\nb", cur: mp(0, 0), put: "X", linewise: true, after: true, n: 1, want: "a\nX\nb", wantCur: mp(1, 0)},
		{name: "linewise cursor on first non-blank", text: "a\nb", cur: mp(0, 0), put: "  X\nY\n", linewise: true, after: true, n: 1, want: "a\n  X\nY\nb", wantCur: mp(1, 2)},
		{name: "linewise n below 1 is 1", text: "a", cur: mp(0, 0), put: "X\n", linewise: true, n: 0, want: "X\na", wantCur: mp(0, 0)},
		{name: "linewise crlf after", text: "a\r\nb\r\nc", cur: mp(1, 0), put: "X\n", linewise: true, after: true, n: 1, want: "a\r\nb\r\nX\r\nc", wantCur: mp(2, 0)},
		{name: "linewise crlf after last", text: "a\r\nb", cur: mp(1, 0), put: "X\n", linewise: true, after: true, n: 1, want: "a\r\nb\r\nX", wantCur: mp(2, 0)},
		{name: "charwise after", text: "abc", cur: mp(0, 0), put: "X", after: true, n: 1, want: "aXbc", wantCur: mp(0, 1)},
		{name: "charwise before", text: "abc", cur: mp(0, 1), put: "X", n: 1, want: "aXbc", wantCur: mp(0, 1)},
		{name: "charwise after on empty line", text: "\nb", cur: mp(0, 0), put: "X", after: true, n: 1, want: "X\nb", wantCur: mp(0, 0)},
		{name: "charwise after at line end", text: "abc", cur: mp(0, 2), put: "X", after: true, n: 1, want: "abcX", wantCur: mp(0, 3)},
		{name: "charwise multi-byte", text: "ab", cur: mp(0, 0), put: "xe\u0301", after: true, n: 1, want: "axe\u0301b", wantCur: mp(0, 2)},
		{name: "charwise multi-line", text: "ab", cur: mp(0, 0), put: "X\nY", after: true, n: 1, want: "aX\nYb", wantCur: mp(0, 1)},
		{name: "charwise multi-line before", text: "ab", cur: mp(0, 1), put: "X\nY", n: 1, want: "aX\nYb", wantCur: mp(0, 1)},
		{name: "charwise count 2", text: "ab", cur: mp(0, 0), put: "XY", after: true, n: 2, want: "aXYXYb", wantCur: mp(0, 4)},
		{name: "charwise crlf multi-line", text: "a\r\nb", cur: mp(0, 0), put: "X\nY", after: true, n: 1, want: "aX\r\nY\r\nb", wantCur: mp(0, 1)},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			b := New(r.text)
			b.MoveTo(r.cur, false)
			b.Put(r.put, r.linewise, r.after, r.n)
			if got := b.String(); got != r.want {
				t.Fatalf("String = %q, want %q", got, r.want)
			}
			if b.Cursor() != r.wantCur {
				t.Fatalf("Cursor = %v, want %v", b.Cursor(), r.wantCur)
			}
			if !b.Undo() || b.String() != r.text {
				t.Fatalf("Undo = %q, want %q", b.String(), r.text)
			}
			if b.Undo() {
				t.Fatalf("more than one undo group")
			}
		})
	}

	t.Run("empty text does nothing", func(t *testing.T) {
		b := New("abc")
		b.Put("", false, true, 1)
		b.Put("", true, true, 1)
		if b.String() != "abc" || b.Undo() {
			t.Fatalf("empty put changed the buffer or opened a group")
		}
	})
	t.Run("clears the selection", func(t *testing.T) {
		b := New("abc")
		b.MoveTo(mp(0, 0), false)
		b.MoveTo(mp(0, 2), true)
		b.Put("X", false, false, 1)
		if _, ok := b.Selection(); ok {
			t.Fatalf("selection left behind")
		}
	})
}
