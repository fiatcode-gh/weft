package views

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
)

func vp(line, col int) buffer.Pos { return buffer.Pos{Line: line, Col: col} }

// vimKeys types s as keys; "\x1b" is the escape key. It returns every
// command the editor handed back.
func vimKeys(e *EditorView, s string) []tea.Cmd {
	var cmds []tea.Cmd
	for _, r := range s {
		msg := key(string(r))
		if r == 0x1b {
			msg = escKey
		}
		if _, cmd := e.Update(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

var vimSentinel = register{text: "sentinel", linewise: false}

// opCase is one vim command run from a cursor position: the text and cursor
// it leaves, and the register it fills (nil: the register is untouched).
type opCase struct {
	name, content string
	at            buffer.Pos
	keys          string
	want          string
	cur           buffer.Pos
	reg           *register
	pre           *register // register before the keys; default vimSentinel
}

func rg(text string, linewise bool) *register { return &register{text: text, linewise: linewise} }

func runOpCases(t *testing.T, cases []opCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := vimEditor(nil, tc.content, 0)
			e.buf.MoveTo(tc.at, false)
			e.goalOK = false
			start := vimSentinel
			if tc.pre != nil {
				start = *tc.pre
			}
			*e.reg = start
			cmds := vimKeys(e, tc.keys)
			if got := text(e); got != tc.want {
				t.Fatalf("keys %q: text %q, want %q", tc.keys, got, tc.want)
			}
			if got := cursorPos(e); got != tc.cur {
				t.Fatalf("keys %q: cursor %v, want %v", tc.keys, got, tc.cur)
			}
			wantReg := start
			if tc.reg != nil {
				wantReg = *tc.reg
			}
			if *e.reg != wantReg {
				t.Fatalf("keys %q: register %+v, want %+v", tc.keys, *e.reg, wantReg)
			}
			if tc.reg != nil && len(cmds) == 0 {
				t.Fatalf("keys %q filled the register but returned no clipboard command", tc.keys)
			}
			if tc.reg == nil && len(cmds) != 0 {
				t.Fatalf("keys %q left the register alone but returned a command", tc.keys)
			}
			if vimModeOf(e) != vimNormal || len(e.vim.pending) != 0 {
				t.Fatalf("keys %q: mode %v pending %q, want normal and idle", tc.keys, vimModeOf(e), e.vim.pending)
			}
		})
	}
}

func TestVimParseOperators(t *testing.T) {
	str := func(s string) []string { return strings.Split(s, "") }
	done := []struct {
		toks []string
		want vimCmd
	}{
		{str("dw"), vimCmd{op: "d", key: "w"}},
		{str("d3w"), vimCmd{count: 3, op: "d", key: "w"}},
		{str("2d3w"), vimCmd{count: 6, op: "d", key: "w"}},
		{str("diw"), vimCmd{op: "d", key: "iw"}},
		{str("ca("), vimCmd{op: "c", key: "a("}},
		{str("ca)"), vimCmd{op: "c", key: "a("}},
		{str("cib"), vimCmd{op: "c", key: "i("}},
		{str("dab"), vimCmd{op: "d", key: "a("}},
		{str("yi}"), vimCmd{op: "y", key: "i{"}},
		{str("yi{"), vimCmd{op: "y", key: "i{"}},
		{str("di]"), vimCmd{op: "d", key: "i["}},
		{str(`di"`), vimCmd{op: "d", key: `i"`}},
		{str("da'"), vimCmd{op: "d", key: "a'"}},
		{str("diW"), vimCmd{op: "d", key: "iW"}},
		{str("3dap"), vimCmd{count: 3, op: "d", key: "ap"}},
		{str("dd"), vimCmd{op: "d", key: "d"}},
		{str("3dd"), vimCmd{count: 3, op: "d", key: "d"}},
		{str("2d2d"), vimCmd{count: 4, op: "d", key: "d"}},
		{str("cc"), vimCmd{op: "c", key: "c"}},
		{str("yy"), vimCmd{op: "y", key: "y"}},
		{str(">>"), vimCmd{op: ">", key: ">"}},
		{str("<<"), vimCmd{op: "<", key: "<"}},
		{str(">ip"), vimCmd{op: ">", key: "ip"}},
		{str("d0"), vimCmd{op: "d", key: "0"}},
		{str("d10j"), vimCmd{count: 10, op: "d", key: "j"}},
		{str("dgg"), vimCmd{op: "d", key: "gg"}},
		{str("dG"), vimCmd{op: "d", key: "G"}},
		{str("dfz"), vimCmd{op: "d", key: "f", arg: "z"}},
		{str("c2tx"), vimCmd{count: 2, op: "c", key: "t", arg: "x"}},
		{str("d;"), vimCmd{op: "d", key: ";"}},
		{str("y%"), vimCmd{op: "y", key: "%"}},
		{str("d}"), vimCmd{op: "d", key: "}"}},
		{str("x"), vimCmd{key: "x"}},
		{str("3x"), vimCmd{count: 3, key: "x"}},
		{str("X"), vimCmd{key: "X"}},
		{str("s"), vimCmd{key: "s"}},
		{str("rx"), vimCmd{key: "r", arg: "x"}},
		{str("3rx"), vimCmd{count: 3, key: "r", arg: "x"}},
		{[]string{"r", "é"}, vimCmd{key: "r", arg: "é"}},
		{str("J"), vimCmd{key: "J"}},
		{str("p"), vimCmd{key: "p"}},
		{str("2P"), vimCmd{count: 2, key: "P"}},
		{str("D"), vimCmd{key: "D"}},
		{str("C"), vimCmd{key: "C"}},
		{str("S"), vimCmd{key: "S"}},
	}
	for _, tc := range done {
		if c, st := parseVim(tc.toks, false); st != vimDone || c != tc.want {
			t.Errorf("parseVim(%q) = %+v, %v; want %+v done", tc.toks, c, st, tc.want)
		}
	}
	more := [][]string{str("d"), str("2d"), str("d3"), str("di"), str("ya"), str("dg"), str("df"), str("r"), str("3r")}
	for _, toks := range more {
		if _, st := parseVim(toks, false); st != vimMore {
			t.Errorf("parseVim(%q) = %v; want vimMore", toks, st)
		}
	}
	bad := [][]string{
		str("dz"), str("diz"), str("dix"), str("d>"), str(">d"), str("dcc"), str("dd0"),
		{"d", "ctrl+d"}, {"d", "ctrl+u"}, {"d", "esc"}, {"r", "esc"}, {"r", "ab"}, {"d", "g", "x"},
		str("xx"), str("Jj"), str("pp"), {"d", "d", "d"},
	}
	for _, toks := range bad {
		if _, st := parseVim(toks, false); st != vimBad {
			t.Errorf("parseVim(%q) = %v; want vimBad", toks, st)
		}
	}
}

func TestVimOperatorMotions(t *testing.T) {
	const a = "alpha beta gamma\n"
	const punct = "a.b c.d e\n"
	const csv = "abc,def,ghi\n"
	const lines = "a\nb\nc\nd\n"
	const wide = "abc\ndef\nghi\n"
	const paras = "a\nb\n\nc\nd\n"
	runOpCases(t, []opCase{
		{name: "dw", content: a, at: vp(0, 6), keys: "dw", want: "alpha gamma\n", cur: vp(0, 6), reg: rg("beta ", false)},
		{name: "d2w", content: a, at: vp(0, 6), keys: "d2w", want: "alpha \n", cur: vp(0, 5), reg: rg("beta gamma", false)},
		{name: "2dw", content: a, at: vp(0, 6), keys: "2dw", want: "alpha \n", cur: vp(0, 5), reg: rg("beta gamma", false)},
		{name: "de", content: a, at: vp(0, 6), keys: "de", want: "alpha  gamma\n", cur: vp(0, 6), reg: rg("beta", false)},
		{name: "db", content: a, at: vp(0, 6), keys: "db", want: "beta gamma\n", cur: vp(0, 0), reg: rg("alpha ", false)},
		{name: "d0", content: a, at: vp(0, 6), keys: "d0", want: "beta gamma\n", cur: vp(0, 0), reg: rg("alpha ", false)},
		{name: "d^", content: "  foo bar\n", at: vp(0, 6), keys: "d^", want: "  bar\n", cur: vp(0, 2), reg: rg("foo ", false)},
		{name: "d$", content: a, at: vp(0, 6), keys: "d$", want: "alpha \n", cur: vp(0, 5), reg: rg("beta gamma", false)},
		{name: "d2$", content: "ab\ncd\nef\n", at: vp(0, 1), keys: "d2$", want: "a\nef\n", cur: vp(0, 0), reg: rg("b\ncd", false)},
		{name: "dh", content: a, at: vp(0, 6), keys: "dh", want: "alphabeta gamma\n", cur: vp(0, 5), reg: rg(" ", false)},
		{name: "dl", content: a, at: vp(0, 6), keys: "dl", want: "alpha eta gamma\n", cur: vp(0, 6), reg: rg("b", false)},
		{name: "d3l", content: a, at: vp(0, 6), keys: "d3l", want: "alpha a gamma\n", cur: vp(0, 6), reg: rg("bet", false)},
		{name: "dl on the last character", content: "abc\n", at: vp(0, 2), keys: "dl", want: "ab\n", cur: vp(0, 1), reg: rg("c", false)},
		{name: "dh at column 0 fails", content: "abc\n", keys: "dh", want: "abc\n", cur: vp(0, 0)},
		{name: "yw", content: a, at: vp(0, 6), keys: "yw", want: a, cur: vp(0, 6), reg: rg("beta ", false)},
		{name: "yb", content: a, at: vp(0, 6), keys: "yb", want: a, cur: vp(0, 0), reg: rg("alpha ", false)},
		{name: "yh", content: a, at: vp(0, 6), keys: "yh", want: a, cur: vp(0, 5), reg: rg(" ", false)},
		{name: "y$", content: a, at: vp(0, 6), keys: "y$", want: a, cur: vp(0, 6), reg: rg("beta gamma", false)},
		{name: "ye", content: a, at: vp(0, 6), keys: "ye", want: a, cur: vp(0, 6), reg: rg("beta", false)},
		{name: "ce", content: a, at: vp(0, 6), keys: "ceX\x1b", want: "alpha X gamma\n", cur: vp(0, 6), reg: rg("beta", false)},
		{name: "c$", content: a, at: vp(0, 6), keys: "c$X\x1b", want: "alpha X\n", cur: vp(0, 6), reg: rg("beta gamma", false)},
		{name: "cb", content: a, at: vp(0, 6), keys: "cbX\x1b", want: "Xbeta gamma\n", cur: vp(0, 0), reg: rg("alpha ", false)},
		{name: "c0", content: a, at: vp(0, 6), keys: "c0X\x1b", want: "Xbeta gamma\n", cur: vp(0, 0), reg: rg("alpha ", false)},
		{name: "dW", content: punct, keys: "dW", want: "c.d e\n", cur: vp(0, 0), reg: rg("a.b ", false)},
		{name: "dE", content: punct, keys: "dE", want: " c.d e\n", cur: vp(0, 0), reg: rg("a.b", false)},
		{name: "dw stops at punctuation", content: punct, keys: "dw", want: ".b c.d e\n", cur: vp(0, 0), reg: rg("a", false)},
		{name: "d2w over punctuation", content: punct, keys: "d2w", want: "b c.d e\n", cur: vp(0, 0), reg: rg("a.", false)},
		{name: "dB", content: punct, at: vp(0, 4), keys: "dB", want: "c.d e\n", cur: vp(0, 0), reg: rg("a.b ", false)},
		{name: "db over punctuation", content: punct, at: vp(0, 4), keys: "db", want: "a.c.d e\n", cur: vp(0, 2), reg: rg("b ", false)},
		{name: "cW", content: punct, keys: "cWX\x1b", want: "X c.d e\n", cur: vp(0, 0), reg: rg("a.b", false)},
		{name: "df,", content: csv, keys: "df,", want: "def,ghi\n", cur: vp(0, 0), reg: rg("abc,", false)},
		{name: "dt,", content: csv, keys: "dt,", want: ",def,ghi\n", cur: vp(0, 0), reg: rg("abc", false)},
		{name: "d2f,", content: csv, keys: "d2f,", want: "ghi\n", cur: vp(0, 0), reg: rg("abc,def,", false)},
		{name: "dF,", content: csv, at: vp(0, 9), keys: "dF,", want: "abc,defhi\n", cur: vp(0, 7), reg: rg(",g", false)},
		{name: "dT,", content: csv, at: vp(0, 9), keys: "dT,", want: "abc,def,hi\n", cur: vp(0, 8), reg: rg("g", false)},
		{name: "d; repeats f", content: csv, keys: "f,d;", want: "abcghi\n", cur: vp(0, 3), reg: rg(",def,", false)},
		{name: "d, repeats backwards", content: csv, keys: "f,;d,", want: "abc,ghi\n", cur: vp(0, 3), reg: rg(",def", false)},
		{name: "dfz without z does nothing", content: csv, keys: "dfz", want: csv, cur: vp(0, 0)},
		{name: "c with a failing find stays normal", content: csv, keys: "cfzX", want: csv, cur: vp(0, 0)},
		{name: "yf,", content: csv, keys: "yf,", want: csv, cur: vp(0, 0), reg: rg("abc,", false)},
		{name: "d% forward", content: "x (a b) y\n", at: vp(0, 2), keys: "d%", want: "x  y\n", cur: vp(0, 2), reg: rg("(a b)", false)},
		{name: "d% backward", content: "x (a b) y\n", at: vp(0, 6), keys: "d%", want: "x  y\n", cur: vp(0, 2), reg: rg("(a b)", false)},
		{name: "y% backward", content: "x (a b) y\n", at: vp(0, 6), keys: "y%", want: "x (a b) y\n", cur: vp(0, 2), reg: rg("(a b)", false)},
		{name: "d% without bracket fails", content: "abc\n", keys: "d%", want: "abc\n", cur: vp(0, 0)},
		{name: "d}", content: paras, at: vp(1, 0), keys: "d}", want: "a\n\nc\nd\n", cur: vp(1, 0), reg: rg("b\n", false)},
		{name: "d{", content: paras, at: vp(3, 0), keys: "d{", want: "a\nb\nc\nd\n", cur: vp(2, 0), reg: rg("\n", false)},
		{name: "d} with no later blank line", content: paras, at: vp(3, 0), keys: "d}", want: "a\nb\n\n\n", cur: vp(3, 0), reg: rg("c\nd", false)},
		{name: "dj", content: lines, keys: "dj", want: "c\nd\n", cur: vp(0, 0), reg: rg("a\nb\n", true)},
		{name: "dk", content: lines, at: vp(2, 0), keys: "dk", want: "a\nd\n", cur: vp(1, 0), reg: rg("b\nc\n", true)},
		{name: "d2j", content: lines, keys: "d2j", want: "d\n", cur: vp(0, 0), reg: rg("a\nb\nc\n", true)},
		{name: "2dj", content: lines, keys: "2dj", want: "d\n", cur: vp(0, 0), reg: rg("a\nb\nc\n", true)},
		{name: "dgg", content: lines, at: vp(1, 0), keys: "dgg", want: "c\nd\n", cur: vp(0, 0), reg: rg("a\nb\n", true)},
		{name: "dG", content: lines, at: vp(1, 0), keys: "dG", want: "a\n", cur: vp(0, 0), reg: rg("b\nc\nd\n", true)},
		{name: "d3G", content: lines, keys: "d3G", want: "d\n", cur: vp(0, 0), reg: rg("a\nb\nc\n", true)},
		{name: "yj keeps the cursor", content: wide, at: vp(0, 1), keys: "yj", want: wide, cur: vp(0, 1), reg: rg("abc\ndef\n", true)},
		{name: "yk moves up", content: wide, at: vp(2, 2), keys: "yk", want: wide, cur: vp(1, 2), reg: rg("def\nghi\n", true)},
		{name: "yG keeps the cursor", content: wide, at: vp(0, 1), keys: "yG", want: wide, cur: vp(0, 1), reg: rg(wide, true)},
		{name: "ygg moves up", content: wide, at: vp(2, 1), keys: "ygg", want: wide, cur: vp(0, 1), reg: rg(wide, true)},
		{name: "cj", content: lines, keys: "cjX\x1b", want: "X\nc\nd\n", cur: vp(0, 0), reg: rg("a\nb\n", true)},
		{name: "ck", content: lines, at: vp(1, 0), keys: "ckX\x1b", want: "X\nc\nd\n", cur: vp(0, 0), reg: rg("a\nb\n", true)},
		{name: "dj on the last line fails", content: lines, at: vp(3, 0), keys: "dj", want: lines, cur: vp(3, 0)},
		{name: ">j", content: lines, keys: ">j", want: "  a\n  b\nc\nd\n", cur: vp(0, 2)},
		{name: "<j", content: "  a\n  b\nc\n", keys: "<j", want: "a\nb\nc\n", cur: vp(0, 0)},
		{name: ">} excludes the blank line", content: "a\nb\n\nc\n", keys: ">}", want: "  a\n  b\n\nc\n", cur: vp(0, 2)},
		{name: ">w shifts the line", content: "x y\n", keys: ">w", want: "  x y\n", cur: vp(0, 2)},
		{name: "d2dw", content: "a b c d e\n", keys: "2d2w", want: "e\n", cur: vp(0, 0), reg: rg("a b c d ", false)},
	})
}

func TestVimTextObjects(t *testing.T) {
	const foo = "foo bar baz\n"
	const say = "say \"hi there\" now\n"
	runOpCases(t, []opCase{
		{name: "diw", content: foo, at: vp(0, 5), keys: "diw", want: "foo  baz\n", cur: vp(0, 4), reg: rg("bar", false)},
		{name: "daw", content: foo, at: vp(0, 5), keys: "daw", want: "foo baz\n", cur: vp(0, 4), reg: rg("bar ", false)},
		{name: "daw on the last word takes the blank before", content: foo, at: vp(0, 9), keys: "daw", want: "foo bar\n", cur: vp(0, 6), reg: rg(" baz", false)},
		{name: "ciw", content: foo, at: vp(0, 5), keys: "ciwX\x1b", want: "foo X baz\n", cur: vp(0, 4), reg: rg("bar", false)},
		{name: "yiw", content: foo, at: vp(0, 5), keys: "yiw", want: foo, cur: vp(0, 4), reg: rg("bar", false)},
		{name: "d2aw", content: foo, keys: "d2aw", want: "baz\n", cur: vp(0, 0), reg: rg("foo bar ", false)},
		{name: "d3iw", content: foo, keys: "d3iw", want: " baz\n", cur: vp(0, 0), reg: rg("foo bar", false)},
		{name: "2diw", content: foo, keys: "2diw", want: "bar baz\n", cur: vp(0, 0), reg: rg("foo ", false)},
		{name: "diW", content: "a.b c.d\n", at: vp(0, 1), keys: "diW", want: " c.d\n", cur: vp(0, 0), reg: rg("a.b", false)},
		{name: "daW", content: "a.b c.d\n", at: vp(0, 1), keys: "daW", want: "c.d\n", cur: vp(0, 0), reg: rg("a.b ", false)},
		{name: "diw on a blank run", content: "a   b\n", at: vp(0, 2), keys: "diw", want: "ab\n", cur: vp(0, 1), reg: rg("   ", false)},
		{name: "di\"", content: say, at: vp(0, 6), keys: `di"`, want: "say \"\" now\n", cur: vp(0, 5), reg: rg("hi there", false)},
		{name: "da\"", content: say, at: vp(0, 6), keys: `da"`, want: "say now\n", cur: vp(0, 4), reg: rg("\"hi there\" ", false)},
		{name: "ci\"", content: say, at: vp(0, 6), keys: "ci\"X\x1b", want: "say \"X\" now\n", cur: vp(0, 5), reg: rg("hi there", false)},
		{name: "yi\"", content: say, at: vp(0, 6), keys: `yi"`, want: say, cur: vp(0, 5), reg: rg("hi there", false)},
		{name: "di\" on empty quotes", content: "\"\"\n", keys: `di"`, want: "\"\"\n", cur: vp(0, 0)},
		{name: "ci\" on empty quotes", content: "\"\"\n", keys: "ci\"X\x1b", want: "\"X\"\n", cur: vp(0, 1)},
		{name: "di'", content: "a 'b' c\n", at: vp(0, 3), keys: "di'", want: "a '' c\n", cur: vp(0, 3), reg: rg("b", false)},
		{name: "da'", content: "a 'b' c\n", at: vp(0, 3), keys: "da'", want: "a c\n", cur: vp(0, 2), reg: rg("'b' ", false)},
		{name: "di(", content: "f(a, c) x\n", at: vp(0, 3), keys: "di(", want: "f() x\n", cur: vp(0, 2), reg: rg("a, c", false)},
		{name: "di)", content: "f(a, c) x\n", at: vp(0, 3), keys: "di)", want: "f() x\n", cur: vp(0, 2), reg: rg("a, c", false)},
		{name: "dib", content: "f(a, c) x\n", at: vp(0, 3), keys: "dib", want: "f() x\n", cur: vp(0, 2), reg: rg("a, c", false)},
		{name: "da(", content: "f(a, c) x\n", at: vp(0, 3), keys: "da(", want: "f x\n", cur: vp(0, 1), reg: rg("(a, c)", false)},
		{name: "dab", content: "f(a, c) x\n", at: vp(0, 3), keys: "dab", want: "f x\n", cur: vp(0, 1), reg: rg("(a, c)", false)},
		{name: "ci(", content: "f(a, c) x\n", at: vp(0, 3), keys: "ci(X\x1b", want: "f(X) x\n", cur: vp(0, 2), reg: rg("a, c", false)},
		{name: "d2i(", content: "(a (b) c)\n", at: vp(0, 4), keys: "d2i(", want: "()\n", cur: vp(0, 1), reg: rg("a (b) c", false)},
		{name: "di( innermost", content: "(a (b) c)\n", at: vp(0, 4), keys: "di(", want: "(a () c)\n", cur: vp(0, 4), reg: rg("b", false)},
		{name: "di[", content: "[a] {b}\n", at: vp(0, 1), keys: "di[", want: "[] {b}\n", cur: vp(0, 1), reg: rg("a", false)},
		{name: "di]", content: "[a] {b}\n", at: vp(0, 1), keys: "di]", want: "[] {b}\n", cur: vp(0, 1), reg: rg("a", false)},
		{name: "di{", content: "[a] {b}\n", at: vp(0, 5), keys: "di{", want: "[a] {}\n", cur: vp(0, 5), reg: rg("b", false)},
		{name: "da}", content: "[a] {b}\n", at: vp(0, 5), keys: "da}", want: "[a] \n", cur: vp(0, 3), reg: rg("{b}", false)},
		{name: "yi}", content: "[a] {b}\n", at: vp(0, 5), keys: "yi}", want: "[a] {b}\n", cur: vp(0, 5), reg: rg("b", false)},
		{name: "di{ over lines", content: "if {\n  a\n  b\n}\n", at: vp(1, 2), keys: "di{", want: "if {\n\n}\n", cur: vp(1, 0), reg: rg("  a\n  b", false)},
		{name: "di( without a bracket does nothing", content: "abc\n", keys: "di(", want: "abc\n", cur: vp(0, 0)},
		{name: "dip", content: "a\nb\n\nc\n", keys: "dip", want: "\nc\n", cur: vp(0, 0), reg: rg("a\nb\n", true)},
		{name: "dap", content: "a\nb\n\nc\n", keys: "dap", want: "c\n", cur: vp(0, 0), reg: rg("a\nb\n\n", true)},
		{name: "cip is linewise", content: "a\nb\n\nc\n", keys: "cipX\x1b", want: "X\n\nc\n", cur: vp(0, 0), reg: rg("a\nb\n", true)},
		{name: "cip keeps the indent", content: "  a\n  b\n\nc\n", at: vp(1, 2), keys: "cipX\x1b", want: "  X\n\nc\n", cur: vp(0, 2), reg: rg("  a\n  b\n", true)},
		{name: "yip moves to the first line", content: "a\nb\n\nc\n", at: vp(1, 0), keys: "yip", want: "a\nb\n\nc\n", cur: vp(0, 0), reg: rg("a\nb\n", true)},
		{name: "d2ip", content: "a\n\nb\n", keys: "d2ip", want: "b\n", cur: vp(0, 0), reg: rg("a\n\n", true)},
		{name: ">ip", content: "a\nb\n\nc\n", keys: ">ip", want: "  a\n  b\n\nc\n", cur: vp(0, 2)},
	})
}

func TestVimShortcuts(t *testing.T) {
	runOpCases(t, []opCase{
		{name: "x", content: "abc\n", at: vp(0, 1), keys: "x", want: "ac\n", cur: vp(0, 1), reg: rg("b", false)},
		{name: "3x", content: "abcd\n", keys: "3x", want: "d\n", cur: vp(0, 0), reg: rg("abc", false)},
		{name: "x on the last character", content: "abc\n", at: vp(0, 2), keys: "x", want: "ab\n", cur: vp(0, 1), reg: rg("c", false)},
		{name: "X", content: "abc\n", at: vp(0, 2), keys: "X", want: "ac\n", cur: vp(0, 1), reg: rg("b", false)},
		{name: "2X", content: "abcd\n", at: vp(0, 3), keys: "2X", want: "ad\n", cur: vp(0, 1), reg: rg("bc", false)},
		{name: "s", content: "abc\n", at: vp(0, 1), keys: "sZ\x1b", want: "aZc\n", cur: vp(0, 1), reg: rg("b", false)},
		{name: "3s", content: "abcd\n", keys: "3sZ\x1b", want: "Zd\n", cur: vp(0, 0), reg: rg("abc", false)},
		{name: "S", content: "  foo\nbar\n", keys: "SX\x1b", want: "  X\nbar\n", cur: vp(0, 2), reg: rg("  foo\n", true)},
		{name: "2S", content: "  foo\nbar\nbaz\n", keys: "2SX\x1b", want: "  X\nbaz\n", cur: vp(0, 2), reg: rg("  foo\nbar\n", true)},
		{name: "r", content: "abc\n", at: vp(0, 1), keys: "rx", want: "axc\n", cur: vp(0, 1)},
		{name: "3r", content: "abcd\n", keys: "3rx", want: "xxxd\n", cur: vp(0, 2)},
		{name: "J", content: "a\n  b\nc\n", keys: "J", want: "a b\nc\n", cur: vp(0, 1)},
		{name: "3J", content: "a\nb\nc\nd\n", keys: "3J", want: "a b c\nd\n", cur: vp(0, 3)},
		{name: "p charwise", content: "abc\n", keys: "p", want: "aXYbc\n", cur: vp(0, 2), pre: rg("XY", false)},
		{name: "P charwise", content: "abc\n", keys: "P", want: "XYabc\n", cur: vp(0, 1), pre: rg("XY", false)},
		{name: "3p charwise", content: "abc\n", keys: "3p", want: "aXYXYXYbc\n", cur: vp(0, 6), pre: rg("XY", false)},
		{name: "p linewise", content: "a\nb\n", keys: "p", want: "a\nl\nb\n", cur: vp(1, 0), pre: rg("l\n", true)},
		{name: "P linewise", content: "a\nb\n", at: vp(1, 0), keys: "P", want: "a\nl\nb\n", cur: vp(1, 0), pre: rg("l\n", true)},
		{name: "2p linewise", content: "a\nb\n", keys: "2p", want: "a\nl\nl\nb\n", cur: vp(1, 0), pre: rg("l\n", true)},
		{name: "p linewise on the last line", content: "a\nb\n", at: vp(1, 0), keys: "p", want: "a\nb\nl\n", cur: vp(2, 0), pre: rg("l\n", true)},
		{name: "D", content: "foo bar\n", at: vp(0, 4), keys: "D", want: "foo \n", cur: vp(0, 3), reg: rg("bar", false)},
		{name: "2D", content: "ab\ncd\nef\n", at: vp(0, 1), keys: "2D", want: "a\nef\n", cur: vp(0, 0), reg: rg("b\ncd", false)},
		{name: "C", content: "foo bar\n", at: vp(0, 4), keys: "CX\x1b", want: "foo X\n", cur: vp(0, 4), reg: rg("bar", false)},
		{name: "dd", content: "a\nb\nc\n", at: vp(1, 0), keys: "dd", want: "a\nc\n", cur: vp(1, 0), reg: rg("b\n", true)},
		{name: "dd on the last line", content: "a\nb\nc\n", at: vp(2, 0), keys: "dd", want: "a\nb\n", cur: vp(1, 0), reg: rg("c\n", true)},
		{name: "2dd", content: "a\nb\nc\n", keys: "2dd", want: "c\n", cur: vp(0, 0), reg: rg("a\nb\n", true)},
		{name: "cc", content: "  foo\nbar\n", keys: "ccX\x1b", want: "  X\nbar\n", cur: vp(0, 2), reg: rg("  foo\n", true)},
		{name: "2cc", content: "a\nb\nc\n", keys: "2ccX\x1b", want: "X\nc\n", cur: vp(0, 0), reg: rg("a\nb\n", true)},
		{name: "yy", content: "a\nb\n", at: vp(1, 0), keys: "yy", want: "a\nb\n", cur: vp(1, 0), reg: rg("b\n", true)},
		{name: "2yy", content: "a\nb\nc\n", at: vp(0, 0), keys: "2yy", want: "a\nb\nc\n", cur: vp(0, 0), reg: rg("a\nb\n", true)},
		{name: "yyp", content: "a\nb\n", keys: "yyp", want: "a\na\nb\n", cur: vp(1, 0), reg: rg("a\n", true)},
		{name: ">> a bullet moves its children", content: "- a\n  - b\n- c\n", keys: ">>", want: "  - a\n    - b\n- c\n", cur: vp(0, 2)},
		{name: "<< a bullet moves its children", content: "  - a\n    - b\n- c\n", keys: "<<", want: "- a\n  - b\n- c\n", cur: vp(0, 0)},
		{name: ">> a plain line", content: "a\nb\n", at: vp(1, 0), keys: ">>", want: "a\n  b\n", cur: vp(1, 2)},
		{name: "3>> shifts three lines only", content: "- a\n  - b\n- c\n- d\n", keys: "3>>", want: "  - a\n    - b\n  - c\n- d\n", cur: vp(0, 2)},
		{name: "2<<", content: "  a\n  b\n  c\n", keys: "2<<", want: "a\nb\n  c\n", cur: vp(0, 0)},
		{name: "3>> near the end", content: "a\nb\nc\n", at: vp(1, 0), keys: "3>>", want: "a\n  b\n  c\n", cur: vp(1, 2)},
	})
}

func TestVimOperatorSpecialCases(t *testing.T) {
	runOpCases(t, []opCase{
		{name: "dw on the last word keeps the line break", content: "foo bar\nbaz\n", at: vp(0, 4), keys: "dw", want: "foo \nbaz\n", cur: vp(0, 3), reg: rg("bar", false)},
		{name: "d2w across a line break", content: "foo bar\nbaz qux\n", keys: "d2w", want: "\nbaz qux\n", cur: vp(0, 0), reg: rg("foo bar", false)},
		{name: "dw on the last word of the buffer", content: "foo bar\n", at: vp(0, 4), keys: "dw", want: "foo \n", cur: vp(0, 3), reg: rg("bar", false)},
		{name: "cw in a word's middle", content: "foo bar baz\n", at: vp(0, 5), keys: "cwX\x1b", want: "foo bX baz\n", cur: vp(0, 5), reg: rg("ar", false)},
		{name: "cw on a word's last grapheme", content: "foo bar baz\n", at: vp(0, 6), keys: "cwX\x1b", want: "foo baX baz\n", cur: vp(0, 6), reg: rg("r", false)},
		{name: "cw on a word start", content: "foo bar baz\n", at: vp(0, 4), keys: "cwX\x1b", want: "foo X baz\n", cur: vp(0, 4), reg: rg("bar", false)},
		{name: "cw on blanks", content: "foo   bar\n", at: vp(0, 4), keys: "cwX\x1b", want: "foo Xbar\n", cur: vp(0, 4), reg: rg("  ", false)},
		{name: "c2w", content: "foo bar baz\n", keys: "c2wX\x1b", want: "X baz\n", cur: vp(0, 0), reg: rg("foo bar", false)},
		{name: "cw on punctuation", content: "a.b\n", at: vp(0, 1), keys: "cwX\x1b", want: "aXb\n", cur: vp(0, 1), reg: rg(".", false)},
		{name: "dj on the last line fails", content: "a\nb\n", at: vp(1, 0), keys: "dj", want: "a\nb\n", cur: vp(1, 0)},
		{name: "dk on the first line fails", content: "a\nb\n", keys: "dk", want: "a\nb\n", cur: vp(0, 0)},
		{name: "dfz with no z does nothing", content: "abc\n", keys: "dfz", want: "abc\n", cur: vp(0, 0)},
		{name: "3dd near the end", content: "a\nb\nc\nd\n", at: vp(2, 0), keys: "3dd", want: "a\nb\n", cur: vp(1, 0), reg: rg("c\nd\n", true)},
		{name: "dG from the first line leaves nothing", content: "a\nb\nc\n", keys: "dG", want: "", cur: vp(0, 0), reg: rg("a\nb\nc\n", true)},
		{name: "dd on the only line", content: "a\n", keys: "dd", want: "", cur: vp(0, 0), reg: rg("a\n", true)},
		{name: "x on an empty line", content: "a\n\nb\n", at: vp(1, 0), keys: "x", want: "a\n\nb\n", cur: vp(1, 0)},
		{name: "5x on abc from b", content: "abc\n", at: vp(0, 1), keys: "5x", want: "a\n", cur: vp(0, 0), reg: rg("bc", false)},
		{name: "3rx with two graphemes left fails", content: "abc\n", at: vp(0, 1), keys: "3rx", want: "abc\n", cur: vp(0, 1)},
		{name: "J on the last line fails", content: "a\nb\n", at: vp(1, 0), keys: "J", want: "a\nb\n", cur: vp(1, 0)},
		{name: "D on an empty line", content: "a\n\n", at: vp(1, 0), keys: "D", want: "a\n\n", cur: vp(1, 0)},
		{name: "C on an empty line enters insert", content: "a\n\nb\n", at: vp(1, 0), keys: "CX\x1b", want: "a\nX\nb\n", cur: vp(1, 0)},
		{name: "s on an empty line enters insert", content: "a\n\nb\n", at: vp(1, 0), keys: "sX\x1b", want: "a\nX\nb\n", cur: vp(1, 0)},
		{name: "bad operator target drops the operator", content: "abc\n", keys: "dzx", want: "bc\n", cur: vp(0, 0), reg: rg("a", false)},
	})
	t.Run("c0 at column 0 enters insert", func(t *testing.T) {
		e := vimEditor(nil, "abc\n", 0)
		vimKeys(e, "c0")
		if vimModeOf(e) != vimInsert {
			t.Fatalf("c0 left mode %v, want insert", vimModeOf(e))
		}
		vimKeys(e, "X\x1b")
		if text(e) != "Xabc\n" {
			t.Fatalf("text %q", text(e))
		}
	})
	t.Run("failed operators add no undo step", func(t *testing.T) {
		for _, keys := range []string{"dj", "dfz", "d%", "x", "5rx", "J"} {
			e := vimEditor(nil, "a\n\nb\n", 0)
			e.buf.MoveTo(vp(2, 0), false)
			if keys == "x" {
				e.buf.MoveTo(vp(1, 0), false)
			}
			vimKeys(e, keys)
			vimKeys(e, "u")
			if e.notice != "nothing to undo" {
				t.Errorf("%s left an undo step (notice %q)", keys, e.notice)
			}
		}
	})
	t.Run("p with an empty register", func(t *testing.T) {
		for _, k := range []string{"p", "P"} {
			e := vimEditor(nil, "abc\n", 0)
			*e.reg = register{}
			cmds := vimKeys(e, k)
			if e.notice != "nothing copied yet" || text(e) != "abc\n" || len(cmds) != 0 {
				t.Fatalf("%s: notice %q text %q cmds %d", k, e.notice, text(e), len(cmds))
			}
			if !strings.Contains(plain(e.statusLine()), "nothing copied yet") {
				t.Fatalf("status %q lacks the notice", plain(e.statusLine()))
			}
		}
	})
}

func TestVimRegister(t *testing.T) {
	t.Run("yank and delete fill the register and the clipboard", func(t *testing.T) {
		for _, tc := range []struct {
			keys, text string
			linewise   bool
		}{
			{"yw", "alpha ", false}, {"yy", "alpha beta\n", true}, {"dw", "alpha ", false},
			{"dd", "alpha beta\n", true}, {"x", "a", false}, {"D", "alpha beta", false},
			{"cwX\x1b", "alpha", false}, {"ccX\x1b", "alpha beta\n", true},
		} {
			e := vimEditor(nil, "alpha beta\n", 0)
			cmds := vimKeys(e, tc.keys)
			if *e.reg != (register{text: tc.text, linewise: tc.linewise}) {
				t.Errorf("%s: register %+v", tc.keys, *e.reg)
			}
			if len(cmds) != 1 {
				t.Fatalf("%s: %d commands, want 1", tc.keys, len(cmds))
			}
			if got, want := cmds[0](), tea.SetClipboard(tc.text)(); !reflect.DeepEqual(got, want) {
				t.Errorf("%s: clipboard message %#v, want %#v", tc.keys, got, want)
			}
		}
	})
	t.Run("an empty delete leaves the register", func(t *testing.T) {
		e := vimEditor(nil, "\n", 0)
		*e.reg = register{text: "keep"}
		if cmds := vimKeys(e, "x"); len(cmds) != 0 || e.reg.text != "keep" {
			t.Fatalf("x on an empty line touched the register: %+v, %d cmds", *e.reg, len(cmds))
		}
	})
	t.Run("ctrl+v pastes a vim yank", func(t *testing.T) {
		e := vimEditor(nil, "abc\n", 0)
		vimKeys(e, "yiwA")
		e.Update(ctrlV)
		if text(e) != "abcabc\n" {
			t.Fatalf("text %q", text(e))
		}
	})
	t.Run("p pastes a ctrl+c copy", func(t *testing.T) {
		e := vimEditor(nil, "alpha\nbeta\n", 0)
		e.Update(ctrlC)
		vimKeys(e, "p")
		if text(e) != "alpha\nalpha\nbeta\n" {
			t.Fatalf("text %q", text(e))
		}
	})
	t.Run("the register outlives the editor", func(t *testing.T) {
		a := openVimApp(t, map[string]string{journalPath: "- one\n- two\n"}, journalPage)
		e := a.editor
		before := e.Content()
		vimKeys(e, "yy")
		if a.clip.text == "" || !a.clip.linewise {
			t.Fatalf("yy did not fill the App's clip: %+v", a.clip)
		}
		a.Update(escKey)
		if a.editor != nil {
			t.Fatal("esc did not leave the clean editor")
		}
		a.Update(key("e"))
		if a.editor == nil {
			t.Fatal("editor did not reopen")
		}
		typeApp(a, "p")
		if got := a.editor.Content(); len(got) != len(before)+len(a.clip.text) {
			t.Fatalf("p pasted %q -> %q", before, got)
		}
	})
}

func TestVimEachCommandOneUndoStep(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		at            buffer.Pos
		keys          string
		reg           register // register before the keys
	}{
		{"dw", "alpha beta gamma\n", vp(0, 6), "dw", register{}},
		{"d2w", "alpha beta gamma\n", vp(0, 0), "d2w", register{}},
		{"d$", "alpha beta\n", vp(0, 3), "d$", register{}},
		{"dj", "a\nb\nc\n", vp(0, 0), "dj", register{}},
		{"dG", "a\nb\nc\n", vp(1, 0), "dG", register{}},
		{"diw", "foo bar\n", vp(0, 5), "diw", register{}},
		{"da(", "f(a) x\n", vp(0, 3), "da(", register{}},
		{"dap", "a\nb\n\nc\n", vp(0, 0), "dap", register{}},
		{"dd", "a\nb\nc\n", vp(1, 0), "dd", register{}},
		{"3dd", "a\nb\nc\nd\n", vp(0, 0), "3dd", register{}},
		{"cw", "foo bar\n", vp(0, 0), "cwXY\x1b", register{}},
		{"c$", "foo bar\n", vp(0, 4), "c$XY\x1b", register{}},
		{"cj", "a\nb\nc\n", vp(0, 0), "cjXY\x1b", register{}},
		{"cc", "  foo\nbar\n", vp(0, 0), "ccXY\x1b", register{}},
		{"ci(", "f(a) x\n", vp(0, 3), "ci(XY\x1b", register{}},
		{"x", "abc\n", vp(0, 1), "x", register{}},
		{"3x", "abcd\n", vp(0, 0), "3x", register{}},
		{"X", "abc\n", vp(0, 2), "X", register{}},
		{"s", "abc\n", vp(0, 1), "sXY\x1b", register{}},
		{"S", "foo\nbar\n", vp(0, 0), "SXY\x1b", register{}},
		{"r", "abc\n", vp(0, 1), "rx", register{}},
		{"3r", "abcd\n", vp(0, 0), "3rx", register{}},
		{"J", "a\nb\nc\n", vp(0, 0), "J", register{}},
		{"3J", "a\nb\nc\nd\n", vp(0, 0), "3J", register{}},
		{"p", "abc\n", vp(0, 0), "p", register{text: "XY"}},
		{"3P", "abc\n", vp(0, 0), "3P", register{text: "XY"}},
		{"p linewise", "a\nb\n", vp(0, 0), "p", register{text: "l\n", linewise: true}},
		{"D", "foo bar\n", vp(0, 4), "D", register{}},
		{"C", "foo bar\n", vp(0, 4), "CXY\x1b", register{}},
		{">>", "- a\n  - b\n- c\n", vp(0, 0), ">>", register{}},
		{"<<", "  - a\n    - b\n- c\n", vp(0, 0), "<<", register{}},
		{"3>>", "a\nb\nc\nd\n", vp(0, 0), "3>>", register{}},
		{">ip", "a\nb\n\nc\n", vp(0, 0), ">ip", register{}},
		{">j", "a\nb\nc\n", vp(0, 0), ">j", register{}},
		{"o", "a\nb\n", vp(0, 0), "oXY\x1b", register{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := vimEditor(nil, tc.content, 0)
			e.buf.MoveTo(tc.at, false)
			e.goalOK = false
			*e.reg = tc.reg
			vimKeys(e, tc.keys)
			after := text(e)
			if after == tc.content {
				t.Fatalf("%s changed nothing", tc.keys)
			}
			vimKeys(e, "u")
			if got := text(e); got != tc.content {
				t.Fatalf("one u left %q, want %q", got, tc.content)
			}
			vimKeys(e, "u")
			if e.notice != "nothing to undo" {
				t.Fatalf("%s took more than one undo step", tc.keys)
			}
			e.Update(ctrlR)
			if got := text(e); got != after {
				t.Fatalf("ctrl+r gave %q, want %q", got, after)
			}
			e.Update(ctrlR)
			if e.notice != "nothing to redo" {
				t.Fatalf("%s took more than one redo step", tc.keys)
			}
		})
	}
}

func TestVimCRLF(t *testing.T) {
	const crlf = "a\r\nb\r\nc\r\n"
	for _, tc := range []struct {
		name, keys, want string
	}{
		{"dd", "dd", "b\r\nc\r\n"},
		{"yyp", "yyp", "a\r\na\r\nb\r\nc\r\n"},
		{"yyP", "yyP", "a\r\na\r\nb\r\nc\r\n"},
		{"J", "J", "a b\r\nc\r\n"},
		{"o", "oX\x1b", "a\r\nX\r\nb\r\nc\r\n"},
		{"O", "OX\x1b", "X\r\na\r\nb\r\nc\r\n"},
		{"cc", "ccX\x1b", "X\r\nb\r\nc\r\n"},
		{"2dd", "2dd", "c\r\n"},
		{">>", ">>", "  a\r\nb\r\nc\r\n"},
		{"dG", "dG", ""},
		{"3J", "3J", "a b c\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := vimEditor(nil, crlf, 0)
			vimKeys(e, tc.keys)
			if got := e.Content(); got != tc.want {
				t.Fatalf("%q, want %q", got, tc.want)
			}
			vimKeys(e, "u")
			if got := e.Content(); got != crlf {
				t.Fatalf("undo gave %q, want the original", got)
			}
		})
	}
}

func TestVimOperatorsMultiByte(t *testing.T) {
	const zwj = "\U0001F468\u200D\U0001F469\u200D\U0001F467"
	runOpCases(t, []opCase{
		{name: "x combining", content: "e\u0301x\n", keys: "x", want: "x\n", cur: vp(0, 0), reg: rg("e\u0301", false)},
		{name: "x emoji ZWJ", content: zwj + "a\n", keys: "x", want: "a\n", cur: vp(0, 0), reg: rg(zwj, false)},
		{name: "x CJK", content: "日本語\n", keys: "x", want: "本語\n", cur: vp(0, 0), reg: rg("日", false)},
		{name: "2x CJK", content: "日本語\n", keys: "2x", want: "語\n", cur: vp(0, 0), reg: rg("日本", false)},
		{name: "r combining", content: "e\u0301x\n", keys: "rZ", want: "Zx\n", cur: vp(0, 0)},
		{name: "r emoji ZWJ", content: zwj + "a\n", keys: "rZ", want: "Za\n", cur: vp(0, 0)},
		{name: "r CJK", content: "日本語\n", keys: "rZ", want: "Z本語\n", cur: vp(0, 0)},
		{name: "2r CJK lands on the last replaced", content: "日本語\n", keys: "2rZ", want: "ZZ語\n", cur: vp(0, 1)},
		{name: "dw combining", content: "e\u0301x y\n", keys: "dw", want: "y\n", cur: vp(0, 0), reg: rg("e\u0301x ", false)},
		{name: "dw emoji", content: zwj + " y\n", keys: "dw", want: "y\n", cur: vp(0, 0), reg: rg(zwj+" ", false)},
		{name: "dw CJK", content: "日本語 abc\n", keys: "dw", want: "abc\n", cur: vp(0, 0), reg: rg("日本語 ", false)},
		{name: "diw combining", content: "x e\u0301 y\n", at: vp(0, 2), keys: "diw", want: "x  y\n", cur: vp(0, 2), reg: rg("e\u0301", false)},
		{name: "diw CJK", content: "a 日本語 b\n", at: vp(0, 2), keys: "diw", want: "a  b\n", cur: vp(0, 2), reg: rg("日本語", false)},
		{name: "diw emoji", content: "a " + zwj + " b\n", at: vp(0, 2), keys: "diw", want: "a  b\n", cur: vp(0, 2), reg: rg(zwj, false)},
		{name: "dl combining", content: "e\u0301e\u0301\n", keys: "dl", want: "e\u0301\n", cur: vp(0, 0), reg: rg("e\u0301", false)},
		{name: "dh CJK", content: "日本語\n", at: vp(0, 3), keys: "dh", want: "本語\n", cur: vp(0, 0), reg: rg("日", false)},
		{name: "p after a wide grapheme", content: "日本\n", keys: "p", want: "日Z本\n", cur: vp(0, 3), pre: rg("Z", false)},
	})
}
