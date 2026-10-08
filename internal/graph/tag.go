package graph

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/fiatcode-gh/weft/v2/internal/search"
)

// MarkdownLinkRe matches a markdown link or image: group 1 is "!" for an
// image, group 2 the text, group 3 the destination. Shared with render, which
// hides destinations before substituting links, so the index and the read view
// agree that a "#" inside a destination is never a tag.
var MarkdownLinkRe = regexp.MustCompile(`(!?)\[([^\]]*)\]\(((?:[^()]|\([^()]*\))*)\)`)

// Tag is one tag on a line.
type Tag struct {
	Start, End int    // bytes of the whole tag: '#' through the name's last byte, or through "]]" of #[[…]]
	Name       string // simple form: the page name after '#'; "" when Bracket
	Bracket    bool   // #[[…]]: line[Start+1:End] is a wiki link whose target is the page
}

// IsTagNameRune reports whether r may appear in a simple tag name.
func IsTagNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '/'
}

// IsSimpleTagName reports whether name can be written as #name: non-empty,
// first rune a letter, every rune a name rune, and not a hex colour.
func IsSimpleTagName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !unicode.IsLetter(r) {
			return false
		}
		if !IsTagNameRune(r) {
			return false
		}
	}
	return !isHexColour(name)
}

// isHexColour reports whether name is shaped like a CSS hex colour without
// its '#': 3, 4, 6 or 8 hex digits with at least one decimal digit.
func isHexColour(name string) bool {
	switch len(name) {
	case 3, 4, 6, 8:
	default:
		return false
	}
	digit := false
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return digit
}

// tagContext holds the spans of one line inside which a '#' never starts a
// tag: inline code, wiki links and markdown link destinations.
type tagContext struct {
	code, wiki, dest []search.Span
}

func newTagContext(line string) tagContext {
	var ctx tagContext
	if strings.IndexByte(line, '`') >= 0 {
		ctx.code = InlineCodeSpans(line)
	}
	if strings.Contains(line, "[[") {
		// Same per-even-segment matching as appendLinks, in line coordinates.
		off := 0
		for seg, part := range strings.Split(line, "`") {
			if seg%2 == 0 {
				for _, m := range wikiLinkRe.FindAllStringIndex(part, -1) {
					ctx.wiki = append(ctx.wiki, search.Span{Start: off + m[0], End: off + m[1]})
				}
			}
			off += len(part) + 1
		}
	}
	if strings.Contains(line, "](") {
		for _, m := range MarkdownLinkRe.FindAllStringSubmatchIndex(line, -1) {
			ctx.dest = append(ctx.dest, search.Span{Start: m[6], End: m[7]})
		}
	}
	return ctx
}

func (c tagContext) inside(i int) bool {
	for _, spans := range [][]search.Span{c.code, c.wiki, c.dest} {
		for _, s := range spans {
			if i >= s.Start && i < s.End {
				return true
			}
		}
	}
	return false
}

// startAt reports whether line[i] is a '#' at a tag-start position outside
// every excluded span.
func (c tagContext) startAt(line string, i int) bool {
	if i < 0 || i >= len(line) || line[i] != '#' {
		return false
	}
	if i > 0 {
		r, _ := utf8.DecodeLastRuneInString(line[:i])
		if !unicode.IsSpace(r) && r != '(' {
			return false
		}
	}
	return !c.inside(i)
}

// TagStartAt reports whether line[i] is a '#' at a tag-start position: the
// line start, or after whitespace or '(', and outside inline code, wiki links
// and markdown link destinations.
func TagStartAt(line string, i int) bool {
	if i < 0 || i >= len(line) || line[i] != '#' {
		return false
	}
	return newTagContext(line).startAt(line, i)
}

// FindTags returns the tags on one line outside inline code, in byte order,
// or nil when there are none. Callers own fence state.
func FindTags(line string) []Tag {
	if strings.IndexByte(line, '#') < 0 {
		return nil
	}
	ctx := newTagContext(line)
	var tags []Tag
	for i := 0; i < len(line); i++ {
		if !ctx.startAt(line, i) {
			continue
		}
		if end, ok := wikiSpanStartingAt(ctx.wiki, i+1); ok {
			tags = append(tags, Tag{Start: i, End: end, Bracket: true})
			i = end - 1
			continue
		}
		end := i + 1
		for end < len(line) {
			r, size := utf8.DecodeRuneInString(line[end:])
			if !IsTagNameRune(r) {
				break
			}
			end += size
		}
		name := line[i+1 : end]
		if name == "" {
			continue
		}
		if r, _ := utf8.DecodeRuneInString(name); !unicode.IsLetter(r) || isHexColour(name) {
			continue
		}
		tags = append(tags, Tag{Start: i, End: end, Name: name})
		i = end - 1
	}
	return tags
}

func wikiSpanStartingAt(spans []search.Span, start int) (end int, ok bool) {
	for _, s := range spans {
		if s.Start == start {
			return s.End, true
		}
	}
	return 0, false
}
