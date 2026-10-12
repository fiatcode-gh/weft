package graph

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

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
// "1 week ago", "1 month ago" or the year ("2025").
type OnThisDayEntry struct {
	Label   string
	Journal PageMeta
}

// OnThisDay lists the journals in journals (sorted ascending by Date, as
// Index.Journals returns) for one week before today, one month before
// (AddMonthsClamped(today, -1)), and the same month and day in every
// earlier year, newest year first, in that order. Dates without a journal are
// left out; a February 29 has no entry in years without one (02-28 is not
// substituted). Returns nil when none exist.
func OnThisDay(today time.Time, journals []PageMeta) []OnThisDayEntry {
	today = civilDay(today)
	var out []OnThisDayEntry
	for _, c := range []struct {
		label string
		day   time.Time
	}{
		{"1 week ago", today.AddDate(0, 0, -7)},
		{"1 month ago", AddMonthsClamped(today, -1)},
	} {
		if i, found := journalSearch(journals, c.day); found {
			out = append(out, OnThisDayEntry{Label: c.label, Journal: journals[i]})
		}
	}
	_, month, day := today.Date()
	for i := len(journals) - 1; i >= 0; i-- {
		d := journals[i].Date
		if d.Year() >= today.Year() || d.Month() != month || d.Day() != day {
			continue
		}
		out = append(out, OnThisDayEntry{Label: strconv.Itoa(d.Year()), Journal: journals[i]})
	}
	return out
}

// journalSearch binary-searches journals (ascending by Date) for day and
// returns its position, or its insertion point and false.
func journalSearch(journals []PageMeta, day time.Time) (int, bool) {
	return slices.BinarySearchFunc(journals, day, func(p PageMeta, d time.Time) int { return p.Date.Compare(d) })
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
