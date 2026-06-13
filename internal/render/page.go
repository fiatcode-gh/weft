package render

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	// Tasks holds the byte offset in Styled of each open task marker
	// (TODO/LATER/DOING/WAITING with non-empty text), in document order.
	// The index matches graph.TodoBullet.Ordinal so the view layer can
	// deep-link a dashboard todo to its rendered row.
	Tasks []int
	// Finds holds the byte offset in Styled of each highlighted emphasis-term
	// occurrence, in document order. Empty unless rendered with an emphasis term.
	Finds []int
}

var (
	wikiLinkRe     = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]*))?\]\]`)
	fenceRe        = regexp.MustCompile("^\\s*```")
	taskMarkerRe   = regexp.MustCompile(`^(\s*-\s+)(TODO|DOING|LATER|WAITING|DONE|CANCELED|CANCELLED|NOW)\b`)
	logbookStartRe = regexp.MustCompile(`(?i)^\s*:LOGBOOK:\s*$`)
	logbookEndRe   = regexp.MustCompile(`(?i)^\s*:END:\s*$`)
	queryOrEmbedRe = regexp.MustCompile(`(?i)^\s*\{\{(query|embed)\b`)
)

var linkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Underline(true)

// emphasisStyle highlights a searched/arrived-at term on the page. Reverse
// video stands out from linkStyle and degrades to plain text under NO_COLOR.
var emphasisStyle = lipgloss.NewStyle().Reverse(true)

// openTaskMarkers mirrors graph.ExtractTodos: only these markers (with
// non-empty text) are "open" and get a recorded position for deep-linking.
var openTaskMarkers = map[string]bool{"TODO": true, "LATER": true, "DOING": true, "WAITING": true}

type taskInfo struct {
	marker string
	open   bool
}

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
	wikiSentinelPad   = ""
	taskSentinelStart = ""
	taskSentinelEnd   = ""
	emphSentinelStart = "" // emphasis sentinel — distinct PUA range from wiki (–) and task (–)
	emphSentinelEnd   = ""
	emphSentinelPad   = "" // width-padding for the emphasis sentinel (see wikiSentinelPad)
)

// sentinelRe matches a wiki-link, task-marker, or emphasis sentinel.
// Group 1 (m[2:3]) captures the wiki id; group 2 (m[4:5]) captures the task id;
// group 3 (m[6:7]) captures the emphasis id.
// Handling all three in one ordered pass keeps recorded byte offsets aligned
// with the bytes actually emitted.
var sentinelRe = regexp.MustCompile(
	wikiSentinelStart + `(\d+)` + wikiSentinelEnd + `(?:` + wikiSentinelPad + `)*` +
		`|` + taskSentinelStart + `(\d+)` + taskSentinelEnd +
		`|` + emphSentinelStart + `(\d+)` + emphSentinelEnd + `(?:` + emphSentinelPad + `)*`,
)

var (
	rendererMu    sync.Mutex
	rendererCache = map[int]*glamour.TermRenderer{}
)

// styleName picks the Glamour style without doing any terminal IO. This is
// deliberate: Glamour's WithAutoStyle issues OSC 11 background-colour queries
// over stdin, which can leave stray reply bytes in the terminal's input
// buffer. When peekseq is quit and immediately re-opened, the next session's
// termenv reads those stale bytes, fails to parse them, and blocks for
// seconds before timing out. Reading env vars sidesteps the problem.
func styleName() string {
	if os.Getenv("NO_COLOR") != "" {
		return "notty"
	}
	if s := os.Getenv("PEEKSEQ_STYLE"); s != "" {
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

// stripQueryAndEmbedBlocks drops Logseq {{query …}} and {{embed …}}
// blocks. Each block is delimited by an opening `{{query` / `{{embed`
// at the start of a line and a closing `}}` on its own line. Fence-aware
// so a code block containing the literal markers stays intact.
func stripQueryAndEmbedBlocks(body string) string {
	var out strings.Builder
	out.Grow(len(body))
	lines := strings.Split(body, "\n")
	inFence := false
	inBlock := false
	for i, line := range lines {
		switch {
		case fenceRe.MatchString(line):
			inFence = !inFence
			out.WriteString(line)
		case inFence:
			out.WriteString(line)
		case !inBlock && queryOrEmbedRe.MatchString(line):
			inBlock = true
			continue // drop the opening line
		case inBlock:
			if strings.TrimSpace(line) == "}}" {
				inBlock = false
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

// replaceWikiLinksOutsideInlineCode preprocesses a single line for wiki links,
// but only OUTSIDE backtick-delimited inline code spans. Markdown treats
// backticked text as literal — `[[Foo]]` should display as the literal text
// "[[Foo]]", not as a styled wiki link. Splits the line on backticks so even-
// indexed parts are literal text (processed) and odd-indexed parts are inline
// code (left alone), then rejoins. Per-line because inline code spans are
// single-line; the cross-line fence state is already handled by the caller.
//
// The `subs` slice is appended to as matches are found; the id-encoded sentinel
// is the same shape `preprocessWikiLinks` uses so the rest of the pipeline
// (Glamour → sentinel substitution) is unchanged.
func replaceWikiLinksOutsideInlineCode(line string, subs *[]linkSubst) string {
	parts := strings.Split(line, "`")
	for i, part := range parts {
		if i%2 == 1 {
			// Odd-indexed part is inside backticks (inline code). Leave the
			// text literal so `[[Foo]]` stays as `[[Foo]]` in the output.
			continue
		}
		parts[i] = wikiLinkRe.ReplaceAllStringFunc(part, func(match string) string {
			m := wikiLinkRe.FindStringSubmatch(match)
			target := m[1]
			// Strip optional #block fragment: [[Alpha#summary]] -> "Alpha".
			if j := strings.IndexByte(target, '#'); j >= 0 {
				target = target[:j]
			}
			if target == "" {
				// No page name (e.g. [[#anchor]] or [[#]]) — leave the
				// literal text in the output so the view layer doesn't see
				// a phantom link with an empty target. Mirrors the empty-
				// target guard in internal/graph/parse.go.
				return match
			}
			display := target
			if m[2] != "" {
				display = m[2]
			}
			id := len(*subs)
			*subs = append(*subs, linkSubst{target: target, display: display})
			core := fmt.Sprintf("%s%d%s", wikiSentinelStart, id, wikiSentinelEnd)
			// Pad sentinel to the rendered link's display width so Glamour's
			// word-wrap reserves enough columns. Otherwise a short sentinel
			// (e.g. <id 0>) at the end of a line lets Glamour fit it within
			// the wrap width, then post-substitution the longer link text
			// overflows the right margin and the terminal crops it.
			if pad := lipgloss.Width(display) - lipgloss.Width(core); pad > 0 {
				core += strings.Repeat(wikiSentinelPad, pad)
			}
			return core
		})
	}
	return strings.Join(parts, "`")
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
			out.WriteString(replaceWikiLinksOutsideInlineCode(line, &subs))
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
func preprocessTaskMarkers(body string) (string, []taskInfo) {
	var markers []taskInfo
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
				rest := line[len(prefix)+len(marker):]
				// "open" mirrors graph.ExtractTodos: an open marker followed by
				// whitespace and then non-empty text. The leading-whitespace
				// check matters because taskMarkerRe ends the marker at a \b
				// boundary (so it also matches "- TODO: x"), whereas graph
				// requires "\s+" after the marker — without this guard the two
				// disagree and the deep-link ordinal misaligns.
				open := openTaskMarkers[marker] && len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') && strings.TrimSpace(rest) != ""
				markers = append(markers, taskInfo{marker: marker, open: open})
				sentinel := fmt.Sprintf("%s%d%s", taskSentinelStart, id, taskSentinelEnd)
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

// bulletLineRe matches a Glamour-rendered bullet row: optional leading spaces,
// a bullet glyph, and at least one space before the content. Glamour swaps
// markdown `-`/`*` for `•` and may use `◦` / `▪` for nested levels.
var bulletLineRe = regexp.MustCompile(`^(\s*)([•◦▪▫])\s+`)

// indentWrappedBullets gives bullets hanging-indent behaviour: Glamour wraps
// continuation lines to the bullet column, but the eye expects them aligned
// with the content column (one bullet + one space to the right). This pass
// walks the styled output and adds the missing two spaces to continuation
// lines until the bullet block ends (blank line or a new bullet).
func indentWrappedBullets(styled string) string {
	lines := strings.Split(styled, "\n")
	contentCol := 0
	inBullet := false
	for i, line := range lines {
		plain := ansi.Strip(line)
		if strings.TrimSpace(plain) == "" {
			inBullet = false
			continue
		}
		if m := bulletLineRe.FindStringSubmatch(plain); m != nil {
			contentCol = len(m[1]) + 2 // leading spaces + bullet (1 cell) + space
			inBullet = true
			continue
		}
		if !inBullet {
			continue
		}
		// Continuation: prepend the gap between current leading spaces and
		// the bullet's content column. Padding goes at the raw start of the
		// line so it sits before any ANSI prefix Glamour emitted.
		existing := 0
		for _, r := range plain {
			if r != ' ' {
				break
			}
			existing++
		}
		if existing < contentCol {
			lines[i] = strings.Repeat(" ", contentCol-existing) + line
		}
	}
	return strings.Join(lines, "\n")
}

func renderTaskMarker(marker string) string {
	if st, ok := taskMarkerStyles[marker]; ok {
		return st.Render(marker)
	}
	return marker
}

// preprocessEmphasis wraps whole-word, case-insensitive occurrences of term in
// emphasis sentinels (outside fences and inline code), returning the rewritten
// body and the original matched substrings indexed by sentinel id (so casing is
// preserved on restore). Returns (body, nil) when term is empty. Run AFTER
// preprocessWikiLinks/preprocessTaskMarkers so [[term]] is already a sentinel
// and only bare mentions match.
func preprocessEmphasis(body, term string) (string, []string) {
	if term == "" {
		return body, nil
	}
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(term))
	var subs []string
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
			out.WriteString(emphasizeOutsideInlineCode(line, re, &subs))
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String(), subs
}

// emphasizeOutsideInlineCode wraps whole-word matches of re (a literal,
// case-insensitive term matcher) in emphasis sentinels, but only outside
// backtick-delimited inline code. Word boundaries are Unicode-aware to mirror
// ripgrep -w (which detection uses): a match counts when each side is the
// string edge or a non-word rune. Go's \b is ASCII-only and would miss names
// like "Über" or "C++".
func emphasizeOutsideInlineCode(line string, re *regexp.Regexp, subs *[]string) string {
	parts := strings.Split(line, "`")
	for i, part := range parts {
		if i%2 == 1 {
			continue // inside inline code
		}
		parts[i] = emphasizeWholeWords(part, re, subs)
	}
	return strings.Join(parts, "`")
}

func emphasizeWholeWords(s string, re *regexp.Regexp, subs *[]string) string {
	locs := re.FindAllStringIndex(s, -1)
	if locs == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		if !wholeWordAt(s, start, end) {
			continue
		}
		b.WriteString(s[last:start])
		match := s[start:end]
		id := len(*subs)
		*subs = append(*subs, match)
		core := fmt.Sprintf("%s%d%s", emphSentinelStart, id, emphSentinelEnd)
		if pad := lipgloss.Width(match) - lipgloss.Width(core); pad > 0 {
			core += strings.Repeat(emphSentinelPad, pad)
		}
		b.WriteString(core)
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

// wholeWordAt reports whether s[start:end] is bounded by string edges or
// non-word runes on both sides (Unicode-aware), mirroring ripgrep -w.
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

// Render returns Glamour-rendered markdown with wiki-link positions annotated.
// width is the target terminal column count.
func Render(body string, width int) (Result, error) {
	return RenderWithEmphasis(body, width, "")
}

// RenderWithEmphasis is Render plus highlighting whole-word occurrences of
// emphasis (recorded in Result.Finds). emphasis == "" is identical to Render.
func RenderWithEmphasis(body string, width int, emphasis string) (Result, error) {
	body = stripLogbookBlocks(body)
	body = stripQueryAndEmbedBlocks(body)
	pre, wikiSubs := preprocessWikiLinks(body)
	pre, taskMarkers := preprocessTaskMarkers(pre)
	pre, emphSubs := preprocessEmphasis(pre, emphasis)

	r, err := rendererFor(width)
	if err != nil {
		return Result{}, err
	}
	styled, err := r.Render(pre)
	if err != nil {
		// Fallback: plain text if Glamour chokes.
		styled = pre
	}

	// indentWrappedBullets must run before sentinel substitution so the byte
	// positions recorded for links, tasks, and finds reflect the final output.
	styled = indentWrappedBullets(styled)

	var out strings.Builder
	out.Grow(len(styled))
	links := make([]Link, 0, len(wikiSubs))
	var tasks []int
	var finds []int
	last := 0
	for _, m := range sentinelRe.FindAllStringSubmatchIndex(styled, -1) {
		out.WriteString(styled[last:m[0]])
		last = m[1]
		switch {
		case m[2] >= 0: // wiki-link sentinel
			id, err := strconv.Atoi(styled[m[2]:m[3]])
			if err != nil || id < 0 || id >= len(wikiSubs) {
				out.WriteString(styled[m[0]:m[1]])
				continue
			}
			rendered := linkStyle.Render(wikiSubs[id].display)
			start := out.Len()
			out.WriteString(rendered)
			links = append(links, Link{
				Target:  wikiSubs[id].target,
				Display: wikiSubs[id].display,
				Start:   start,
				End:     start + len(rendered),
			})
		case m[4] >= 0: // task-marker sentinel
			id, err := strconv.Atoi(styled[m[4]:m[5]])
			if err != nil || id < 0 || id >= len(taskMarkers) {
				out.WriteString(styled[m[0]:m[1]])
				continue
			}
			if taskMarkers[id].open {
				tasks = append(tasks, out.Len())
			}
			out.WriteString(renderTaskMarker(taskMarkers[id].marker))
		case m[6] >= 0: // emphasis sentinel
			id, err := strconv.Atoi(styled[m[6]:m[7]])
			if err != nil || id < 0 || id >= len(emphSubs) {
				out.WriteString(styled[m[0]:m[1]])
				continue
			}
			finds = append(finds, out.Len())
			out.WriteString(emphasisStyle.Render(emphSubs[id]))
		}
	}
	out.WriteString(styled[last:])

	return Result{Styled: out.String(), Links: links, Tasks: tasks, Finds: finds}, nil
}
