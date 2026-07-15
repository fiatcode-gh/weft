package graph

import "regexp"

// FenceDelimiterRe matches a code-fence delimiter line: optional
// indentation, an optional list-bullet prefix (Logseq nests fences
// inside bullets), then a backtick or tilde fence marker. Exported so
// the editor tint (internal/views) classifies delimiter rows with the
// same grammar the index and renderer use.
var FenceDelimiterRe = regexp.MustCompile("^\\s*(?:[-*+]\\s+)?(```|~~~)")

// FenceState tracks fenced-code state across a top-to-bottom line walk.
// The zero value means "outside any fence". A fence opened with
// backticks is closed only by backticks (tildes only by tildes),
// matching CommonMark: the other marker inside an open fence is content.
type FenceState struct {
	open byte // '`' or '~'; 0 = outside a fence
}

// Step consumes one line and reports whether that line belongs to
// fenced code — either as content or as a fence delimiter itself.
func (f *FenceState) Step(line string) bool {
	m := FenceDelimiterRe.FindStringSubmatch(line)
	if m == nil {
		return f.open != 0
	}
	marker := m[1][0]
	switch {
	case f.open == 0:
		f.open = marker
	case f.open == marker:
		f.open = 0
	}
	return true
}
