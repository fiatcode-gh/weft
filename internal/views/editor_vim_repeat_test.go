package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// repCase runs keys on content and expects text want.
type repCase struct {
	name, content, keys, want string
	reg                       *register
}

func runRepCases(t *testing.T, cases []repCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := vimEditor(nil, tc.content, 0)
			if tc.reg != nil {
				*e.reg = *tc.reg
			}
			vimKeys(e, tc.keys)
			if got := text(e); got != tc.want {
				t.Fatalf("keys %q: text %q, want %q", tc.keys, got, tc.want)
			}
			if vimModeOf(e) != vimNormal {
				t.Fatalf("keys %q: mode %v, want normal", tc.keys, vimModeOf(e))
			}
		})
	}
}

func TestVimDotRepeat(t *testing.T) {
	runRepCases(t, []repCase{
		{name: "dw", content: "a b c d\n", keys: "dw.", want: "c d\n"},
		{name: "cw", content: "aa bb cc\n", keys: "cwX\x1bw.", want: "X X cc\n"},
		{name: "x twice", content: "abcd\n", keys: "x..", want: "d\n"},
		{name: "3x then counts replace", content: "abcdefghijklmnop\n", keys: "3x.", want: "ghijklmnop\n"},
		{name: "2. replaces the count", content: "abcdefghijklmnop\n", keys: "3x.2.", want: "ijklmnop\n"},
		{name: "stored count sticks", content: "abcdefghijklmnop\n", keys: "3x.2..", want: "klmnop\n"},
		{name: "dd", content: "1\n2\n3\n4\n", keys: "ddj.", want: "2\n4\n"},
		{name: "indent", content: "- a\n- b\n", keys: ">>j.", want: "  - a\n  - b\n"},
		{name: "join", content: "a\nb\nc\n", keys: "J.", want: "a b c\n"},
		{name: "put", content: "abc\n", keys: "p.", want: "aZZbc\n", reg: rg("Z", false)},
		{name: "append", content: "a\nb\n", keys: "A;\x1bj.", want: "a;\nb;\n"},
		{name: "open line", content: "a\n", keys: "oX\x1b.", want: "a\nX\nX\n"},
		{name: "ciw", content: "foo bar\n", keys: "ciwX\x1bw.", want: "X X\n"},
		{name: "replace", content: "abc\n", keys: "rxl.", want: "xxc\n"},
		{name: "before any change", content: "abc\n", keys: ".", want: "abc\n"},
		{name: "yank is not recorded", content: "a b c d\n", keys: "dwyw.", want: "c d\n"},
		{name: "undo is not recorded", content: "abc\n", keys: "xu.", want: "bc\n"},
	})
	t.Run("each dot is one undo step", func(t *testing.T) {
		e := vimEditor(nil, "abcd\n", 0)
		vimKeys(e, "x..")
		if got := text(e); got != "d\n" {
			t.Fatalf("text %q, want d", got)
		}
		vimKeys(e, "u")
		if got := text(e); got != "cd\n" {
			t.Fatalf("one u gave %q, want cd", got)
		}
	})
	t.Run("a dot with an insert is one undo step", func(t *testing.T) {
		e := vimEditor(nil, "a\nb\n", 0)
		vimKeys(e, "A;\x1bj.u")
		if got := text(e); got != "a;\nb\n" {
			t.Fatalf("text %q, want the dot undone as one", got)
		}
	})
}

func TestVimCountedInsert(t *testing.T) {
	runRepCases(t, []repCase{
		{name: "3i", content: "a\n", keys: "3ix\x1b", want: "xxxa\n"},
		{name: "2o", content: "a\n", keys: "2oab\x1b", want: "a\nab\nab\n"},
		{name: "3A", content: "a\n", keys: "3A!\x1b", want: "a!!!\n"},
		{name: "2I", content: "  a\n", keys: "2Ia\x1b", want: "  aaa\n"},
		{name: "dot repeats the count", content: "a\n", keys: "3ix\x1b.", want: "xxxxxxa\n"},
		{name: "2O", content: "a\n", keys: "2Ob\x1b", want: "b\nb\na\n"},
	})
	t.Run("cursor on the last inserted character", func(t *testing.T) {
		e := vimEditor(nil, "a\n", 0)
		vimKeys(e, "3ix\x1b")
		if got := cursorPos(e); got != vp(0, 2) {
			t.Fatalf("cursor %v, want line 0 col 2", got)
		}
	})
	t.Run("one undo step", func(t *testing.T) {
		e := vimEditor(nil, "a\n", 0)
		vimKeys(e, "3ix\x1bu")
		if got := text(e); got != "a\n" {
			t.Fatalf("text %q, want the whole insert undone", got)
		}
		e = vimEditor(nil, "a\n", 0)
		vimKeys(e, "2oab\x1bu")
		if got := text(e); got != "a\n" {
			t.Fatalf("2o undo: text %q, want a", got)
		}
	})
}

func TestVimInsertRecording(t *testing.T) {
	t.Run("backspace enter and tab replay identically", func(t *testing.T) {
		e := vimEditor(nil, "- a\n- b\n", 0)
		vimKeys(e, "Ax")
		e.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		vimKeys(e, "y")
		e.Update(enterKey)
		vimKeys(e, "z")
		e.Update(tabKey)
		vimKeys(e, "\x1b")
		vimKeys(e, "G.")
		got := text(e)
		want := "- ay\n  - z\n- by\n  - z\n"
		if got != want {
			t.Fatalf("text %q, want %q", got, want)
		}
	})
	t.Run("ctrl+s is not replayed", func(t *testing.T) {
		e := vimEditor(nil, "a\nb\n", 0)
		vimKeys(e, "Ax")
		if res, _ := e.Update(ctrl('s')); !res.Save {
			t.Fatal("live ctrl+s did not ask to save")
		}
		vimKeys(e, "y\x1b")
		for _, in := range e.vim.last.inputs {
			if in.key.String() == "ctrl+s" {
				t.Fatal("ctrl+s was recorded")
			}
		}
		e.Update(key("j"))
		res, _ := e.Update(key("."))
		if res.Save {
			t.Fatal("dot saved")
		}
		if got := text(e); got != "axy\nbxy\n" {
			t.Fatalf("text %q, want both lines with xy", got)
		}
	})
	t.Run("excluded chords", func(t *testing.T) {
		e := vimEditor(nil, "a\n", 0)
		vimKeys(e, "A")
		for _, m := range []tea.KeyPressMsg{ctrl('f'), {Code: 's', Mod: tea.ModAlt}, {Code: 'e', Mod: tea.ModAlt}} {
			e.Update(m)
			if e.find != nil {
				e.Update(escKey)
			}
			e.prompt = nil
		}
		vimKeys(e, "\x1b")
		if e.vim.last == nil || len(e.vim.last.inputs) != 0 {
			t.Fatalf("recorded inputs %v, want none", e.vim.last)
		}
	})
	t.Run("bracketed paste is replayed", func(t *testing.T) {
		e := vimEditor(nil, "a\nb\n", 0)
		vimKeys(e, "A")
		e.Paste(tea.PasteMsg{Content: "P1"})
		vimKeys(e, "\x1bj.")
		if got := text(e); got != "aP1\nbP1\n" {
			t.Fatalf("text %q, want the paste on both lines", got)
		}
	})
	t.Run("completion acceptance replays", func(t *testing.T) {
		e := vimEditor(loadFixture(t), "x\ny\n", 0)
		vimKeys(e, "A[[Alp")
		if !e.completer.active {
			t.Fatal("the completion strip did not open")
		}
		e.Update(enterKey)
		vimKeys(e, "\x1b")
		live := text(e)
		vimKeys(e, "j.")
		if live != "x[[Alpha]]\ny\n" || text(e) != "x[[Alpha]]\ny[[Alpha]]\n" {
			t.Fatalf("live %q, after dot %q", live, text(e))
		}
	})
}
