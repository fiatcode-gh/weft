package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v3/internal/buffer"
)

// visualRange is the editor's visual selection, and whether there is one.
func visualRange(e *EditorView) (buffer.Range, bool) { return e.selection() }

func TestVimParseVisual(t *testing.T) {
	str := func(s string) []string { return strings.Split(s, "") }
	done := []struct {
		toks   []string
		visual bool
		want   vimCmd
	}{
		{str("v"), false, vimCmd{key: "v"}},
		{str("V"), false, vimCmd{key: "V"}},
		{str("v"), true, vimCmd{key: "v"}},
		{str("o"), true, vimCmd{key: "o"}},
		{str("3l"), true, vimCmd{count: 3, key: "l"}},
		{str("gg"), true, vimCmd{key: "gg"}},
		{str("fx"), true, vimCmd{key: "f", arg: "x"}},
		{str("d"), true, vimCmd{key: "d"}},
		{str("x"), true, vimCmd{key: "x"}},
		{str("c"), true, vimCmd{key: "c"}},
		{str("y"), true, vimCmd{key: "y"}},
		{str("2>"), true, vimCmd{count: 2, key: ">"}},
		{str("<"), true, vimCmd{key: "<"}},
		{str("J"), true, vimCmd{key: "J"}},
		{str("~"), true, vimCmd{key: "~"}},
		{str("u"), true, vimCmd{key: "u"}},
		{str("U"), true, vimCmd{key: "U"}},
	}
	for _, tc := range done {
		got, st := parseVim(tc.toks, tc.visual)
		if st != vimDone || got != tc.want {
			t.Errorf("parseVim(%q, %v) = %+v %v, want %+v done", tc.toks, tc.visual, got, st, tc.want)
		}
	}
	for _, toks := range [][]string{str("p"), str("r"), str("i"), str("."), str("ctrl+r"), str("dw"), str("a")} {
		if _, st := parseVim(toks, true); st != vimBad {
			t.Errorf("parseVim(%q, true) = %v, want bad", toks, st)
		}
	}
	if _, st := parseVim([]string{"~"}, false); st != vimBad {
		t.Errorf("normal-mode ~ must stay unparsed, got %v", st)
	}
}

func TestVimVisualMotions(t *testing.T) {
	const two = "abcdef\nghijkl\n"
	cases := []struct {
		name, content, keys string
		at                  buffer.Pos
		mode                vimMode
		sel                 buffer.Range
		cur                 buffer.Pos
	}{
		{"vll covers three graphemes", two, "vll", vp(0, 0), vimVisual, buffer.Range{Start: vp(0, 0), End: vp(0, 3)}, vp(0, 2)},
		{"v alone covers one", two, "v", vp(0, 2), vimVisual, buffer.Range{Start: vp(0, 2), End: vp(0, 3)}, vp(0, 2)},
		{"j extends a line", two, "vj", vp(0, 1), vimVisual, buffer.Range{Start: vp(0, 1), End: vp(1, 2)}, vp(1, 1)},
		{"w extends", "ab cd ef\n", "vw", vp(0, 0), vimVisual, buffer.Range{Start: vp(0, 0), End: vp(0, 4)}, vp(0, 3)},
		{"$ extends to the last grapheme", two, "v$", vp(0, 1), vimVisual, buffer.Range{Start: vp(0, 1), End: vp(0, 6)}, vp(0, 5)},
		{"G extends to the last line", "a\nb\nc\n", "vG", vp(0, 0), vimVisual, buffer.Range{Start: vp(0, 0), End: vp(2, 1)}, vp(2, 0)},
		{"3l extends", two, "v3l", vp(0, 0), vimVisual, buffer.Range{Start: vp(0, 0), End: vp(0, 4)}, vp(0, 3)},
		{"backwards is the same range", two, "vhh", vp(0, 3), vimVisual, buffer.Range{Start: vp(0, 1), End: vp(0, 4)}, vp(0, 1)},
		{"o swaps the ends", two, "vllo", vp(0, 0), vimVisual, buffer.Range{Start: vp(0, 0), End: vp(0, 3)}, vp(0, 0)},
		{"o then l moves the start", two, "vlloll", vp(0, 0), vimVisual, buffer.Range{Start: vp(0, 2), End: vp(0, 3)}, vp(0, 2)},
		{"esc leaves", two, "vll\x1b", vp(0, 0), vimNormal, buffer.Range{}, vp(0, 2)},
		{"v leaves", two, "vllv", vp(0, 0), vimNormal, buffer.Range{}, vp(0, 2)},
		{"V covers whole lines", two, "V", vp(0, 2), vimVisualLine, buffer.Range{Start: vp(0, 0), End: vp(0, 6)}, vp(0, 2)},
		{"Vj covers both lines", two, "Vj", vp(0, 2), vimVisualLine, buffer.Range{Start: vp(0, 0), End: vp(1, 6)}, vp(1, 2)},
		{"V esc leaves", two, "V\x1b", vp(0, 2), vimNormal, buffer.Range{}, vp(0, 2)},
		{"V V leaves", two, "VV", vp(0, 2), vimNormal, buffer.Range{}, vp(0, 2)},
		{"vV switches to lines", two, "vlV", vp(0, 1), vimVisualLine, buffer.Range{Start: vp(0, 0), End: vp(0, 6)}, vp(0, 2)},
		{"Vv switches to chars", two, "Vlv", vp(0, 1), vimVisual, buffer.Range{Start: vp(0, 1), End: vp(0, 3)}, vp(0, 2)},
		{"ctrl+r does nothing", two, "vl\x12", vp(0, 0), vimVisual, buffer.Range{Start: vp(0, 0), End: vp(0, 2)}, vp(0, 1)},
		{"p does nothing", two, "vlp", vp(0, 0), vimVisual, buffer.Range{Start: vp(0, 0), End: vp(0, 2)}, vp(0, 1)},
		{"r does nothing", two, "vlr", vp(0, 0), vimVisual, buffer.Range{Start: vp(0, 0), End: vp(0, 2)}, vp(0, 1)},
		{"i does nothing", two, "vli", vp(0, 0), vimVisual, buffer.Range{Start: vp(0, 0), End: vp(0, 2)}, vp(0, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := vimEditor(nil, tc.content, 0)
			e.buf.MoveTo(tc.at, false)
			e.goalOK = false
			// ctrl+r is the one key that is not a rune.
			keys := strings.ReplaceAll(tc.keys, "\x12", "")
			vimKeys(e, keys)
			if strings.Contains(tc.keys, "\x12") {
				e.Update(ctrlR)
			}
			if text(e) != tc.content {
				t.Fatalf("text changed to %q", text(e))
			}
			if got := vimModeOf(e); got != tc.mode {
				t.Fatalf("mode %v, want %v", got, tc.mode)
			}
			if got := cursorPos(e); got != tc.cur {
				t.Errorf("cursor %v, want %v", got, tc.cur)
			}
			r, ok := visualRange(e)
			if tc.mode == vimNormal {
				if ok {
					t.Errorf("selection %v after leaving visual", r)
				}
				return
			}
			if !ok || r != tc.sel {
				t.Errorf("selection %v %v, want %v", r, ok, tc.sel)
			}
			if _, buffered := e.buf.Selection(); buffered {
				t.Error("visual mode left an anchor in the buffer")
			}
		})
	}
}

func TestVimVisualOperators(t *testing.T) {
	const two = "alpha beta gamma\nsecond line\n"
	runOpCases(t, []opCase{
		{name: "vwd", content: two, at: vp(0, 0), keys: "vwd", want: "eta gamma\nsecond line\n", cur: vp(0, 0), reg: rg("alpha b", false)},
		{name: "vx is vd", content: two, at: vp(0, 0), keys: "vlx", want: "pha beta gamma\nsecond line\n", cur: vp(0, 0), reg: rg("al", false)},
		{name: "vjd across lines", content: two, at: vp(0, 2), keys: "vjd", want: "alond line\n", cur: vp(0, 2), reg: rg("pha beta gamma\nsec", false)},
		{name: "vec then text", content: two, at: vp(0, 0), keys: "vecX\x1b", want: "X beta gamma\nsecond line\n", cur: vp(0, 0), reg: rg("alpha", false)},
		{name: "vey then P", content: two, at: vp(0, 0), keys: "veyP", want: "alphaalpha beta gamma\nsecond line\n", cur: vp(0, 4), reg: rg("alpha", false)},
		{name: "vey leaves the cursor at the start", content: two, at: vp(0, 6), keys: "vey", want: two, cur: vp(0, 6), reg: rg("beta", false)},
		{name: "vjy puts the cursor at the start", content: two, at: vp(0, 2), keys: "vjy", want: two, cur: vp(0, 2), reg: rg("pha beta gamma\nsec", false)},
		{name: "v> shifts the line", content: "- a\n- b\n- c\n", at: vp(1, 0), keys: "v>", want: "- a\n  - b\n- c\n", cur: vp(1, 2), reg: nil},
		{name: "Vj2< shifts two levels back", content: "    - a\n    - b\n    - c\n", at: vp(0, 0), keys: "Vj2<", want: "- a\n- b\n    - c\n", cur: vp(0, 0), reg: nil},
		{name: "Vj2> shifts two levels", content: "- a\n- b\n- c\n", at: vp(0, 0), keys: "Vj2>", want: "    - a\n    - b\n- c\n", cur: vp(0, 4), reg: nil},
		{name: "charwise > touches whole lines", content: "- a\n- b\n- c\n", at: vp(0, 2), keys: "vj>", want: "  - a\n  - b\n- c\n", cur: vp(0, 2), reg: nil},
		{name: "vjJ joins two lines", content: "a\nb\nc\n", at: vp(0, 0), keys: "vjJ", want: "a b\nc\n", cur: vp(0, 1), reg: nil},
		{name: "vjjJ joins three lines", content: "a\nb\nc\n", at: vp(0, 0), keys: "vjjJ", want: "a b c\n", cur: vp(0, 3), reg: nil},
		{name: "vJ joins at least two", content: "a\nb\nc\n", at: vp(0, 0), keys: "vJ", want: "a b\nc\n", cur: vp(0, 1), reg: nil},
		{name: "v~ toggles", content: "abc\n", at: vp(0, 0), keys: "vl~", want: "ABc\n", cur: vp(0, 0), reg: nil},
		{name: "vU upper", content: "abc\n", at: vp(0, 1), keys: "vU", want: "aBc\n", cur: vp(0, 1), reg: nil},
		{name: "vlu lower", content: "ABC\n", at: vp(0, 0), keys: "vlu", want: "abC\n", cur: vp(0, 0), reg: nil},
		{name: "backwards range cases at its start", content: "abcdef\n", at: vp(0, 3), keys: "vhhU", want: "aBCDef\n", cur: vp(0, 1), reg: nil},
		{name: "V~ toggles whole lines", content: "aB\ncD\nef\n", at: vp(0, 1), keys: "Vj~", want: "Ab\nCd\nef\n", cur: vp(0, 0), reg: nil},
		{name: "combining mark is one grapheme", content: "e\u0301x\n", at: vp(0, 0), keys: "vd", want: "x\n", cur: vp(0, 0), reg: rg("e\u0301", false)},
		{name: "emoji is one grapheme", content: "😀a\n", at: vp(0, 0), keys: "vd", want: "a\n", cur: vp(0, 0), reg: rg("😀", false)},
		{name: "multi-byte U", content: "éa\n", at: vp(0, 0), keys: "vU", want: "Éa\n", cur: vp(0, 0), reg: nil},
		{name: "crlf vjd", content: "ab\r\ncd\r\n", at: vp(0, 0), keys: "vjd", want: "d\r\n", cur: vp(0, 0), reg: rg("ab\nc", false)},
		{name: "crlf Vd", content: "ab\r\ncd\r\n", at: vp(0, 0), keys: "Vd", want: "cd\r\n", cur: vp(0, 0), reg: rg("ab\n", true)},
		{name: "crlf vjJ", content: "ab\r\ncd\r\n", at: vp(0, 0), keys: "vjJ", want: "ab cd\r\n", cur: vp(0, 2), reg: nil},
	})
}

func TestVimVisualOneUndoStep(t *testing.T) {
	cases := []struct{ name, content, keys string }{
		{"vwd", "alpha beta gamma\n", "vwd"},
		{"Vjd", "a\nb\nc\n", "Vjd"},
		{"vec session", "alpha beta\n", "vecXYZ\x1b"},
		{"Vjc session", "a\nb\nc\n", "VjcXYZ\x1b"},
		{"v>", "- a\n- b\n", "vj>"},
		{"vjJ", "a\nb\nc\n", "vjjJ"},
		{"vU", "abc def\n", "vwU"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := vimEditor(nil, tc.content, 0)
			vimKeys(e, tc.keys)
			if text(e) == tc.content {
				t.Fatalf("keys %q changed nothing", tc.keys)
			}
			vimKeys(e, "u")
			if got := text(e); got != tc.content {
				t.Fatalf("one u gave %q, want %q", got, tc.content)
			}
		})
	}
}

func TestVimVisualLine(t *testing.T) {
	runOpCases(t, []opCase{
		{name: "Vd", content: "a\nb\nc\n", at: vp(1, 0), keys: "Vd", want: "a\nc\n", cur: vp(1, 0), reg: rg("b\n", true)},
		{name: "Vjd", content: "a\nb\nc\nd\n", at: vp(1, 0), keys: "Vjd", want: "a\nd\n", cur: vp(1, 0), reg: rg("b\nc\n", true)},
		{name: "Vjy then p is linewise", content: "a\nb\nc\n", at: vp(0, 0), keys: "Vjyp", want: "a\na\nb\nb\nc\n", cur: vp(1, 0), reg: rg("a\nb\n", true)},
		{name: "Vc keeps the indent", content: "  foo\nbar\n", at: vp(0, 3), keys: "VcX\x1b", want: "  X\nbar\n", cur: vp(0, 2), reg: rg("  foo\n", true)},
		{name: "V on the last line", content: "a\nb\n", at: vp(1, 0), keys: "Vd", want: "a\n", cur: vp(0, 0), reg: rg("b\n", true)},
		{name: "Vj on the last line stays put", content: "a\nb\n", at: vp(1, 0), keys: "Vjd", want: "a\n", cur: vp(0, 0), reg: rg("b\n", true)},
		{name: "Vj> moves only the lines", content: "- a\n- b\n  - child\n", at: vp(0, 0), keys: "Vj>", want: "  - a\n  - b\n  - child\n", cur: vp(0, 2), reg: nil},
		{name: "V> on a parent leaves its children", content: "- a\n  - child\n", at: vp(0, 0), keys: "V>", want: "  - a\n  - child\n", cur: vp(0, 2), reg: nil},
		{name: "Vk goes up", content: "a\nb\nc\n", at: vp(2, 0), keys: "Vkd", want: "a\n", cur: vp(0, 0), reg: rg("b\nc\n", true)},
	})
}

// frameBody is the editor's frame without its status row, plain and raw.
func frameBody(e *EditorView) (plainRows, raw string) {
	drop := func(s string) string {
		rows := strings.Split(s, "\n")
		return strings.Join(rows[:len(rows)-1], "\n")
	}
	v := e.View()
	return drop(plain(v)), drop(v)
}

func TestVimVisualDraw(t *testing.T) {
	quietTerm(t)
	cases := []struct {
		name, content string
		vimKeys       string
		modeless      []tea.KeyPressMsg
	}{
		{"vll", "abcdef\nghijkl\n", "vll", []tea.KeyPressMsg{shiftKey(tea.KeyRight), shiftKey(tea.KeyRight), shiftKey(tea.KeyRight)}},
		{"vjl across lines", "abc\ndef\nghi\n", "lvjl", []tea.KeyPressMsg{named(tea.KeyRight), shiftKey(tea.KeyDown), shiftKey(tea.KeyRight), shiftKey(tea.KeyRight)}},
		{"Vj", "abc\ndef\nghi\n", "Vj", []tea.KeyPressMsg{shiftKey(tea.KeyDown), shiftKey(tea.KeyEnd)}},
		{"V", "abc\ndef\nghi\n", "V", []tea.KeyPressMsg{shiftKey(tea.KeyEnd)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref := editorAt(nil, "P", "/tmp/p.md", tc.content, false, 80, 24, 0)
			press(ref, tc.modeless...)
			wantPlain, wantRaw := frameBody(ref)

			e := vimEditor(nil, tc.content, 0)
			vimKeys(e, tc.vimKeys)
			gotPlain, gotRaw := frameBody(e)
			if gotPlain != wantPlain {
				t.Errorf("plain frame differs from the modeless selection:\n%s\nwant:\n%s", gotPlain, wantPlain)
			}
			if gotRaw != wantRaw {
				t.Errorf("styled frame differs from the modeless selection:\n%q\nwant:\n%q", gotRaw, wantRaw)
			}
		})
	}
}

func TestVimVisualDot(t *testing.T) {
	runRepCases(t, []repCase{
		{name: "vlld then dot", content: "abcdefghij\n", keys: "vlld.", want: "ghij\n"},
		{name: "Vjd then dot", content: "1\n2\n3\n4\n5\n", keys: "Vjd.", want: "5\n"},
		{name: "dot clamps to the last line", content: "1\n2\n3\n", keys: "Vjd.", want: ""},
		{name: "vjd multi-line then dot", content: "abcd\nefgh\nijkl\nmnop\n", keys: "lvjd.", want: "akl\nmnop\n"},
		{name: "vU then dot", content: "abc\n", keys: "vUl.", want: "ABc\n"},
		{name: "v~ then dot", content: "abcd\n", keys: "vl~ll.", want: "ABCD\n"},
		{name: "vc then dot", content: "abc def\n", keys: "vcX\x1bw.", want: "Xbc Xef\n"},
		{name: "v> then dot", content: "- a\n- b\n", keys: "v>j.", want: "  - a\n  - b\n"},
		{name: "vjJ then dot", content: "a\nb\nc\nd\ne\n", keys: "vjJj.", want: "a b\nc d\ne\n"},
		{name: "yank is not recorded", content: "abcd\nefgh\n", keys: "vlxvly.", want: "\nefgh\n"},
	})
	t.Run("dot is one undo step", func(t *testing.T) {
		e := vimEditor(nil, "abcdefghij\n", 0)
		vimKeys(e, "vlld.u")
		if got := text(e); got != "defghij\n" {
			t.Fatalf("text %q, want defghij", got)
		}
	})
	t.Run("a count replaces the stored one", func(t *testing.T) {
		e := vimEditor(nil, "    - a\n    - b\n", 0)
		vimKeys(e, "v2<")
		vimKeys(e, "j")
		if got := text(e); got != "- a\n    - b\n" {
			t.Fatalf("text %q after v2<", got)
		}
		vimKeys(e, "1.")
		if got := text(e); got != "- a\n  - b\n" {
			t.Fatalf("text %q after 1.", got)
		}
	})
}

func TestVimVisualPasteLeavesVisual(t *testing.T) {
	e := vimEditor(nil, "abc\n", 0)
	vimKeys(e, "vl")
	e.Paste(tea.PasteMsg{Content: "Z"})
	if vimModeOf(e) != vimNormal {
		t.Fatalf("mode %v, want normal", vimModeOf(e))
	}
	if _, ok := visualRange(e); ok {
		t.Fatal("selection survived the paste")
	}
	if !strings.Contains(text(e), "Z") {
		t.Fatalf("paste lost: %q", text(e))
	}
}

func TestVimVisualCtrlS(t *testing.T) {
	e := vimEditor(nil, "abc\n", 0)
	vimKeys(e, "vl")
	res, _ := e.Update(ctrlS)
	if !res.Save {
		t.Fatal("ctrl+s did not save in visual mode")
	}
	if vimModeOf(e) != vimVisual {
		t.Fatalf("mode %v, want visual after saving", vimModeOf(e))
	}
}
