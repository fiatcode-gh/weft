package layout

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type token int

const (
	tokYear token = iota
	tokMonth
	tokDay
)

// pattern is a compiled journal_file_pattern. Tokens YYYY, MM and DD may
// repeat; everything else is literal.
type pattern struct {
	segments []segment
	full     *regexp.Regexp // whole pattern, anchored
	fullToks []token        // token of each capture group in full
}

type segment struct {
	parts []part
	re    *regexp.Regexp // this segment alone, anchored
	toks  []token
}

type part struct {
	lit string
	tok token
	isT bool
}

func compilePattern(s string) (pattern, error) {
	if s == "" {
		return pattern{}, errors.New("must not be empty")
	}
	raw := strings.Split(s, "/")
	for _, seg := range raw {
		if seg == "" || seg == "." || seg == ".." {
			return pattern{}, errors.New(`has an empty, "." or ".." path segment`)
		}
		if strings.HasPrefix(seg, ".") {
			return pattern{}, errors.New(`has a path segment starting with "."`)
		}
	}
	var p pattern
	var full strings.Builder
	full.WriteString("^")
	for i, seg := range raw {
		sg := segment{parts: tokenise(seg)}
		var b strings.Builder
		for _, pt := range sg.parts {
			switch {
			case !pt.isT:
				b.WriteString(regexp.QuoteMeta(pt.lit))
			case pt.tok == tokYear:
				b.WriteString(`(\d{4})`)
				sg.toks = append(sg.toks, pt.tok)
			default:
				b.WriteString(`(\d{2})`)
				sg.toks = append(sg.toks, pt.tok)
			}
		}
		sg.re = regexp.MustCompile("^" + b.String() + "$")
		if i > 0 {
			full.WriteString("/")
		}
		full.WriteString(b.String())
		p.fullToks = append(p.fullToks, sg.toks...)
		p.segments = append(p.segments, sg)
	}
	full.WriteString("$")
	p.full = regexp.MustCompile(full.String())
	if !hasAll(p.segments[len(p.segments)-1].toks) {
		return pattern{}, errors.New("the last path segment must contain YYYY, MM and DD")
	}
	return p, nil
}

func hasAll(toks []token) bool {
	var y, m, d bool
	for _, t := range toks {
		switch t {
		case tokYear:
			y = true
		case tokMonth:
			m = true
		case tokDay:
			d = true
		}
	}
	return y && m && d
}

func tokenise(seg string) []part {
	var parts []part
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			parts = append(parts, part{lit: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(seg); {
		switch {
		case strings.HasPrefix(seg[i:], "YYYY"):
			flush()
			parts = append(parts, part{tok: tokYear, isT: true})
			i += 4
		case strings.HasPrefix(seg[i:], "MM"):
			flush()
			parts = append(parts, part{tok: tokMonth, isT: true})
			i += 2
		case strings.HasPrefix(seg[i:], "DD"):
			flush()
			parts = append(parts, part{tok: tokDay, isT: true})
			i += 2
		default:
			lit.WriteByte(seg[i])
			i++
		}
	}
	flush()
	return parts
}

// date turns regexp submatches into a civil day. Repeated tokens must agree
// and the result must be a real calendar date.
func date(toks []token, sub []string) (time.Time, bool) {
	if sub == nil {
		return time.Time{}, false
	}
	var v [3]int
	var seen [3]bool
	for i, t := range toks {
		n, err := strconv.Atoi(sub[i+1])
		if err != nil || (seen[t] && v[t] != n) {
			return time.Time{}, false
		}
		v[t], seen[t] = n, true
	}
	d := time.Date(v[tokYear], time.Month(v[tokMonth]), v[tokDay], 0, 0, 0, 0, time.UTC)
	if d.Year() != v[tokYear] || int(d.Month()) != v[tokMonth] || d.Day() != v[tokDay] {
		return time.Time{}, false
	}
	return d, true
}

func (p pattern) format(day time.Time, from int) string {
	var b strings.Builder
	for i := from; i < len(p.segments); i++ {
		if i > from {
			b.WriteByte('/')
		}
		for _, pt := range p.segments[i].parts {
			switch {
			case !pt.isT:
				b.WriteString(pt.lit)
			case pt.tok == tokYear:
				fmt.Fprintf(&b, "%04d", day.Year())
			case pt.tok == tokMonth:
				fmt.Fprintf(&b, "%02d", int(day.Month()))
			default:
				fmt.Fprintf(&b, "%02d", day.Day())
			}
		}
	}
	return b.String()
}

// Journal reports the date of rel, a graph-relative file path with ".md",
// when it is a journal: inside the journals dir and matching the pattern.
func (l Layout) Journal(rel string) (time.Time, bool) {
	stem, ok := strings.CutSuffix(rel, ".md")
	if !ok {
		return time.Time{}, false
	}
	if l.journalsDir != "" {
		if stem, ok = strings.CutPrefix(stem, l.journalsDir+"/"); !ok {
			return time.Time{}, false
		}
	}
	return date(l.pattern.fullToks, l.pattern.full.FindStringSubmatch(stem))
}

// JournalName is the journal's file stem: the last pattern segment.
func (l Layout) JournalName(day time.Time) string {
	return l.pattern.format(day, len(l.pattern.segments)-1)
}

// JournalRel is the journal's graph-relative path, with ".md".
func (l Layout) JournalRel(day time.Time) string {
	return joinRel(l.journalsDir, l.pattern.format(day, 0)+".md")
}

// ParseJournalName maps a file stem to its date.
func (l Layout) ParseJournalName(name string) (time.Time, bool) {
	last := l.pattern.segments[len(l.pattern.segments)-1]
	return date(last.toks, last.re.FindStringSubmatch(name))
}
