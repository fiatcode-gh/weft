package graph

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Heading is one ATX or setext heading of a page body.
type Heading struct {
	Line  int    // 0-based line holding the heading text
	Under bool   // setext: Line+1 is its underline
	Level int    // 1–6 (setext: '=' is 1, '-' is 2)
	Text  string // the text as displayed (see HeadingKey)
}

// The recognisers below mirror render.Scanner.classify line for line; graph
// cannot import render, and render's heading_parity_test pins the two together.
var (
	atxHeadingRe  = regexp.MustCompile(`^ {0,3}(#{1,6})(?:\s+|$)`)
	atxClosingRe  = regexp.MustCompile(`(?:^|[ \t]+)#+[ \t]*$`)
	setextLineRe  = regexp.MustCompile(`^ {0,3}(=+|-+) *$`)
	bulletLineRe  = regexp.MustCompile(`^\s*[-*+](?:\s+|$)`)
	orderedLineRe = regexp.MustCompile(`^\s*\d{1,9}[.)](?:\s+|$)`)
	quoteLineRe   = regexp.MustCompile(`^\s*> ?`)
	tableDelimRe  = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
	headingWikiRe = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]*))?\]\]`)
	emphasisMarks = strings.NewReplacer("`", "", "**", "", "__", "", "~~", "", "*", "")
)

type headingState struct {
	hidden uint8 // 0 none, 1 logbook, 2 query/embed block
	fence  FenceState
	setext int  // pending underline level of the previous line
	table  int  // 0 none, 1 header seen, 2 body
	inList bool // a list is open at the current line
}

// Headings returns body's headings in line order.
func Headings(body string) []Heading {
	lines := strings.Split(body, "\n")
	var (
		st  headingState
		out []Heading
	)
	for i, line := range lines {
		next, hasNext := "", i+1 < len(lines)
		if hasNext {
			next = lines[i+1]
		}
		if h, ok := st.step(i, line, next, hasNext); ok {
			out = append(out, h)
		}
	}
	return out
}

func indentOf(line string) int {
	cells := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			cells++
		case '\t':
			cells += 4 - cells%4
		default:
			return cells
		}
	}
	return cells
}

func isThematicBreak(line string) bool {
	rest := strings.TrimLeft(line, " \t")
	if len(line)-len(rest) > 3 || rest == "" {
		return false
	}
	c := rest[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	count := 0
	for i := range len(rest) {
		switch rest[i] {
		case c:
			count++
		case ' ':
		default:
			return false
		}
	}
	return count >= 3
}

func (st *headingState) clearList(indent int) {
	if indent == 0 {
		st.inList = false
	}
}

func (st *headingState) step(idx int, line, next string, hasNext bool) (Heading, bool) {
	indent := indentOf(line)
	switch st.hidden {
	case 1:
		if LogbookEndRe.MatchString(line) {
			st.hidden = 0
		}
		return Heading{}, false
	case 2:
		if strings.TrimSpace(line) == "}}" {
			st.hidden = 0
		}
		return Heading{}, false
	}
	if st.fence.Step(line) {
		return Heading{}, false
	}
	if LogbookStartRe.MatchString(line) {
		st.hidden = 1
		return Heading{}, false
	}
	if loc := QueryOrEmbedRe.FindStringIndex(line); loc != nil {
		if !strings.Contains(line[loc[1]:], "}}") {
			st.hidden = 2
		}
		return Heading{}, false
	}

	setext := st.setext
	st.setext = 0
	table := st.table
	st.table = 0

	if setext > 0 || strings.TrimSpace(line) == "" {
		return Heading{}, false
	}
	if table == 1 || (table == 2 && strings.Contains(line, "|")) {
		st.table = 2
		return Heading{}, false
	}

	switch {
	case atxHeadingRe.MatchString(line):
		m := atxHeadingRe.FindStringSubmatch(line)
		rest := strings.TrimRight(line[len(m[0]):], " \t\r")
		rest = atxClosingRe.ReplaceAllString(rest, "")
		st.clearList(indent)
		return Heading{Line: idx, Level: len(m[1]), Text: displayText(rest)}, true
	case isThematicBreak(line):
		st.clearList(indent)
	case bulletLineRe.MatchString(line), orderedLineRe.MatchString(line):
		st.inList = true
	case quoteLineRe.MatchString(line):
		st.clearList(indent)
	case strings.Contains(line, "|") && hasNext &&
		strings.Contains(next, "|") && tableDelimRe.MatchString(next):
		st.table = 1
		st.clearList(indent)
	default:
		if indent > 0 && st.inList {
			return Heading{}, false
		}
		st.clearList(indent)
		if m := setextLineRe.FindStringSubmatch(next); hasNext && m != nil {
			level := 1
			if m[1][0] == '-' {
				level = 2
			}
			st.setext = level
			return Heading{Line: idx, Under: true, Level: level, Text: displayText(line)}, true
		}
	}
	return Heading{}, false
}

// HeadingKey is the case-folded display text of s: what a [[Page#Heading]]
// fragment is matched against.
func HeadingKey(s string) string { return strings.ToLower(displayText(s)) }

// displayText renders heading markup the way the page shows it: wiki links by
// alias or target, markdown links by their text, emphasis and code marks and
// word-edge underscores dropped, whitespace collapsed.
func displayText(s string) string {
	s = headingWikiRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := headingWikiRe.FindStringSubmatch(m)
		if sub[2] != "" {
			return sub[2]
		}
		return sub[1]
	})
	s = MarkdownLinkRe.ReplaceAllString(s, "$2")
	s = emphasisMarks.Replace(s)
	s = dropEdgeUnderscores(s)
	return strings.Join(strings.Fields(s), " ")
}

func dropEdgeUnderscores(s string) string {
	if !strings.Contains(s, "_") {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if r == '_' {
			before, _ := utf8.DecodeLastRuneInString(s[:i])
			after, _ := utf8.DecodeRuneInString(s[i+1:])
			if !isAlnum(before) || !isAlnum(after) {
				continue
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isAlnum(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
