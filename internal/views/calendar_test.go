package views

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

var calNow = time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)

func TestMonthGrid(t *testing.T) {
	may := monthGrid(2026, time.May)
	if len(may) != 5 {
		t.Fatalf("May 2026: want 5 rows, got %d", len(may))
	}
	if may[0] != [7]int{0, 0, 0, 0, 1, 2, 3} {
		t.Errorf("May 2026 row 0 = %v", may[0])
	}
	if last := may[4]; last != [7]int{25, 26, 27, 28, 29, 30, 31} {
		t.Errorf("May 2026 last row = %v", last)
	}
	if got := len(monthGrid(2027, time.February)); got != 4 {
		t.Errorf("Feb 2027: want 4 rows, got %d", got)
	}
	if got := len(monthGrid(2026, time.August)); got != 6 {
		t.Errorf("Aug 2026: want 6 rows, got %d", got)
	}
	feb := monthGrid(2028, time.February)
	maxDay := 0
	for _, row := range feb {
		maxDay = max(maxDay, slices.Max(row[:]))
	}
	if maxDay != 29 {
		t.Errorf("Feb 2028: last day = %d, want 29", maxDay)
	}
}

func TestCalendarStartCursor(t *testing.T) {
	for _, tc := range []struct {
		page string
		want time.Time
	}{
		{"2026-05-15", day(2026, 5, 15)},
		{"Alpha", day(2026, 5, 26)},
		{"2026-13-99", day(2026, 5, 26)},
	} {
		if got := NewCalendar(nil, tc.page, calNow, 80, 24).cursor; !got.Equal(tc.want) {
			t.Errorf("page %q: cursor %v, want %v", tc.page, got, tc.want)
		}
	}
}

func TestCalendarMovement(t *testing.T) {
	for _, tc := range []struct {
		start, key, want string
	}{
		{"2026-05-31", "right", "2026-06-01"},
		{"2026-01-01", "left", "2025-12-31"},
		{"2026-01-03", "up", "2025-12-27"},
		{"2025-12-29", "down", "2026-01-05"},
		{"2026-01-31", "pgdown", "2026-02-28"},
		{"2026-03-31", "pgup", "2026-02-28"},
		{"2025-12-15", "pgdown", "2026-01-15"},
		{"2026-01-31", "t", "2026-05-26"},
		{"2026-05-10", "l", "2026-05-11"},
		{"2026-05-10", "h", "2026-05-09"},
		{"2026-05-10", "j", "2026-05-17"},
		{"2026-05-10", "k", "2026-05-03"},
	} {
		c := NewCalendar(nil, tc.start, calNow, 80, 24)
		c.Update(tc.key)
		if got := c.cursor.Format("2006-01-02"); got != tc.want {
			t.Errorf("%s %s: got %s, want %s", tc.start, tc.key, got, tc.want)
		}
	}
}

func TestCalendarKeyStrings(t *testing.T) {
	if got := (tea.KeyPressMsg{Code: tea.KeyPgUp}).String(); got != "pgup" {
		t.Errorf("pgup key = %q", got)
	}
	if got := (tea.KeyPressMsg{Code: tea.KeyPgDown}).String(); got != "pgdown" {
		t.Errorf("pgdown key = %q", got)
	}
}

func TestCalendarEnterAndCancel(t *testing.T) {
	c := NewCalendar(nil, "2026-05-10", calNow, 80, 24)
	if res := c.Update("enter"); res.kind != overlayResultOpen || res.page != "2026-05-10" {
		t.Errorf("enter = %+v", res)
	}
	if res := c.Update("c"); res.kind != overlayResultCapture {
		t.Errorf("c = %+v", res)
	}
	for _, k := range []string{"esc", "q"} {
		if res := c.Update(k); res.kind != overlayResultCancel {
			t.Errorf("%s = %+v", k, res)
		}
	}
}

func TestCalendarMarksAndToday(t *testing.T) {
	quietTerm(t)
	c := NewCalendar(loadFixture(t).Journals(), "Alpha", calNow, 80, 24)
	v := plain(c.View())
	for _, want := range []string{"25•", "[26 ]", " 1•", "2026-05-26 Tue · no journal", "today 2026-05-26 Tue", "May 2026"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "12•") {
		t.Errorf("12 must not be marked:\n%s", v)
	}
	c.Update("left")
	v = plain(c.View())
	for _, want := range []string{"[25•]", "· journal"} {
		if !strings.Contains(v, want) {
			t.Errorf("after left, view missing %q:\n%s", want, v)
		}
	}
	if !strings.Contains(c.View(), calToday.Render("26")) {
		t.Errorf("today's digits not rendered with calToday")
	}
}

func TestCalendarFitsTerminal(t *testing.T) {
	c := NewCalendar(nil, "2026-08-10", calNow, 80, 24) // 6-week month
	if h := strings.Count(c.View(), "\n") + 1; h > 24 {
		t.Errorf("calendar is %d rows, want <= 24", h)
	}
}

func TestCalendarGolden(t *testing.T) {
	quietTerm(t)
	c := NewCalendar(loadFixture(t).Journals(), "Alpha", calNow, 80, 24)
	teatest.RequireEqualOutput(t, []byte(plain(c.View())))
}

func calBoot(t *testing.T) *App {
	return bootApp(t, bootConfig{now: time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)})
}

func TestAppCalendarOpensJournal(t *testing.T) {
	a := calBoot(t)
	a.Update(key("C"))
	if _, ok := a.active.(*Calendar); !ok {
		t.Fatalf("C: active = %T, want *Calendar", a.active)
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	a.Update(keyEnt)
	if got := a.page.Page(); got != "2026-05-25" || a.active != nil {
		t.Errorf("page %q active %v", got, a.active)
	}
}

func TestAppCalendarMissingDayOpensPhantom(t *testing.T) {
	a := calBoot(t)
	a.Update(key("C"))
	a.Update(keyEnt)
	if got := a.page.Page(); got != "2026-05-26" {
		t.Fatalf("page = %q", got)
	}
	if _, err := os.Stat(filepath.Join(a.graphPath, "journals", "2026_05_26.md")); !os.IsNotExist(err) {
		t.Errorf("journal file must not be created: %v", err)
	}
	if !strings.Contains(appText(a), "(no entry yet for this page)") {
		t.Errorf("phantom page text missing:\n%s", appText(a))
	}
}

func TestAppCalendarCaptureAndClose(t *testing.T) {
	a := calBoot(t)
	a.Update(key("C"))
	a.Update(key("c"))
	if a.capture == nil {
		t.Fatal("c should open capture")
	}
	if _, ok := a.active.(*Calendar); !ok {
		t.Fatalf("calendar should stay open, active = %T", a.active)
	}
	a.Update(keyEscK)
	if a.capture != nil {
		t.Fatal("esc should close capture")
	}
	a.Update(keyEscK)
	if a.active != nil {
		t.Errorf("esc should close calendar, active = %T", a.active)
	}
}
