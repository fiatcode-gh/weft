package render

import (
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

	"github.com/fiatcode-gh/weft/v2/internal/graph"
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
	// FallbackErr is non-nil when Glamour failed and Styled carries the
	// un-styled source (sentinels still substituted). Callers must not
	// cache the result — the failure may be transient.
	FallbackErr error
}

var (
	wikiLinkRe     = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]*))?\]\]`)
	markdownLinkRe = regexp.MustCompile(`(!?)\[([^\]]*)\]\(((?:[^()]|\([^()]*\))*)\)`)
	taskMarkerRe   = regexp.MustCompile(`^(\s*-\s+)(TODO|DOING|LATER|WAITING|DONE|CANCELED|CANCELLED|NOW)\b`)
	logbookStartRe = regexp.MustCompile(`(?i)^\s*:LOGBOOK:\s*$`)
	logbookEndRe   = regexp.MustCompile(`(?i)^\s*:END:\s*$`)
	queryOrEmbedRe = regexp.MustCompile(`(?i)^\s*\{\{(query|embed)\b`)
)

var linkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Underline(true)

// emphasisStyle highlights a searched/arrived-at term on the page. Reverse
// video stands out from linkStyle and degrades to plain text under NO_COLOR.
var emphasisStyle = lipgloss.NewStyle().Reverse(true)

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
	wikiSentinelStart = "\ue000"
	wikiSentinelEnd   = "\ue001"
	wikiSentinelPad   = "\ue004"
	taskSentinelStart = "\ue002"
	taskSentinelEnd   = "\ue003"
	emphSentinelStart = "\ue005" // emphasis sentinel — distinct PUA range from wiki (E000–E001) and task (E002–E003)
	emphSentinelEnd   = "\ue006"
	emphSentinelPad   = "\ue007" // width-padding for the emphasis sentinel (see wikiSentinelPad)
	taskSentinelPad   = "\ue008" // width-padding for the task sentinel (see wikiSentinelPad)
)

// sentinelRe matches a wiki-link, task-marker, or emphasis sentinel.
// Group 1 (m[2:3]) captures the wiki id; group 2 (m[4:5]) captures the task id;
// group 3 (m[6:7]) captures the emphasis id.
// Handling all three in one ordered pass keeps recorded byte offsets aligned
// with the bytes actually emitted.
//
// The id is matched as a run of PUA digit runes (U+E010–U+E019), not ASCII
// `\d+`: an ASCII-digit id would be reachable by a numeric emphasis/search
// term, since the PUA delimiters on either side act as word boundaries. See
// encodeSentinelID/decodeSentinelID.
var sentinelRe = regexp.MustCompile(
	wikiSentinelStart + `([\x{E010}-\x{E019}]+)` + wikiSentinelEnd + `(?:` + wikiSentinelPad + `)*` +
		`|` + taskSentinelStart + `([\x{E010}-\x{E019}]+)` + taskSentinelEnd + `(?:` + taskSentinelPad + `)*` +
		`|` + emphSentinelStart + `([\x{E010}-\x{E019}]+)` + emphSentinelEnd + `(?:` + emphSentinelPad + `)*`,
)

// sentinelDigit0 is the first of ten PUA runes (U+E010..U+E019) that encode
// the decimal digits of sentinel ids. Ids must not be ASCII digits: a
// numeric emphasis term would match inside a sentinel (the PUA delimiters
// are word boundaries) and corrupt it.
const sentinelDigit0 = '\ue010'

// encodeSentinelID renders id as a run of PUA digit runes for embedding
// inside a sentinel. Each rune is exactly one cell wide (like an ASCII
// digit), so the padding/word-wrap width math elsewhere is unaffected.
func encodeSentinelID(id int) string {
	var b strings.Builder
	for _, r := range strconv.Itoa(id) {
		b.WriteRune(sentinelDigit0 + (r - '0'))
	}
	return b.String()
}

// decodeSentinelID is the inverse of encodeSentinelID. It reports false if s
// contains any rune outside the PUA digit range.
func decodeSentinelID(s string) (int, bool) {
	var b strings.Builder
	for _, r := range s {
		if r < sentinelDigit0 || r > sentinelDigit0+9 {
			return 0, false
		}
		b.WriteRune('0' + (r - sentinelDigit0))
	}
	id, err := strconv.Atoi(b.String())
	return id, err == nil
}

var (
	rendererMu    sync.Mutex
	rendererCache = map[int]*glamour.TermRenderer{}
)

// glamourRender invokes the width-cached renderer. A var so tests can
// simulate a Glamour failure — no markdown input reliably triggers one.
var glamourRender = func(r *glamour.TermRenderer, in string) (string, error) {
	return r.Render(in)
}

// styleSelection is which Glamour style weft should render with. selectStyle
// derives it from env vars only — never querying the terminal.
type styleSelection struct {
	terminal bool   // use terminalStyleConfig (weft's default; honors the terminal palette)
	name     string // standard style name, meaningful only when !terminal
}

// selectStyle chooses the render style without any terminal IO. This is
// deliberate: Glamour's WithAutoStyle issues OSC 11 background-colour queries
// over stdin, which can leave stray reply bytes in the terminal's input
// buffer. When weft is quit and immediately re-opened, the next session's
// termenv reads those stale bytes, fails to parse them, and blocks for
// seconds before timing out. Reading env vars sidesteps the problem.
//
// NO_COLOR wins over WEFT_STYLE.
func selectStyle() styleSelection {
	if os.Getenv("NO_COLOR") != "" {
		return styleSelection{name: "notty"}
	}
	if s := os.Getenv("WEFT_STYLE"); s != "" {
		return styleSelection{name: s}
	}
	return styleSelection{terminal: true}
}

// styleOptions turns the selection into Glamour renderer options. The default
// (terminal-palette) path also pins chroma's terminal16 formatter, which
// downsamples the Chroma block's hex anchors to the terminal's 16-color
// palette. A named WEFT_STYLE keeps chroma's default formatter so that theme
// renders at full fidelity.
func styleOptions() []glamour.TermRendererOption {
	sel := selectStyle()
	if sel.terminal {
		return []glamour.TermRendererOption{
			glamour.WithStyles(terminalStyleConfig),
			glamour.WithChromaFormatter("terminal16"),
		}
	}
	return []glamour.TermRendererOption{glamour.WithStandardStyle(sel.name)}
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
	opts := append(styleOptions(), glamour.WithWordWrap(width))
	r, err := glamour.NewTermRenderer(opts...)
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
// markers stays intact. The []int holds, for each line of the result, its
// index among strings.Split(body, "\n").
func stripLogbookBlocks(body string) (string, []int) {
	var out strings.Builder
	var kept []int
	out.Grow(len(body))
	lines := strings.Split(body, "\n")
	var fence graph.FenceState
	inLogbook := false
	for i, line := range lines {
		if inLogbook {
			// Everything inside the block is dropped without touching
			// fence state — a ``` line here is metadata garbage.
			if logbookEndRe.MatchString(line) {
				inLogbook = false
			}
			continue
		}
		switch {
		case fence.Step(line):
			out.WriteString(line)
			kept = append(kept, i)
		case logbookStartRe.MatchString(line):
			inLogbook = true
			continue // drop the :LOGBOOK: line; no newline either
		default:
			out.WriteString(line)
			kept = append(kept, i)
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String(), padKept(kept, out.String(), len(lines))
}

// stripQueryAndEmbedBlocks drops Logseq {{query …}} and {{embed …}}
// blocks. A block closes with `}}` on its own line, or on the opening line
// for the self-closing form. Fence-aware so fenced literals remain intact.
// The []int holds, for each line of the result, its index among
// strings.Split(body, "\n").
func stripQueryAndEmbedBlocks(body string) (string, []int) {
	var out strings.Builder
	var kept []int
	out.Grow(len(body))
	lines := strings.Split(body, "\n")
	var fence graph.FenceState
	inBlock := false
	for i, line := range lines {
		if inBlock {
			// Everything inside the block is dropped without touching
			// fence state — a ``` line here is metadata garbage.
			if strings.TrimSpace(line) == "}}" {
				inBlock = false
			}
			continue
		}
		switch {
		case fence.Step(line):
			out.WriteString(line)
			kept = append(kept, i)
		case queryOrEmbedRe.MatchString(line):
			// A same-line `}}` closes the block immediately — `{{embed [[X]]}}`
			// is single-line in practice. The closer is looked for anywhere
			// after the opener match (not just as the line's trailing
			// suffix) so trailing prose after `}}` (e.g. "{{embed [[X]]}}
			// notes") doesn't get mistaken for an unclosed block, which
			// would swallow the rest of the page waiting for a bare `}}`
			// line that never comes. Such lines are metadata either way, so
			// the whole line is still dropped rather than keeping the
			// trailing prose.
			loc := queryOrEmbedRe.FindStringIndex(line)
			if !strings.Contains(line[loc[1]:], "}}") {
				inBlock = true
			}
			continue // drop the opening line either way
		default:
			out.WriteString(line)
			kept = append(kept, i)
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String(), padKept(kept, out.String(), len(lines))
}

// padKept covers the trailing-empty artefact of the strip passes: when the
// last input lines were all dropped, the output ends in "\n" and so has one
// more (empty) row than lines were kept; that row stands for the last input
// line.
func padKept(kept []int, out string, nLines int) []int {
	for len(kept) < strings.Count(out, "\n")+1 {
		kept = append(kept, nLines-1)
	}
	return kept
}

// mapLinesOutsideFences rewrites body line by line: lines inside (or
// delimiting) a fenced code block pass through verbatim; every other line
// goes through f. The shared walker keeps the fence-detection logic in one
// place for all the preprocessing passes that transform lines in place
// (as opposed to stripLogbookBlocks/stripQueryAndEmbedBlocks, which drop
// lines and so don't fit this shape).
func mapLinesOutsideFences(body string, f func(line string) string) string {
	var out strings.Builder
	out.Grow(len(body))
	lines := strings.Split(body, "\n")
	var fence graph.FenceState
	for i, line := range lines {
		if fence.Step(line) {
			out.WriteString(line)
		} else {
			out.WriteString(f(line))
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
			if target == "" {
				// Defensive no-op: wikiLinkRe's capture is [^\]\|]+ (always ≥1
				// char) and render does not TrimSpace, so target is never empty
				// here — unlike internal/graph/parse.go, which trims m[1] and
				// genuinely relies on its empty-target guard. Kept only to guard
				// against future regex changes.
				return match
			}
			display := target
			if m[2] != "" {
				display = m[2]
			}
			id := len(*subs)
			*subs = append(*subs, linkSubst{target: target, display: display})
			core := wikiSentinelStart + encodeSentinelID(id) + wikiSentinelEnd
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
	body = mapLinesOutsideFences(body, func(line string) string {
		return replaceWikiLinksOutsideInlineCode(line, &subs)
	})
	return body, subs
}

// hideMarkdownLinkURLs rewrites the URL of every inline markdown link
// [text](url) to the bare anchor "#". Glamour's LinkElement skips rendering a
// URL that is only an anchor (ansi/link.go), so this drops the noisy inline
// href Glamour otherwise appends after every link — turning link-dense pages
// into unreadable walls — while keeping the link text and its LinkText styling
// untouched. Images (![alt](url)) are left alone so an image reference isn't
// silently emptied, and links inside fenced or inline code stay literal
// (they're syntax examples). The real URL remains in the source file; only the
// read view hides it. Runs on the raw body before wiki-link/task preprocessing;
// the pattern never matches [[wiki links]] or the sentinels those produce.
func hideMarkdownLinkURLs(body string) string {
	return mapLinesOutsideFences(body, hideMarkdownLinkURLsOutsideInlineCode)
}

// hideMarkdownLinkURLsOutsideInlineCode applies the [text](url) -> [text](#)
// rewrite to a single line. A match FULLY inside a backtick code span is a
// literal syntax example and stays untouched; a link whose text merely
// contains a code span is still a real link and gets rewritten. Uses
// graph.InlineCodeSpans so render and parse agree on what counts as code.
func hideMarkdownLinkURLsOutsideInlineCode(line string) string {
	locs := markdownLinkRe.FindAllStringSubmatchIndex(line, -1)
	if locs == nil {
		return line
	}
	code := graph.InlineCodeSpans(line)
	inCode := func(start, end int) bool {
		for _, s := range code {
			if start >= s.Start && end <= s.End {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	last := 0
	for _, m := range locs {
		start, end := m[0], m[1]
		if inCode(start, end) || line[m[2]:m[3]] == "!" {
			continue // literal example, or an image — leave verbatim
		}
		b.WriteString(line[last:start])
		b.WriteString("[" + line[m[4]:m[5]] + "](#)")
		last = end
	}
	b.WriteString(line[last:])
	return b.String()
}

// preprocessTaskMarkers replaces leading TODO/DOING/etc. markers on non-fenced
// bullet lines with sentinels, returning the rewritten body and the captured
// marker text indexed by sentinel id.
func preprocessTaskMarkers(body string) (string, []taskInfo) {
	var markers []taskInfo
	body = mapLinesOutsideFences(body, func(line string) string {
		m := taskMarkerRe.FindStringSubmatch(line)
		if m == nil {
			return line
		}
		prefix := m[1]
		marker := m[2]
		id := len(markers)
		rest := line[len(prefix)+len(marker):]
		// Graph owns open-task classification so dashboard ordinals and
		// rendered task offsets stay aligned.
		open := graph.IsOpenTask(line)
		markers = append(markers, taskInfo{marker: marker, open: open})
		sentinel := taskSentinelStart + encodeSentinelID(id) + taskSentinelEnd
		// Pad to the marker's display width so Glamour's word-wrap reserves
		// the columns the restored marker text will occupy (same trick as
		// the wiki-link and emphasis sentinels, see preprocessWikiLinks).
		if pad := lipgloss.Width(marker) - lipgloss.Width(sentinel); pad > 0 {
			sentinel += strings.Repeat(taskSentinelPad, pad)
		}
		return prefix + sentinel + rest
	})
	return body, markers
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
	body = mapLinesOutsideFences(body, func(line string) string {
		return emphasizeOutsideInlineCode(line, re, &subs)
	})
	return body, subs
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
		core := emphSentinelStart + encodeSentinelID(id) + emphSentinelEnd
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
	f := preprocess(body, emphasis)
	wikiSubs, taskMarkers, emphSubs := f.wikiSubs, f.taskMarkers, f.emphSubs

	r, err := rendererFor(width)
	if err != nil {
		return Result{}, err
	}
	styled, fallbackErr := glamourRender(r, f.pre)
	if fallbackErr != nil {
		// Fallback: plain text if Glamour chokes. Recorded on the Result so
		// the caller can skip its cache and log the cause.
		styled = f.pre
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
			id, ok := decodeSentinelID(styled[m[2]:m[3]])
			if !ok || id >= len(wikiSubs) {
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
			id, ok := decodeSentinelID(styled[m[4]:m[5]])
			if !ok || id >= len(taskMarkers) {
				out.WriteString(styled[m[0]:m[1]])
				continue
			}
			if taskMarkers[id].open {
				tasks = append(tasks, out.Len())
			}
			out.WriteString(renderTaskMarker(taskMarkers[id].marker))
		case m[6] >= 0: // emphasis sentinel
			id, ok := decodeSentinelID(styled[m[6]:m[7]])
			if !ok || id >= len(emphSubs) {
				out.WriteString(styled[m[0]:m[1]])
				continue
			}
			finds = append(finds, out.Len())
			out.WriteString(emphasisStyle.Render(emphSubs[id]))
		}
	}
	out.WriteString(styled[last:])

	return Result{Styled: out.String(), Links: links, Tasks: tasks, Finds: finds, FallbackErr: fallbackErr}, nil
}
