package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
)

// openTodosAt opens the T panel and selects the row whose text is text.
func openTodosAt(t *testing.T, a *App, text string) *Todos {
	t.Helper()
	a.Update(key("T"))
	td, ok := a.active.(*Todos)
	if !ok {
		t.Fatalf("T should open the todos panel; got %T", a.active)
	}
	for i, b := range td.visible {
		if b.Page == "Workbench" && b.Text == text {
			td.sel = i
			return td
		}
	}
	t.Fatalf("no Workbench row %q", text)
	return nil
}

func workbenchPath(a *App) string {
	return filepath.Join(a.graphPath, "pages", "Workbench.md")
}

func TestAppTodosXMarksDoneAndUndo(t *testing.T) {
	a := bootApp(t)
	path := workbenchPath(a)
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	td := openTodosAt(t, a, "Replace the dust collector filter")

	a.Update(key("x"))

	got, _ := os.ReadFile(path)
	want := strings.Replace(string(orig),
		"- TODO [#B] Replace the dust collector filter",
		"- DONE [#B] Replace the dust collector filter", 1)
	if string(got) != want {
		t.Fatalf("after x:\n%q\nwant\n%q", got, want)
	}
	for _, b := range a.idx.Todos {
		if b.Page == "Workbench" && b.Text == "Replace the dust collector filter" {
			t.Error("a.idx.Todos still lists the done task")
		}
	}
	if a.active != td {
		t.Fatalf("panel should stay open; got %T", a.active)
	}
	if v := plain(td.View()); !strings.Contains(v, "DONE [#B] Replace the dust collector filter") {
		t.Errorf("done row missing from view:\n%s", v)
	}

	a.Update(key("x"))

	got, _ = os.ReadFile(path)
	if string(got) != string(orig) {
		t.Errorf("undo should restore the original bytes:\n%q\nwant\n%q", got, orig)
	}
}

func TestAppTodosXRestoresExactMarker(t *testing.T) {
	for _, text := range []string{"Wax the bench top", "Router bit delivery"} {
		t.Run(text, func(t *testing.T) {
			a := bootApp(t)
			path := workbenchPath(a)
			orig, _ := os.ReadFile(path)
			openTodosAt(t, a, text)

			a.Update(key("x"))
			mid, _ := os.ReadFile(path)
			if !strings.Contains(string(mid), "- DONE "+text) {
				t.Fatalf("x should write DONE; file:\n%s", mid)
			}
			a.Update(key("x"))

			got, _ := os.ReadFile(path)
			if string(got) != string(orig) {
				t.Errorf("undo should restore the exact marker:\n%q\nwant\n%q", got, orig)
			}
		})
	}
}

func TestAppTodosXTaskGone(t *testing.T) {
	a := bootApp(t)
	path := workbenchPath(a)
	td := openTodosAt(t, a, "Wax the bench top")
	const rewritten = "- nothing here now\n"
	if err := os.WriteFile(path, []byte(rewritten), 0o644); err != nil {
		t.Fatal(err)
	}

	a.Update(key("x"))

	got, _ := os.ReadFile(path)
	if string(got) != rewritten {
		t.Errorf("file must be untouched; got %q", got)
	}
	if !strings.Contains(td.errMsg, "task not found") {
		t.Errorf("errMsg = %q, want task not found", td.errMsg)
	}
}

func TestAppTodosXFollowsLineShift(t *testing.T) {
	a := bootApp(t)
	path := workbenchPath(a)
	td := openTodosAt(t, a, "Replace the dust collector filter")
	orig, _ := os.ReadFile(path)
	shifted := "- added above\n" + string(orig)
	if err := os.WriteFile(path, []byte(shifted), 0o644); err != nil {
		t.Fatal(err)
	}
	oldLine := td.visible[td.sel].LineNumber

	a.Update(key("x"))

	got, _ := os.ReadFile(path)
	want := strings.Replace(shifted,
		"- TODO [#B] Replace the dust collector filter",
		"- DONE [#B] Replace the dust collector filter", 1)
	if string(got) != want {
		t.Fatalf("after x:\n%q\nwant\n%q", got, want)
	}
	sel := td.visible[td.sel]
	if sel.Text != "Replace the dust collector filter" || sel.LineNumber != oldLine+1 {
		t.Fatalf("selected row = %q at %d, want the done row at %d", sel.Text, sel.LineNumber, oldLine+1)
	}
	if _, ok := td.done[taskKey{"Workbench", oldLine + 1}]; !ok {
		t.Errorf("done rows %v lack the shifted key", td.done)
	}

	a.Update(key("x"))

	got, _ = os.ReadFile(path)
	if string(got) != shifted {
		t.Errorf("second x should restore the shifted file:\n%q\nwant\n%q", got, shifted)
	}
}

func TestAppTodosXFileChangedBetweenReadAndWrite(t *testing.T) {
	a := bootApp(t)
	path := workbenchPath(a)
	td := openTodosAt(t, a, "Wax the bench top")
	orig, _ := os.ReadFile(path)
	changed := string(orig) + "- added meanwhile\n"
	mutated := false
	real := a.readSnapshot
	a.readSnapshot = func(p string) (edit.Snapshot, error) {
		s, err := real(p)
		if p == path && !mutated {
			mutated = true
			if werr := os.WriteFile(p, []byte(changed), 0o644); werr != nil {
				t.Error(werr)
			}
		}
		return s, err
	}

	a.Update(key("x"))

	got, _ := os.ReadFile(path)
	if string(got) != changed {
		t.Errorf("file = %q, want the outside change untouched", got)
	}
	if !strings.Contains(td.errMsg, "changed on disk") {
		t.Errorf("errMsg = %q, want changed on disk", td.errMsg)
	}
}

func TestAppTodosXBlockedWhileSyncing(t *testing.T) {
	a := bootApp(t)
	path := workbenchPath(a)
	orig, _ := os.ReadFile(path)
	td := openTodosAt(t, a, "Wax the bench top")
	a.syncing = true

	a.Update(key("x"))

	got, _ := os.ReadFile(path)
	if string(got) != string(orig) {
		t.Error("file must be unchanged while syncing")
	}
	if !strings.Contains(td.errMsg, "sync in progress") {
		t.Errorf("errMsg = %q, want sync in progress", td.errMsg)
	}
}

func openAgenda(t *testing.T, a *App) *Agenda {
	t.Helper()
	a.Update(key("A"))
	ag, ok := a.active.(*Agenda)
	if !ok {
		t.Fatalf("A should open the agenda; got %T", a.active)
	}
	return ag
}

func TestAppAKeyOpensAgenda(t *testing.T) {
	a := bootApp(t, bootConfig{now: agendaNow})
	ag := openAgenda(t, a)
	if !ag.today.Equal(agendaNow) {
		t.Errorf("today = %v, want injected clock %v", ag.today, agendaNow)
	}
}

func TestAppAgendaXMarksDoneAndUndo(t *testing.T) {
	a := bootApp(t, bootConfig{now: agendaNow})
	path := workbenchPath(a)
	orig, _ := os.ReadFile(path)
	ag := openAgenda(t, a)
	if got := ag.items[0].Todo.Text; got != "Replace the dust collector filter" {
		t.Fatalf("setup: first row = %q", got)
	}

	a.Update(key("x"))

	got, _ := os.ReadFile(path)
	want := strings.Replace(string(orig),
		"- TODO [#B] Replace the dust collector filter",
		"- DONE [#B] Replace the dust collector filter", 1)
	if string(got) != want {
		t.Fatalf("after x:\n%q\nwant\n%q", got, want)
	}
	if ag.items[0].Todo.Text != "Replace the dust collector filter" || ag.items[0].Section.String() != "Overdue" {
		t.Errorf("done row should stay first in Overdue; got %+v", ag.items[0])
	}
	if v := plain(ag.View()); !strings.Contains(v, "DONE [#B] Replace the dust") {
		t.Errorf("done row missing from view:\n%s", v)
	}

	a.Update(key("x"))

	got, _ = os.ReadFile(path)
	if string(got) != string(orig) {
		t.Errorf("undo should restore the original bytes:\n%q\nwant\n%q", got, orig)
	}
}

func TestAppAgendaXFollowsLineShift(t *testing.T) {
	a := bootApp(t, bootConfig{now: agendaNow})
	path := workbenchPath(a)
	orig, _ := os.ReadFile(path)
	ag := openAgenda(t, a)
	oldLine := ag.items[0].Todo.LineNumber
	shifted := "- added above\n" + string(orig)
	if err := os.WriteFile(path, []byte(shifted), 0o644); err != nil {
		t.Fatal(err)
	}

	a.Update(key("x"))

	got, _ := os.ReadFile(path)
	want := strings.Replace(shifted,
		"- TODO [#B] Replace the dust collector filter",
		"- DONE [#B] Replace the dust collector filter", 1)
	if string(got) != want {
		t.Fatalf("after x:\n%q\nwant\n%q", got, want)
	}
	sel := ag.items[ag.sel].Todo
	if sel.Text != "Replace the dust collector filter" || sel.LineNumber != oldLine+1 {
		t.Fatalf("selected row = %q at %d, want the done row at %d", sel.Text, sel.LineNumber, oldLine+1)
	}
	if _, ok := ag.done[taskKey{"Workbench", oldLine + 1}]; !ok {
		t.Errorf("done rows %v lack the shifted key", ag.done)
	}

	a.Update(key("x"))

	got, _ = os.ReadFile(path)
	if string(got) != shifted {
		t.Errorf("second x should restore the shifted file:\n%q\nwant\n%q", got, shifted)
	}
}

func TestAppAgendaXBlockedWhileSyncing(t *testing.T) {
	a := bootApp(t, bootConfig{now: agendaNow})
	path := workbenchPath(a)
	orig, _ := os.ReadFile(path)
	ag := openAgenda(t, a)
	a.syncing = true

	a.Update(key("x"))

	got, _ := os.ReadFile(path)
	if string(got) != string(orig) {
		t.Error("file must be unchanged while syncing")
	}
	if !strings.Contains(ag.errMsg, "sync in progress") {
		t.Errorf("errMsg = %q, want sync in progress", ag.errMsg)
	}
}
