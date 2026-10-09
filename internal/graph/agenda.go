package graph

import (
	"cmp"
	"slices"
	"time"
)

// AgendaSection groups agenda items relative to today.
type AgendaSection uint8

const (
	AgendaOverdue AgendaSection = iota
	AgendaToday
	AgendaUpcoming
)

func (s AgendaSection) String() string {
	switch s {
	case AgendaOverdue:
		return "Overdue"
	case AgendaToday:
		return "Today"
	default:
		return "Upcoming"
	}
}

// AgendaDays is how many days past today the Upcoming section reaches (inclusive).
const AgendaDays = 7

// AgendaItem is one dated open task placed in a section. Date and Time come
// from the relevant stamp (the earlier date; on equal dates the earliest time).
type AgendaItem struct {
	Todo    TodoBullet
	Section AgendaSection
	Date    string
	Time    string
}

// BuildAgenda returns the open dated tasks in Overdue, Today, Upcoming order.
// "Today" is today's calendar date in its own location. todos is not mutated.
func BuildAgenda(todos []TodoBullet, today time.Time) []AgendaItem {
	y, m, d := today.Date()
	todayS := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	lastS := time.Date(y, m, d+AgendaDays, 0, 0, 0, 0, time.UTC).Format("2006-01-02")

	items := make([]AgendaItem, 0, len(todos))
	for _, t := range todos {
		date, tm, ok := relevantStamp(t)
		if !ok {
			continue
		}
		var sec AgendaSection
		switch {
		case date < todayS:
			sec = AgendaOverdue
		case date == todayS:
			sec = AgendaToday
		case date <= lastS:
			sec = AgendaUpcoming
		default:
			continue
		}
		items = append(items, AgendaItem{Todo: t, Section: sec, Date: date, Time: tm})
	}
	slices.SortStableFunc(items, compareAgenda)
	return items
}

func relevantStamp(t TodoBullet) (date, tm string, ok bool) {
	s, d := t.Scheduled, t.Deadline
	switch {
	case s.Date == "" && d.Date == "":
		return "", "", false
	case d.Date == "":
		return s.Date, s.Time, true
	case s.Date == "":
		return d.Date, d.Time, true
	case s.Date < d.Date:
		return s.Date, s.Time, true
	case d.Date < s.Date:
		return d.Date, d.Time, true
	}
	tm = s.Time
	if tm == "" || (d.Time != "" && d.Time < tm) {
		tm = d.Time
	}
	return s.Date, tm, true
}

func priorityRank(p string) int {
	switch p {
	case "A":
		return 0
	case "B":
		return 1
	case "C":
		return 2
	}
	return 3
}

func compareAgenda(a, b AgendaItem) int {
	if c := cmp.Compare(a.Section, b.Section); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Date, b.Date); c != 0 {
		return c
	}
	if (a.Time == "") != (b.Time == "") {
		if a.Time != "" {
			return -1
		}
		return 1
	}
	if c := cmp.Compare(a.Time, b.Time); c != 0 {
		return c
	}
	if c := cmp.Compare(priorityRank(a.Todo.Priority), priorityRank(b.Todo.Priority)); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Todo.Page, b.Todo.Page); c != 0 {
		return c
	}
	return cmp.Compare(a.Todo.LineNumber, b.Todo.LineNumber)
}
