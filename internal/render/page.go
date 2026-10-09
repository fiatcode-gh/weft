package render

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
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
	wikiLinkRe   = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]*))?\]\]`)
	taskMarkerRe = regexp.MustCompile(`^(\s*-\s+)(TODO|DOING|LATER|WAITING|DONE|CANCELED|CANCELLED|NOW)\b`)
)

// EmphasisStyle highlights a searched/arrived-at term on the page. Reverse
// video stands out from Theme.Link and degrades to plain text under NO_COLOR.
var EmphasisStyle = lipgloss.NewStyle().Reverse(true)

type taskInfo struct {
	marker, priority, gap string
	stamp                 string
	stampKind             graph.StampKind
	open                  bool
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

// orphanPadReplacer deletes sentinel pad runes left behind when Glamour v2
// hard-wraps a sentinel wider than the column: sentinelRe consumes the pad
// run on the sentinel's own row, and the rest of the wrapped run would show
// as private-use glyphs. Deleting runes keeps the row count, so SourceRows
// stays aligned with Styled.
var orphanPadReplacer = strings.NewReplacer(wikiSentinelPad, "", taskSentinelPad, "", emphSentinelPad, "")

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

// rendererKey is what a cached renderer was built for: a changed
// WEFT_STYLE/NO_COLOR must never reuse a stale one.
type rendererKey struct {
	name  string
	width int
}

var (
	rendererMu    sync.Mutex
	rendererCache = map[rendererKey]*glamour.TermRenderer{}
)

// glamourRender invokes the width-cached renderer. A var so tests can
// simulate a Glamour failure — no markdown input reliably triggers one.
var glamourRender = func(r *glamour.TermRenderer, in string) (string, error) {
	out, err := r.Render(in)
	return hyperlinkRe.ReplaceAllString(out, ""), err
}

// hyperlinkRe matches an OSC 8 hyperlink open or close sequence. Glamour v2
// wraps reference links and autolinks in them; weft v2.5 rendered none, so
// they are stripped to keep the read view identical (no clickable links).
var hyperlinkRe = regexp.MustCompile(`\x1b\]8;[^\x07\x1b]*(?:\x07|\x1b\\)`)

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
	theme, err := CurrentTheme()
	if err != nil {
		return nil, err
	}
	key := rendererKey{theme.Name, width}
	rendererMu.Lock()
	defer rendererMu.Unlock()
	if r, ok := rendererCache[key]; ok {
		return r, nil
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(theme.Config),
		glamour.WithChromaFormatter(theme.Formatter),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	rendererCache[key] = r
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
			if graph.LogbookEndRe.MatchString(line) {
				inLogbook = false
			}
			continue
		}
		switch {
		case fence.Step(line):
			out.WriteString(line)
			kept = append(kept, i)
		case graph.LogbookStartRe.MatchString(line):
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
		case graph.QueryOrEmbedRe.MatchString(line):
			// A same-line `}}` closes the block immediately — `{{embed [[X]]}}`
			// is single-line in practice. The closer is looked for anywhere
			// after the opener match (not just as the line's trailing
			// suffix) so trailing prose after `}}` (e.g. "{{embed [[X]]}}
			// notes") doesn't get mistaken for an unclosed block, which
			// would swallow the rest of the page waiting for a bare `}}`
			// line that never comes. Such lines are metadata either way, so
			// the whole line is still dropped rather than keeping the
			// trailing prose.
			loc := graph.QueryOrEmbedRe.FindStringIndex(line)
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

// linkHit is one link to substitute on a line: the bytes [start, end) become
// a sentinel that restores to display and navigates to target.
type linkHit struct {
	start, end      int
	target, display string
}

// replaceLinksOutsideInlineCode preprocesses a single line for wiki links and
// tags, but only OUTSIDE backtick-delimited inline code spans. Markdown treats
// backticked text as literal — `[[Foo]]` should display as the literal text
// "[[Foo]]", not as a styled wiki link. Splits the line on backticks so even-
// indexed parts are literal text (processed) and odd-indexed parts are inline
// code (left alone). Per-line because inline code spans are single-line; the
// cross-line fence state is already handled by the caller.
//
// A simple tag (#name) is its own hit; a bracket tag (#[[Name]]) is the wiki
// link at its '#'+1, extended one byte left over the '#' and shown with a
// leading '#'. Hits are emitted in byte order, bytes between them verbatim.
//
// The `subs` slice is appended to as matches are found; the id-encoded sentinel
// is the same shape `preprocessLinks` uses so the rest of the pipeline
// (Glamour → sentinel substitution) is unchanged.
func replaceLinksOutsideInlineCode(line string, base int, subs *[]linkSubst) string {
	tags := graph.FindTags(line)
	if len(tags) == 0 && !strings.Contains(line, "[[") {
		return line
	}
	var hits []linkHit
	off := 0
	for i, part := range strings.Split(line, "`") {
		// Odd-indexed part is inside backticks (inline code). Leave the
		// text literal so `[[Foo]]` stays as `[[Foo]]` in the output.
		if i%2 == 0 {
			for _, m := range wikiLinkRe.FindAllStringSubmatchIndex(part, -1) {
				target := part[m[2]:m[3]]
				if target == "" {
					// Defensive no-op: wikiLinkRe's capture is [^\]\|]+ (always ≥1
					// char) and render does not TrimSpace, so target is never empty
					// here — unlike internal/graph/parse.go, which trims m[1] and
					// genuinely relies on its empty-target guard. Kept only to guard
					// against future regex changes.
					continue
				}
				display := target
				if m[5] > m[4] {
					display = part[m[4]:m[5]]
				}
				hits = append(hits, linkHit{off + m[0], off + m[1], target, display})
			}
		}
		off += len(part) + 1
	}
	simple := false
	for _, t := range tags {
		if !t.Bracket {
			hits = append(hits, linkHit{t.Start, t.End, t.Name, "#" + t.Name})
			simple = true
			continue
		}
		for k := range hits {
			if hits[k].start == t.Start+1 {
				hits[k].start = t.Start
				hits[k].display = "#" + hits[k].display
				break
			}
		}
	}
	if simple {
		slices.SortFunc(hits, func(a, b linkHit) int { return a.start - b.start })
	}
	var out strings.Builder
	last := 0
	for _, h := range hits {
		out.WriteString(line[last:h.start])
		last = h.end
		id := base + len(*subs)
		*subs = append(*subs, linkSubst{target: h.target, display: h.display})
		core := wikiSentinelStart + encodeSentinelID(id) + wikiSentinelEnd
		// Pad sentinel to the rendered link's display width so Glamour's
		// word-wrap reserves enough columns. Otherwise a short sentinel
		// (e.g. <id 0>) at the end of a line lets Glamour fit it within
		// the wrap width, then post-substitution the longer link text
		// overflows the right margin and the terminal crops it.
		if pad := lipgloss.Width(h.display) - lipgloss.Width(core); pad > 0 {
			core += strings.Repeat(wikiSentinelPad, pad)
		}
		out.WriteString(core)
	}
	out.WriteString(line[last:])
	return out.String()
}

// preprocessLinks replaces non-fenced [[X]], [[X|alias]], #tag and #[[X]]
// occurrences in body with sentinels that survive Glamour rendering. Returns the rewritten
// body and a slice of substitutions indexed by the id encoded in each sentinel.
//
// Scanning the *original* body (instead of the post-render styled output)
// avoids Glamour's habit of interleaving ANSI escapes between the two opening
// brackets — which silently breaks any regex that requires a contiguous "[[".
//
// base is the id of the first substitution: a chunk of a larger document
// numbers its sentinels from the count the document has before it, because a
// sentinel's width depends on the digits of its id.
func preprocessLinks(body string, base int) (string, []linkSubst) {
	var subs []linkSubst
	body = mapLinesOutsideFences(body, func(line string) string {
		return replaceLinksOutsideInlineCode(line, base, &subs)
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
	locs := graph.MarkdownLinkRe.FindAllStringSubmatchIndex(line, -1)
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

// styledPriority reports whether a task bullet line, whose marker starts at
// markerStart, carries a "[#A]" cookie that gets its own style: the cookie
// must follow the marker after spaces only.
func styledPriority(line string, markerStart int) (graph.TaskPrefix, bool) {
	p, ok := graph.ParseTaskPrefix(line)
	if !ok || p.Priority == "" || p.MarkerStart != markerStart ||
		strings.Trim(line[p.MarkerEnd:p.PriorityStart], " ") != "" {
		return graph.TaskPrefix{}, false
	}
	return p, true
}

// preprocessTaskMarkers replaces leading TODO/DOING/etc. markers (with an
// adjacent "[#A]" priority) and whole SCHEDULED:/DEADLINE: stamp lines on
// non-fenced lines with sentinels, returning the rewritten body and what each
// sentinel stands for, indexed by sentinel id.
// base is the id of the first marker, as in preprocessLinks.
func preprocessTaskMarkers(body string, base int) (string, []taskInfo) {
	var markers []taskInfo
	body = mapLinesOutsideFences(body, func(line string) string {
		var info taskInfo
		var covered string // the text the sentinel stands for
		var head, rest string
		if m := taskMarkerRe.FindStringSubmatch(line); m != nil {
			prefix := m[1]
			info = taskInfo{marker: m[2], open: graph.IsOpenTask(line)}
			covered = info.marker
			head, rest = prefix, line[len(prefix)+len(info.marker):]
			if p, ok := styledPriority(line, len(prefix)); ok {
				info.priority = p.Priority
				info.gap = line[p.MarkerEnd:p.PriorityStart]
				covered = line[p.MarkerStart:p.PriorityEnd]
				rest = line[p.PriorityEnd:]
			}
		} else if st, ok := graph.ParseStampLine(line); ok {
			info = taskInfo{stamp: line[st.Start:st.End], stampKind: st.Kind}
			covered = info.stamp
			head, rest = line[:st.Start], line[st.End:]
		} else {
			return line
		}
		id := base + len(markers)
		markers = append(markers, info)
		sentinel := taskSentinelStart + encodeSentinelID(id) + taskSentinelEnd
		// Pad to the covered text's display width so Glamour's word-wrap
		// reserves the columns the restored text will occupy (same trick as
		// the wiki-link and emphasis sentinels, see preprocessLinks).
		if pad := lipgloss.Width(covered) - lipgloss.Width(sentinel); pad > 0 {
			sentinel += strings.Repeat(taskSentinelPad, pad)
		}
		return head + sentinel + rest
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

// preprocessEmphasis wraps whole-word, case-insensitive occurrences of term in
// emphasis sentinels (outside fences and inline code), returning the rewritten
// body and the original matched substrings indexed by sentinel id (so casing is
// preserved on restore). Returns (body, nil) when term is empty. Run AFTER
// preprocessLinks/preprocessTaskMarkers so [[term]] is already a sentinel
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
	f := preprocess(body, emphasis, idBase{})

	theme, err := CurrentTheme()
	if err != nil {
		return Result{}, err
	}
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

	res := finish(styled, f, theme, fallbackErr)
	return res, nil
}

// finish turns Glamour's output for f.pre into the Result: hanging indents
// for wrapped bullets, then every sentinel restored to its styled text.
// fallbackErr is recorded on the Result.
func finish(styled string, f frontend, theme Theme, fallbackErr error) Result {
	wikiSubs, taskMarkers, emphSubs := f.wikiSubs, f.taskMarkers, f.emphSubs

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
		orphanPadReplacer.WriteString(&out, styled[last:m[0]])
		last = m[1]
		switch {
		case m[2] >= 0: // wiki-link sentinel
			id, ok := decodeSentinelID(styled[m[2]:m[3]])
			id -= f.base.wiki
			if !ok || id < 0 || id >= len(wikiSubs) {
				out.WriteString(styled[m[0]:m[1]])
				continue
			}
			rendered := theme.Link.Render(wikiSubs[id].display)
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
			id -= f.base.task
			if !ok || id < 0 || id >= len(taskMarkers) {
				out.WriteString(styled[m[0]:m[1]])
				continue
			}
			if taskMarkers[id].open {
				tasks = append(tasks, out.Len())
			}
			ti := taskMarkers[id]
			switch {
			case ti.stamp != "" && ti.stampKind == graph.StampDeadline:
				out.WriteString(theme.Deadline.Render(ti.stamp))
			case ti.stamp != "":
				out.WriteString(theme.Scheduled.Render(ti.stamp))
			default:
				if st, ok := theme.Markers[ti.marker]; ok {
					out.WriteString(st.Render(ti.marker))
				} else {
					out.WriteString(ti.marker)
				}
				if ti.priority != "" {
					out.WriteString(ti.gap)
					out.WriteString(theme.Priority[ti.priority].Render("[#" + ti.priority + "]"))
				}
			}
		case m[6] >= 0: // emphasis sentinel
			id, ok := decodeSentinelID(styled[m[6]:m[7]])
			if !ok || id >= len(emphSubs) {
				out.WriteString(styled[m[0]:m[1]])
				continue
			}
			finds = append(finds, out.Len())
			out.WriteString(EmphasisStyle.Render(emphSubs[id]))
		}
	}
	orphanPadReplacer.WriteString(&out, styled[last:])

	return Result{Styled: out.String(), Links: links, Tasks: tasks, Finds: finds, FallbackErr: fallbackErr}
}
