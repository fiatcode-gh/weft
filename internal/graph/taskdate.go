package graph

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// StampKind is which Logseq planning keyword a stamp line carries.
type StampKind uint8

const (
	StampScheduled StampKind = 1
	StampDeadline  StampKind = 2
)

// Keyword is the literal text of the stamp's keyword.
func (k StampKind) Keyword() string {
	if k == StampDeadline {
		return "DEADLINE"
	}
	return "SCHEDULED"
}

// Stamp is a task's planned date and optional time of day. The zero value
// means "none".
type Stamp struct{ Date, Time string }

// StampLine is one parsed "SCHEDULED: <…>" or "DEADLINE: <…>" line. Start and
// End span "KEYWORD: <…>" itself, excluding indent and trailing whitespace.
// Repeater is kept verbatim but has no effect.
type StampLine struct {
	Kind                          StampKind
	Start, End                    int
	Date, Weekday, Time, Repeater string
}

// Stamp is the date and time of s without its weekday and repeater.
func (s StampLine) Stamp() Stamp { return Stamp{Date: s.Date, Time: s.Time} }

var stampLineRe = regexp.MustCompile(`^[ \t]*(SCHEDULED|DEADLINE): +<(\d{4}-\d{2}-\d{2})(?: +(\p{L}+))?(?: +(\d{2}:\d{2}))?(?: +((?:\.\+|\+\+|\+)\d+[hdwmy]))?>\s*$`)

// ParseStampLine reports whether line is, in its entirety, a stamp line. The
// date must be a real calendar date and the time 00:00–23:59; a weekday that
// does not match the date is accepted and ignored.
func ParseStampLine(line string) (StampLine, bool) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i == len(line) || (line[i] != 'S' && line[i] != 'D') {
		return StampLine{}, false
	}
	m := stampLineRe.FindStringSubmatchIndex(line)
	if m == nil {
		return StampLine{}, false
	}
	sub := func(g int) string {
		if m[2*g] < 0 {
			return ""
		}
		return line[m[2*g]:m[2*g+1]]
	}
	s := StampLine{
		Kind:     StampScheduled,
		Start:    m[2],
		Date:     sub(2),
		Weekday:  sub(3),
		Time:     sub(4),
		Repeater: sub(5),
	}
	if sub(1) == "DEADLINE" {
		s.Kind = StampDeadline
	}
	// End is just after '>': the closing bracket precedes only whitespace.
	end := len(line)
	for end > 0 && line[end-1] != '>' {
		end--
	}
	s.End = end
	if _, err := time.Parse("2006-01-02", s.Date); err != nil {
		return StampLine{}, false
	}
	if s.Time != "" {
		h := int(s.Time[0]-'0')*10 + int(s.Time[1]-'0')
		min := int(s.Time[3]-'0')*10 + int(s.Time[4]-'0')
		if h > 23 || min > 59 {
			return StampLine{}, false
		}
	}
	return s, true
}

var relDateRe = regexp.MustCompile(`^\+(\d{1,4})([dw])$`)

// ParseDateInput reads a date typed into the editor's date prompt, relative to
// today's civil date: YYYY-MM-DD, "today", "tomorrow", "+Nd"/"+Nw", or an
// English weekday name (full or three letters) meaning the next such day
// strictly after today. Case and surrounding space are ignored. The result is
// YYYY-MM-DD.
func ParseDateInput(s string, today time.Time) (date string, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	y, m, d := today.Date()
	base := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	const layout = "2006-01-02"
	switch s {
	case "today":
		return base.Format(layout), true
	case "tomorrow":
		return base.AddDate(0, 0, 1).Format(layout), true
	}
	if t, err := time.Parse(layout, s); err == nil {
		return t.Format(layout), true
	}
	if m := relDateRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		if m[2] == "w" {
			n *= 7
		}
		return base.AddDate(0, 0, n).Format(layout), true
	}
	for wd := time.Sunday; wd <= time.Saturday; wd++ {
		name := strings.ToLower(wd.String())
		if s == name || s == name[:3] {
			ahead := (int(wd)-int(base.Weekday())+6)%7 + 1
			return base.AddDate(0, 0, ahead).Format(layout), true
		}
	}
	return "", false
}

// StampText is the stamp's text "KEYWORD: <date Www[ time][ repeater]>"; Www is
// the date's weekday. date must be a valid YYYY-MM-DD.
func StampText(kind StampKind, date, timeOfDay, repeater string) string {
	var b strings.Builder
	b.WriteString(kind.Keyword())
	b.WriteString(": <")
	b.WriteString(date)
	if t, err := time.Parse("2006-01-02", date); err == nil {
		b.WriteByte(' ')
		b.WriteString(t.Weekday().String()[:3])
	}
	if timeOfDay != "" {
		b.WriteByte(' ')
		b.WriteString(timeOfDay)
	}
	if repeater != "" {
		b.WriteByte(' ')
		b.WriteString(repeater)
	}
	b.WriteByte('>')
	return b.String()
}
