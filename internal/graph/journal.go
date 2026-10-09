package graph

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

const journalLayout = "2006-01-02"

// JournalDate parses a journal page name (YYYY-MM-DD) as a real calendar date.
// ok is false for any other name, including date-shaped names that are not
// dates ("2026-13-01", "2027-02-29").
func JournalDate(name string) (time.Time, bool) {
	t, err := time.Parse(journalLayout, name)
	if err != nil {
		return time.Time{}, false
	}
	return civilDay(t), true
}

// civilDay drops the time of day, keeping the calendar date in UTC.
func civilDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// AddMonthsClamped moves day by n calendar months, keeping the day of month
// or clamping it to the target month's last day (Mar 31 − 1 → Feb 28/29).
func AddMonthsClamped(day time.Time, n int) time.Time {
	y, m, d := day.Date()
	first := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(d, last), 0, 0, 0, 0, time.UTC)
}

// OnThisDayEntry is one existing journal "on this day": Label is
// "1 week ago", "1 month ago" or the year ("2025"); Name is the journal page name.
type OnThisDayEntry struct{ Label, Name string }

// OnThisDay lists the journals in journals (sorted page names, as
// Index.Journals returns) for one week before today, one month before
// (AddMonthsClamped(today, -1)), and the same month and day in every
// earlier year, newest year first, in that order. Dates without a journal are
// left out; a February 29 has no entry in years without one (names that are
// not real dates are ignored, so 2027-02-29 never matches and 02-28 is not
// substituted). Returns nil when none exist.
func OnThisDay(today time.Time, journals []string) []OnThisDayEntry {
	today = civilDay(today)
	var out []OnThisDayEntry
	for _, c := range []struct {
		label string
		day   time.Time
	}{
		{"1 week ago", today.AddDate(0, 0, -7)},
		{"1 month ago", AddMonthsClamped(today, -1)},
	} {
		name := c.day.Format(journalLayout)
		if _, found := slices.BinarySearch(journals, name); found {
			out = append(out, OnThisDayEntry{Label: c.label, Name: name})
		}
	}
	_, month, day := today.Date()
	for i := len(journals) - 1; i >= 0; i-- {
		d, ok := JournalDate(journals[i])
		if !ok || d.Year() >= today.Year() || d.Month() != month || d.Day() != day {
			continue
		}
		out = append(out, OnThisDayEntry{Label: strconv.Itoa(d.Year()), Name: journals[i]})
	}
	return out
}

// AppendLine returns content with line added as a new last line. A missing
// final line break is added first. The break used is "\r\n" when content's
// last line break is "\r\n", otherwise "\n"; the appended line ends with the
// same break. Empty content gives line + "\n". line must not contain a line
// break.
func AppendLine(content, line string) string {
	eol := "\n"
	if strings.HasSuffix(content, "\r\n") {
		eol = "\r\n"
	} else if i := strings.LastIndexByte(content, '\n'); i > 0 && content[i-1] == '\r' {
		eol = "\r\n"
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += eol
	}
	return content + line + eol
}
