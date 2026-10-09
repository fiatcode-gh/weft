package views

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

const (
	calendarInnerMin = 44
	calendarInnerMax = 48
	calDayFormat     = "2006-01-02"
)

// calToday marks today's digits in the month grid.
var calToday = lipgloss.NewStyle().Bold(true).Underline(true)

// Calendar is the month-grid overlay for moving between journals.
type Calendar struct {
	journals      []string  // idx.Journals(), sorted
	today, cursor time.Time // civil dates (UTC midnight)
	width, height int
	errMsg        string // in-panel feedback (panelFeedback)
}

// NewCalendar starts the cursor on page when it is a journal date, otherwise
// on today.
func NewCalendar(journals []string, page string, today time.Time, width, height int) *Calendar {
	y, m, d := today.Date()
	c := &Calendar{
		journals: journals,
		today:    time.Date(y, m, d, 0, 0, 0, 0, time.UTC),
		width:    width,
		height:   height,
	}
	c.cursor = c.today
	if t, ok := graph.JournalDate(page); ok {
		c.cursor = t
	}
	return c
}

func (c *Calendar) SetSize(w, h int) { c.width, c.height = w, h }

// SetError shows msg inside the panel until the next key.
func (c *Calendar) SetError(msg string) { c.errMsg = msg }

func (c *Calendar) Update(key string) OverlayResult {
	c.errMsg = ""
	switch key {
	case keyEsc, keyQ:
		return overlayCancel()
	case "left", "h":
		c.cursor = c.cursor.AddDate(0, 0, -1)
	case "right", "l":
		c.cursor = c.cursor.AddDate(0, 0, 1)
	case keyUp, keyK:
		c.cursor = c.cursor.AddDate(0, 0, -7)
	case keyDown, keyJ:
		c.cursor = c.cursor.AddDate(0, 0, 7)
	case "pgup":
		c.cursor = graph.AddMonthsClamped(c.cursor, -1)
	case "pgdown":
		c.cursor = graph.AddMonthsClamped(c.cursor, 1)
	case "t":
		c.cursor = c.today
	case keyEnter:
		return overlayOpen(c.cursor.Format(calDayFormat))
	case "c":
		return overlayCapture()
	}
	return OverlayResult{}
}

// monthGrid lays out a month in weeks starting Monday: each row holds the day
// numbers Monday..Sunday, 0 for a cell outside the month. 4–6 rows.
func monthGrid(year int, month time.Month) [][7]int {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	lead := (int(first.Weekday()) + 6) % 7 // Monday = 0
	days := first.AddDate(0, 1, -1).Day()
	rows := make([][7]int, (lead+days+6)/7)
	for d := 1; d <= days; d++ {
		i := lead + d - 1
		rows[i/7][i%7] = d
	}
	return rows
}

// calWeekdayHeader uses the day-cell scheme: 5 cells per column.
func calWeekdayHeader() string {
	var sb strings.Builder
	for _, abbr := range []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"} {
		sb.WriteString(" " + abbr + "  ")
	}
	return sb.String()
}

func (c *Calendar) hasJournal(day time.Time) bool {
	_, found := slices.BinarySearch(c.journals, day.Format(calDayFormat))
	return found
}

// dayCell renders one 5-cell day: [ or space, two digits, journal mark, ] or space.
func (c *Calendar) dayCell(year int, month time.Month, d int) string {
	if d == 0 {
		return "     "
	}
	day := time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
	isCursor := day.Equal(c.cursor)
	digits := fmt.Sprintf("%2d", d)
	mark := " "
	if c.hasJournal(day) {
		mark = "•"
	}
	left, right := " ", " "
	switch {
	case isCursor:
		left, right = "[", "]"
		digits = styleSel.Render(digits)
		mark = styleSel.Render(mark)
	case day.Equal(c.today):
		digits = calToday.Render(digits)
	}
	return left + digits + mark + right
}

func (c *Calendar) View() string {
	inner := clampInt(c.width-10, calendarInnerMin, calendarInnerMax)
	var sb strings.Builder
	row := func(s string) {
		sb.WriteString(s)
		sb.WriteString("\n")
	}
	row(styleTitle.Render("Calendar") + styleFaint.Render("   · today "+c.today.Format(calDayFormat+" Mon")))
	row("")
	row(styleTitle.Render(c.cursor.Format("January 2006")))
	row(styleFaint.Render(clamp(calWeekdayHeader(), inner)))
	y, m, _ := c.cursor.Date()
	for _, week := range monthGrid(y, m) {
		var line strings.Builder
		for _, d := range week {
			line.WriteString(c.dayCell(y, m, d))
		}
		row(line.String())
	}
	row("")
	status := "no journal"
	if c.hasJournal(c.cursor) {
		status = "journal"
	}
	row(clamp(c.cursor.Format(calDayFormat+" Mon")+" · "+status, inner))
	row("")
	if c.errMsg != "" {
		row(styleTitle.Render(clamp(c.errMsg, inner)))
	}
	row(styleFaint.Render(clamp("←/→ day · ↑/↓ week · pgup/pgdn month", inner)))
	sb.WriteString(styleFaint.Render(clamp("t today · enter open · c capture · esc back", inner)))
	return renderBordered(inner+4, sb.String())
}
