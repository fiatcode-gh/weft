package graph

import (
	"strings"
	"testing"
)

func TestExtractTodosDates(t *testing.T) {
	d := func(date string) Stamp { return Stamp{Date: date} }
	cases := []struct {
		name         string
		body         string
		sched, dead  Stamp
		todoIdx      int
		wantTodoLine int
	}{
		{"both dates and a time",
			"- TODO a\n  SCHEDULED: <2026-05-20 Wed>\n  DEADLINE: <2026-05-25 Mon 17:00>",
			d("2026-05-20"), Stamp{Date: "2026-05-25", Time: "17:00"}, 0, 1},
		{"repeater kept out of stamp",
			"- TODO a\n  DEADLINE: <2026-06-01 Mon 08:00 .+1w>",
			Stamp{}, Stamp{Date: "2026-06-01", Time: "08:00"}, 0, 1},
		{"child stamp does not leak to parent",
			"- TODO a\n  - child\n    DEADLINE: <2026-05-26>",
			Stamp{}, Stamp{}, 0, 1},
		{"stamp after DONE sibling does not attach to earlier open task",
			"- TODO a\n- DONE b\n  SCHEDULED: <2026-05-24>",
			Stamp{}, Stamp{}, 0, 1},
		{"fenced stamp ignored",
			"- TODO a\n  ```\n  SCHEDULED: <2026-05-20>\n  ```",
			Stamp{}, Stamp{}, 0, 1},
		{"logbook stamp ignored",
			"- TODO a\n  :LOGBOOK:\n  SCHEDULED: <2026-05-20>\n  :END:\n  DEADLINE: <2026-05-21>",
			Stamp{}, d("2026-05-21"), 0, 1},
		{"duplicate first wins",
			"- TODO a\n  SCHEDULED: <2026-05-20>\n  SCHEDULED: <2026-05-22>",
			d("2026-05-20"), Stamp{}, 0, 1},
		{"crlf body",
			"- TODO a\r\n  SCHEDULED: <2026-05-20 Wed>\r\n  DEADLINE: <2026-05-21>\r\n",
			d("2026-05-20"), d("2026-05-21"), 0, 1},
		{"star bullet ends ownership",
			"- TODO a\n* note\n  SCHEDULED: <2026-05-20>",
			Stamp{}, Stamp{}, 0, 1},
		{"unindented line after task attaches",
			"- TODO a\nSCHEDULED: <2026-05-20>",
			d("2026-05-20"), Stamp{}, 0, 1},
		{"blank line between still attaches",
			"- TODO a\n\n  SCHEDULED: <2026-05-20>",
			d("2026-05-20"), Stamp{}, 0, 1},
		{"stamp before any bullet ignored",
			"SCHEDULED: <2026-05-20>\n- TODO a",
			Stamp{}, Stamp{}, 0, 2},
		{"fence opener bullet ends ownership",
			"- TODO a\n- ```\n  SCHEDULED: <2026-05-20>\n  ```",
			Stamp{}, Stamp{}, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			todos := ExtractTodos(tc.body)
			if len(todos) == 0 {
				t.Fatalf("no todos in %q", tc.body)
			}
			got := todos[tc.todoIdx]
			if got.Scheduled != tc.sched || got.Deadline != tc.dead {
				t.Errorf("sched/deadline = %+v/%+v, want %+v/%+v", got.Scheduled, got.Deadline, tc.sched, tc.dead)
			}
			if got.Line != tc.wantTodoLine || got.Marker != "TODO" || got.Text != "a" || got.Priority != "" {
				t.Errorf("todo basics changed: %+v", got)
			}
		})
	}
}

func TestExtractTodosDatesSecondTask(t *testing.T) {
	todos := ExtractTodos("- TODO a\n  SCHEDULED: <2026-05-20>\n- LATER [#A] b\n  DEADLINE: <2026-05-21>")
	if len(todos) != 2 {
		t.Fatalf("want 2 todos, got %+v", todos)
	}
	if todos[0].Scheduled.Date != "2026-05-20" || todos[0].Deadline.Date != "" {
		t.Errorf("first: %+v", todos[0])
	}
	if todos[1].Deadline.Date != "2026-05-21" || todos[1].Scheduled.Date != "" || todos[1].Priority != "A" {
		t.Errorf("second: %+v", todos[1])
	}
}

func TestExtractTodosLogbookTodoDoesNotOwnLaterStamp(t *testing.T) {
	todos := ExtractTodos("- TODO a\n  :LOGBOOK:\n  - TODO inner\n  :END:\n  SCHEDULED: <2026-05-20 Wed>")
	if len(todos) != 2 {
		t.Fatalf("want a and inner indexed, got %+v", todos)
	}
	if todos[0].Text != "a" || todos[0].Scheduled.Date != "2026-05-20" {
		t.Errorf("a should own the stamp: %+v", todos[0])
	}
	if todos[1].Scheduled.Date != "" {
		t.Errorf("inner must not own the stamp: %+v", todos[1])
	}
}

func TestTaskBlockAt(t *testing.T) {
	src := []string{
		"intro",                      // 0
		"- TODO a",                   // 1
		"  SCHEDULED: <2026-05-20>",  // 2
		"",                           // 3
		"  ```",                      // 4
		"  - fenced",                 // 5
		"  ```",                      // 6
		"  DEADLINE: <2026-05-21>",   // 7
		"  - child",                  // 8
		"    DEADLINE: <2026-05-22>", // 9
		"- note",                     // 10
		"- DONE b",                   // 11
		"  SCHEDULED: <2026-05-24>",  // 12
		"- ",                         // 13
	}
	line := func(i int) string { return src[i] }
	n := len(src)

	cases := []struct {
		name string
		at   int
		ok   bool
		want TaskBlock
	}{
		{"first line", 1, true, TaskBlock{First: 1, End: 8, Scheduled: 2, Deadline: 7}},
		{"stamp line", 2, true, TaskBlock{First: 1, End: 8, Scheduled: 2, Deadline: 7}},
		{"blank own line", 3, true, TaskBlock{First: 1, End: 8, Scheduled: 2, Deadline: 7}},
		{"inside fence", 5, true, TaskBlock{First: 1, End: 8, Scheduled: 2, Deadline: 7}},
		{"child plain bullet", 8, false, TaskBlock{}},
		{"child stamp line", 9, false, TaskBlock{}},
		{"non-task bullet", 10, false, TaskBlock{}},
		{"before first bullet", 0, false, TaskBlock{}},
		{"done task", 11, true, TaskBlock{First: 11, End: 13, Scheduled: 12, Deadline: -1}},
		{"done task stamp", 12, true, TaskBlock{First: 11, End: 13, Scheduled: 12, Deadline: -1}},
		{"empty bullet", 13, false, TaskBlock{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := TaskBlockAt(n, line, tc.at)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (%+v)", ok, tc.ok, got)
			}
			if ok && got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestTaskBlockAtNoStamps(t *testing.T) {
	src := strings.Split("- TODO a\n- TODO b", "\n")
	got, ok := TaskBlockAt(len(src), func(i int) string { return src[i] }, 0)
	if !ok || got != (TaskBlock{First: 0, End: 1, Scheduled: -1, Deadline: -1}) {
		t.Errorf("got %+v ok=%v", got, ok)
	}
}

func TestBuildIndexFixtureDates(t *testing.T) {
	idx := buildFixtureIndex(t)
	byText := map[string]TodoBullet{}
	for _, td := range idx.Todos {
		if td.Page == "Workbench" {
			byText[td.Text] = td
		}
	}
	cases := []struct {
		text        string
		sched, dead Stamp
	}{
		{"Replace the dust collector filter", Stamp{Date: "2026-05-20"}, Stamp{}},
		{"Glue up the drawer fronts", Stamp{}, Stamp{Date: "2026-05-25", Time: "17:00"}},
		{"Plane the walnut slab", Stamp{Date: "2026-05-28"}, Stamp{Date: "2026-05-27"}},
		{"Sharpen the plane iron", Stamp{}, Stamp{}},
		{"Fit the vise jaws", Stamp{}, Stamp{}},
	}
	for _, tc := range cases {
		td, ok := byText[tc.text]
		if !ok {
			t.Errorf("missing Workbench todo %q", tc.text)
			continue
		}
		if td.Scheduled != tc.sched || td.Deadline != tc.dead {
			t.Errorf("%s: got %+v/%+v, want %+v/%+v", tc.text, td.Scheduled, td.Deadline, tc.sched, tc.dead)
		}
	}
}
