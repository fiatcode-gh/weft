package views

import (
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// Row-classification patterns for the in-app editor's live tinting. Most of
// these intentionally mirror (a subset of) internal/render/page.go's patterns
// rather than importing them: those symbols are unexported, and theme.go
// documents that mirroring the handful of marker colors across the
// views/render boundary is preferred over a premature shared package. Fence
// delimiters are the exception: lineBaseStyle uses graph.FenceDelimiterRe
// directly so the editor, index, and read view all agree on what is fenced.
var (
	editorHeadingRe = regexp.MustCompile(`^#{1,6}(\s|$)`)
	editorQuoteRe   = regexp.MustCompile(`^\s*>`)
	editorTaskRe    = regexp.MustCompile(`^(\s*-\s+)(TODO|DOING|LATER|WAITING|DONE|CANCELED|CANCELLED|NOW)\b`)
	editorLinkRe    = regexp.MustCompile(`\[\[[^\]\n]*\]\]`)
)

// editorLinkTint is the wiki-link color, matching the read view (linkStyle in
// internal/render/page.go uses color 12) and theme.go's colorHighlight. Unlike
// linkStyle it omits the underline — in the raw-source editor the brackets
// already delimit the link, so color alone is enough.
var editorLinkTint = lipgloss.NewStyle().Foreground(colorHighlight)

// editorMarkerTint mirrors internal/render/page.go's taskMarkerStyles so the
// editor, read view, and todos dashboard color workflow markers identically.
var editorMarkerTint = map[string]lipgloss.Style{
	"TODO":      lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
	"DOING":     lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true),
	"LATER":     lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
	"WAITING":   lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true),
	"DONE":      lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true),
	"CANCELED":  lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true),
	"CANCELLED": lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true),
	"NOW":       lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true),
}

// tintSpan is a byte range within a row that gets its own style, composed over
// the row's whole-line base style.
type tintSpan struct {
	start, end int
	style      lipgloss.Style
}

// tintView decorates the textarea's rendered output. Each line is tinted
// EXCEPT lines that already contain an ANSI escape — in practice the only such
// lines are the cursor's current row (which textarea renders with its CursorLine
// background) and the end-of-buffer filler rows. Leaving them verbatim both
// avoids clobbering the cursor styling and gives the deliberate "raw source on
// the line you're editing" affordance. This relies on EditorView neutralizing
// the textarea's prompt style (see NewEditorView); otherwise the styled empty
// prompt would put an escape on every row.
func tintView(view string) string {
	lines := strings.Split(view, "\n")
	for i, ln := range lines {
		if strings.ContainsRune(ln, 0x1b) {
			continue // pre-styled (cursor row / end-of-buffer filler)
		}
		lines[i] = tintLine(ln)
	}
	return strings.Join(lines, "\n")
}

// tintLine classifies one plain (ANSI-free) row and returns it styled. A normal
// row (no heading/quote/fence/task class and no wiki-link) is returned
// unchanged so it stays ANSI-free.
func tintLine(line string) string {
	if strings.TrimSpace(line) == "" {
		return line
	}

	base, hasBase := lineBaseStyle(line)
	spans := collectSpans(line)

	if !hasBase && len(spans) == 0 {
		return line
	}
	if len(spans) == 0 {
		return base.Render(line)
	}
	return renderSpans(line, base, hasBase, spans)
}

// lineBaseStyle returns the whole-line style for a row's block class and
// whether any class matched. Task-marker lines carry no whole-line style (only
// the marker token is colored, via collectSpans), so they return ok=false here.
func lineBaseStyle(line string) (lipgloss.Style, bool) {
	switch {
	case editorHeadingRe.MatchString(line):
		return lipgloss.NewStyle().Bold(true), true
	case graph.FenceDelimiterRe.MatchString(line):
		return lipgloss.NewStyle().Faint(true), true
	case editorQuoteRe.MatchString(line):
		return lipgloss.NewStyle().Faint(true), true
	}
	return lipgloss.Style{}, false
}

// collectSpans gathers the inline token spans for a row: every wiki-link, plus
// the workflow marker on a task line. Spans are returned sorted by start and
// are non-overlapping for these patterns.
func collectSpans(line string) []tintSpan {
	var spans []tintSpan
	for _, loc := range editorLinkRe.FindAllStringIndex(line, -1) {
		spans = append(spans, tintSpan{loc[0], loc[1], editorLinkTint})
	}
	if m := editorTaskRe.FindStringSubmatchIndex(line); m != nil {
		// Submatch 2 (group index 2) is the marker word: indices m[4]:m[5].
		marker := line[m[4]:m[5]]
		if st, ok := editorMarkerTint[marker]; ok {
			spans = append(spans, tintSpan{m[4], m[5], st})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	return spans
}

// renderSpans rebuilds the row by segments: gap text gets the base style, span
// text gets its own style composed over the base (so a link inside a heading is
// both colored and bold). Building by concatenation (rather than nesting
// styles) avoids premature ANSI resets clobbering surrounding styling.
func renderSpans(line string, base lipgloss.Style, hasBase bool, spans []tintSpan) string {
	var b strings.Builder
	pos := 0
	emitGap := func(s string) {
		if s == "" {
			return
		}
		if hasBase {
			b.WriteString(base.Render(s))
		} else {
			b.WriteString(s)
		}
	}
	for _, sp := range spans {
		if sp.start < pos { // defensive: skip an overlapping span
			continue
		}
		emitGap(line[pos:sp.start])
		style := sp.style
		if hasBase {
			style = style.Inherit(base) // keep span color, gain base bold/faint
		}
		b.WriteString(style.Render(line[sp.start:sp.end]))
		pos = sp.end
	}
	emitGap(line[pos:])
	return b.String()
}
