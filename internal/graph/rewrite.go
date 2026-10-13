package graph

import "strings"

// pageLink is a counted link that resolves to a given page.
type pageLink struct {
	linkSpan
	heading bool // ResolveLink gave a fragment
}

// linksToPage returns body's lines and the counted links in body that resolve
// to page (Path equality), in body order. It uses parseBody, the scanner
// BuildIndex counts with, so it agrees with the index by construction.
func (idx *Index) linksToPage(body string, page *PageMeta) (lines []string, links []pageLink) {
	lines, spans, _ := parseBody(body)
	for _, s := range spans {
		if dest, ok := idx.ResolveLink(s.target); ok && dest.Page.Path == page.Path {
			links = append(links, pageLink{s, dest.Heading != ""})
		}
	}
	return lines, links
}

// RewriteLinks returns body with every link the index resolves to page
// rewritten to key, and how many it rewrote. Alias and fragment are kept; a
// simple tag takes the simple form when key allows it, a bracket tag stays
// bracket. Text outside the rewritten spans is byte-identical; a body with no
// such link is returned unchanged with 0.
func (idx *Index) RewriteLinks(body string, page *PageMeta, key string) (string, int) {
	lines, links := idx.linksToPage(body, page)
	if len(links) == 0 {
		return body, 0
	}
	// Splice right to left within each line so earlier byte offsets stay valid.
	for i := len(links) - 1; i >= 0; i-- {
		s := links[i]
		line := lines[s.line-1]
		t := key
		if s.heading {
			t = key + s.target[strings.LastIndexByte(s.target, '#'):]
		}
		switch {
		case s.kind == linkTag && IsSimpleTagName(key):
			line = line[:s.start] + "#" + key + line[s.end:]
		case s.kind == linkTag:
			line = line[:s.start] + "#[[" + key + "]]" + line[s.end:]
		default: // linkWiki, linkBracketTag
			line = line[:s.targetStart] + t + line[s.targetEnd:]
		}
		lines[s.line-1] = line
	}
	return strings.Join(lines, "\n"), len(links)
}
