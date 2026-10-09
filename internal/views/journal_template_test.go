package views

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
)

const (
	tplText   = "## Morning\n\n## Evening\n"
	tplToday  = "2026-06-05"
	tplJFile  = "journals/2026_06_05.md"
	tplAnchor = "pages/Anchor.md"
)

// bootTemplateApp boots an App on a temp graph at 2026-06-05 with the given
// files, the way the cold-start journal tests do.
func bootTemplateApp(t *testing.T, files map[string]string) (*App, string) {
	t.Helper()
	quietTerm(t)
	dir, _ := writeGraph(t, files)
	a := New(dir, "test")
	a.nowFunc = func() time.Time { return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC) }
	if cmd := a.Init(); cmd != nil {
		a.Update(cmd())
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return a, dir
}

func readJournalFile(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, tplJFile))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	return string(b)
}

func requireNoJournalFile(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, tplJFile)); !os.IsNotExist(err) {
		t.Fatalf("journal file should not exist; stat err=%v", err)
	}
}

func TestDotWritesJournalTemplate(t *testing.T) {
	a, dir := bootTemplateApp(t, map[string]string{"pages/Journal Template.md": tplText, tplAnchor: "anchor\n"})
	a.Update(key("."))
	if got := readJournalFile(t, dir); got != tplText {
		t.Errorf("journal bytes = %q, want %q", got, tplText)
	}
	info, err := os.Stat(filepath.Join(dir, tplJFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
	if _, ok := a.idx.ByName[tplToday]; !ok {
		t.Errorf("%q should be indexed", tplToday)
	}
	if !strings.Contains(appText(a), "Morning") {
		t.Errorf("view should show the template; got:\n%s", appText(a))
	}
}

func TestDotTemplateResolvesCaseInsensitively(t *testing.T) {
	a, dir := bootTemplateApp(t, map[string]string{"pages/journal template.md": tplText, tplAnchor: "anchor\n"})
	a.Update(key("."))
	if got := readJournalFile(t, dir); got != tplText {
		t.Errorf("journal bytes = %q, want %q", got, tplText)
	}
}

func TestDotTemplateReadAtUseTime(t *testing.T) {
	a, dir := bootTemplateApp(t, map[string]string{"pages/Journal Template.md": tplText, tplAnchor: "anchor\n"})
	if err := os.WriteFile(filepath.Join(dir, "pages", "Journal Template.md"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.Update(key("."))
	if got := readJournalFile(t, dir); got != "v2\n" {
		t.Errorf("journal bytes = %q, want the template as it is now", got)
	}
}

func TestDotEmptyTemplateCreatesEmptyFile(t *testing.T) {
	a, dir := bootTemplateApp(t, map[string]string{"pages/Journal Template.md": "", tplAnchor: "anchor\n"})
	a.Update(key("."))
	if got := readJournalFile(t, dir); got != "" {
		t.Errorf("journal bytes = %q, want empty", got)
	}
}

func TestDotNeverTouchesExistingJournal(t *testing.T) {
	a, dir := bootTemplateApp(t, map[string]string{"pages/Journal Template.md": tplText, tplJFile: "old\n"})
	a.Update(key("."))
	if got := readJournalFile(t, dir); got != "old\n" {
		t.Errorf("journal bytes = %q, want untouched", got)
	}
}

func TestDotTemplateReadErrorCreatesNothing(t *testing.T) {
	a, dir := bootTemplateApp(t, map[string]string{"pages/Journal Template.md": tplText, tplAnchor: "anchor\n"})
	tplPath := filepath.Join(dir, "pages", "Journal Template.md")
	real := a.readSnapshot
	a.readSnapshot = func(p string) (edit.Snapshot, error) {
		if p == tplPath {
			return edit.Snapshot{}, errors.New("boom")
		}
		return real(p)
	}
	a.Update(key("."))
	requireNoJournalFile(t, dir)
	if !strings.Contains(a.hint, "cannot read Journal Template") {
		t.Errorf("hint = %q, want cannot read Journal Template", a.hint)
	}
}

func TestShiftEWritesJournalTemplate(t *testing.T) {
	a, dir := bootTemplateApp(t, map[string]string{"pages/Journal Template.md": tplText, tplAnchor: "anchor\n"})
	if cmd := pressShiftE(t, a); cmd == nil {
		t.Fatal("E should return an editor cmd")
	}
	if got := readJournalFile(t, dir); got != tplText {
		t.Errorf("journal bytes = %q, want %q", got, tplText)
	}
}

func TestEditorNewJournalStartsWithTemplate(t *testing.T) {
	a, dir := bootTemplateApp(t, map[string]string{"pages/Journal Template.md": tplText, tplAnchor: "anchor\n"})
	pressE(t, a)
	if a.editor == nil {
		t.Fatalf("e should open the editor; hint %q", a.hint)
	}
	if got := a.editor.Content(); got != tplText {
		t.Errorf("Content() = %q, want %q", got, tplText)
	}
	requireNoJournalFile(t, dir)
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.editor != nil {
		t.Errorf("Esc on an untouched template buffer should leave without a prompt")
	}
	requireNoJournalFile(t, dir)
}

func TestEditorNewJournalTemplateSaves(t *testing.T) {
	a, dir := bootTemplateApp(t, map[string]string{"pages/Journal Template.md": tplText, tplAnchor: "anchor\n"})
	pressE(t, a)
	if a.editor == nil {
		t.Fatalf("e should open the editor; hint %q", a.hint)
	}
	a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if got := readJournalFile(t, dir); got != tplText {
		t.Errorf("journal bytes = %q, want %q", got, tplText)
	}
	if a.editor == nil || a.editor.mode != editing {
		t.Errorf("save should not clash; editor %v", a.editor)
	}
}

func TestEditorNewJournalTemplateTypedThenDiscard(t *testing.T) {
	a, dir := bootTemplateApp(t, map[string]string{"pages/Journal Template.md": tplText, tplAnchor: "anchor\n"})
	pressE(t, a)
	if a.editor == nil {
		t.Fatalf("e should open the editor; hint %q", a.hint)
	}
	a.Update(key("x"))
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	a.Update(key("d"))
	requireNoJournalFile(t, dir)
}

func TestEditorNewPageIgnoresTemplate(t *testing.T) {
	a, _ := bootTemplateApp(t, map[string]string{"pages/Journal Template.md": tplText, tplAnchor: "anchor\n"})
	a.navigate("Brand New")
	pressE(t, a)
	if a.editor == nil {
		t.Fatalf("e should open the editor; hint %q", a.hint)
	}
	if got := a.editor.Content(); got != "" {
		t.Errorf("Content() = %q, want empty", got)
	}
}

func TestEditorExistingEmptyJournalIgnoresTemplate(t *testing.T) {
	a, _ := bootTemplateApp(t, map[string]string{"pages/Journal Template.md": tplText, tplJFile: ""})
	pressE(t, a)
	if a.editor == nil {
		t.Fatalf("e should open the editor; hint %q", a.hint)
	}
	if got := a.editor.Content(); got != "" {
		t.Errorf("Content() = %q, want empty", got)
	}
}

func TestNewEditorViewNewFileDiskIsZero(t *testing.T) {
	e := NewEditorView(nil, tplToday, "/tmp/x.md", "tmpl\n", true, 80, 24, Anchor{0, 0, 1}, nil, false)
	if e.disk != (edit.Snapshot{}) {
		t.Errorf("new file disk = %+v, want zero Snapshot", e.disk)
	}
	if e.dirty() {
		t.Errorf("template-only buffer should not be dirty")
	}
	e = NewEditorView(nil, tplToday, "/tmp/x.md", "tmpl\n", false, 80, 24, Anchor{0, 0, 1}, nil, false)
	if want := (edit.Snapshot{Content: "tmpl\n", Exists: true}); e.disk != want {
		t.Errorf("existing file disk = %+v, want %+v", e.disk, want)
	}
}
