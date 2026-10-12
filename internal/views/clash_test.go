package views

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

func bootVault(t *testing.T) *App {
	t.Helper()
	return bootApp(t, bootConfig{fixture: "obsidian-vault"})
}

func TestVaultPickerLabelsClashesByPath(t *testing.T) {
	a := bootVault(t)
	a.active = NewPicker(a.idx, a.width, a.height)
	p := a.active.(*Picker)

	var labels []string
	for _, m := range p.matches {
		labels = append(labels, m.Str)
	}
	if !slices.Contains(labels, "a/Note") || !slices.Contains(labels, "b/Note") {
		t.Errorf("labels = %v, want a/Note and b/Note rows", labels)
	}
	if slices.Contains(labels, "Note") {
		t.Errorf("labels = %v: bare Note must not be a row", labels)
	}
	if !slices.Contains(labels, "Alpha") {
		t.Errorf("labels = %v: unshared Alpha keeps its name", labels)
	}

	// Enter on b/Note opens b/Note.md, not the page that owns the name.
	for _, r := range "b/Note" {
		a.Update(key(string(r)))
	}
	if sel := p.matches[p.sel].Str; sel != "b/Note" {
		t.Fatalf("top match = %q, want b/Note", sel)
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := a.page.Page(); got != "b/Note" {
		t.Fatalf("page = %q, want b/Note", got)
	}
	if v := appText(a); !strings.Contains(v, "note in b") {
		t.Errorf("page body lacks %q:\n%s", "note in b", v)
	}
}

func TestVaultCompletionOffersKeys(t *testing.T) {
	a := bootVault(t)
	c := newLinkCompleter(a.idx)
	c.refresh("[[Not", "", true, true)
	var got []string
	for _, cand := range c.cands {
		got = append(got, cand.name)
	}
	if !slices.Contains(got, "Note") || !slices.Contains(got, "b/Note") {
		t.Errorf("candidates = %v, want Note and b/Note", got)
	}
}

func TestVaultNavigationCachesByPath(t *testing.T) {
	a := bootVault(t)
	a.navigate("Note")
	if v := appText(a); !strings.Contains(v, "note in a") {
		t.Fatalf("Note body:\n%s", v)
	}
	a.navigate("b/Note")
	v := appText(a)
	if !strings.Contains(v, "note in b") || strings.Contains(v, "note in a") {
		t.Errorf("b/Note shows the wrong body:\n%s", v)
	}
	a.navigate("Note")
	if v := appText(a); !strings.Contains(v, "note in a") {
		t.Errorf("Note again shows the wrong body:\n%s", v)
	}
}

func TestVaultBacklinksOfLoser(t *testing.T) {
	a := bootVault(t)
	a.navigate("b/Note")
	a.Update(key("b"))
	if _, ok := a.active.(*Backlinks); !ok {
		t.Fatalf("active = %T, want *Backlinks", a.active)
	}
	if v := appText(a); !strings.Contains(v, "Home") {
		t.Errorf("backlinks of b/Note lack Home:\n%s", v)
	}
	if got := a.unlinkedRefs("b/Note"); len(got) != 0 {
		t.Errorf("unlinkedRefs(b/Note) = %+v, want none", got)
	}
}

func TestVaultTodosMarkLoserFile(t *testing.T) {
	a := bootVault(t)
	a.Update(key("T"))
	td, ok := a.active.(*Todos)
	if !ok {
		t.Fatalf("active = %T, want *Todos", a.active)
	}
	i := slices.IndexFunc(td.visible, func(b graph.TodoBullet) bool { return b.Text == "b task" })
	if i < 0 {
		t.Fatalf("no b task row in %+v", td.visible)
	}
	if td.visible[i].Page != "b/Note" {
		t.Errorf("b task page = %q, want b/Note", td.visible[i].Page)
	}
	td.sel = i
	a.Update(key("x"))
	body, err := os.ReadFile(filepath.Join(a.graphPath, "b", "Note.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "DONE b task") {
		t.Errorf("b/Note.md = %q, want the task marked DONE", body)
	}
	other, _ := os.ReadFile(filepath.Join(a.graphPath, "a", "Note.md"))
	if strings.Contains(string(other), "DONE") {
		t.Errorf("a/Note.md was touched: %q", other)
	}
}

func TestVaultSearchOpensLoser(t *testing.T) {
	skipIfNoRipgrep(t)
	a := bootVault(t)
	a.Update(key("/"))
	s, ok := a.active.(*SearchView)
	if !ok {
		t.Fatalf("active = %T, want *SearchView", a.active)
	}
	s.SetQuery("note in b")
	_, cmd := a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter started no search")
	}
	a.Update(cmd())
	if len(s.hits) != 1 {
		t.Fatalf("hits = %+v, want one", s.hits)
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := a.page.Page(); got != "b/Note" {
		t.Errorf("page = %q, want b/Note", got)
	}
}

func TestVaultEditOpensLoserFile(t *testing.T) {
	a := bootVault(t)
	a.navigate("b/Note")
	pressE(t, a)
	if a.editor == nil {
		t.Fatal("e did not open the editor")
	}
	if want := filepath.Join(a.graphPath, "b", "Note.md"); a.editor.path != want {
		t.Errorf("editor path = %q, want %q", a.editor.path, want)
	}
}

func TestVaultNewFolderPageIsReachableAfterSave(t *testing.T) {
	a := bootVault(t)
	a.active = NewPicker(a.idx, a.width, a.height)
	for _, r := range "c/New" {
		a.Update(key(string(r)))
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.editor == nil {
		t.Fatal("create did not open the editor")
	}
	a.Update(key("h"))
	a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if _, err := os.Stat(filepath.Join(a.graphPath, "c", "New.md")); err != nil {
		t.Fatalf("c/New.md not written: %v", err)
	}
	if err := a.reindex(); err != nil {
		t.Fatal(err)
	}
	if p, ok := a.idx.Resolve("c/New"); !ok || p.Rel != "c/New.md" {
		t.Errorf("Resolve(c/New) = %+v, %v", p, ok)
	}
}
