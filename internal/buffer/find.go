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
	if query == "" || strings.Contains(query, "\n") {
		return nil
	}
	var out []Range
	for i, line := range b.lines {
		for _, m := range MatchIn(line, query) {
			out = append(out, Range{Pos{i, m[0]}, Pos{i, m[1]}})
		}
	}
	return out
}

// MatchIn returns the byte ranges of the non-overlapping matches of query in s under
// Find's rules (smart case, simple folding, matches start at grapheme boundaries). An
// empty query, or one containing "\n", returns nil.
func MatchIn(s, query string) [][2]int {
	if query == "" || strings.Contains(query, "\n") {
		return nil
	}
	if strings.IndexFunc(query, unicode.IsUpper) >= 0 {
		return findExact(nil, s, query)
	}
	return findFold(nil, s, []rune(query))
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
