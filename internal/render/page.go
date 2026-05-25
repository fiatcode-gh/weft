package render

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// Link is one wiki-link occurrence inside the styled output.
//
// Display is the user-visible link text (target or alias). It's exposed so
// the view layer can re-render the link with combined cursor + link styling
// in one pass — slicing Styled[Start:End] yields bytes that already contain
// SGR codes, and wrapping those produces nested resets that clobber the
// cursor highlight and any surrounding Glamour styling.
type Link struct {
	Target  string
	Display string
	Start   int // byte offset in Styled (start of the styled link span)
	End     int // byte offset in Styled (exclusive, end of the styled link span)
}

// Result is the rendered page.
type Result struct {
	Styled string
	Links  []Link
}

var (
	wikiLinkRe     = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]*))?\]\]`)
	fenceRe        = regexp.MustCompile("^\\s*```")
	taskMarkerRe   = regexp.MustCompile(`^(\s*-\s+)(TODO|DOING|LATER|WAITING|DONE|CANCELED|CANCELLED|NOW)\b`)
	logbookStartRe = regexp.MustCompile(`(?i)^\s*:LOGBOOK:\s*$`)
	logbookEndRe   = regexp.MustCompile(`(?i)^\s*:END:\s*$`)
)

var linkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Underline(true)

// taskMarkerStyles colour-codes the workflow markers that appear at the
// start of Logseq bullets. Same palette as the todos dashboard so the page
// view and the dashboard read consistently.
var taskMarkerStyles = map[string]lipgloss.Style{
	"TODO":      lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),  // red
	"DOING":     lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true), // yellow
	"LATER":     lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true), // blue
	"WAITING":   lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true),  // dim
	"DONE":      lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true), // green
	"CANCELED":  lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true),
	"CANCELLED": lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true),
	"NOW":       lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true), // magenta
}

// Private-use Unicode codepoints bracket each sentinel. They survive Glamour's
// rendering and ANSI-styling pipeline intact because they have no markdown
// semantics and word-wrap treats the contiguous run as a single token. Wiki
// links and task markers use distinct PUA ranges so the two substitution
// passes can't collide.
const (
	wikiSentinelStart = ""
	wikiSentinelEnd   = ""
	taskSentinelStart = ""
	taskSentinelEnd   = ""
)

var (
	wikiSentinelRe = regexp.MustCompile(wikiSentinelStart + `(\d+)` + wikiSentinelEnd)
	taskSentinelRe = regexp.MustCompile(taskSentinelStart + `(\d+)` + taskSentinelEnd)
)

var (
	rendererMu    sync.Mutex
	rendererCache = map[int]*glamour.TermRenderer{}
)

// styleName picks the Glamour style without doing any terminal IO. This is
// deliberate: Glamour's WithAutoStyle issues OSC 11 background-colour queries
// over stdin, which can leave stray reply bytes in the terminal's input
// buffer. When lstui is quit and immediately re-opened, the next session's
// termenv reads those stale bytes, fails to parse them, and blocks for
// seconds before timing out. Reading env vars sidesteps the problem.
func styleName() string {
	if os.Getenv("NO_COLOR") != "" {
		return "notty"
	}
	if s := os.Getenv("LSTUI_STYLE"); s != "" {
		return s
	}
	return "dark"
}

// Warmup pre-builds the renderer cache so the first page render inside the
// TUI doesn't pay chroma's syntax-highlighter init cost (~100ms).
func Warmup() {
	_, _ = rendererFor(80)
}

// rendererFor returns a TermRenderer for the given word-wrap width, building
// and caching one on first use. Glamour's chroma-based syntax highlighter is
// expensive to initialise; reusing a renderer per width drops per-page cost
// from hundreds of milliseconds to a few.
func rendererFor(width int) (*glamour.TermRenderer, error) {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	if r, ok := rendererCache[width]; ok {
		return r, nil
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(styleName()),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	rendererCache[width] = r
	return r, nil
}

type linkSubst struct {
	target  string
	display string
}

// stripLogbookBlocks removes :LOGBOOK: / :END: blocks from body. These are
// Logseq's per-bullet time-tracking metadata and they're pure noise in a
// read-only browser. Fence-aware so a code block containing the literal
// markers stays intact.
func stripLogbookBlocks(body string) string {
	var out strings.Builder
	out.Grow(len(body))
	lines := strings.Split(body, "\n")
	inFence := false
	inLogbook := false
	for i, line := range lines {
		switch {
		case fenceRe.MatchString(line):
			inFence = !inFence
			out.WriteString(line)
		case inFence:
			out.WriteString(line)
		case !inLogbook && logbookStartRe.MatchString(line):
			inLogbook = true
			continue // drop the :LOGBOOK: line; no newline either
		case inLogbook:
			if logbookEndRe.MatchString(line) {
				inLogbook = false
			}
			continue
		default:
			out.WriteString(line)
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// preprocessWikiLinks replaces non-fenced [[X]] and [[X|alias]] occurrences
// in body with sentinels that survive Glamour rendering. Returns the rewritten
// body and a slice of substitutions indexed by the id encoded in each sentinel.
//
// Scanning the *original* body (instead of the post-render styled output)
// avoids Glamour's habit of interleaving ANSI escapes between the two opening
// brackets — which silently breaks any regex that requires a contiguous "[[".
func preprocessWikiLinks(body string) (string, []linkSubst) {
	var subs []linkSubst
	var out strings.Builder
	out.Grow(len(body))
	inFence := false
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		switch {
		case fenceRe.MatchString(line):
			inFence = !inFence
			out.WriteString(line)
		case inFence:
			out.WriteString(line)
		default:
			rewritten := wikiLinkRe.ReplaceAllStringFunc(line, func(match string) string {
				m := wikiLinkRe.FindStringSubmatch(match)
				target := m[1]
				display := target
				if m[2] != "" {
					display = m[2]
				}
				id := len(subs)
				subs = append(subs, linkSubst{target: target, display: display})
				return fmt.Sprintf("%s%d%s", wikiSentinelStart, id, wikiSentinelEnd)
			})
			out.WriteString(rewritten)
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String(), subs
}

// preprocessTaskMarkers replaces leading TODO/DOING/etc. markers on non-fenced
// bullet lines with sentinels, returning the rewritten body and the captured
// marker text indexed by sentinel id.
func preprocessTaskMarkers(body string) (string, []string) {
	var markers []string
	var out strings.Builder
	out.Grow(len(body))
	inFence := false
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		switch {
		case fenceRe.MatchString(line):
			inFence = !inFence
			out.WriteString(line)
		case inFence:
			out.WriteString(line)
		default:
			if m := taskMarkerRe.FindStringSubmatch(line); m != nil {
				prefix := m[1]
				marker := m[2]
				id := len(markers)
				markers = append(markers, marker)
				sentinel := fmt.Sprintf("%s%d%s", taskSentinelStart, id, taskSentinelEnd)
				rest := line[len(prefix)+len(marker):]
				out.WriteString(prefix)
				out.WriteString(sentinel)
				out.WriteString(rest)
			} else {
				out.WriteString(line)
			}
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String(), markers
}

// substituteTaskSentinels replaces each task-marker sentinel in styled with
// its rendered (colour-coded) marker text. Position-stable: it doesn't track
// offsets the way the wiki-link pass does, so it runs first.
func substituteTaskSentinels(styled string, markers []string) string {
	return taskSentinelRe.ReplaceAllStringFunc(styled, func(match string) string {
		m := taskSentinelRe.FindStringSubmatch(match)
		id, err := strconv.Atoi(m[1])
		if err != nil || id < 0 || id >= len(markers) {
			return match
		}
		marker := markers[id]
		if st, ok := taskMarkerStyles[marker]; ok {
			return st.Render(marker)
		}
		return marker
	})
}

// Render returns Glamour-rendered markdown with wiki-link positions annotated.
// width is the target terminal column count.
func Render(body string, width int) (Result, error) {
	body = stripLogbookBlocks(body)
	pre, wikiSubs := preprocessWikiLinks(body)
	pre, taskMarkers := preprocessTaskMarkers(pre)

	r, err := rendererFor(width)
	if err != nil {
		return Result{}, err
	}
	styled, err := r.Render(pre)
	if err != nil {
		// Fallback: plain text if Glamour chokes.
		styled = pre
	}

	// Substitute task markers first so the byte positions we record for wiki
	// links in the next pass reflect the final output bytes.
	styled = substituteTaskSentinels(styled, taskMarkers)

	// Walk the styled output once, replacing each wiki-link sentinel with its
	// rendered display string and recording the byte position in the result.
	var out strings.Builder
	out.Grow(len(styled))
	links := make([]Link, 0, len(wikiSubs))
	last := 0
	for _, m := range wikiSentinelRe.FindAllStringSubmatchIndex(styled, -1) {
		id, err := strconv.Atoi(styled[m[2]:m[3]])
		if err != nil || id < 0 || id >= len(wikiSubs) {
			continue
		}
		out.WriteString(styled[last:m[0]])
		rendered := linkStyle.Render(wikiSubs[id].display)
		start := out.Len()
		out.WriteString(rendered)
		links = append(links, Link{
			Target:  wikiSubs[id].target,
			Display: wikiSubs[id].display,
			Start:   start,
			End:     start + len(rendered),
		})
		last = m[1]
	}
	out.WriteString(styled[last:])

	return Result{Styled: out.String(), Links: links}, nil
}
