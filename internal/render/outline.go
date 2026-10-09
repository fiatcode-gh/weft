package render

import (
	"slices"
	"strings"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// FoldKind says what kind of line a fold hangs from.
type FoldKind uint8

const (
	FoldHeading FoldKind = iota + 1
	FoldBullet           // a bullet or ordered list item
)

// Fold is a line that can hide the lines under it: a heading's section or a
// list item's children. The hidden lines are [Start, End); End-Start is the
// count the read view's fold marker shows.
//
// A heading also has a body, [BodyStart, BodyEnd): its own content up to the
// next heading of any level. The read view's outline level folds that range so
// every heading stays visible. BodyEnd <= BodyStart means no body of its own.
type Fold struct {
	Line       int
	Kind       FoldKind
	Level      int // heading level 1–6; list depth (0 = top level) for bullets
	Start, End int
	BodyStart  int
	BodyEnd    int
}

type lineSlice []string

func (l lineSlice) Len() int          { return len(l) }
func (l lineSlice) Line(i int) string { return l[i] }

// Outline returns body's folds in line order: every heading whose section
// holds a non-blank line and every list item with a non-blank child line.
// Lines are body lines of the string the read view renders, so they index
// SourceRows lines directly.
func Outline(body string) []Fold {
	lines := strings.Split(body, "\n")
	sc := NewScanner()
	src := lineSlice(lines)
	infos := make([]LineInfo, len(lines))
	for i := range lines {
		infos[i] = sc.Info(src, i)
	}
	blank := make([]bool, len(lines))
	for i, l := range lines {
		blank[i] = strings.TrimSpace(l) == ""
	}
	// lastContent is the end (exclusive) of the last line in [from, to) that
	// renders something, or -1 when the range holds none. Blank lines and
	// hidden blocks (logbook, query drawers) render nothing, so a fold over
	// only those would show a marker and a count with nothing under it.
	lastContent := func(from, to int) int {
		for i := to - 1; i >= from; i-- {
			if !blank[i] && infos[i].Kind != KindHidden {
				return i + 1
			}
		}
		return -1
	}

	headings := graph.Headings(body)
	isHeading := make([]bool, len(lines))
	for _, h := range headings {
		isHeading[h.Line] = true
	}

	var folds []Fold
	for i, h := range headings {
		start := h.Line + 1
		if h.Under {
			start++
		}
		raw := len(lines)
		for _, n := range headings[i+1:] {
			if n.Level <= h.Level {
				raw = n.Line
				break
			}
		}
		if end := lastContent(start, raw); end >= 0 {
			fold := Fold{Line: h.Line, Kind: FoldHeading, Level: h.Level, Start: start, End: end}
			bodyRaw := len(lines)
			if i+1 < len(headings) {
				bodyRaw = headings[i+1].Line
			}
			if be := lastContent(start, bodyRaw); be >= 0 {
				fold.BodyStart, fold.BodyEnd = start, be
			}
			folds = append(folds, fold)
		}
	}

	for b := range lines {
		info := infos[b]
		if info.Kind != KindBullet && info.Kind != KindOrdered {
			continue
		}
		raw := len(lines)
		for j := b + 1; j < len(lines); j++ {
			if blank[j] {
				continue
			}
			c := infos[j]
			if isHeading[j] || (c.Indent <= info.Indent && endsChildren(c, b, j)) {
				raw = j
				break
			}
		}
		if end := lastContent(b+1, raw); end >= 0 {
			folds = append(folds, Fold{Line: b, Kind: FoldBullet, Level: info.Level, Start: b + 1, End: end})
		}
	}
	slices.SortFunc(folds, func(a, b Fold) int { return a.Line - b.Line })
	return folds
}

// endsChildren reports whether a non-indented line c at index j ends the
// children of the list item at line b: code, hidden blocks and the closing
// delimiter of a fence opened inside the item never do.
func endsChildren(c LineInfo, b, j int) bool {
	switch c.Kind {
	case KindCode, KindHidden:
		return false
	case KindFence:
		return !(c.Fence > b && c.Fence != j)
	}
	return true
}
