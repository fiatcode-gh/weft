package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v3/internal/buffer"
)

var (
	ctrlD = tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
	ctrlU = tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
)

// clampModal is the cursor a normal-mode key leaves: on a line and a
// character of the buffer.
func clampModal(b *buffer.Buffer, p buffer.Pos) buffer.Pos {
	p.Line = min(p.Line, b.LastLine())
	if last := b.LineLast(p); p.Col > last.Col {
		p = last
	}
	return p
}

func TestVimMotions(t *testing.T) {
	const (
		punct  = "foo.bar baz\nqux\n"
		multi  = "héllo wörld\n"
		emoji  = "a😀b\n"
		commas = "a,b,c,d\n"
		brk    = "(a [b] c)\nx\n"
		paras  = "a\nb\n\nc\nd\n\ne\n"
		odd    = "a,b  (c)\n"
	)
	p := func(l, c int) buffer.Pos { return buffer.Pos{Line: l, Col: c} }
	for _, tc := range []struct {
		name    string
		content string
		start   buffer.Pos
		keys    string
		want    buffer.Pos
	}{
		// words with punctuation
		{"w stops on punctuation", punct, p(0, 0), "w", p(0, 3)},
		{"2w", punct, p(0, 0), "2w", p(0, 4)},
		{"3w", punct, p(0, 0), "3w", p(0, 8)},
		{"4w crosses the line", punct, p(0, 0), "4w", p(1, 0)},
		{"W skips punctuation", punct, p(0, 0), "W", p(0, 8)},
		{"e", punct, p(0, 0), "e", p(0, 2)},
		{"2e", punct, p(0, 0), "2e", p(0, 3)},
		{"3e", punct, p(0, 0), "3e", p(0, 6)},
		{"E", punct, p(0, 0), "E", p(0, 6)},
		{"b", punct, p(0, 8), "b", p(0, 4)},
		{"2b", punct, p(0, 8), "2b", p(0, 3)},
		{"B", punct, p(0, 8), "B", p(0, 0)},
		{"w over lone punctuation", odd, p(0, 0), "w", p(0, 1)},
		{"5w", odd, p(0, 0), "5w", p(0, 7)},
		{"e on a one-char word", odd, p(0, 0), "e", p(0, 1)},
		{"3b", odd, p(0, 7), "3b", p(0, 2)},
		{"W on punctuated words", odd, p(0, 0), "W", p(0, 5)},
		{"E", odd, p(0, 0), "E", p(0, 2)},
		{"w at the end of the buffer", "ab\n", p(0, 0), "w", p(0, 1)},
		{"w onto an empty line", "a\n\nb\n", p(0, 0), "w", p(1, 0)},
		{"w twice over an empty line", "a\n\nb\n", p(0, 0), "2w", p(2, 0)},
		// line motions
		{"0", punct, p(0, 5), "0", p(0, 0)},
		{"^", "  indented text\n", p(0, 10), "^", p(0, 2)},
		{"$", punct, p(0, 0), "$", p(0, 10)},
		{"2$ ends the next line", punct, p(0, 0), "2$", p(1, 2)},
		{"9$ stops at the last line", punct, p(0, 0), "9$", p(1, 2)},
		{"l", punct, p(0, 0), "l", p(0, 1)},
		{"3l", punct, p(0, 0), "3l", p(0, 3)},
		{"20l stops on the last grapheme", punct, p(0, 0), "20l", p(0, 10)},
		{"h", punct, p(0, 5), "h", p(0, 4)},
		{"3h", punct, p(0, 5), "3h", p(0, 2)},
		{"99h stops at the start", punct, p(0, 5), "99h", p(0, 0)},
		{"h at column 0", punct, p(0, 0), "h", p(0, 0)},
		// multi-byte graphemes
		{"w over multi-byte", multi, p(0, 0), "w", p(0, 7)},
		{"e over multi-byte", multi, p(0, 0), "e", p(0, 5)},
		{"l steps one grapheme", multi, p(0, 0), "l", p(0, 1)},
		{"2l", multi, p(0, 0), "2l", p(0, 3)},
		{"$ lands on the last grapheme", multi, p(0, 0), "$", p(0, 12)},
		{"b over multi-byte", multi, p(0, 12), "b", p(0, 7)},
		{"fö", multi, p(0, 0), "fö", p(0, 8)},
		{"tö", multi, p(0, 0), "tö", p(0, 7)},
		{"l over an emoji", emoji, p(0, 0), "l", p(0, 1)},
		{"2l over an emoji", emoji, p(0, 0), "2l", p(0, 5)},
		{"h over an emoji", emoji, p(0, 5), "h", p(0, 1)},
		// CRLF
		{"$ on CRLF", "one two\r\nthree\r\n", p(0, 0), "$", p(0, 6)},
		{"2$ on CRLF", "one two\r\nthree\r\n", p(0, 0), "2$", p(1, 4)},
		{"w across CRLF", "one two\r\nthree\r\n", p(0, 4), "w", p(1, 0)},
		{"l stops before the CRLF", "one two\r\nthree\r\n", p(0, 6), "l", p(0, 6)},
		{"j on CRLF", "one two\r\nthree\r\n", p(0, 2), "j", p(1, 2)},
		// first and last line, final empty line
		{"k on the first line", "a\nb\n", p(0, 0), "k", p(0, 0)},
		{"j on the last line", "a\nb\n", p(1, 0), "j", p(1, 0)},
		{"gg on the first line", "a\nb\n", p(0, 0), "gg", p(0, 0)},
		{"G never rests on the final empty line", "a\nb\n", p(0, 0), "G", p(1, 0)},
		{"999j", "a\nb\nc\n", p(0, 0), "999j", p(2, 0)},
		{"999999j saturates", "a\nb\nc\n", p(0, 0), "999999j", p(2, 0)},
		{"} with no blank line", "a\nb\n", p(0, 0), "}", p(1, 0)},
		{"$ past the end", "a\nb\n", p(0, 0), "5$", p(1, 0)},
		{"w on the last character", "a\nb\n", p(1, 0), "w", p(1, 0)},
		// f t ; ,
		{"f,", commas, p(0, 0), "f,", p(0, 1)},
		{"2f,", commas, p(0, 0), "2f,", p(0, 3)},
		{"3f,", commas, p(0, 0), "3f,", p(0, 5)},
		{"f with too few matches", commas, p(0, 0), "4f,", p(0, 0)},
		{"2t,", commas, p(0, 0), "2t,", p(0, 2)},
		{"f, then ;", commas, p(0, 0), "f,;", p(0, 3)},
		{"f, then ;;", commas, p(0, 0), "f,;;", p(0, 5)},
		{"t, then ; skips the adjacent match", commas, p(0, 0), "t,;", p(0, 2)},
		{"f, ; then , goes back", commas, p(0, 0), "f,;,", p(0, 1)},
		{"F,", commas, p(0, 6), "F,", p(0, 5)},
		{"2F,", commas, p(0, 6), "2F,", p(0, 3)},
		{"2T,", commas, p(0, 6), "2T,", p(0, 4)},
		{"T, then ; skips the adjacent match", commas, p(0, 6), "T,;", p(0, 4)},
		{"t, then , flips to T", commas, p(0, 4), "t,,", p(0, 2)},
		{"; without a find", commas, p(0, 2), ";", p(0, 2)},
		{", without a find", commas, p(0, 2), ",", p(0, 2)},
		{"3; with too few matches", commas, p(0, 0), "f,3;", p(0, 1)},
		{"2; repeats the find twice", commas, p(0, 0), "f,2;", p(0, 5)},
		// %
		{"% on an opener", brk, p(0, 0), "%", p(0, 8)},
		{"% on a closer", brk, p(0, 8), "%", p(0, 0)},
		{"% before a bracket", brk, p(0, 1), "%", p(0, 5)},
		{"% after the last bracket", brk, p(0, 6), "%", p(0, 0)},
		{"% unmatched", "(a\nx\n", p(0, 0), "%", p(0, 0)},
		{"% without a bracket", "abc\n", p(0, 0), "%", p(0, 0)},
		{"50%", "a\nb\nc\nd\n", p(0, 0), "50%", p(1, 0)},
		{"100%", "a\nb\nc\nd\n", p(0, 0), "100%", p(3, 0)},
		{"1%", "a\nb\nc\nd\n", p(3, 0), "1%", p(0, 0)},
		{"N% lands on the first non-blank", "a\n  b\nc\nd\n", p(0, 0), "50%", p(1, 2)},
		// gg G
		{"G", "a\nb\nc\nd\n", p(0, 0), "G", p(3, 0)},
		{"2G", "a\nb\nc\nd\n", p(0, 0), "2G", p(1, 0)},
		{"9G clamps", "a\nb\nc\nd\n", p(0, 0), "9G", p(3, 0)},
		{"gg", "a\nb\nc\nd\n", p(3, 0), "gg", p(0, 0)},
		{"3gg", "a\nb\nc\nd\n", p(0, 0), "3gg", p(2, 0)},
		{"G on the first non-blank", "a\n  b\n", p(0, 0), "G", p(1, 2)},
		{"gg on the first non-blank", "  a\nb\n", p(1, 0), "gg", p(0, 2)},
		// paragraphs
		{"}", paras, p(0, 0), "}", p(2, 0)},
		{"2}", paras, p(0, 0), "2}", p(5, 0)},
		{"3} with no more blank lines", paras, p(0, 0), "3}", p(6, 0)},
		{"} from a blank line", paras, p(2, 0), "}", p(5, 0)},
		{"{", paras, p(6, 0), "{", p(5, 0)},
		{"2{", paras, p(6, 0), "2{", p(2, 0)},
		{"3{ to the start", paras, p(6, 0), "3{", p(0, 0)},
		{"{ from mid paragraph", paras, p(3, 0), "{", p(2, 0)},
		// vertical goal
		{"$ then j keeps the line end", "abcdef\nab\nabcdefgh\n", p(0, 0), "$jj", p(2, 7)},
		{"j keeps the column goal", "abcdef\nab\nabcdefgh\n", p(0, 0), "lllllljj", p(2, 5)},
		{"3j", "abc\nd\nefg\nhij\n", p(0, 2), "3j", p(3, 2)},
		{"2k", "abc\nd\nefg\nhij\n", p(3, 2), "2k", p(1, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := vimEditor(nil, tc.content, 0)
			e.buf.MoveTo(tc.start, false)
			e.goalOK = false
			vimType(e, tc.keys)
			if got := e.buf.Cursor(); got != tc.want {
				t.Fatalf("keys %q from %v: cursor %v, want %v", tc.keys, tc.start, got, tc.want)
			}
			if m := vimModeOf(e); m != vimNormal {
				t.Fatalf("mode %v, want normal", m)
			}
			if got := e.buf.String(); got != tc.content {
				t.Fatalf("a motion changed the buffer: %q", got)
			}
		})
	}
}

func TestVimMotionBadContinuationDrops(t *testing.T) {
	e := vimEditor(nil, "abc def\n", 0)
	vimType(e, "2gx")
	if len(e.vim.pending) != 0 {
		t.Fatalf("pending %q after a bad continuation", e.vim.pending)
	}
	vimType(e, "w")
	if got := e.buf.Cursor(); got != (buffer.Pos{Col: 4}) {
		t.Fatalf("w after a dropped command: cursor %v", got)
	}
}

func TestVimJKMatchArrows(t *testing.T) {
	long := "- " + strings.Repeat("alpha beta gamma delta ", 8) + "end"
	content := long + "\nshort\n\n" + long + "\n  " + strings.Repeat("wrapped words go here ", 6) + "\nlast line\n"
	down := tea.KeyPressMsg{Code: tea.KeyDown}
	up := tea.KeyPressMsg{Code: tea.KeyUp}
	steps := []struct {
		keys string
		arr  tea.KeyPressMsg
		n    int
	}{
		{"j", down, 1}, {"3j", down, 3}, {"j", down, 1}, {"2k", up, 2}, {"10j", down, 10},
		{"k", up, 1}, {"4k", up, 4}, {"7j", down, 7}, {"99k", up, 99}, {"99j", down, 99}, {"3k", up, 3},
	}
	for _, mode := range perfModes {
		t.Run(mode.name, func(t *testing.T) {
			mk := func() *EditorView {
				return editorAtLook(nil, "P", "/tmp/p.md", content, false, 40, 12, 0, mode.source)
			}
			arrows, vim := mk(), mk()
			vim.enableVim()
			for _, s := range steps {
				for range s.n {
					arrows.Update(s.arr)
				}
				vimType(vim, s.keys)
				want := clampModal(arrows.buf, arrows.buf.Cursor())
				if got := vim.buf.Cursor(); got != want {
					t.Fatalf("after %q: vim cursor %v, arrows (clamped) %v", s.keys, got, want)
				}
				// Vim clamps after every key; give the arrows the same cursor.
				if arrows.buf.Cursor() != want {
					arrows.buf.MoveTo(want, false)
				}
			}
		})
	}
}

func TestVimHalfPage(t *testing.T) {
	var lines []string
	for i := range 120 {
		lines = append(lines, "- item "+strings.Repeat("x", i%7))
	}
	content := strings.Join(lines, "\n") + "\n"
	for _, mode := range perfModes {
		t.Run(mode.name, func(t *testing.T) {
			mk := func() *EditorView {
				e := NewEditorView(nil, "P", "/tmp/p.md", content, false, 60, 16, Anchor{Line: 40, ScreenRow: 4}, nil, mode.source)
				e.enableVim()
				_ = e.View()
				return e
			}
			e := mk()
			half := e.textHeight() / 2
			sr := e.cursorScreenRow()
			if sr != 4 || e.buf.Cursor().Line != 40 {
				t.Fatalf("setup: line %d screen row %d", e.buf.Cursor().Line, sr)
			}
			e.Update(ctrlD)
			if got := e.buf.Cursor().Line; got != 40+half {
				t.Fatalf("ctrl+d: line %d, want %d", got, 40+half)
			}
			if got := e.cursorScreenRow(); got != sr {
				t.Fatalf("ctrl+d: screen row %d, want %d", got, sr)
			}
			e.Update(ctrlU)
			if got := e.buf.Cursor().Line; got != 40 {
				t.Fatalf("ctrl+u: line %d, want 40", got)
			}
			if got := e.cursorScreenRow(); got != sr {
				t.Fatalf("ctrl+u: screen row %d, want %d", got, sr)
			}

			e = mk()
			vimType(e, "5")
			e.Update(ctrlD)
			if got := e.buf.Cursor().Line; got != 45 {
				t.Fatalf("5 ctrl+d: line %d, want 45", got)
			}
			vimType(e, "2")
			e.Update(ctrlU)
			if got := e.buf.Cursor().Line; got != 43 {
				t.Fatalf("2 ctrl+u: line %d, want 43", got)
			}

			e = mk()
			vimType(e, "G")
			e.Update(ctrlD)
			if got := e.buf.Cursor().Line; got != e.buf.LastLine() {
				t.Fatalf("ctrl+d at the end: line %d, want %d", got, e.buf.LastLine())
			}
			vimType(e, "gg")
			e.Update(ctrlU)
			if got := e.buf.Cursor(); got.Line != 0 {
				t.Fatalf("ctrl+u at the start: cursor %v", got)
			}
		})
	}
}
