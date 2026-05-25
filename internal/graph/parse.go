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
	fenceRe    = regexp.MustCompile("^\\s*```")
)

// ExtractWikiLinks returns every [[link]] in body, skipping fenced code blocks.
// Targets like [[A|alias]] are recorded as "A".
// An unterminated fence in body causes following lines to be skipped — that
// matches how a markdown renderer would treat the rest of the page as code.
func ExtractWikiLinks(body string) []LinkHit {
	var out []LinkHit
	inFence := false
	for i, line := range strings.Split(body, "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range wikiLinkRe.FindAllStringSubmatch(line, -1) {
			out = append(out, LinkHit{Target: strings.TrimSpace(m[1]), Line: i + 1})
		}
	}
	return out
}

// ExtractTodos returns open TODO/LATER/DOING/WAITING bullets in body.
// DONE and CANCELED are intentionally ignored (we only surface open tasks).
// An unterminated fence in body causes following lines to be skipped — that
// matches how a markdown renderer would treat the rest of the page as code.
func ExtractTodos(body string) []TodoHit {
	var out []TodoHit
	inFence := false
	for i, line := range strings.Split(body, "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := todoRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out = append(out, TodoHit{
			Marker:   m[1],
			Priority: m[2],
			Text:     m[3],
			Line:     i + 1,
		})
	}
	return out
}
