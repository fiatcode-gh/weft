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
	wikiLinkRe = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]*))?\]\]`)
	fenceRe    = regexp.MustCompile("^\\s*```")
)

var linkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Underline(true)

// Private-use Unicode codepoints bracket each sentinel. They survive Glamour's
// rendering and ANSI-styling pipeline intact because they have no markdown
// semantics and word-wrap treats the contiguous run as a single token.
const (
	sentinelStart = ""
	sentinelEnd   = ""
)

var sentinelRe = regexp.MustCompile(sentinelStart + `(\d+)` + sentinelEnd)

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
				return fmt.Sprintf("%s%d%s", sentinelStart, id, sentinelEnd)
			})
			out.WriteString(rewritten)
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String(), subs
}

// Render returns Glamour-rendered markdown with wiki-link positions annotated.
// width is the target terminal column count.
func Render(body string, width int) (Result, error) {
	pre, subs := preprocessWikiLinks(body)

	r, err := rendererFor(width)
	if err != nil {
		return Result{}, err
	}
	styled, err := r.Render(pre)
	if err != nil {
		// Fallback: plain text if Glamour chokes.
		styled = pre
	}

	// Walk the styled output once, replacing each sentinel with its rendered
	// display string and recording the byte position in the assembled result.
	var out strings.Builder
	out.Grow(len(styled))
	links := make([]Link, 0, len(subs))
	last := 0
	for _, m := range sentinelRe.FindAllStringSubmatchIndex(styled, -1) {
		id, err := strconv.Atoi(styled[m[2]:m[3]])
		if err != nil || id < 0 || id >= len(subs) {
			continue
		}
		out.WriteString(styled[last:m[0]])
		rendered := linkStyle.Render(subs[id].display)
		start := out.Len()
		out.WriteString(rendered)
		links = append(links, Link{
			Target:  subs[id].target,
			Display: subs[id].display,
			Start:   start,
			End:     start + len(rendered),
		})
		last = m[1]
	}
	out.WriteString(styled[last:])

	return Result{Styled: out.String(), Links: links}, nil
}
