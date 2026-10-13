package views

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/fiatcode-gh/weft/v2/internal/edit"
)

var (
	keyF2    = tea.KeyPressMsg{Code: tea.KeyF2}
	keyCtrlU = tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	keyCtrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
)

// renamePrompted opens the rename prompt on page and types name over the
// prefilled text, without pressing enter.
func renamePrompted(t *testing.T, a *App, page, name string) {
	t.Helper()
	a.navigate(page)
	a.Update(keyF2)
	if a.rename == nil {
		t.Fatalf("F2 on %q opened no prompt (hint %q)", page, a.hint)
	}
	a.Update(keyCtrlU)
	typeText(a, name)
}

// renamePreviewed carries a fixture through the prompt to the preview panel.
func renamePreviewed(t *testing.T, a *App, page, name string) *RenamePreview {
	t.Helper()
	renamePrompted(t, a, page, name)
	a.Update(keyEnt)
	pv, ok := a.active.(*RenamePreview)
	if !ok {
		t.Fatalf("active = %T, want *RenamePreview; frame:\n%s", a.active, appText(a))
	}
	return pv
}

func TestRenameStartRefusals(t *testing.T) {
	t.Run("journal", func(t *testing.T) {
		a := bootApp(t, fixtureCfg)
		a.navigate("2026_05_24")
		a.Update(keyF2)
		if a.rename != nil || a.hint != "journals cannot be renamed" {
			t.Errorf("rename = %v, hint = %q", a.rename, a.hint)
		}
	})
	t.Run("no file", func(t *testing.T) {
		a := bootApp(t, fixtureCfg)
		a.navigate("DoesNotExist")
		a.Update(keyF2)
		if a.rename != nil || a.hint != "no file to rename yet" {
			t.Errorf("rename = %v, hint = %q", a.rename, a.hint)
		}
	})
	t.Run("syncing", func(t *testing.T) {
		a := bootApp(t, fixtureCfg)
		a.navigate("Alpha")
		a.syncing = true
		a.Update(keyF2)
		if a.rename != nil || !strings.Contains(a.hint, syncBusyMsg) {
			t.Errorf("rename = %v, hint = %q", a.rename, a.hint)
		}
	})
}

func TestRenamePromptRefusals(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	before := graphFiles(t, a.graphPath)
	renamePrompted(t, a, "Beta", "alpha")
	a.Update(keyEnt)
	if a.rename == nil || !strings.Contains(appText(a), `"alpha" is already a page: Alpha`) {
		t.Fatalf("clash not shown:\n%s", appText(a))
	}
	a.Update(keyCtrlU)
	a.Update(keyEnt)
	if a.rename == nil || !strings.Contains(appText(a), "new name is empty") {
		t.Fatalf("empty not shown:\n%s", appText(a))
	}
	a.Update(keyCtrlU)
	typeText(a, "Beta")
	a.Update(keyEnt)
	if a.rename == nil || !strings.Contains(appText(a), "name unchanged") {
		t.Fatalf("unchanged not shown:\n%s", appText(a))
	}
	a.Update(keyEscK)
	if a.rename != nil || a.active != nil {
		t.Errorf("esc left rename=%v active=%T", a.rename, a.active)
	}
	if after := graphFiles(t, a.graphPath); fmt.Sprint(before) != fmt.Sprint(after) {
		t.Error("refusals wrote to disk")
	}
}

func TestRenameF2EndToEnd(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	renamePreviewed(t, a, "Hub", "Center")
	text := appText(a)
	for _, want := range []string{"pages/Hub.md → pages/Center.md", "pages/Hub.md", "pages/kb___notes.md", "journals/2026_05_25.md"} {
		if !strings.Contains(text, want) {
			t.Errorf("preview lacks %q:\n%s", want, text)
		}
	}
	a.Update(key("y"))
	if a.active != nil || a.page.Page() != "Center" {
		t.Fatalf("active = %T, page = %q", a.active, a.page.Page())
	}
	if !strings.HasPrefix(a.hint, "renamed Hub → Center · 3 links in 3 files") {
		t.Errorf("hint = %q", a.hint)
	}
	if !existsRel(a, "pages/Center.md") || existsRel(a, "pages/Hub.md") {
		t.Error("page file not moved")
	}
}

func TestRenamePreviewCancel(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{key("n"), keyEscK} {
		a := bootApp(t, fixtureCfg)
		before := graphFiles(t, a.graphPath)
		renamePreviewed(t, a, "Hub", "Center")
		a.Update(k)
		if a.active != nil {
			t.Errorf("%v left %T open", k, a.active)
		}
		if after := graphFiles(t, a.graphPath); fmt.Sprint(before) != fmt.Sprint(after) {
			t.Errorf("%v changed files", k)
		}
	}
}

func TestRenamePreviewStoppedPanel(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	renamePreviewed(t, a, "Alpha", "Gamma")
	hub := readRel(t, a, "pages/Hub.md")
	writeRel(t, a, "pages/Hub.md", hub+"changed\n")
	a.Update(key("y"))
	text := appText(a)
	for _, want := range []string{"Rename stopped", "rewritten (1)", "not rewritten (3)"} {
		if !strings.Contains(text, want) {
			t.Errorf("stopped panel lacks %q:\n%s", want, text)
		}
	}
	a.Update(key("y"))
	if _, ok := a.active.(*RenamePreview); !ok || !strings.Contains(appText(a), "Rename stopped") {
		t.Error("y acted in the stopped panel")
	}
	a.Update(keyEscK)
	if a.active != nil || a.page.Page() != "Alpha" {
		t.Errorf("active = %T, page = %q", a.active, a.page.Page())
	}
}

func TestRenamePreviewScrolls(t *testing.T) {
	files := map[string]string{"pages/Target.md": "target\n"}
	for i := range 40 {
		files[fmt.Sprintf("pages/P%02d.md", i)] = "see [[Target]]\n"
	}
	a := bootApp(t, bootConfig{files: files})
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	renamePreviewed(t, a, "Target", "Goal")
	check := func() string {
		t.Helper()
		text := appText(a)
		if n := strings.Count(text, "\n") + 1; n > 20 {
			t.Fatalf("frame has %d lines, want <= 20:\n%s", n, text)
		}
		return text
	}
	first := check()
	if !strings.Contains(first, "files 1–") {
		t.Fatalf("no window indicator:\n%s", first)
	}
	a.Update(key("j"))
	if second := check(); !strings.Contains(second, "files 2–") {
		t.Errorf("j did not move the window:\n%s", second)
	}
}

func TestRenamePreviewSyncBlocked(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	before := graphFiles(t, a.graphPath)
	renamePreviewed(t, a, "Hub", "Center")
	a.syncing = true
	a.Update(key("y"))
	if !strings.Contains(appText(a), syncBusyMsg) {
		t.Errorf("no sync message:\n%s", appText(a))
	}
	if _, ok := a.active.(*RenamePreview); !ok {
		t.Errorf("active = %T", a.active)
	}
	if after := graphFiles(t, a.graphPath); fmt.Sprint(before) != fmt.Sprint(after) {
		t.Error("blocked rename wrote to disk")
	}
}

func TestRenameCtrlCQuits(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	renamePrompted(t, a, "Hub", "x")
	_, cmd := a.Update(keyCtrlC)
	if cmd == nil {
		t.Fatal("ctrl+c returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c did not quit")
	}
}

func TestRenamePromptGolden(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	p := newRenamePrompt(*pageMeta(t, a, "Hub"), 80, 24)
	block, _, _ := p.View()
	teatest.RequireEqualOutput(t, []byte(plain(block)))
}

func goldenPreview(t *testing.T) *RenamePreview {
	t.Helper()
	a := bootApp(t, fixtureCfg)
	plan, err := a.planRename(pageMeta(t, a, "Hub"), "Center")
	if err != nil {
		t.Fatal(err)
	}
	return NewRenamePreview(plan, 80, 24)
}

func TestRenamePreviewGolden(t *testing.T) {
	teatest.RequireEqualOutput(t, []byte(plain(goldenPreview(t).View())))
}

func TestRenameStoppedGolden(t *testing.T) {
	pv := goldenPreview(t)
	pv.SetStopped(renameStop{
		reason:  "pages/Hub.md changed on disk — nothing overwritten",
		done:    []string{"pages/Beta.md"},
		pending: []string{"pages/Hub.md", "journals/2026_05_15.md", "journals/2026_05_24.md"},
	}, nil)
	teatest.RequireEqualOutput(t, []byte(plain(pv.View())))
}

// stoppedFooter is the panel's last text row, without its border.
func stoppedFooter(t *testing.T, pv *RenamePreview) string {
	t.Helper()
	rows := strings.Split(plain(pv.View()), "\n")
	for i := len(rows) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(strings.Trim(strings.TrimSpace(rows[i]), "│╭╮╰╯─")); text != "" {
			return text
		}
	}
	t.Fatal("empty panel")
	return ""
}

func TestRenameStoppedFooterRewriteStop(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	p, err := a.planRename(pageMeta(t, a, "Alpha"), "Gamma")
	if err != nil {
		t.Fatal(err)
	}
	writeRel(t, a, "pages/Hub.md", readRel(t, a, "pages/Hub.md")+"- added elsewhere\n")
	res, err := a.applyRename(p)
	if err != nil || res.stop == nil {
		t.Fatalf("res = %+v err = %v, want a stop", res, err)
	}
	pv := NewRenamePreview(p, 80, 24)
	pv.SetStopped(*res.stop, res.reindexErr)
	if got, want := stoppedFooter(t, pv), "F2 with the same name finishes the rename · esc close"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
}

func TestRenameStoppedFooterPageChanged(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	p, err := a.planRename(pageMeta(t, a, "Alpha"), "Gamma")
	if err != nil {
		t.Fatal(err)
	}
	writeRel(t, a, "pages/Alpha.md", readRel(t, a, "pages/Alpha.md")+"- more\n")
	read := a.readSnapshot
	a.readSnapshot = func(path string) (edit.Snapshot, error) {
		if path == p.page.Path {
			return p.pageRead, nil // let the guard at y pass; only the rename step sees the change
		}
		return read(path)
	}
	res, err := a.applyRename(p)
	if err != nil || res.stop == nil {
		t.Fatalf("res = %+v err = %v, want a stop", res, err)
	}
	pv := NewRenamePreview(p, 80, 24)
	pv.SetStopped(*res.stop, res.reindexErr)
	if got, want := stoppedFooter(t, pv), "esc close · check the page, then F2 with the same name"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
}

func TestRenameStoppedFooterByKind(t *testing.T) {
	tests := []struct {
		name string
		kind stopKind
		want string
	}{
		{"dest taken", stopDestTaken, "esc close · move that file away, then F2 with the same name"},
		{"page failed", stopPageFailed, "esc close · fix the cause above, then F2 with the same name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pv := goldenPreview(t)
			pv.SetStopped(renameStop{kind: tt.kind, reason: "stopped", done: []string{"pages/Beta.md"}}, nil)
			if got := stoppedFooter(t, pv); got != tt.want {
				t.Errorf("footer = %q, want %q", got, tt.want)
			}
		})
	}
}
