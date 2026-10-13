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
	Marker              string // TODO | LATER | DOING | WAITING
	Priority            string // "" | "A" | "B" | "C"
	Text                string
	Line                int // 1-based
	Scheduled, Deadline Stamp
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
func parseBody(body string) (lines []string, links []linkSpan, todos []TodoHit) {
	lines = strings.Split(body, "\n")
	var w lineWalker
	cur := -1 // index in todos of the open task owning the current lines
	for i, line := range lines {
		fenced, bullet, stamp, isStamp := w.step(line)
		if bullet {
			cur = -1
		}
		if fenced {
			continue
		}
		links = appendLineLinks(links, line, i+1)
		if m := openTaskMatch(line); m != nil {
			todos = append(todos, TodoHit{
				Marker:   m[1],
				Priority: m[2],
				Text:     m[3],
				Line:     i + 1,
			})
			// A todo inside a LOGBOOK drawer is indexed but is not a bullet,
			// so it never owns the stamps after the drawer.
			if bullet {
				cur = len(todos) - 1
			}
		}
		if isStamp && cur >= 0 {
			slot := &todos[cur].Scheduled
			if stamp.Kind == StampDeadline {
				slot = &todos[cur].Deadline
			}
			if *slot == (Stamp{}) {
				*slot = stamp.Stamp()
			}
		}
	}
	return lines, links, todos
}

// linkKind is the written form of a counted link.
type linkKind uint8

const (
	linkWiki       linkKind = iota // [[Target]] / [[Target|alias]]
	linkTag                        // #name
	linkBracketTag                 // #[[Target]] / #[[Target|alias]]
)

// linkSpan is one link the index counts, located on its line.
type linkSpan struct {
	target                 string // as indexed: trimmed text before '|', or the tag name
	line                   int    // 1-based
	start, end             int    // line bytes of the whole occurrence: "[[…]]", "#name", "#[[…]]"
	targetStart, targetEnd int    // line bytes of the target as written (untrimmed): between "[[" and '|' or "]]"; the name after '#'
	kind                   linkKind
}

// appendLineLinks appends every [[link]], #tag and #[[tag]] on line (1-based
// lineNo) to out, in byte order. Targets like [[A|alias]] are recorded as "A";
// a bracket tag #[[X]] is its wiki link and is counted once.
func appendLineLinks(out []linkSpan, line string, lineNo int) []linkSpan {
	// Fast path: a line with neither "[[" nor "#" holds no link or tag, so
	// skip the backtick split + regex scan that every other line would pay for.
	hasWiki := strings.Contains(line, "[[")
	if !hasWiki && strings.IndexByte(line, '#') < 0 {
		return out
	}
	first := len(out)
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
					out = append(out, linkSpan{
						target: raw, line: lineNo,
						start: off + m[0], end: off + m[1],
						targetStart: off + m[2], targetEnd: off + m[3],
						kind: linkWiki,
					})
				}
			}
			off += len(part) + 1
		}
	}
	for _, t := range FindTags(line) {
		if !t.Bracket {
			out = append(out, linkSpan{
				target: t.Name, line: lineNo,
				start: t.Start, end: t.End,
				targetStart: t.Start + 1, targetEnd: t.End,
				kind: linkTag,
			})
			continue
		}
		for k := first; k < len(out); k++ {
			if out[k].kind == linkWiki && out[k].start == t.Start+1 {
				out[k].kind = linkBracketTag
				out[k].start = t.Start
				break
			}
		}
	}
	added := out[first:]
	sort.SliceStable(added, func(i, j int) bool { return added[i].start < added[j].start })
	return out
}

// ExtractLinks returns every [[link]] and #tag in body, skipping fenced code.
// Targets like [[A|alias]] are recorded as "A"; #name is recorded as "name".
func ExtractLinks(body string) []LinkHit {
	_, spans, _ := parseBody(body)
	if len(spans) == 0 {
		return nil
	}
	links := make([]LinkHit, len(spans))
	for i, s := range spans {
		links[i] = LinkHit{Target: s.target, Line: s.line}
	}
	return links
}

// ExtractTodos returns open TODO/LATER/DOING/WAITING bullets in body.
// DONE and CANCELED are intentionally ignored (we only surface open tasks).
func ExtractTodos(body string) []TodoHit {
	_, _, todos := parseBody(body)
	return todos
}
