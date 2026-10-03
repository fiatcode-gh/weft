package graph

import (
	"path/filepath"
	"strings"

	"github.com/fiatcode-gh/weft/v2/internal/search"
)

// UnlinkedRef is a bare-text mention of a page elsewhere in the graph that is
// not already a [[link]]. PageName is the referencing page (for the row label
// and navigation); Match locates the mention within Context.
type UnlinkedRef struct {
	FilePath string
	PageName string
	Line     int
	Context  string
	Match    search.Span
}

// FilterUnlinked turns raw ripgrep hits for the target page into unlinked
// references, dropping any hit that is in the target's own file, inside a
// fenced code block, or already inside a [[…]] link. read returns a file's
// body; a read error drops every hit from that file (we can't verify its
// fence state).
func FilterUnlinked(hits []search.Hit, targetPath string, read func(path string) (string, error)) []UnlinkedRef {
	fencedByFile := map[string]map[int]bool{} // nil value == unreadable file
	var out []UnlinkedRef
	for _, h := range hits {
		if h.FilePath == targetPath {
			continue
		}
		fenced, seen := fencedByFile[h.FilePath]
		if !seen {
			body, err := read(h.FilePath)
			if err != nil {
				fenced = nil
			} else {
				fenced = fencedLines(body)
			}
			fencedByFile[h.FilePath] = fenced
		}
		if fenced == nil { // unreadable
			continue
		}
		if fenced[h.Line] {
			continue
		}
		span, ok := firstUnlinkedMatch(h.Context, h.Matches)
		if !ok {
			continue
		}
		out = append(out, UnlinkedRef{
			FilePath: h.FilePath,
			PageName: PageNameFromFilename(filepath.Base(h.FilePath)),
			Line:     h.Line,
			Context:  h.Context,
			Match:    span,
		})
	}
	return out
}

// fencedLines returns the set of 1-based line numbers that fall inside (or are)
// a code fence, mirroring how ExtractWikiLinks skips fenced content.
func fencedLines(body string) map[int]bool {
	fenced := map[int]bool{}
	var fence FenceState
	for i, line := range strings.Split(body, "\n") {
		if fence.Step(line) {
			fenced[i+1] = true
		}
	}
	return fenced
}

// firstUnlinkedMatch returns the first match span on line that does NOT fall
// within a [[…]] link span or an inline-code span. ok is false when every
// match is already linked, in code, or there are no matches.
func firstUnlinkedMatch(line string, matches []search.Span) (search.Span, bool) {
	links := wikiLinkRe.FindAllStringIndex(line, -1)
	code := InlineCodeSpans(line)
	for _, m := range matches {
		inside := false
		for _, l := range links {
			if m.Start >= l[0] && m.End <= l[1] {
				inside = true
				break
			}
		}
		if inside || spanInside(m, code) {
			continue
		}
		return m, true
	}
	return search.Span{}, false
}

// InlineCodeSpans returns the byte ranges of inline code spans on line —
// the odd segments of a backtick split, mirroring how parse.go
// (appendWikiLinks) and render.replaceWikiLinksOutsideInlineCode treat
// backticks: split the line on "`", even-indexed segments are literal text,
// odd-indexed segments are inline code. Each returned span includes its
// leading backtick (and trailing backtick, when the code run is closed).
// An unpaired trailing backtick still opens a code span that runs to the end
// of the line — strings.Split leaves that final segment at an odd index too,
// so parse/render already treat it as code, and this must match.
//
// Exported because internal/render also needs it (hideMarkdownLinkURLsOutsideInlineCode):
// render and parse must agree byte-for-byte on what counts as inline code.
func InlineCodeSpans(line string) []search.Span {
	var spans []search.Span
	start := -1
	for i := 0; i < len(line); i++ {
		if line[i] != '`' {
			continue
		}
		if start < 0 {
			start = i
		} else {
			spans = append(spans, search.Span{Start: start, End: i + 1})
			start = -1
		}
	}
	if start >= 0 {
		// Unpaired trailing backtick: the segment after it is still an
		// odd-indexed (code) segment per strings.Split, just unclosed.
		spans = append(spans, search.Span{Start: start, End: len(line)})
	}
	return spans
}

// spanInside reports whether m falls entirely within one of spans.
func spanInside(m search.Span, spans []search.Span) bool {
	for _, s := range spans {
		if m.Start >= s.Start && m.End <= s.End {
			return true
		}
	}
	return false
}
