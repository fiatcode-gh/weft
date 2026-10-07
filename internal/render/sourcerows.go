package render

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// SourceRows is computed from a second Glamour render of a tagged
// copy of the preprocessed body, never from the real render, and only on
// demand (the editor opens at the reading position), so ordinary renders pay
// nothing. Tagging the real
// input would change Glamour's layout (a prefix or suffix marker changes
// wrapping and paragraph joining), and byte-identical Styled/Links/Tasks/
// Finds is a hard requirement. The tagged copy is free to be disturbed: it
// is only read for the line number each of its rows carries, then aligned
// with the real rows by visible text.
//
// A row tag is a zero-width escape sequence, ESC + decimal digits spelled
// with CSI-intermediate bytes + 'z', inserted after the last letter/number/
// sentinel of a source line. It is chosen so every consumer between goldmark
// and the terminal treats it as zero width and none treats it as markdown:
// see rowTagDigits. Where a tag could change the layout anyway (inside link
// destinations, angle constructs, fence info strings, bare list markers,
// link reference definitions) it is not placed, and when the two renders
// still diverge, alignRows realigns the matching prefix and suffix and maps
// the unmatched middle to the line where the divergence starts.
//
// A line with no letter, number or sentinel (a rule, a table's delimiter
// row, a "- ..." bullet) gets no tag, so its row is tagless too. When a gap
// between two tagged rows holds exactly as many letterless rows as there are
// untagged source lines between the two tags, the rows are paired with those
// lines in order and are the lines' own. Any other letterless row (a table's
// outer border, a margin) belongs to no source line: it takes the line below
// it, as a tagless row always did, but is reported as not its own.

// rowTagDigits spells decimal digit d as rowTagDigits[d]: CSI-intermediate
// bytes, so ESC + digits + 'z' parses as one zero-width escape for reflow
// (ESC…letter) and x/ansi (ESC intermediates final), and none is a goldmark
// inline trigger. 'z' is a letter so reflow's indent/padding writers (which
// end sequences only on A-Z/a-z) terminate it; heading Upper may make it 'Z'.
const rowTagDigits = "\"#$%'+,-./"

var (
	rowTagRe            = regexp.MustCompile("\x1b([\"#$%'+,\\-./]{1,9})[zZ]")
	fenceOpenerRe       = regexp.MustCompile("^\\s*(?:[-*+]\\s+)?(?:`{3,}|~{3,})")
	bareOrderedMarkerRe = regexp.MustCompile(`^[\s>*+-]*\d{1,9}[.)]\s*$`)
	linkRefDefRe        = regexp.MustCompile(`^\s{0,3}\[[^\]]+\]:`)
)

func encodeRowTag(line int) string {
	digits := []byte(strconv.Itoa(line))
	for i, d := range digits {
		digits[i] = rowTagDigits[d-'0']
	}
	return "\x1b" + string(digits) + "z"
}

// decodeRowTag is the inverse of encodeRowTag over rowTagRe group 1.
func decodeRowTag(digits string) int {
	n := 0
	for i := range len(digits) {
		n = n*10 + strings.IndexByte(rowTagDigits, digits[i])
	}
	return n
}

// tagSourceLines returns pre with one row tag per eligible line, tagging
// line j with src[j], and loose: the src values (ascending) of the non-blank
// lines left untagged because they hold nothing to tag after, excluding fence
// delimiters, bare ordered markers and link reference definitions, whose
// rendering is not a row of their own.
func tagSourceLines(pre string, src []int) (tagged string, loose []int) {
	lines := strings.Split(pre, "\n")
	var fence graph.FenceState
	for j, line := range lines {
		wasOpen := fence.Open()
		in := fence.Step(line)
		nowOpen := fence.Open()
		switch {
		case in && !wasOpen && nowOpen:
			// Opening delimiter: plaintext info string keeps each code line
			// one chroma token, so tags inside code stay contiguous.
			lines[j] = fenceOpenerRe.FindString(line) + "text"
		case in && wasOpen && !nowOpen:
			// Closing delimiter: unchanged.
		case strings.TrimSpace(line) == "":
		case in:
			lines[j] = line + encodeRowTag(src[j])
		case bareOrderedMarkerRe.MatchString(line), linkRefDefRe.MatchString(line):
		default:
			if cut := rowTagCut(line); cut >= 0 {
				lines[j] = line[:cut] + encodeRowTag(src[j]) + line[cut:]
			} else {
				loose = append(loose, src[j])
			}
		}
	}
	return strings.Join(lines, "\n"), loose
}

// rowTagCut is the byte offset just after the last rune that is a letter,
// number or private-use rune (unicode.Co: weft's sentinels) and lies outside
// an inline link/image destination and outside an angle construct; -1 if none.
func rowTagCut(line string) int {
	cut := -1
	dest := 0
	angle := false
	var prev rune
	for i, r := range line {
		size := utf8.RuneLen(r)
		switch {
		case r == '(' && (prev == ']' || dest > 0):
			dest++
		case r == ')' && dest > 0:
			dest--
		case r == '<' && !angle:
			if next, _ := utf8.DecodeRuneInString(line[i+size:]); next < utf8.RuneSelf &&
				(unicode.IsLetter(next) || next == '/' || next == '!' || next == '?') {
				angle = true
			}
		case r == '>' && angle:
			angle = false
		case dest == 0 && !angle && isTextRune(r):
			cut = i + size
		}
		prev = r
	}
	return cut
}

// frontend is the shared front of the render pipeline: the preprocessed body
// that Glamour sees plus the substitution tables RenderWithEmphasis restores
// afterwards. src[j] is the line of the original body behind line j of pre.
type frontend struct {
	pre         string
	src         []int
	wikiSubs    []linkSubst
	taskMarkers []taskInfo
	emphSubs    []string
}

// preprocess runs the strip passes and the line-preserving substitutions.
// RenderWithEmphasis and SourceRows both call it so their inputs to Glamour
// cannot drift.
func preprocess(body, emphasis string) frontend {
	body, keptLog := stripLogbookBlocks(body)
	body, keptQuery := stripQueryAndEmbedBlocks(body)
	src := make([]int, len(keptQuery))
	for i, k := range keptQuery {
		src[i] = keptLog[k]
	}
	body = hideMarkdownLinkURLs(body)
	pre, wikiSubs := preprocessWikiLinks(body)
	pre, taskMarkers := preprocessTaskMarkers(pre)
	pre, emphSubs := preprocessEmphasis(pre, emphasis)
	return frontend{pre: pre, src: src, wikiSubs: wikiSubs, taskMarkers: taskMarkers, emphSubs: emphSubs}
}

// SourceRows maps each row of the Styled output that RenderWithEmphasis
// produces for the same arguments to the 0-based line of body behind it, and
// reports for each row whether it is one of that line's own rows (own[i]) or
// only borrows the line below it (a blank margin or a table's outer border).
// Both are nil when the map cannot be produced; callers treat nil as "no map".
func SourceRows(body string, width int, emphasis string) (lines []int, own []bool) {
	f := preprocess(body, emphasis)
	r, err := rendererFor(width)
	if err != nil {
		return nil, nil
	}
	styled, err := glamourRender(r, f.pre)
	if err != nil {
		// Fallback: Styled is pre itself, one row per pre line.
		if strings.Count(f.pre, "\n")+1 != len(f.src) {
			return nil, nil
		}
		own = make([]bool, len(f.src))
		for i := range own {
			own[i] = true
		}
		return f.src, own
	}
	return sourceRows(r, styled, f.pre, f.src)
}

// sourceRows renders the tagged copy of pre with r and maps every row of
// styled (raw Glamour output for pre) to a body line. Nil when
// strings.Count(pre, "\n")+1 != len(src) (defensive: a future preprocessing
// pass that changed line count must degrade, not panic) or when the tagged
// render fails.
func sourceRows(r *glamour.TermRenderer, styled, pre string, src []int) (lines []int, own []bool) {
	if strings.Count(pre, "\n")+1 != len(src) {
		return nil, nil
	}
	tagSrc, loose := tagSourceLines(pre, src)
	tagged, err := glamourRender(r, tagSrc)
	if err != nil {
		return nil, nil
	}
	taggedRows := strings.Split(tagged, "\n")
	taggedLines, taggedOwn := taggedRowLines(taggedRows, src[len(src)-1], loose)
	return alignRows(strings.Split(styled, "\n"), taggedRows, taggedLines, taggedOwn)
}

// isTextRune is what a row tag can follow: a letter, a number or a sentinel.
func isTextRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.Is(unicode.Co, r)
}

// taggedRowLines maps tagged rows to lines. A row takes the line of its first
// tag; a tagless row takes the line of the nearest tagged row below it; rows
// below the last tag take the last tag's line, and with no tags every row is
// 0. Tags decoding above maxLine are ignored. (A pairing, below, counts as a
// tag.)
//
// own[i] is true for a tagged row and for a tagless row with text in it (an
// early row of the wrapped line below). A letterless non-blank row is paired
// with a line of loose (ascending, see tagSourceLines) as the package comment
// says, and then takes that line and is its own; otherwise it, like a blank
// row, is not. len(lines) == len(own) == len(rows).
func taggedRowLines(rows []string, maxLine int, loose []int) (lines []int, own []bool) {
	lines = make([]int, len(rows))
	own = make([]bool, len(rows))
	anchored := make([]bool, len(rows)) // the row's line is known, not borrowed
	for i, row := range rows {
		for _, m := range rowTagRe.FindAllStringSubmatch(row, -1) {
			if n := decodeRowTag(m[1]); n <= maxLine {
				lines[i], anchored[i] = n, true
				break
			}
		}
		own[i] = anchored[i] || strings.IndexFunc(ansi.Strip(row), isTextRune) >= 0
	}

	// Letterless rows, collected per gap between tagged rows.
	var letterless []int
	above := -1
	pair := func(below int) {
		if lo, hi := sort.SearchInts(loose, above+1), sort.SearchInts(loose, below); above < below && hi-lo == len(letterless) {
			for k, row := range letterless {
				lines[row], own[row], anchored[row] = loose[lo+k], true, true
			}
		}
		letterless = letterless[:0]
	}
	for i, row := range rows {
		switch {
		case anchored[i]:
			pair(lines[i])
			above = lines[i]
		case !own[i] && strings.TrimSpace(ansi.Strip(row)) != "":
			letterless = append(letterless, i)
		}
	}
	pair(math.MaxInt)

	// Every other row takes the line of the nearest anchored row below it, and
	// rows below the last anchored row take its line.
	cur := 0
	for i := len(rows) - 1; i >= 0; i-- {
		if anchored[i] {
			cur = lines[i]
			break
		}
	}
	for i := len(rows) - 1; i >= 0; i-- {
		if anchored[i] {
			cur = lines[i]
		} else {
			lines[i] = cur
		}
	}
	return lines, own
}

// alignRows maps real rows through tagged rows: common prefix p and suffix s
// by ansi.Strip equality (p+s <= min(len(real), len(tagged))); unmatched
// middle rows take lines[min(p, len(lines)-1)] and count as their own.
// len(result) == len(real).
func alignRows(real, tagged []string, lines []int, own []bool) (outLines []int, outOwn []bool) {
	realPlain := make([]string, len(real))
	for i, row := range real {
		realPlain[i] = ansi.Strip(row)
	}
	taggedPlain := make([]string, len(tagged))
	for i, row := range tagged {
		taggedPlain[i] = ansi.Strip(row)
	}
	limit := min(len(real), len(tagged))
	p := 0
	for p < limit && realPlain[p] == taggedPlain[p] {
		p++
	}
	s := 0
	for p+s < limit && realPlain[len(real)-1-s] == taggedPlain[len(tagged)-1-s] {
		s++
	}
	outLines = make([]int, len(real))
	outOwn = make([]bool, len(real))
	middle := lines[min(p, len(lines)-1)]
	for i := range outLines {
		switch {
		case i < p:
			outLines[i], outOwn[i] = lines[i], own[i]
		case i >= len(real)-s:
			j := len(tagged) - (len(real) - i)
			outLines[i], outOwn[i] = lines[j], own[j]
		default:
			outLines[i], outOwn[i] = middle, true
		}
	}
	return outLines, outOwn
}
