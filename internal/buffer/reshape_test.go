package buffer

import "testing"

// reshape runs act on a buffer of content and checks the result, the cursor, and
// that one Undo restores content exactly (or that no group was opened).
func reshape(t *testing.T, content string, act func(b *Buffer), want string, wantCur Pos) {
	t.Helper()
	b := New(content)
	act(b)
	if got := b.String(); got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	if got := b.Cursor(); got != wantCur {
		t.Errorf("cursor = %v, want %v", got, wantCur)
	}
	if _, ok := b.Selection(); ok {
		t.Errorf("selection left set")
	}
	changed := want != content
	if undone := b.Undo(); undone != changed {
		t.Fatalf("Undo() = %v, want %v (a group only when the text changed)", undone, changed)
	}
	if got := b.String(); got != content {
		t.Errorf("after one Undo content = %q, want %q", got, content)
	}
	if changed && b.Undo() {
		t.Errorf("a second group was opened")
	}
}

func TestCaseRange(t *testing.T) {
	cases := []struct {
		name    string
		content string
		r       Range
		m       CaseMode
		want    string
		wantCur Pos
	}{
		{"upper single line", "hello world", Range{Pos{0, 2}, Pos{0, 8}}, CaseUpper, "heLLO WOrld", Pos{0, 2}},
		{"lower single line", "HELLO WORLD", Range{Pos{0, 0}, Pos{0, 5}}, CaseLower, "hello WORLD", Pos{0, 0}},
		{"multi line", "abc\ndef\nghi", Range{Pos{0, 1}, Pos{2, 2}}, CaseUpper, "aBC\nDEF\nGHi", Pos{0, 1}},
		{"toggle mixed", "aBc DeF", Range{Pos{0, 0}, Pos{0, 7}}, CaseToggle, "AbC dEf", Pos{0, 0}},
		{"toggle keeps non letters", "a1-B", Range{Pos{0, 0}, Pos{0, 4}}, CaseToggle, "A1-b", Pos{0, 0}},
		{"eszett stays valid", "straße", Range{Pos{0, 0}, Pos{0, 7}}, CaseUpper, "STRAßE", Pos{0, 0}},
		{"dotted capital I shrinks", "İx", Range{Pos{0, 0}, Pos{0, 3}}, CaseLower, "ix", Pos{0, 0}},
		{"greek upper", "αβγ", Range{Pos{0, 0}, Pos{0, 6}}, CaseUpper, "ΑΒΓ", Pos{0, 0}},
		{"greek lower", "ΑΒΓ", Range{Pos{0, 2}, Pos{0, 6}}, CaseLower, "Αβγ", Pos{0, 2}},
		{"invalid byte left alone", "ab\xffcd", Range{Pos{0, 0}, Pos{0, 5}}, CaseUpper, "ab\xffcd", Pos{0, 0}},
		{"invalid line skipped other changes", "a\xff\nbc", Range{Pos{0, 0}, Pos{1, 2}}, CaseUpper, "a\xff\nBC", Pos{0, 0}},
		{"nothing to change", "ABC def", Range{Pos{0, 0}, Pos{0, 3}}, CaseUpper, "ABC def", Pos{0, 0}},
		{"capitalize not accepted", "abc", Range{Pos{0, 0}, Pos{0, 3}}, CaseCapitalize, "abc", Pos{0, 0}},
		{"crlf", "ab\r\ncd\r\n", Range{Pos{0, 0}, Pos{1, 2}}, CaseUpper, "AB\r\nCD\r\n", Pos{0, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reshape(t, c.content, func(b *Buffer) {
				b.MoveTo(Pos{0, 1}, false)
				b.MoveTo(c.r.End, true) // a selection that CaseRange must clear
				b.CaseRange(c.r, c.m)
			}, c.want, c.wantCur)
		})
	}
}

func TestJoinLines(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		line, n    int
		want       string
		wantCur    Pos
		wantResult bool
	}{
		{"two lines", "foo\nbar", 0, 2, "foo bar", Pos{0, 3}, true},
		{"count 3", "a\nb\nc\nd", 0, 3, "a b c\nd", Pos{0, 3}, true},
		{"count below 2 is 2", "a\nb\nc", 0, 1, "a b\nc", Pos{0, 1}, true},
		{"count past the end", "a\nb\nc", 1, 9, "a\nb c", Pos{1, 1}, true},
		{"indented next", "foo\n   \tbar", 0, 2, "foo bar", Pos{0, 3}, true},
		{"empty current", "\nbar", 0, 2, "bar", Pos{0, 0}, true},
		{"empty next", "foo\n\nbar", 0, 2, "foo\nbar", Pos{0, 3}, true},
		{"blank next", "foo\n  \nbar", 0, 2, "foo\nbar", Pos{0, 3}, true},
		{"next starts with paren", "foo\n)bar", 0, 2, "foo)bar", Pos{0, 3}, true},
		{"current ends in space", "foo \nbar", 0, 2, "foo bar", Pos{0, 4}, true},
		{"current ends in tab", "foo\t\n  bar", 0, 2, "foo\tbar", Pos{0, 4}, true},
		{"on last line", "foo\nbar", 1, 2, "foo\nbar", Pos{0, 0}, false},
		{"final newline never joins the empty line", "foo\nbar\n", 1, 2, "foo\nbar\n", Pos{0, 0}, false},
		{"final newline joins real lines", "foo\nbar\n", 0, 5, "foo bar\n", Pos{0, 3}, true},
		{"crlf", "foo\r\nbar\r\nbaz", 0, 2, "foo bar\r\nbaz", Pos{0, 3}, true},
		{"multibyte cursor in bytes", "é\nb", 0, 2, "é b", Pos{0, 2}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ok bool
			reshape(t, c.content, func(b *Buffer) {
				ok = b.JoinLines(c.line, c.n)
			}, c.want, c.wantCur)
			if ok != c.wantResult {
				t.Errorf("JoinLines = %v, want %v", ok, c.wantResult)
			}
		})
	}
}

func TestShiftLines(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		first, last int
		dir         int
		cursor      Pos
		want        string
		wantCur     Pos
	}{
		{"indent plain", "a\nb\nc", 0, 1, +1, Pos{0, 1}, "  a\n  b\nc", Pos{0, 3}},
		{"outdent plain", "    a\n  b\nc", 0, 2, -1, Pos{0, 4}, "  a\nb\nc", Pos{0, 2}},
		{"bullets get no subtree", "- a\n  - b\n  - c", 0, 0, +1, Pos{0, 0}, "  - a\n  - b\n  - c", Pos{0, 2}},
		{"tab lines", "\ta\n\tb", 0, 1, +1, Pos{1, 1}, "\t\ta\n\t\tb", Pos{1, 2}},
		{"tab outdent", "\t\ta\nb", 0, 1, -1, Pos{0, 2}, "\ta\nb", Pos{0, 1}},
		{"blank lines unchanged", "a\n\n  \nb", 0, 3, +1, Pos{3, 1}, "  a\n\n  \n  b", Pos{3, 3}},
		{"nothing to outdent", "a\nb", 0, 1, -1, Pos{0, 1}, "a\nb", Pos{0, 1}},
		{"crlf", "a\r\nb\r\n", 0, 1, +1, Pos{0, 0}, "  a\r\n  b\r\n", Pos{0, 2}},
		{"range clamped", "a\nb", 0, 9, +1, Pos{0, 0}, "  a\n  b", Pos{0, 2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reshape(t, c.content, func(b *Buffer) {
				b.MoveTo(c.cursor, false)
				b.ShiftLines(c.first, c.last, c.dir)
			}, c.want, c.wantCur)
		})
	}
}
