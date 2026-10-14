package views

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest/v2"
)

var agendaNow = time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)

func TestAgendaShowsSections(t *testing.T) {
	quietTerm(t)
	ag := NewAgenda(loadFixture(t), agendaNow, 100, 40)
	v := plain(ag.View())

	order := []string{
		"Overdue", "Replace the dust collector filter", "Wax the bench top",
		"Today", "Order more hide glue", "Glue up the drawer fronts",
		"Upcoming", "Plane the walnut slab", "Router bit delivery",
	}
	at := 0
	for _, s := range order {
		i := strings.Index(v[at:], s)
		if i < 0 {
			t.Fatalf("%q missing or out of order in:\n%s", s, v)
		}
		at += i + len(s)
	}
	if want := "scheduled 2026-05-28 · deadline 2026-05-27 · Workbench"; !strings.Contains(v, want) {
		t.Errorf("view lacks %q:\n%s", want, v)
	}
	for _, s := range []string{"Sweep the shop floor", "Sharpen the plane iron", "Fit the vise jaws"} {
		if strings.Contains(v, s) {
			t.Errorf("view must not list %q:\n%s", s, v)
		}
	}
}

func TestAgendaEmpty(t *testing.T) {
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{"pages/P.md": "- TODO no date here\n"})
	ag := NewAgenda(idx, agendaNow, 100, 40)
	if v := plain(ag.View()); !strings.Contains(v, "nothing due") {
		t.Errorf("empty agenda view:\n%s", v)
	}
}

func TestAgendaEnterOpensTask(t *testing.T) {
	quietTerm(t)
	ag := NewAgenda(loadFixture(t), agendaNow, 100, 40)
	first := ag.items[0].Todo
	if first.Page != "Workbench" {
		t.Fatalf("setup: first row page = %q", first.Page)
	}
	res := ag.Update("enter")
	if res.kind != overlayResultOpenTask || res.page != "Workbench" || res.taskOrdinal != first.Ordinal {
		t.Errorf("enter = %+v, want open task Workbench #%d", res, first.Ordinal)
	}
}

func TestAgendaGolden(t *testing.T) {
	quietTerm(t)
	ag := NewAgenda(loadFixture(t), agendaNow, 80, 24)
	teatest.RequireEqualOutput(t, []byte(plain(ag.View())))
}

func agendaRowOf(t *testing.T, ag *Agenda, text string) int {
	t.Helper()
	for i, it := range ag.items {
		if strings.Contains(it.Todo.Text, text) {
			return i
		}
	}
	t.Fatalf("no agenda row %q", text)
	return -1
}

func TestAgendaRefreshUsesNewToday(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	ag := NewAgenda(idx, agendaNow, 100, 40)
	ag.sel = agendaRowOf(t, ag, "Order more hide glue")
	key, _ := ag.selectedKey()
	before := ag.items[ag.sel].Section

	tomorrow := agendaNow.AddDate(0, 0, 1)
	ag.refresh(idx, tomorrow)

	if !ag.today.Equal(tomorrow) {
		t.Errorf("today = %v, want %v", ag.today, tomorrow)
	}
	if got, _ := ag.selectedKey(); got != key {
		t.Errorf("selected = %+v, want %+v", got, key)
	}
	if after := ag.items[ag.sel].Section; after == before {
		t.Errorf("section stayed %v; a task scheduled yesterday should be overdue", before)
	}
}

func TestAgendaClearDoneDropsStruckRows(t *testing.T) {
	quietTerm(t)
	ag := NewAgenda(loadFixture(t), agendaNow, 100, 40)
	i := agendaRowOf(t, ag, "Order more hide glue")
	row := ag.items[i].Todo
	body := strings.Replace(fixtureBody(t, "pages/Workbench.md"), "- TODO [#A] Order more hide glue", "- DONE [#A] Order more hide glue", 1)
	ag.sel = i
	ag.marked(taskMark{page: row.Page, line: row.LineNumber, from: row.Marker, to: "DONE"}, row.LineNumber,
		fixtureIndexWith(t, map[string]string{"pages/Workbench.md": body}))
	if len(ag.done) != 1 || agendaRowOf(t, ag, "Order more hide glue") < 0 {
		t.Fatalf("setup: done rows = %d", len(ag.done))
	}

	ag.clearDone()

	if len(ag.done) != 0 {
		t.Errorf("done = %v, want empty", ag.done)
	}
	for _, it := range ag.items {
		if strings.Contains(it.Todo.Text, "Order more hide glue") {
			t.Errorf("struck row still listed: %+v", it.Todo)
		}
	}
}
