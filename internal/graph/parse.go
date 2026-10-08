package graph

import (
	"regexp"
	"sort"
	"strings"
)

// LinkHit is one link occurrence in a page body: a wiki link or a tag.
type LinkHit struct {
	Target string
	Line   int // 1-based
}

// TodoHit is one open task bullet occurrence in a page body.
type TodoHit struct {
	Marker   string // TODO | LATER | DOING | WAITING
	Priority string // "" | "A" | "B" | "C"
	Text     string
	Line     int // 1-based
}

var (
	wikiLinkRe = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|[^\]]*)?\]\]`)
	todoRe     = regexp.MustCompile(`^\s*-\s+(TODO|LATER|DOING|WAITING)(?:\s+\[#([ABC])\])?\s+(.*\S)\s*$`)
)

// openTaskMatch returns the TODO/LATER/DOING/WAITING captures for line.
func openTaskMatch(line string) []string {
	return todoRe.FindStringSubmatch(line)
}

// IsOpenTask reports whether line is an open TODO/LATER/DOING/WAITING bullet.
// It classifies one line only and is fence-context-free; callers scanning
// documents must track and exclude fenced lines.
func IsOpenTask(line string) bool {
	return openTaskMatch(line) != nil
}

// parseBody walks body's lines once, tracking fenced-code state, and returns
// the split lines (callers reuse them for context lookup) alongside every
// wiki-link and open todo. A single pass over a single line-split avoids
// re-splitting and re-scanning the same body once per extractor.
//
// An unterminated fence causes following lines to be skipped — that matches how
// a markdown renderer would treat the rest of the page as code.
func parseBody(body string) (lines []string, links []LinkHit, todos []TodoHit) {
	lines = strings.Split(body, "\n")
	var fence FenceState
	for i, line := range lines {
		if fence.Step(line) {
			continue
		}
		links = appendLinks(links, line, i+1)
		if m := openTaskMatch(line); m != nil {
			todos = append(todos, TodoHit{
				Marker:   m[1],
				Priority: m[2],
				Text:     m[3],
				Line:     i + 1,
			})
		}
	}
	return lines, links, todos
}

// appendLinks appends every [[link]] and simple #tag on line (1-based lineNo)
// to out, in byte order. Targets like [[A|alias]] are recorded as "A"; a
// bracket tag #[[X]] is its wiki link and is counted once.
func appendLinks(out []LinkHit, line string, lineNo int) []LinkHit {
	// Fast path: a line with neither "[[" nor "#" holds no link or tag, so
	// skip the backtick split + regex scan that every other line would pay for.
	hasWiki := strings.Contains(line, "[[")
	if !hasWiki && strings.IndexByte(line, '#') < 0 {
		return out
	}
	type hit struct {
		at     int
		target string
	}
	var hits []hit
	if hasWiki {
		// Scan only OUTSIDE inline-backtick code spans, matching the renderer
		// (render.replaceLinksOutsideInlineCode): split on backticks, where
		// even-indexed segments are literal text and odd-indexed are inline
		// code. Keeps the backlink index and the rendered links in agreement,
		// so a `[[Foo]]` written as a literal example isn't a phantom backlink.
		off := 0
		for seg, part := range strings.Split(line, "`") {
			if seg%2 == 0 {
				for _, m := range wikiLinkRe.FindAllStringSubmatchIndex(part, -1) {
					raw := strings.TrimSpace(part[m[2]:m[3]])
					if raw == "" {
						continue
					}
					hits = append(hits, hit{off + m[0], raw})
				}
			}
			off += len(part) + 1
		}
	}
	for _, t := range FindTags(line) {
		if !t.Bracket {
			hits = append(hits, hit{t.Start, t.Name})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].at < hits[j].at })
	for _, h := range hits {
		out = append(out, LinkHit{Target: h.target, Line: lineNo})
	}
	return out
}

// ExtractLinks returns every [[link]] and #tag in body, skipping fenced code.
// Targets like [[A|alias]] are recorded as "A"; #name is recorded as "name".
func ExtractLinks(body string) []LinkHit {
	_, links, _ := parseBody(body)
	return links
}

// ExtractTodos returns open TODO/LATER/DOING/WAITING bullets in body.
// DONE and CANCELED are intentionally ignored (we only surface open tasks).
func ExtractTodos(body string) []TodoHit {
	_, _, todos := parseBody(body)
	return todos
}
