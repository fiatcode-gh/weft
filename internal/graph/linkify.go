package graph

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"git.fiatcode.dev/fiatcode/peekseq/internal/search"
)

// ErrMentionNotFound is returned by LinkifyMention when the target line is out
// of range or holds no unlinked, whole-word occurrence of the name to wrap.
var ErrMentionNotFound = errors.New("mention not found")

// LinkifyMention wraps the first unlinked, whole-word, case-insensitive
// occurrence of target on the 1-based line in body as [[<matched>]]. The matched
// bytes are preserved verbatim (Resolve is fold-keyed, so casing need not be
// normalised). The splice is by absolute byte offset, so line endings (incl.
// CRLF) and any trailing newline are preserved untouched. span is the matched
// range in the original body. It returns ErrMentionNotFound when no such
// occurrence exists — which also guards against double-wrapping an already
// [[linked]] mention and against a line that changed since detection.
func LinkifyMention(body string, line int, target string) (string, search.Span, error) {
	lineStart, lineEnd, ok := lineBounds(body, line)
	if !ok {
		return "", search.Span{}, ErrMentionNotFound
	}
	rel, ok := firstUnlinkedOccurrence(body[lineStart:lineEnd], target)
	if !ok {
		return "", search.Span{}, ErrMentionNotFound
	}
	start, end := lineStart+rel.Start, lineStart+rel.End
	newBody := body[:start] + "[[" + body[start:end] + "]]" + body[end:]
	return newBody, search.Span{Start: start, End: end}, nil
}

// lineBounds returns the [start, end) byte offsets of the 1-based line's content
// (excluding the trailing newline) within body. ok is false when line < 1 or
// runs past the end of body.
func lineBounds(body string, line int) (start, end int, ok bool) {
	if line < 1 {
		return 0, 0, false
	}
	for cur := 1; cur < line; cur++ {
		i := strings.IndexByte(body[start:], '\n')
		if i < 0 {
			return 0, 0, false
		}
		start += i + 1
	}
	if i := strings.IndexByte(body[start:], '\n'); i >= 0 {
		return start, start + i, true
	}
	return start, len(body), true
}

// firstUnlinkedOccurrence finds the first whole-word, case-insensitive, literal
// occurrence of target in line that is not inside a [[…]] link. The returned
// span is relative to line.
func firstUnlinkedOccurrence(line, target string) (search.Span, bool) {
	re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(target))
	locs := re.FindAllStringIndex(line, -1)
	if locs == nil {
		return search.Span{}, false
	}
	links := wikiLinkRe.FindAllStringIndex(line, -1)
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		if !wholeWordAt(line, start, end) {
			continue
		}
		inside := false
		for _, l := range links {
			if start >= l[0] && end <= l[1] {
				inside = true
				break
			}
		}
		if !inside {
			return search.Span{Start: start, End: end}, true
		}
	}
	return search.Span{}, false
}

// wholeWordAt reports whether s[start:end] is bounded by string edges or
// non-word runes on both sides (Unicode-aware), mirroring ripgrep -w. This is a
// deliberate local copy of the identical helper in internal/render: graph must
// not depend on render, and a shared package for two ~5-line helpers would be
// premature (cf. the theme-color note in theme.go).
func wholeWordAt(s string, start, end int) bool {
	if start > 0 {
		if r, _ := utf8.DecodeLastRuneInString(s[:start]); isWordRune(r) {
			return false
		}
	}
	if end < len(s) {
		if r, _ := utf8.DecodeRuneInString(s[end:]); isWordRune(r) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}
