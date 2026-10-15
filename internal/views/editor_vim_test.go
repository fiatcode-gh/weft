package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

var ctrlR = tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}

// vimEditor opens an editor with vim modal editing on, in the look the suite
// runs in, with the cursor on line.
func vimEditor(idx *graph.Index, content string, line int) *EditorView {
	e := editorAt(idx, "P", "/tmp/p.md", content, false, 80, 24, line)
	e.enableVim()
	return e
}

// vimType sends each rune of s as a typed key.
func vimType(e *EditorView, s string) {
	for _, r := range s {
		e.Update(key(string(r)))
	}
}

// openVimApp boots a throwaway graph and opens the editor on page with vim
// opted in.
func openVimApp(t *testing.T, files map[string]string, page string) *App {
	t.Helper()
	a := bootApp(t, bootConfig{files: files})
	a.SetVimEditor(true)
	a.navigate(page)
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatalf("editor did not open on %q", page)
	}
	return a
}

// vimModeOf is the editor's vim mode, or -1 when it is modeless.
func vimModeOf(e *EditorView) vimMode {
	if e.vim == nil {
		return -1
	}
	return e.vim.mode
}

func TestVimOffByDefault(t *testing.T) {
	a, _ := openJournal(t)
	e := a.editor
	if e.vim != nil {
		t.Fatal("vim is on without SetVimEditor")
	}
	typeApp(a, "iQ")
	if !strings.Contains(e.Content(), "iQ") {
		t.Fatalf("typing iQ must insert it, got %q", e.Content())
	}
	look := "preview"
	if e.source {
		look = "source"
	}
	other := map[string]string{"preview": "source", "source": "preview"}[look]
	want := journalPage + " [edit · " + look + "] ●  ^S save · ^F find · ^R " + other + " · esc exit"
	if got := plain(e.statusLine()); got != want {
		t.Fatalf("status %q, want %q", got, want)
	}
}

func TestVimAppOptIn(t *testing.T) {
	a := openVimApp(t, map[string]string{journalPath: journalBody}, journalPage)
	e := a.editor
	if vimModeOf(e) != vimNormal {
		t.Fatalf("editor must open in vim normal mode, got %v", vimModeOf(e))
	}
	before := e.Content()
	typeApp(a, "i")
	typeApp(a, "Q")
	a.Update(escKey)
	got := e.Content()
	if strings.Count(got, "Q") != 1 || strings.Contains(got, "iQ") || len(got) != len(before)+1 {
		t.Fatalf("iQ esc must insert only Q: %q -> %q", before, got)
	}
}

func TestVimStartsInNormal(t *testing.T) {
	e := vimEditor(nil, "abc\n", 0)
	if vimModeOf(e) != vimNormal {
		t.Fatal("not in normal mode")
	}
	vimType(e, "zQ")
	if text(e) != "abc\n" {
		t.Fatalf("normal mode typed text: %q", text(e))
	}
	if s := plain(e.statusLine()); strings.Contains(s, "--") {
		t.Fatalf("normal mode shows a tag: %q", s)
	}
}

func TestVimInsertEntry(t *testing.T) {
	const bullet = "  - foo bar\n"
	cases := []struct {
		name, content string
		line, col     int // cursor, rune column
		key           string
		want          string
		wantPos       buffer.Pos
	}{
		{"bullet i start", bullet, 0, 0, "i", "Z  - foo bar\n", buffer.Pos{Line: 0, Col: 0}},
		{"bullet a start", bullet, 0, 0, "a", " Z - foo bar\n", buffer.Pos{Line: 0, Col: 1}},
		{"bullet i middle", bullet, 0, 6, "i", "  - foZo bar\n", buffer.Pos{Line: 0, Col: 6}},
		{"bullet a middle", bullet, 0, 6, "a", "  - fooZ bar\n", buffer.Pos{Line: 0, Col: 7}},
		{"bullet a end", bullet, 0, 10, "a", "  - foo barZ\n", buffer.Pos{Line: 0, Col: 11}},
		{"bullet I", bullet, 0, 6, "I", "  Z- foo bar\n", buffer.Pos{Line: 0, Col: 2}},
		{"bullet A", bullet, 0, 6, "A", "  - foo barZ\n", buffer.Pos{Line: 0, Col: 11}},
		{"bullet o", bullet, 0, 6, "o", "  - foo bar\n  - Z\n", buffer.Pos{Line: 1, Col: 4}},
		{"bullet O", bullet, 0, 6, "O", "  - Z\n  - foo bar\n", buffer.Pos{Line: 0, Col: 4}},
		{"abc i", "abc", 0, 0, "i", "Zabc", buffer.Pos{Line: 0, Col: 0}},
		{"abc a middle", "abc", 0, 1, "a", "abZc", buffer.Pos{Line: 0, Col: 2}},
		{"abc a end", "abc", 0, 2, "a", "abcZ", buffer.Pos{Line: 0, Col: 3}},
		{"abc I", "abc", 0, 1, "I", "Zabc", buffer.Pos{Line: 0, Col: 0}},
		{"abc A", "abc", 0, 1, "A", "abcZ", buffer.Pos{Line: 0, Col: 3}},
		{"abc o", "abc", 0, 1, "o", "abc\nZ", buffer.Pos{Line: 1, Col: 0}},
		{"abc O", "abc", 0, 1, "O", "Z\nabc", buffer.Pos{Line: 0, Col: 0}},
		{"empty i", "", 0, 0, "i", "Z", buffer.Pos{Line: 0, Col: 0}},
		{"empty a", "", 0, 0, "a", "Z", buffer.Pos{Line: 0, Col: 0}},
		{"empty I", "", 0, 0, "I", "Z", buffer.Pos{Line: 0, Col: 0}},
		{"empty A", "", 0, 0, "A", "Z", buffer.Pos{Line: 0, Col: 0}},
		{"empty o", "", 0, 0, "o", "\nZ", buffer.Pos{Line: 1, Col: 0}},
		{"empty O", "", 0, 0, "O", "Z\n", buffer.Pos{Line: 0, Col: 0}},
		{"crlf i", "\tx\r\ny\r\n", 0, 0, "i", "Z\tx\r\ny\r\n", buffer.Pos{Line: 0, Col: 0}},
		{"crlf A", "\tx\r\ny\r\n", 0, 1, "A", "\txZ\r\ny\r\n", buffer.Pos{Line: 0, Col: 2}},
		{"crlf o keeps indent", "\tx\r\ny\r\n", 0, 1, "o", "\tx\r\n\tZ\r\ny\r\n", buffer.Pos{Line: 1, Col: 1}},
		{"crlf O", "\tx\r\ny\r\n", 1, 0, "O", "\tx\r\nZ\r\ny\r\n", buffer.Pos{Line: 1, Col: 0}},
		{"crlf I", "\tx\r\ny\r\n", 1, 0, "I", "\tx\r\nZy\r\n", buffer.Pos{Line: 1, Col: 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := editorAt(nil, "P", "/tmp/p.md", c.content, false, 80, 24, 0)
			setCursor(e, c.line, c.col)
			e.enableVim()
			setCursor(e, c.line, c.col)
			vimType(e, c.key)
			if vimModeOf(e) != vimInsert {
				t.Fatalf("%s did not enter insert mode", c.key)
			}
			vimType(e, "Z")
			e.Update(escKey)
			if vimModeOf(e) != vimNormal {
				t.Fatal("esc did not return to normal mode")
			}
			if text(e) != c.want || cursorPos(e) != c.wantPos {
				t.Fatalf("text %q cursor %v; want %q %v", text(e), cursorPos(e), c.want, c.wantPos)
			}
		})
	}
}

func TestVimEscMovesLeft(t *testing.T) {
	cases := []struct {
		name, content, keys string
		want                buffer.Pos
	}{
		{"A then esc lands on the last character", "abc", "A", buffer.Pos{Line: 0, Col: 2}},
		{"i then esc at column 0 stays", "abc", "i", buffer.Pos{Line: 0, Col: 0}},
		{"combining mark lands on the grapheme start", "ae\u0301", "A", buffer.Pos{Line: 0, Col: 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := vimEditor(nil, c.content, 0)
			vimType(e, c.keys)
			e.Update(escKey)
			if got := cursorPos(e); got != c.want {
				t.Fatalf("cursor %v, want %v", got, c.want)
			}
		})
	}
}

func TestVimInsertSessionOneUndoStep(t *testing.T) {
	const orig = "- foo\n"
	e := vimEditor(nil, orig, 0)
	vimType(e, "o")
	vimType(e, "abc")
	press(e, named(tea.KeyBackspace), named(tea.KeyEnter))
	vimType(e, "d")
	press(e, tabKey, escKey)
	session := text(e)
	if session == orig {
		t.Fatal("the session changed nothing")
	}
	vimType(e, "u")
	if text(e) != orig || cursorPos(e).Line != 0 {
		t.Fatalf("one u must restore %q on line 0, got %q at %v", orig, text(e), cursorPos(e))
	}
	e.Update(ctrlR)
	if text(e) != session {
		t.Fatalf("ctrl+r must redo the session: %q, want %q", text(e), session)
	}
}

func TestVimInsertUndoKeys(t *testing.T) {
	const orig = "base\n"
	e := vimEditor(nil, orig, 0)
	vimType(e, "A")
	vimType(e, "ab")
	e.Update(ctrlZ)
	if text(e) != orig || vimModeOf(e) != vimInsert {
		t.Fatalf("ctrl+z in insert: %q mode %v", text(e), vimModeOf(e))
	}
	vimType(e, "c")
	e.Update(escKey)
	vimType(e, "u")
	if text(e) != orig {
		t.Fatalf("u must remove only c: %q", text(e))
	}
	for _, redo := range []tea.KeyPressMsg{ctrlY, ctrlR} {
		e := vimEditor(nil, orig, 0)
		vimType(e, "A")
		vimType(e, "ab")
		e.Update(ctrlZ)
		e.Update(redo)
		if text(e) != "baseab\n" || vimModeOf(e) != vimInsert {
			t.Fatalf("%v in insert must redo: %q mode %v", redo, text(e), vimModeOf(e))
		}
	}
}

func TestVimMergeSplitsSession(t *testing.T) {
	e := vimEditor(nil, "base\n", 0)
	vimType(e, "A")
	vimType(e, "ab")
	e.replaceBuffer(text(e)+"out\n", 0, 6)
	vimType(e, "c")
	e.Update(escKey)
	if text(e) != "baseabc\nout\n" {
		t.Fatalf("unexpected text %q", text(e))
	}
	for _, want := range []string{"baseab\nout\n", "baseab\n", "base\n"} {
		vimType(e, "u")
		if text(e) != want {
			t.Fatalf("after u: %q, want %q", text(e), want)
		}
	}
}

func TestVimEscRules(t *testing.T) {
	t.Run("clean normal exits", func(t *testing.T) {
		e := vimEditor(nil, "a\n", 0)
		res, _ := e.Update(escKey)
		if !res.Exit {
			t.Fatal("esc on a clean buffer must exit")
		}
	})
	t.Run("dirty normal asks", func(t *testing.T) {
		e := vimEditor(nil, "a\n", 0)
		vimType(e, "i")
		vimType(e, "Z")
		e.Update(escKey)
		res, _ := e.Update(escKey)
		if res.Exit || e.mode != confirmingExit {
			t.Fatalf("esc after a change must confirm: exit %v mode %v", res.Exit, e.mode)
		}
	})
	t.Run("esc clears a pending count and does not exit", func(t *testing.T) {
		e := vimEditor(nil, "a\n", 0)
		vimType(e, "2")
		if len(e.vim.pending) != 1 {
			t.Fatalf("pending %q, want the count", e.vim.pending)
		}
		res, _ := e.Update(escKey)
		if res.Exit || len(e.vim.pending) != 0 || vimModeOf(e) != vimNormal {
			t.Fatalf("esc: exit %v pending %q mode %v", res.Exit, e.vim.pending, vimModeOf(e))
		}
		vimType(e, "j")
		if got := e.buf.Cursor(); got != (buffer.Pos{}) {
			t.Fatalf("the cleared count still applied: cursor %v", got)
		}
	})
	t.Run("insert esc closes the strip and leaves insert", func(t *testing.T) {
		e := vimEditor(loadFixture(t), "x\n", 0)
		vimType(e, "A")
		vimType(e, "[[A")
		if !e.completer.active {
			t.Fatal("the completion strip did not open")
		}
		e.Update(escKey)
		if e.completer.active || vimModeOf(e) != vimNormal {
			t.Fatalf("one esc must close the strip and leave insert: active %v mode %v", e.completer.active, vimModeOf(e))
		}
	})
}

func TestVimWeftKeysNormal(t *testing.T) {
	shiftRight := tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift}
	type step struct {
		name    string
		content string
		line    int
		keys    []tea.KeyPressMsg
		check   func(t *testing.T, e *EditorView, res EditorResult, cmd tea.Cmd)
	}
	steps := []step{
		{"ctrl+t", "- task\n", 0, []tea.KeyPressMsg{ctrl('t')}, func(t *testing.T, e *EditorView, _ EditorResult, _ tea.Cmd) {
			if text(e) != "- TODO task\n" {
				t.Fatalf("text %q", text(e))
			}
		}},
		{"alt+p", "- TODO task\n", 0, []tea.KeyPressMsg{altKey('p')}, func(t *testing.T, e *EditorView, _ EditorResult, _ tea.Cmd) {
			if !strings.Contains(text(e), "[#") {
				t.Fatalf("no priority: %q", text(e))
			}
		}},
		{"alt+down", "- a\n- b\n", 0, []tea.KeyPressMsg{altDownKey}, func(t *testing.T, e *EditorView, _ EditorResult, _ tea.Cmd) {
			if text(e) != "- b\n- a\n" {
				t.Fatalf("text %q", text(e))
			}
		}},
		{"ctrl+a", "a\nb\n", 0, []tea.KeyPressMsg{ctrl('a')}, func(t *testing.T, e *EditorView, _ EditorResult, _ tea.Cmd) {
			if _, ok := e.buf.Selection(); !ok || cursorPos(e) != e.buf.End() {
				t.Fatalf("select all: cursor %v selection %v", cursorPos(e), ok)
			}
		}},
		{"shift+right", "abc\n", 0, []tea.KeyPressMsg{shiftRight}, func(t *testing.T, e *EditorView, _ EditorResult, _ tea.Cmd) {
			if _, ok := e.buf.Selection(); !ok {
				t.Fatal("no selection")
			}
		}},
		{"ctrl+c", "abc\n", 0, []tea.KeyPressMsg{ctrl('a'), ctrl('c')}, func(t *testing.T, e *EditorView, _ EditorResult, cmd tea.Cmd) {
			if e.reg.text == "" || cmd == nil {
				t.Fatalf("copy: register %q cmd %v", e.reg.text, cmd)
			}
		}},
		{"ctrl+v", "abc\n", 0, []tea.KeyPressMsg{ctrl('v')}, func(t *testing.T, e *EditorView, _ EditorResult, _ tea.Cmd) {
			if text(e) != "Qabc\n" {
				t.Fatalf("paste: %q", text(e))
			}
		}},
		{"ctrl+s", "abc\n", 0, []tea.KeyPressMsg{ctrlS}, func(t *testing.T, e *EditorView, res EditorResult, _ tea.Cmd) {
			if !res.Save {
				t.Fatal("no save")
			}
		}},
		{"ctrl+f", "abc\n", 0, []tea.KeyPressMsg{ctrl('f')}, func(t *testing.T, e *EditorView, _ EditorResult, _ tea.Cmd) {
			if e.find == nil {
				t.Fatal("no find bar")
			}
		}},
		{"ctrl+z ctrl+y", "abc\n", 0, []tea.KeyPressMsg{key("i"), key("Z"), escKey, ctrlZ}, func(t *testing.T, e *EditorView, _ EditorResult, _ tea.Cmd) {
			if text(e) != "abc\n" {
				t.Fatalf("undo: %q", text(e))
			}
			e.Update(ctrlY)
			if text(e) != "Zabc\n" {
				t.Fatalf("redo: %q", text(e))
			}
		}},
		{"alt+s", "- TODO abc\n", 0, []tea.KeyPressMsg{altKey('s')}, func(t *testing.T, e *EditorView, _ EditorResult, _ tea.Cmd) {
			if e.prompt == nil {
				t.Fatal("no date prompt")
			}
		}},
		{"pgdown", strings.Repeat("line\n", 60), 0, []tea.KeyPressMsg{named(tea.KeyPgDown)}, func(t *testing.T, e *EditorView, _ EditorResult, _ tea.Cmd) {
			if cursorPos(e).Line == 0 {
				t.Fatal("cursor did not move")
			}
		}},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			e := vimEditor(nil, s.content, s.line)
			if s.name == "ctrl+v" {
				e.reg.text = "Q"
			}
			var res EditorResult
			var cmd tea.Cmd
			for _, k := range s.keys {
				res, cmd = e.Update(k)
			}
			s.check(t, e, res, cmd)
			if vimModeOf(e) != vimNormal {
				t.Fatalf("mode %v, want normal", vimModeOf(e))
			}
		})
	}
}

func TestVimWeftKeysInsert(t *testing.T) {
	t.Run("tab indents a bullet", func(t *testing.T) {
		e := vimEditor(nil, "- a\n", 0)
		vimType(e, "A")
		e.Update(tabKey)
		if text(e) != "  - a\n" {
			t.Fatalf("text %q", text(e))
		}
	})
	t.Run("enter continues a bullet", func(t *testing.T) {
		e := vimEditor(nil, "- a\n", 0)
		vimType(e, "A")
		e.Update(named(tea.KeyEnter))
		if text(e) != "- a\n- \n" {
			t.Fatalf("text %q", text(e))
		}
	})
	t.Run("[[ opens the strip", func(t *testing.T) {
		e := vimEditor(loadFixture(t), "x\n", 0)
		vimType(e, "A")
		vimType(e, "[[A")
		if !e.completer.active {
			t.Fatal("no strip")
		}
	})
	t.Run("ctrl+d deletes forward", func(t *testing.T) {
		e := vimEditor(nil, "abc\n", 0)
		vimType(e, "i")
		e.Update(ctrl('d'))
		if text(e) != "bc\n" {
			t.Fatalf("text %q", text(e))
		}
	})
	t.Run("ctrl+u deletes to line start", func(t *testing.T) {
		e := vimEditor(nil, "abc\n", 0)
		vimType(e, "A")
		e.Update(ctrl('u'))
		if text(e) != "\n" {
			t.Fatalf("text %q", text(e))
		}
	})
}

func TestVimCtrlRRedo(t *testing.T) {
	e := vimEditor(nil, "abc\n", 0)
	src := e.source
	vimType(e, "i")
	vimType(e, "Z")
	e.Update(escKey)
	vimType(e, "u")
	if text(e) != "abc\n" {
		t.Fatalf("undo: %q", text(e))
	}
	e.Update(ctrlR)
	if text(e) != "Zabc\n" || e.source != src {
		t.Fatalf("ctrl+r: text %q source %v (was %v)", text(e), e.source, src)
	}
}

func TestVimNormalClamp(t *testing.T) {
	open := func(content string, line, col int) *EditorView {
		e := editorAt(nil, "P", "/tmp/p.md", content, false, 80, 24, 0)
		setCursor(e, line, col)
		e.enableVim()
		return e
	}
	if e := open("abc\n", 0, 3); cursorPos(e) != (buffer.Pos{Line: 0, Col: 2}) {
		t.Errorf("line end opens on %v, want the last grapheme", cursorPos(e))
	}
	if e := open("a\nb\n", 2, 0); cursorPos(e) != (buffer.Pos{Line: 1, Col: 0}) {
		t.Errorf("final empty line opens on %v, want line 1", cursorPos(e))
	}
	e := vimEditor(nil, "\nabc\n", 0)
	vimType(e, "A")
	e.Update(escKey)
	if cursorPos(e) != (buffer.Pos{}) {
		t.Errorf("A esc on an empty line: %v", cursorPos(e))
	}
	e = vimEditor(nil, "abc\n", 0)
	vimType(e, "A")
	vimType(e, "Z")
	e.Update(escKey)
	vimType(e, "u")
	if cursorPos(e) != (buffer.Pos{Line: 0, Col: 2}) {
		t.Errorf("u restoring a past-end cursor: %v", cursorPos(e))
	}
}

func TestVimCursorShape(t *testing.T) {
	e := vimEditor(nil, "abc\n", 0)
	c := e.Cursor()
	if c.Shape != tea.CursorBlock || !c.Blink {
		t.Errorf("normal cursor %+v, want blinking block", c)
	}
	vimType(e, "i")
	if c := e.Cursor(); c.Shape != tea.CursorBar {
		t.Errorf("insert cursor shape %v, want bar", c.Shape)
	}
	m := editorAt(nil, "P", "/tmp/p.md", "abc\n", false, 80, 24, 0)
	mc := m.Cursor()
	if *mc != *tea.NewCursor(mc.Position.X, mc.Position.Y) {
		t.Errorf("modeless cursor %+v is not the default", mc)
	}
}

func TestVimStatusTagsAndHints(t *testing.T) {
	e := vimEditor(nil, "abc\n", 0)
	other := "preview"
	if !e.source {
		other = "source"
	}
	if s := plain(e.statusLine()); !strings.HasSuffix(s, ":w save · :q quit · :preview "+other) {
		t.Errorf("normal status %q", s)
	}
	vimType(e, "i")
	if s := plain(e.statusLine()); !strings.Contains(s, "  -- INSERT --  ") || !strings.HasSuffix(s, "esc normal · ^S save") {
		t.Errorf("insert status %q", s)
	}
	e.Update(escKey)
	if e.vim == nil {
		t.Fatal("modeless editor")
	}
	e.vim.pending = []string{"2", "d"}
	if s := plain(e.statusLine()); !strings.HasSuffix(s, "2d") {
		t.Errorf("pending status %q", s)
	}
}

func TestEditorVimStatusGolden(t *testing.T) {
	e := sourceEditorAt(nil, "Alpha", "/tmp/a.md", "# Alpha\n\n- one\n", false, 80, 10, 0)
	e.enableVim()
	last := func() string {
		rows := strings.Split(plain(e.View()), "\n")
		return rows[len(rows)-1]
	}
	var out []string
	out = append(out, "normal: "+last())
	vimType(e, "2g")
	out = append(out, "pending 2g: "+last())
	e.Update(escKey)
	vimType(e, "2d")
	out = append(out, "pending 2d: "+last())
	e.Update(escKey)
	vimType(e, "i")
	out = append(out, "insert: "+last())
	e.Update(escKey)
	vimType(e, "v")
	out = append(out, "visual: "+last())
	e.Update(escKey)
	vimType(e, "V")
	out = append(out, "visual line: "+last())
	teatest.RequireEqualOutput(t, []byte(strings.Join(out, "\n")))
}
