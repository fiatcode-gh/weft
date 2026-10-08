package views

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
)

// toggleMode is one Ctrl+R.
func toggleMode(e *EditorView) { e.Update(ctrl('r')) }

func TestEditorCtrlRToggles(t *testing.T) {
	quietTerm(t)
	page := "# Title\n\n- item **bold**\n\nparagraph [[Link]] text\n"
	e := NewEditorView(nil, "P", "/tmp/p.md", page, false, 60, 14, Anchor{Line: 4, ScreenRow: 6}, nil, false)
	e.Update(key("x"))
	e.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	e.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	wantText, wantCur := text(e), cursorPos(e)
	wantSel, selOK := e.buf.Selection()
	if !selOK {
		t.Fatal("setup: no selection")
	}

	if e.source || !strings.Contains(statusRow(e), "[edit · preview]") {
		t.Fatalf("setup: not live: %q", statusRow(e))
	}
	if !strings.Contains(strings.Join(plainFrame(e), "\n"), "• item") {
		t.Fatal("setup: live preview shows the raw bullet away from the cursor")
	}

	toggleMode(e)
	if !e.source || !strings.Contains(statusRow(e), "[edit · source]") {
		t.Errorf("after Ctrl+R: source=%v status %q", e.source, statusRow(e))
	}
	if !strings.Contains(strings.Join(plainFrame(e), "\n"), "- item **bold**") || strings.Contains(strings.Join(plainFrame(e), "\n"), "•") {
		t.Errorf("source mode shows the bullet rendered:\n%s", strings.Join(plainFrame(e), "\n"))
	}
	if text(e) != wantText || cursorPos(e) != wantCur {
		t.Errorf("source: text/cursor changed: %q %v", text(e), cursorPos(e))
	}
	if sel, ok := e.buf.Selection(); !ok || sel != wantSel {
		t.Errorf("source: selection %v %v, want %v", sel, ok, wantSel)
	}

	toggleMode(e)
	if e.source || !strings.Contains(statusRow(e), "[edit · preview]") {
		t.Errorf("after the second Ctrl+R: source=%v status %q", e.source, statusRow(e))
	}
	if !strings.Contains(strings.Join(plainFrame(e), "\n"), "• item") {
		t.Errorf("live preview shows the raw bullet again:\n%s", strings.Join(plainFrame(e), "\n"))
	}
	if text(e) != wantText || cursorPos(e) != wantCur {
		t.Errorf("live: text/cursor changed: %q %v", text(e), cursorPos(e))
	}
	if sel, ok := e.buf.Selection(); !ok || sel != wantSel {
		t.Errorf("live: selection %v %v, want %v", sel, ok, wantSel)
	}
	// The typed x is still one undo step.
	e.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	e.Update(ctrl('z'))
	if text(e) != page {
		t.Errorf("undo after two toggles: %q, want %q", text(e), page)
	}
}

func TestEditorCtrlRKeepsScreenRow(t *testing.T) {
	quietTerm(t)
	var b strings.Builder
	fill := func(n int) {
		for i := range n {
			fmt.Fprintf(&b, "- fill %d\n", i)
		}
	}
	fill(30)
	b.WriteString("\n- one row bullet\n\n" +
		"- wrapped bullet that is long enough to wrap onto several rows in a forty column window\n\n" +
		"| A | B |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |\n\n" +
		"```go\nx := 1\ny := 2\n```\n\n")
	fill(30)
	page := b.String()
	lines := strings.Split(page, "\n")
	find := func(prefix string) int {
		for i, l := range lines {
			if strings.HasPrefix(l, prefix) {
				return i
			}
		}
		t.Fatalf("no line %q", prefix)
		return 0
	}
	const h = 14 // 12 text rows
	cases := []struct {
		name      string
		line, row int
	}{
		{"one row", find("- one row"), 0},
		{"second row of a wrapped line", find("- wrapped"), 1},
		{"table row", find("| 3 |"), 0},
		{"inside a fence", find("y := 2"), 0},
	}
	for _, tc := range cases {
		for _, sr := range []int{0, 5, 11} {
			t.Run(fmt.Sprintf("%s at %d", tc.name, sr), func(t *testing.T) {
				e := NewEditorView(nil, "P", "/tmp/p.md", page, false, 40, h, Anchor{Line: tc.line, RowInLine: tc.row, ScreenRow: sr}, nil, false)
				for step := range 4 {
					before, cur, was := e.cursorScreenRow(), cursorPos(e), e.source
					toggleMode(e)
					if e.source == was {
						t.Fatalf("switch %d: Ctrl+R did not change the look", step)
					}
					if got := e.cursorScreenRow(); got != before {
						t.Fatalf("switch %d (source=%v): cursor on screen row %d, was %d", step, e.source, got, before)
					}
					if cursorPos(e) != cur {
						t.Fatalf("switch %d: cursor moved %v -> %v", step, cur, cursorPos(e))
					}
				}
			})
		}
	}
}

func TestEditorCtrlRWithBarsOpen(t *testing.T) {
	quietTerm(t)
	t.Run("find bar", func(t *testing.T) {
		e := NewEditorView(nil, "P", "/tmp/p.md", "alpha beta\nbeta\n", false, 60, 12, Anchor{ScreenRow: 1}, nil, false)
		e.Update(ctrl('f'))
		typeRunes(e, "beta")
		query, cur := e.find.query, e.find.cur
		toggleMode(e)
		if !e.source {
			t.Error("Ctrl+R with the find bar open did not switch")
		}
		if e.find == nil || e.find.query != query || e.find.cur != cur {
			t.Errorf("find bar changed: %+v, want query %q cur %d", e.find, query, cur)
		}
		toggleMode(e)
		if e.source || e.find == nil || e.find.query != query {
			t.Errorf("second Ctrl+R: source=%v find=%+v", e.source, e.find)
		}
	})
	t.Run("completion strip", func(t *testing.T) {
		e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 16, Anchor{ScreenRow: 1}, nil, false)
		typeRunes(e, "link to [[A")
		if !e.completer.active {
			t.Fatal("setup: strip closed")
		}
		partial, sel := e.completer.partial, e.completer.sel
		toggleMode(e)
		if !e.source || !e.completer.active || e.completer.partial != partial || e.completer.sel != sel {
			t.Errorf("strip after Ctrl+R: source=%v active=%v partial=%q sel=%d", e.source, e.completer.active, e.completer.partial, e.completer.sel)
		}
	})
	t.Run("exit prompt", func(t *testing.T) {
		e := NewEditorView(nil, "P", "/tmp/p.md", "x\n", false, 60, 12, Anchor{ScreenRow: 1}, nil, false)
		e.Update(key("y"))
		e.Update(esc)
		if e.mode != confirmingExit {
			t.Fatal("setup: no exit prompt")
		}
		toggleMode(e)
		if e.source || e.mode != confirmingExit {
			t.Errorf("Ctrl+R in the exit prompt: source=%v mode=%v", e.source, e.mode)
		}
	})
	t.Run("clash prompt", func(t *testing.T) {
		e := NewEditorView(nil, "P", "/tmp/p.md", "x\n", false, 60, 12, Anchor{ScreenRow: 1}, nil, false)
		e.showClash(edit.Snapshot{Content: "theirs\n", Exists: true}, false)
		toggleMode(e)
		if e.source || e.mode != confirmingClash {
			t.Errorf("Ctrl+R in the clash prompt: source=%v mode=%v", e.source, e.mode)
		}
	})
}

// tree is every file's name, size and mtime under dir.
func tree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, fmt.Sprintf("%s %d %d", p, info.Size(), info.ModTime().UnixNano()))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEditorModeLastsForTheRun(t *testing.T) {
	startInLive(t)
	files := map[string]string{"pages/Doc.md": "# Doc\n\n- item **bold**\n"}
	a := newApp(t, files, 80, 24)
	a.navigate("Doc")
	before := tree(t, a.graphPath)
	time.Sleep(10 * time.Millisecond)

	a.Update(key("e"))
	if a.editor == nil || a.editor.source {
		t.Fatal("the editor did not open in live preview")
	}
	toggleMode(a.editor)
	a.Update(esc)
	if a.editor != nil {
		t.Fatal("editor did not close")
	}
	a.Update(key("e"))
	if a.editor == nil || !a.editor.source {
		t.Fatal("the second session did not open in source mode")
	}
	toggleMode(a.editor)
	a.Update(esc)
	a.Update(key("e"))
	if a.editor == nil || a.editor.source {
		t.Fatal("a toggle back to live was forgotten")
	}
	a.Update(esc)

	if after := tree(t, a.graphPath); !slices.Equal(before, after) {
		t.Errorf("toggling changed the graph:\nbefore %v\nafter  %v", before, after)
	}
	b := newApp(t, files, 80, 24)
	b.navigate("Doc")
	b.Update(key("e"))
	if b.editor == nil || b.editor.source {
		t.Error("a new App did not open live")
	}
}

func TestHelpListsCtrlR(t *testing.T) {
	quietTerm(t)
	view := plain(NewHelp("", 0, 0).View())
	if !strings.Contains(view, "ctrl-r") || !strings.Contains(view, "live preview ↔ source") {
		t.Errorf("help does not list Ctrl+R:\n%s", view)
	}
}
