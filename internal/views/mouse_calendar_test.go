package views

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// calDayCell is the screen cell of day d's digits in the shown calendar, found
// from the weekday header (maxX limits the search to the sidebar column).
func calDayCell(t *testing.T, a *App, cal *Calendar, d, maxX int) (x, y int) {
	t.Helper()
	hx, hy := mustCell(t, a, "Mo   Tu", maxX)
	yr, mo, _ := cal.cursor.Date()
	lead := (int(time.Date(yr, mo, 1, 0, 0, 0, 0, time.UTC).Weekday()) + 6) % 7
	i := lead + d - 1
	return hx - 1 + 5*(i%7) + 2, hy + 1 + i/7
}

// calMoveTo presses arrow keys until the cursor is on day.
func calMoveTo(t *testing.T, a *App, cal *Calendar, day time.Time) {
	t.Helper()
	for n := 0; !cal.cursor.Equal(day); n++ {
		if n > 400 {
			t.Fatalf("cursor %v never reached %v", cal.cursor, day)
		}
		switch {
		case cal.cursor.Before(day):
			a.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		default:
			a.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		}
	}
}

func calStateOf(a *App, cal *Calendar) (listState, time.Time) {
	return listStateOf(a), cal.cursor
}

func may(d int) time.Time { return time.Date(2026, 5, d, 0, 0, 0, 0, time.UTC) }

// calMouseCases runs the select, open and wheel checks against one place the
// calendar can live; open returns a fresh app showing it focused and its
// calendar, maxX limits cell lookups to the calendar's column, and
// afterOpen says whether the focus must still be in the sidebar.
func calMouseCases(t *testing.T, open func(t *testing.T) (*App, *Calendar), maxX int, sidebar bool) {
	for _, tc := range []struct {
		name string
		day  int
		page string
	}{{"day with a journal", 15, "2026_05_15"}, {"day without a journal", 12, "2026_05_12"}} {
		t.Run(tc.name, func(t *testing.T) {
			a, cal := open(t)
			x, y := calDayCell(t, a, cal, tc.day, maxX)
			// the digits are the middle of the 5-cell day
			a.Update(click(x, y))
			if !cal.cursor.Equal(may(tc.day)) || a.page.Page() == tc.page {
				t.Fatalf("first click: cursor %v page %q, want cursor %v and no navigation", cal.cursor, a.page.Page(), may(tc.day))
			}
			if !sidebar && a.active != cal {
				t.Fatal("first click closed the calendar")
			}
			a.Update(click(x, y))

			twin, tcal := open(t)
			calMoveTo(t, twin, tcal, may(tc.day))
			twin.Update(keyEnt)
			if got, want := listStateOf(a), listStateOf(twin); got != want {
				t.Errorf("second click != enter:\n got %+v\nwant %+v", got, want)
			}
			if a.page.Page() != tc.page {
				t.Errorf("page %q, want %q", a.page.Page(), tc.page)
			}
			if sidebar && !a.sidebarFocused() {
				t.Error("open moved the focus out of the sidebar")
			}
			if !sidebar && a.active != nil {
				t.Errorf("overlay still open: %T", a.active)
			}
			if _, err := os.Stat(filepath.Join(a.graphPath, "journals", tc.page+".md")); tc.day == 12 && !os.IsNotExist(err) {
				t.Errorf("opening a missing journal wrote a file: %v", err)
			}
		})
	}

	t.Run("chrome and empty cells", func(t *testing.T) {
		a, cal := open(t)
		hx, hy := mustCell(t, a, "Mo   Tu", maxX)
		tx, ty := mustCell(t, a, "Calendar", maxX)
		mx, my := mustCell(t, a, "May 2026", maxX)
		sx, sy, ok := findCell(a, "· journal", maxX)
		if !ok {
			sx, sy = mustCell(t, a, "· no journal", maxX)
		}
		fx, fy := mustCell(t, a, "↑/↓ week", maxX)
		kx, ky := mustCell(t, a, "t today", maxX)
		// May 2026 starts on a Friday: Monday of the first week is empty
		ex, ey := calDayCell(t, a, cal, 1, maxX)
		cells := map[string][2]int{
			"title": {tx, ty}, "month": {mx, my}, "weekdays": {hx, hy}, "status": {sx, sy},
			"hint": {fx, fy}, "keys": {kx, ky}, "empty cell": {ex - 20, ey}, "blank under title": {tx, ty + 1},
		}
		wantState, wantCursor := calStateOf(a, cal)
		for name, xy := range cells {
			a, cal := open(t)
			a.Update(click(xy[0], xy[1]))
			state, cursor := calStateOf(a, cal)
			if state != wantState || !cursor.Equal(wantCursor) {
				t.Errorf("click on %s (%d,%d) changed state:\n got %+v %v\nwant %+v %v", name, xy[0], xy[1], state, cursor, wantState, wantCursor)
			}
		}
	})

	t.Run("wheel moves a week", func(t *testing.T) {
		a, cal := open(t)
		start := cal.cursor
		a.Update(wheel(5, 5, +1))
		if want := start.AddDate(0, 0, 7); !cal.cursor.Equal(want) {
			t.Errorf("wheel down: cursor %v, want %v", cal.cursor, want)
		}
		a.Update(wheel(5, 5, -1))
		a.Update(wheel(5, 5, -1))
		if want := start.AddDate(0, 0, -7); !cal.cursor.Equal(want) {
			t.Errorf("wheel up: cursor %v, want %v", cal.cursor, want)
		}
	})
}

func TestMouseCalendarOverlay(t *testing.T) {
	calMouseCases(t, func(t *testing.T) (*App, *Calendar) {
		a := bootApp(t, bootConfig{now: paletteNow})
		a.Update(key("C"))
		cal, ok := a.active.(*Calendar)
		if !ok {
			t.Fatalf("C: active = %T", a.active)
		}
		return a, cal
	}, 0, false)
}

func TestMouseCalendarSidebar(t *testing.T) {
	calMouseCases(t, func(t *testing.T) (*App, *Calendar) {
		a := sidebarApp(t, 120, 30)
		a.Update(key("5"))
		if !a.sidebarFocused() || a.side.calendar == nil {
			t.Fatal("calendar panel not focused after 5")
		}
		return a, a.side.calendar
	}, sidebarCols, true)
}

func TestCalendarDayAt(t *testing.T) {
	c := NewCalendar(loadFixture(t), "2026_05_10", calNow, 80, 24)
	c.View()
	// May 2026: Fri 1 .. Sun 31 over five weeks, first row starts after 4 blanks
	for d := 1; d <= 31; d++ {
		i := 4 + d - 1
		for col := range 5 {
			got, ok := c.dayAt(c.gridTop+i/7, 5*(i%7)+col)
			if !ok || !got.Equal(may(d)) {
				t.Fatalf("day %d col %d: got %v %v", d, col, got, ok)
			}
		}
	}
	for _, p := range [][2]int{
		{c.gridTop - 1, 0}, {c.gridTop + 5, 0}, {c.gridTop, -1}, {c.gridTop, 35},
		{c.gridTop, 0}, {c.gridTop, 19}, // empty cells before Friday the 1st
		{c.gridTop + 2, -3},
	} {
		if d, ok := c.dayAt(p[0], p[1]); ok {
			t.Errorf("dayAt(%d,%d) = %v, want none", p[0], p[1], d)
		}
	}
}
