package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
)

var backspaceKey = tea.KeyPressMsg{Code: tea.KeyBackspace}

// exEditor opens a vim editor whose path is in a temp dir, so a test can prove
// the editor never creates the file itself.
func exEditor(t *testing.T, content string) (*EditorView, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p.md")
	e := editorAt(nil, "P", path, content, false, 80, 24, 0)
	e.enableVim()
	return e, path
}

// exRun types ":" + line and Enter into e and returns the Enter's result.
func exRun(e *EditorView, line string) (EditorResult, tea.Cmd) {
	vimType(e, ":"+line)
	return e.Update(enterKey)
}

// exDirty makes the buffer differ from its baseline.
func exDirty(e *EditorView) {
	vimType(e, "iX")
	e.Update(escKey)
}

func TestVimExCommands(t *testing.T) {
	const body = "- one\n- two\n  - three\n- four\n"
	t.Run("w", func(t *testing.T) {
		e, _ := exEditor(t, body)
		if res, _ := exRun(e, "w"); res != (EditorResult{Save: true}) {
			t.Fatalf(":w = %+v", res)
		}
	})
	t.Run("q clean", func(t *testing.T) {
		e, _ := exEditor(t, body)
		if res, _ := exRun(e, "q"); res != (EditorResult{Exit: true}) {
			t.Fatalf(":q clean = %+v", res)
		}
	})
	t.Run("q dirty", func(t *testing.T) {
		e, _ := exEditor(t, body)
		exDirty(e)
		res, _ := exRun(e, "q")
		if res != (EditorResult{}) || e.mode != confirmingExit {
			t.Fatalf(":q dirty = %+v mode %v, want the unsaved prompt", res, e.mode)
		}
	})
	t.Run("wq", func(t *testing.T) {
		e, _ := exEditor(t, body)
		if res, _ := exRun(e, "wq"); res != (EditorResult{Save: true, Exit: true}) {
			t.Fatalf(":wq = %+v", res)
		}
	})
	t.Run("x clean", func(t *testing.T) {
		e, _ := exEditor(t, body)
		if res, _ := exRun(e, "x"); res != (EditorResult{Exit: true}) {
			t.Fatalf(":x clean = %+v", res)
		}
	})
	t.Run("x dirty", func(t *testing.T) {
		e, _ := exEditor(t, body)
		exDirty(e)
		if res, _ := exRun(e, "x"); res != (EditorResult{Save: true, Exit: true}) {
			t.Fatalf(":x dirty = %+v", res)
		}
	})
	t.Run("q bang", func(t *testing.T) {
		e, _ := exEditor(t, body)
		exDirty(e)
		if res, _ := exRun(e, "q!"); res != (EditorResult{Exit: true}) {
			t.Fatalf(":q! dirty = %+v", res)
		}
	})
	t.Run("spaces", func(t *testing.T) {
		e, _ := exEditor(t, body)
		if res, _ := exRun(e, " w "); res != (EditorResult{Save: true}) {
			t.Fatalf(": w  = %+v", res)
		}
	})
	t.Run("line numbers", func(t *testing.T) {
		e, _ := exEditor(t, body)
		exRun(e, "3")
		if got := cursorPos(e); got != (buffer.Pos{Line: 2, Col: 2}) {
			t.Errorf(":3 at %v, want line 3's first non-blank", got)
		}
		exRun(e, "0")
		if got := cursorPos(e); got != (buffer.Pos{}) {
			t.Errorf(":0 at %v, want line 1", got)
		}
		exRun(e, "999")
		if got := cursorPos(e).Line; got != e.buf.LastLine() {
			t.Errorf(":999 on line %d, want LastLine %d", got, e.buf.LastLine())
		}
		if e.vim.ex != nil {
			t.Error("the ex line stayed open")
		}
	})
	t.Run("preview", func(t *testing.T) {
		e, _ := exEditor(t, plainLines(60))
		setCursor(e, 20, 0)
		e.afterKey(false)
		row, src := e.cursorScreenRow(), e.source
		exRun(e, "preview")
		if e.source == src {
			t.Error(":preview did not toggle the look")
		}
		if got := e.cursorScreenRow(); got != row {
			t.Errorf("cursor moved from screen row %d to %d", row, got)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		e, _ := exEditor(t, body)
		res, _ := exRun(e, "foo")
		if res != (EditorResult{}) || e.vim.ex != nil {
			t.Fatalf(":foo = %+v, ex open %v", res, e.vim.ex != nil)
		}
		if want := "not an editor command: foo"; e.notice != want || !strings.Contains(plain(e.statusLine()), want) {
			t.Errorf("notice %q, status %q", e.notice, plain(e.statusLine()))
		}
	})
	t.Run("never writes", func(t *testing.T) {
		e, path := exEditor(t, body)
		exDirty(e)
		for _, c := range []string{"w", "wq", "x", "q!", "q"} {
			e.mode = editing
			exRun(e, c)
		}
		vimType(e, "ZZ")
		vimType(e, "ZQ")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("the editor touched %s itself (err %v)", path, err)
		}
	})
}

func TestVimExLineKeys(t *testing.T) {
	e, _ := exEditor(t, "- one\n- two\n")
	vimType(e, ":wq")
	if got := plain(e.statusLine()); got != ":wq" {
		t.Fatalf("status %q, want :wq", got)
	}
	if strings.Contains(plain(e.statusLine()), "[edit") {
		t.Error("the ex line must replace the whole status line")
	}
	c := e.Cursor()
	if want := min(e.geo.Margin+1+ansi.StringWidth("wq"), e.width-1); c == nil || c.Position.X != want || c.Position.Y != e.height-1 {
		t.Errorf("cursor %+v, want (%d, %d)", c, want, e.height-1)
	}
	if c.Shape != tea.CursorBlock {
		t.Errorf("ex cursor shape %v, want the default", c.Shape)
	}

	e.Update(backspaceKey)
	if got := plain(e.statusLine()); got != ":w" {
		t.Errorf("after backspace %q", got)
	}
	vimType(e, "é")
	vimType(e, "👍🏽")
	e.Update(backspaceKey)
	if got := plain(e.statusLine()); got != ":wé" {
		t.Errorf("backspace over an emoji cluster: %q", got)
	}

	e.Paste(tea.PasteMsg{Content: "zzz"})
	if got := plain(e.statusLine()); got != ":wé" || text(e) != "- one\n- two\n" {
		t.Errorf("a paste changed things: status %q text %q", got, text(e))
	}

	e.Update(ctrlU)
	if got := plain(e.statusLine()); got != ":" {
		t.Errorf("ctrl+u: %q", got)
	}
	e.Update(backspaceKey)
	if e.vim.ex != nil {
		t.Error("backspace on an empty line must close it")
	}
	if strings.HasPrefix(plain(e.statusLine()), ":") {
		t.Error("status still shows the ex line")
	}

	vimType(e, ":q")
	res, _ := e.Update(escKey)
	if res != (EditorResult{}) || e.vim.ex != nil || e.mode != editing {
		t.Errorf("esc: %+v ex open %v mode %v, want it closed unrun", res, e.vim.ex != nil, e.mode)
	}

	vimType(e, ":a\x01b")
	if got := plain(e.statusLine()); strings.ContainsAny(got, "\x01") {
		t.Errorf("raw control byte on the status line: %q", got)
	}
}

func TestVimExSaveGuard(t *testing.T) {
	exApp := func(a *App, line string) {
		typeApp(a, ":"+line)
		a.Update(enterKey)
	}
	files := map[string]string{journalPath: journalBody}

	t.Run("w writes", func(t *testing.T) {
		a := openVimApp(t, files, journalPage)
		path := a.editor.path
		exDirty(a.editor)
		want := a.editor.Content()
		exApp(a, "w")
		if got := readFile(t, path); got != want || got == journalBody {
			t.Fatalf("file %q, want %q", got, want)
		}
		if a.editor == nil || a.editor.dirty() {
			t.Error(":w must stay in the editor, clean")
		}
	})
	t.Run("w merges an outside append", func(t *testing.T) {
		a := openVimApp(t, files, journalPage)
		path := a.editor.path
		exDirty(a.editor)
		appendTo(t, path, "- agent\n")
		exApp(a, "w")
		got := readFile(t, path)
		if !strings.Contains(got, "X") || !strings.HasSuffix(got, "- agent\n") {
			t.Fatalf("file %q, want mine and the append", got)
		}
		if a.editor == nil || a.editor.mode != editing || a.editor.Content() != got {
			t.Fatal("merged save must stay in the editor with the merged buffer")
		}
		if !strings.Contains(plain(a.editor.View()), "merged") {
			t.Error("view lacks the merged notice")
		}
	})
	t.Run("wq clash then overwrite", func(t *testing.T) {
		a := openVimApp(t, files, journalPage)
		path := a.editor.path
		exDirty(a.editor)
		mine := a.editor.Content()
		writeOutside(t, path, clashTheirs)
		exApp(a, "wq")
		if a.editor == nil || a.editor.mode != confirmingClash {
			t.Fatal("want the clash prompt")
		}
		if got := readFile(t, path); got != clashTheirs {
			t.Fatalf("the clash wrote %q", got)
		}
		a.Update(key("o"))
		if a.editor != nil {
			t.Error("overwrite after :wq must leave")
		}
		if got := readFile(t, path); got != mine {
			t.Errorf("file %q, want %q", got, mine)
		}
	})
	t.Run("q dirty then discard", func(t *testing.T) {
		a := openVimApp(t, files, journalPage)
		path := a.editor.path
		exDirty(a.editor)
		exApp(a, "q")
		if a.editor == nil || !strings.Contains(plain(a.editor.statusLine()), "Save changes?") {
			t.Fatal("want the unsaved prompt")
		}
		a.Update(key("d"))
		if a.editor != nil {
			t.Error("d must leave")
		}
		if got := readFile(t, path); got != journalBody {
			t.Errorf("file %q, want it unchanged", got)
		}
	})
	t.Run("q bang", func(t *testing.T) {
		a := openVimApp(t, files, journalPage)
		path := a.editor.path
		exDirty(a.editor)
		exApp(a, "q!")
		if a.editor != nil {
			t.Error(":q! must leave")
		}
		if got := readFile(t, path); got != journalBody {
			t.Errorf("file %q, want it unchanged", got)
		}
	})
	t.Run("x clash then keep editing", func(t *testing.T) {
		a := openVimApp(t, files, journalPage)
		path := a.editor.path
		exDirty(a.editor)
		writeOutside(t, path, clashTheirs)
		exApp(a, "x")
		if a.editor == nil || a.editor.mode != confirmingClash || !a.editor.clash.exitAfter {
			t.Fatal("want the save-and-exit clash prompt")
		}
	})
}

func TestVimZZ(t *testing.T) {
	files := map[string]string{journalPath: journalBody}
	t.Run("dirty saves and leaves", func(t *testing.T) {
		a := openVimApp(t, files, journalPage)
		path := a.editor.path
		exDirty(a.editor)
		want := a.editor.Content()
		typeApp(a, "ZZ")
		if a.editor != nil {
			t.Error("ZZ must leave")
		}
		if got := readFile(t, path); got != want {
			t.Errorf("file %q, want %q", got, want)
		}
	})
	t.Run("clean leaves without writing", func(t *testing.T) {
		a := openVimApp(t, files, journalPage)
		path := a.editor.path
		old := time.Now().Add(-time.Hour).Truncate(time.Second)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		typeApp(a, "ZZ")
		if a.editor != nil {
			t.Error("ZZ must leave")
		}
		st, err := os.Stat(path)
		if err != nil || !st.ModTime().Equal(old) {
			t.Errorf("mtime %v (err %v), want %v: the file was written", st.ModTime(), err, old)
		}
	})
	t.Run("ZQ leaves without writing", func(t *testing.T) {
		a := openVimApp(t, files, journalPage)
		path := a.editor.path
		exDirty(a.editor)
		typeApp(a, "ZQ")
		if a.editor != nil {
			t.Error("ZQ must leave")
		}
		if got := readFile(t, path); got != journalBody {
			t.Errorf("file %q, want it unchanged", got)
		}
	})
	t.Run("Zx is invalid", func(t *testing.T) {
		e, _ := exEditor(t, journalBody)
		before := text(e)
		vimType(e, "Z")
		if len(e.vim.pending) != 1 {
			t.Fatalf("Z must wait for a second key, pending %v", e.vim.pending)
		}
		res, _ := e.Update(key("x"))
		if res != (EditorResult{}) || len(e.vim.pending) != 0 || text(e) != before || e.mode != editing {
			t.Errorf("Zx: %+v pending %v text %q mode %v", res, e.vim.pending, text(e), e.mode)
		}
	})
}
