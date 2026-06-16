package graph

import (
	"path/filepath"
	"strings"

	"git.fiatcode.dev/fiatcode/weft/internal/search"
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

// FilterUnlinked turns raw ripgrep hits for `target` into unlinked references,
// dropping any hit that is in the target's own file, inside a fenced code
// block, or already inside a [[…]] link. read returns a file's body; a read
// error drops every hit from that file (we can't verify its fence state).
func FilterUnlinked(hits []search.Hit, target, targetPath string, read func(path string) (string, error)) []UnlinkedRef {
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
// a ``` fence, mirroring how ExtractWikiLinks skips fenced content.
func fencedLines(body string) map[int]bool {
	fenced := map[int]bool{}
	inFence := false
	for i, line := range strings.Split(body, "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			fenced[i+1] = true
			continue
		}
		if inFence {
			fenced[i+1] = true
		}
	}
	return fenced
}

// firstUnlinkedMatch returns the first match span on line that does NOT fall
// within a [[…]] link span. ok is false when every match is already linked
// (or there are no matches).
func firstUnlinkedMatch(line string, matches []search.Span) (search.Span, bool) {
	links := wikiLinkRe.FindAllStringIndex(line, -1)
	for _, m := range matches {
		inside := false
		for _, l := range links {
			if m.Start >= l[0] && m.End <= l[1] {
				inside = true
				break
			}
		}
		if !inside {
			return m, true
		}
	}
	return search.Span{}, false
}
