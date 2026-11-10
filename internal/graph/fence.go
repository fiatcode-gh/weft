package graph

import (
	"regexp"
	"strings"
)

// FenceDelimiterRe matches the start of a code-fence delimiter line:
// optional indentation, an optional list-bullet prefix (Logseq nests
// fences inside bullets), then a run of three or more backticks or
// tildes (group 1). It does not decide whether the line opens or closes a
// fence — an opener's info string may not hold a backtick, a closer has no
// info string at all — that is FenceState.Step's job. Exported so the
// editor source scanner reads an opener's prefix and marker with the same
// grammar the index and renderer use.
var FenceDelimiterRe = regexp.MustCompile("^\\s*(?:[-*+]\\s+)?(`{3,}|~{3,})")

var fenceLineRe = regexp.MustCompile("^\\s*(?:[-*+]\\s+)?(`{3,}|~{3,})(.*)$")

// FenceState tracks fenced-code state across a top-to-bottom line walk.
// The zero value means "outside any fence". It follows CommonMark: a
// backtick opener's info string holds no backtick (so "```x``` y" is inline
// code, not a fence); a fence closes only on a line of the same marker, at
// least as long as the opener, with nothing after it; the other marker
// inside an open fence is content.
type FenceState struct {
	open byte // '`' or '~'; 0 = outside a fence
	n    int  // length of the opening marker run
}

// Step consumes one line and reports whether that line belongs to
// fenced code — either as content or as a fence delimiter itself.
func (f *FenceState) Step(line string) bool {
	m := fenceLineRe.FindStringSubmatch(line)
	if f.open == 0 {
		if m == nil || (m[1][0] == '`' && strings.Contains(m[2], "`")) {
			return false
		}
		f.open, f.n = m[1][0], len(m[1])
		return true
	}
	if m != nil && m[1][0] == f.open && len(m[1]) >= f.n && strings.TrimSpace(m[2]) == "" {
		f.open, f.n = 0, 0
	}
	return true
}

// Open reports whether the walk is inside a fenced code block after the
// last Step (a closing delimiter leaves it false).
func (f *FenceState) Open() bool { return f.open != 0 }
