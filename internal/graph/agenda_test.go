package graph

import (
	"reflect"
	"testing"
	"time"
)

type agendaRow struct {
	Section AgendaSection
	Text    string
}

func TestBuildAgendaFixture(t *testing.T) {
	idx, err := BuildIndex("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	items := BuildAgenda(idx.Todos, time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC))
	var got []agendaRow
	for _, it := range items {
		got = append(got, agendaRow{it.Section, it.Todo.Text})
	}
	want := []agendaRow{
		{AgendaOverdue, "Replace the dust collector filter"},
		{AgendaOverdue, "Wax the bench top"},
		{AgendaToday, "Order more hide glue"},
		{AgendaToday, "Glue up the drawer fronts"},
		{AgendaUpcoming, "Plane the walnut slab"},
		{AgendaUpcoming, "Router bit delivery"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agenda = %v\nwant %v", got, want)
	}
	for _, it := range items {
		if it.Todo.Text == "Plane the walnut slab" && it.Date != "2026-05-27" {
			t.Errorf("walnut slab Date = %q, want 2026-05-27", it.Date)
		}
	}
}

func sched(text, date string) TodoBullet {
	return TodoBullet{Page: "P", LineNumber: 1, Text: text, Scheduled: Stamp{Date: date}}
}

func TestBuildAgendaWindowBoundaries(t *testing.T) {
	today := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	todos := []TodoBullet{
		sched("m1", "2026-05-24"),
		sched("t0", "2026-05-25"),
		sched("p1", "2026-05-26"),
		sched("p7", "2026-06-01"),
		sched("p8", "2026-06-02"),
	}
	var got []string
	for _, it := range BuildAgenda(todos, today) {
		got = append(got, it.Todo.Text)
	}
	want := []string{"m1", "t0", "p1", "p7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildAgendaBothDatesOnce(t *testing.T) {
	today := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	todos := []TodoBullet{
		{Page: "P", LineNumber: 1, Text: "split", Scheduled: Stamp{Date: "2026-05-27"}, Deadline: Stamp{Date: "2026-05-20"}},
		{Page: "P", LineNumber: 2, Text: "equal", Scheduled: Stamp{Date: "2026-05-26", Time: "14:00"}, Deadline: Stamp{Date: "2026-05-26", Time: "09:00"}},
	}
	items := BuildAgenda(todos, today)
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(items), items)
	}
	byText := map[string]AgendaItem{}
	for _, it := range items {
		byText[it.Todo.Text] = it
	}
	if s := byText["split"]; s.Section != AgendaOverdue || s.Date != "2026-05-20" {
		t.Errorf("split = %+v, want overdue 2026-05-20", s)
	}
	if e := byText["equal"]; e.Time != "09:00" || e.Date != "2026-05-26" {
		t.Errorf("equal = %+v, want 09:00 on 2026-05-26", e)
	}
}

func TestBuildAgendaOrderWithinDate(t *testing.T) {
	today := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	d := "2026-05-25"
	mk := func(text, tm, prio, page string, line int) TodoBullet {
		return TodoBullet{Page: page, LineNumber: line, Text: text, Priority: prio, Scheduled: Stamp{Date: d, Time: tm}}
	}
	todos := []TodoBullet{
		mk("none", "", "", "A", 1),
		mk("line2", "", "C", "B", 2),
		mk("c", "", "C", "A", 5),
		mk("t10", "10:00", "", "A", 1),
		mk("pageB", "", "B", "B", 1),
		mk("b", "", "B", "A", 1),
		mk("line1", "", "C", "B", 1),
		mk("a", "", "A", "Z", 9),
		mk("t08", "08:00", "C", "A", 1),
	}
	var got []string
	for _, it := range BuildAgenda(todos, today) {
		got = append(got, it.Todo.Text)
	}
	want := []string{"t08", "t10", "a", "b", "pageB", "c", "line1", "line2", "none"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestBuildAgendaUsesTodaysLocation(t *testing.T) {
	today := time.Date(2026, 5, 26, 1, 0, 0, 0, time.FixedZone("plus10", 10*3600))
	todos := []TodoBullet{sched("now", "2026-05-26"), sched("old", "2026-05-25")}
	got := map[string]AgendaSection{}
	for _, it := range BuildAgenda(todos, today) {
		got[it.Todo.Text] = it.Section
	}
	if got["now"] != AgendaToday || got["old"] != AgendaOverdue {
		t.Fatalf("sections = %v", got)
	}
}

func TestBuildAgendaIgnoresUndatedAndEmpty(t *testing.T) {
	today := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	if got := BuildAgenda(nil, today); len(got) != 0 {
		t.Fatalf("nil input gave %v", got)
	}
	if got := BuildAgenda([]TodoBullet{{Page: "P", Text: "undated"}}, today); len(got) != 0 {
		t.Fatalf("undated gave %v", got)
	}
}

func TestAgendaSectionString(t *testing.T) {
	for s, want := range map[AgendaSection]string{AgendaOverdue: "Overdue", AgendaToday: "Today", AgendaUpcoming: "Upcoming"} {
		if s.String() != want {
			t.Errorf("%d = %q, want %q", s, s.String(), want)
		}
	}
}
