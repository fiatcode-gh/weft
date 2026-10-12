package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	subCfg   = ".weft/config.toml"
	subPat   = "journal_file_pattern = \"YYYY/MM/YYYY-MM-DD\"\n"
	subToday = "journals/2026/06/2026-06-05.md"
)

var subNow = time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

func bootSub(t *testing.T, files map[string]string) *App {
	t.Helper()
	files[subCfg] = subPat
	if _, ok := files["pages/Anchor.md"]; !ok {
		files["pages/Anchor.md"] = "anchor\n"
	}
	return bootApp(t, bootConfig{files: files, now: subNow})
}

func readFileAt(t *testing.T, a *App, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(a.graphPath, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDotCreatesJournalInPatternFolders(t *testing.T) {
	a := bootSub(t, map[string]string{})
	a.navigate("Anchor")

	a.Update(key("."))

	if got := a.page.Page(); got != "2026-06-05" {
		t.Fatalf("page = %q, want 2026-06-05", got)
	}
	if got := readFileAt(t, a, subToday); got != "" {
		t.Errorf("journal = %q, want empty", got)
	}
	if _, ok := a.idx.JournalOn(subNow); !ok {
		t.Error("created journal should be indexed")
	}
}

func TestDotWritesTemplateIntoPatternFolders(t *testing.T) {
	a := bootSub(t, map[string]string{"pages/Journal Template.md": tplText})

	a.Update(key("."))

	if got := readFileAt(t, a, subToday); got != tplText {
		t.Errorf("journal = %q, want the template", got)
	}
}

func TestEditColdStartCreatesJournalInPatternFolders(t *testing.T) {
	t.Setenv("EDITOR", "true")
	a := bootSub(t, map[string]string{})
	if _, ok := a.idx.JournalOn(subNow); ok {
		t.Fatal("precondition: today's journal must not exist")
	}

	if cmd := pressShiftE(t, a); cmd == nil {
		t.Fatal("E should return an editor cmd")
	}

	if got := readFileAt(t, a, subToday); got != "" {
		t.Errorf("journal = %q, want empty", got)
	}
	if _, ok := a.idx.JournalOn(subNow); !ok {
		t.Error("journal should be indexed after E")
	}
}

func TestInAppEditorOnColdStartTodayTargetsPatternPath(t *testing.T) {
	a := bootSub(t, map[string]string{"pages/Journal Template.md": tplText})

	a.Update(key("e"))

	if a.editor == nil {
		t.Fatal("e should open the editor")
	}
	if want := filepath.Join(a.graphPath, "journals", "2026", "06", "2026-06-05.md"); a.editor.path != want {
		t.Errorf("editor path = %q, want %q", a.editor.path, want)
	}
}

func TestCaptureAppendsToJournalInPatternFolders(t *testing.T) {
	a := bootSub(t, map[string]string{})

	a.Update(key("c"))
	typeText(a, "buy milk")
	a.Update(keyEnt)

	if got := readFileAt(t, a, subToday); got != "- buy milk\n" {
		t.Errorf("journal = %q", got)
	}
	if a.hint != "captured to 2026-06-05 (new journal)" {
		t.Errorf("hint = %q", a.hint)
	}
}

func TestCalendarOpensPatternJournalByName(t *testing.T) {
	a := bootSub(t, map[string]string{"journals/2026/05/2026-05-24.md": "- x\n"})

	a.Update(key("C"))
	cal, ok := a.active.(*Calendar)
	if !ok {
		t.Fatalf("active = %T", a.active)
	}
	if !cal.hasJournal(time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC)) {
		t.Error("calendar should mark 2026-05-24")
	}
	cal.cursor = time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC)
	a.Update(keyEnt)

	if got := a.page.Page(); got != "2026-05-24" {
		t.Errorf("page = %q, want 2026-05-24", got)
	}
}

func TestOnThisDayReadsJournalInPatternFolders(t *testing.T) {
	a := bootApp(t, bootConfig{
		files: map[string]string{
			subCfg:                           subPat,
			"journals/2026/05/2026-05-24.md": "\n- last week entry\n",
			"pages/Anchor.md":                "anchor\n",
		},
		now: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
	})

	a.Update(key("O"))
	o, ok := a.active.(*OnThisDay)
	if !ok {
		t.Fatalf("active = %T", a.active)
	}
	v := plain(o.View())
	if !strings.Contains(v, "1 week ago · 2026-05-24") || !strings.Contains(v, "last week entry") {
		t.Errorf("overlay:\n%s", v)
	}
}

func TestPrevNextFollowDateOrderForDayFirstPattern(t *testing.T) {
	a := bootApp(t, bootConfig{
		files: map[string]string{
			".weft/config.toml":      "journal_file_pattern = \"DD-MM-YYYY\"\n",
			"journals/01-06-2026.md": "a\n",
			"journals/15-05-2026.md": "b\n",
			"journals/02-05-2026.md": "c\n",
			"pages/Anchor.md":        "anchor\n",
		},
		now: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
	})
	if got := a.page.Page(); got != "01-06-2026" {
		t.Fatalf("boot page = %q", got)
	}

	a.Update(key("<"))
	if got := a.page.Page(); got != "15-05-2026" {
		t.Errorf("after <: %q, want 15-05-2026", got)
	}
	a.Update(key("<"))
	if got := a.page.Page(); got != "02-05-2026" {
		t.Errorf("after <<: %q, want 02-05-2026", got)
	}
	a.Update(key(">"))
	a.Update(key(">"))
	if got := a.page.Page(); got != "01-06-2026" {
		t.Errorf("after >>: %q, want 01-06-2026", got)
	}
}

func TestPickerNoCreateRowForJournalNames(t *testing.T) {
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)
	typeQuery(p, "2026_10_12")
	if p.createName != "" {
		t.Errorf("default layout: createName = %q for a journal name", p.createName)
	}

	_, idx := writeGraph(t, map[string]string{subCfg: "journal_file_pattern = \"YYYY-MM-DD\"\n"})
	p = NewPicker(idx, 80, 30)
	typeQuery(p, "2026-10-12")
	if p.createName != "" {
		t.Errorf("obsidian pattern: createName = %q for a journal name", p.createName)
	}
}
