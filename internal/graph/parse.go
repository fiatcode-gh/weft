package graph

import (
	"regexp"
	"strings"
)

// LinkHit is one wiki-link occurrence in a page body.
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
		links = appendWikiLinks(links, line, i+1)
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

// appendWikiLinks appends every [[link]] on line (1-based lineNo) to out.
// Targets like [[A|alias]] are recorded as "A".
func appendWikiLinks(out []LinkHit, line string, lineNo int) []LinkHit {
	// Fast path: a line with no "[[" can't hold a link, so skip the backtick
	// split + regex scan that every other line would otherwise pay for.
	if !strings.Contains(line, "[[") {
		return out
	}
	// Scan only OUTSIDE inline-backtick code spans, matching the renderer
	// (render.replaceWikiLinksOutsideInlineCode): split on backticks, where
	// even-indexed segments are literal text and odd-indexed are inline code.
	// Keeps the backlink index and the rendered links in agreement, so a
	// `[[Foo]]` written as a literal example isn't a phantom backlink.
	for seg, part := range strings.Split(line, "`") {
		if seg%2 == 1 {
			continue // inside inline code
		}
		for _, m := range wikiLinkRe.FindAllStringSubmatch(part, -1) {
			raw := strings.TrimSpace(m[1])
			if raw == "" {
				continue
			}
			out = append(out, LinkHit{Target: raw, Line: lineNo})
		}
	}
	return out
}

// ExtractWikiLinks returns every [[link]] in body, skipping fenced code blocks.
// Targets like [[A|alias]] are recorded as "A".
func ExtractWikiLinks(body string) []LinkHit {
	_, links, _ := parseBody(body)
	return links
}

// ExtractTodos returns open TODO/LATER/DOING/WAITING bullets in body.
// DONE and CANCELED are intentionally ignored (we only surface open tasks).
func ExtractTodos(body string) []TodoHit {
	_, _, todos := parseBody(body)
	return todos
}
