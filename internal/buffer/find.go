package buffer

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// Find returns every non-overlapping match of query in document order. Matching is literal and
// never crosses a line break. A query with no upper-case rune (unicode.IsUpper) ignores case by
// Unicode simple folding; otherwise case must match (ripgrep --smart-case, as `/` search).
// An empty query, or one containing "\n", returns nil.
func (b *Buffer) Find(query string) []Range {
	m, ok := newMatcher(query)
	if !ok {
		return nil
	}
	var out []Range
	var line [][2]int // one line's matches, reused
	for i, text := range b.lines {
		line = m.appendIn(line[:0], text)
		for _, r := range line {
			out = append(out, Range{Pos{i, r[0]}, Pos{i, r[1]}})
		}
	}
	return out
}

// MatchIn returns the byte ranges of the non-overlapping matches of query in s under
// Find's rules (smart case, simple folding, matches start at grapheme boundaries). An
// empty query, or one containing "\n", returns nil.
func MatchIn(s, query string) [][2]int {
	m, ok := newMatcher(query)
	if !ok {
		return nil
	}
	return m.appendIn(nil, s)
}

// matcher is a query prepared once for many lines: smart case decides between
// exact bytes and rune-wise folding.
type matcher struct {
	query string
	fold  []rune // the query's runes when it ignores case, else nil
}

// newMatcher prepares query; false for a query that matches nothing (empty, or
// containing "\n").
func newMatcher(query string) (matcher, bool) {
	if query == "" || strings.Contains(query, "\n") {
		return matcher{}, false
	}
	if strings.IndexFunc(query, unicode.IsUpper) >= 0 {
		return matcher{query: query}, true
	}
	return matcher{query: query, fold: []rune(query)}, true
}

// appendIn appends the byte ranges of the matches in s to out.
func (m matcher) appendIn(out [][2]int, s string) [][2]int {
	if m.fold == nil {
		return findExact(out, s, m.query)
	}
	return findFold(out, s, m.fold)
}

func findExact(out [][2]int, line, query string) [][2]int {
	for from := 0; from <= len(line)-len(query); {
		j := strings.Index(line[from:], query)
		if j < 0 {
			break
		}
		s := from + j
		out = append(out, [2]int{s, s + len(query)})
		from = s + len(query)
	}
	return out
}

// findFold matches qr rune by rune under simple folding, starting only at grapheme
// boundaries; offsets are bytes of line itself.
func findFold(out [][2]int, line string, qr []rune) [][2]int {
	ascii := true
	for i := 0; i < len(line); i++ {
		if line[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	next := func(i int) int { return i + 1 }
	if !ascii {
		next = func(i int) int {
			cl, _, _, _ := uniseg.FirstGraphemeClusterInString(line[i:], -1)
			if cl == "" {
				return i + 1
			}
			return i + len(cl)
		}
	}
	for i := 0; i < len(line); {
		if end, ok := foldMatchAt(line, i, qr); ok {
			out = append(out, [2]int{i, end})
			i = end
			continue
		}
		i = next(i)
	}
	return out
}

func foldMatchAt(line string, at int, qr []rune) (int, bool) {
	i := at
	for _, q := range qr {
		if i >= len(line) {
			return 0, false
		}
		r, w := utf8.DecodeRuneInString(line[i:])
		if !foldEq(q, r) {
			return 0, false
		}
		i += w
	}
	return i, true
}

func foldEq(a, b rune) bool {
	if a == b {
		return true
	}
	for f := unicode.SimpleFold(a); f != a; f = unicode.SimpleFold(f) {
		if f == b {
			return true
		}
	}
	return false
}

// ReplaceRanges replaces each range (sorted, non-overlapping, valid in the current text) with
// repl as ONE undo group and returns the count; the cursor ends after the last replacement.
// Zero ranges: no group, returns 0.
func (b *Buffer) ReplaceRanges(rs []Range, repl string) int {
	if len(rs) == 0 {
		return 0
	}
	raw := b.toRaw(repl)
	b.edit(func() {
		var (
			lineDelta int
			prevEnd   Pos // original end of the previous range
			newEnd    Pos // where that replacement ended in the current text
		)
		for k, r := range rs {
			start := Pos{r.Start.Line + lineDelta, r.Start.Col}
			if k > 0 && r.Start.Line == prevEnd.Line {
				start = Pos{newEnd.Line, newEnd.Col + r.Start.Col - prevEnd.Col}
			}
			end := Pos{r.End.Line + lineDelta, r.End.Col}
			if r.End.Line == r.Start.Line {
				end.Line, end.Col = start.Line, start.Col+r.End.Col-r.Start.Col
			}
			b.replace(Range{start, end}, raw)
			newEnd = endPos(start, raw)
			lineDelta = newEnd.Line - r.End.Line
			prevEnd = r.End
		}
		b.cursor, b.hasAnchor = newEnd, false
	})
	return len(rs)
}
