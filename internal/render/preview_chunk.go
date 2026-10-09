package render

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// Chunk starts. A live-preview chunk is a run of source lines rendered on its
// own; it may only start where nothing above can change how the line and the
// lines after it render. chunkState emulates, line by line, the read view's
// three preprocessing passes and the parts of CommonMark block structure that
// survive a blank line. Merging chunks is always safe, so every rule errs
// towards "not a start": only a start the full document does not reset at is
// a defect. See docs/decisions/live-preview.md.

var (
	gmFenceOpenRe = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	taskItemRe    = regexp.MustCompile(`^[-*+][ \t]+\[[ xX]\]([ \t]|$)`)
	html1Re       = regexp.MustCompile(`(?i)^ {0,3}<(script|pre|style|textarea)([ \t>]|$)`)
	html1EndRe    = regexp.MustCompile(`(?i)</(script|pre|style|textarea)>`)
	html4Re       = regexp.MustCompile(`^ {0,3}<![A-Za-z]`)
)

// chunkState carries the forward pass between lines. It holds only values, so
// a copy is a snapshot.
type chunkState struct {
	seen, inLog, inQ         bool
	logFence, qFence, mFence graph.FenceState
	gmFence                  byte // marker of the open CommonMark fence; 0 = none
	gmLen                    int
	html                     uint8 // open HTML block type 1-5; 0 = none
	top                      byte  // last top-level block: bullet marker, '1' ordered, '?' empty item, 0 other
	prevBlank, lastHTML      bool
	lastDef, sawCR           bool
	lastStyled               bool // the last non-blank line closes a heading or is a thematic break
}

// blankLine is goldmark's notion of blank: only spaces and tabs.
func blankLine(s string) bool { return strings.Trim(s, " \t") == "" }

// startsAtColumn0 is true for a line that begins with something other than a space or a tab.
func startsAtColumn0(s string) bool { return s != "" && s[0] != ' ' && s[0] != '\t' }

// bulletMarker is the marker of a bullet item line, or 0.
func bulletMarker(s string) byte {
	if s != "" && (s[0] == '-' || s[0] == '*' || s[0] == '+') && bulletRe.MatchString(s) && !isRule(s) {
		return s[0]
	}
	return 0
}

// paraLike reports whether s can continue or begin a plain paragraph.
func paraLike(s string) bool {
	return !(atxHeadingRe.MatchString(s) || bulletMarker(s) != 0 || orderedRe.MatchString(s) ||
		quoteRe.MatchString(s) || gmFenceOpenRe.MatchString(s) || isRule(s) ||
		strings.HasPrefix(s, "<") || strings.Contains(s, "|"))
}

// listContent is what follows the list marker of a bullet or ordered item.
func listContent(s string) string {
	if m := bulletRe.FindString(s); m != "" {
		return s[len(m):]
	}
	return s[len(orderedRe.FindString(s)):]
}

// step advances past line. It reports whether the line starts a chunk, the
// bullet marker when that start is a top-level bullet item, and whether the
// read view's substitution passes see the line (it is kept and outside a
// read-view fence).
func (c *chunkState) step(line string) (start bool, bullet byte, subst bool) {
	if !c.seen {
		if strings.TrimSpace(line) == "" {
			return false, 0, false
		}
		c.seen = true
		line = strings.TrimLeftFunc(line, unicode.IsSpace)
		start = true
		bullet = bulletMarker(line)
	} else if c.quiescent() && !c.sawCR && !blankLine(line) && startsAtColumn0(line) &&
		line[0] != ':' && line[0] != '<' && !c.lastHTML && !c.lastStyled &&
		!graph.LogbookStartRe.MatchString(line) && !queryOrEmbedRe.MatchString(line) &&
		!linkRefDefRe.MatchString(line) && !(c.lastDef && paraLike(line)) &&
		!setextRe.MatchString(line) {
		switch m := bulletMarker(line); {
		case m != 0:
			start = !taskItemRe.MatchString(line) && strings.TrimSpace(line[1:]) != ""
			bullet = m
		case orderedRe.MatchString(line):
			start = c.top != '1' && c.top != '?' && c.prevBlank
		default:
			start = c.prevBlank
		}
	}

	// The read view's passes, in its order: logbook strip, query/embed strip on
	// the kept lines, then the substitutions outside fences.
	kept := false
	switch {
	case c.inLog:
		if graph.LogbookEndRe.MatchString(line) {
			c.inLog = false
		}
	case c.logFence.Step(line):
		kept = true
	case graph.LogbookStartRe.MatchString(line):
		c.inLog = true
	default:
		kept = true
	}
	kept2 := false
	if kept {
		switch {
		case c.inQ:
			if strings.TrimSpace(line) == "}}" {
				c.inQ = false
			}
		case c.qFence.Step(line):
			kept2 = true
		case queryOrEmbedRe.MatchString(line):
			loc := queryOrEmbedRe.FindStringIndex(line)
			if !strings.Contains(line[loc[1]:], "}}") {
				c.inQ = true
			}
		default:
			kept2 = true
		}
	}
	if kept2 {
		subst = !c.mFence.Step(line)
		c.stepBlocks(line)
	}
	if strings.ContainsRune(line, '\r') {
		c.sawCR = true
	}
	if !start {
		bullet = 0
	}
	return start, bullet, subst
}

// quiescent is true when no hidden block, fence or HTML block is open.
func (c *chunkState) quiescent() bool {
	return !c.inLog && !c.inQ && !c.logFence.Open() && !c.qFence.Open() &&
		!c.mFence.Open() && c.gmFence == 0 && c.html == 0
}

// stepBlocks tracks the CommonMark structure of a kept line: top-level
// fences, HTML blocks 1-5, HTML and definition-list adjacency, and the kind of
// the last top-level block.
func (c *chunkState) stepBlocks(line string) {
	tl := strings.TrimLeft(line, " ")
	indent := len(line) - len(tl)
	blank := blankLine(line)
	col0 := startsAtColumn0(line)

	switch {
	case c.gmFence != 0:
		t := strings.TrimRight(line, " \t\r")
		r := strings.TrimLeft(t, " ")
		if len(t)-len(r) <= 3 && len(r) >= c.gmLen && strings.Trim(r, string(c.gmFence)) == "" {
			c.gmFence = 0
		}
	case c.html != 0:
		if c.htmlEnds(line) {
			c.html = 0
		}
	default:
		if m := gmFenceOpenRe.FindStringSubmatch(line); m != nil && (m[1][0] == '~' || !strings.Contains(m[2], "`")) {
			c.gmFence, c.gmLen = m[1][0], len(m[1])
		} else if indent <= 3 {
			switch {
			case html1Re.MatchString(line) && !html1EndRe.MatchString(line):
				c.html = 1
			case strings.HasPrefix(tl, "<!--") && !strings.Contains(tl[4:], "-->"):
				c.html = 2
			case strings.HasPrefix(tl, "<?") && !strings.Contains(tl[2:], "?>"):
				c.html = 3
			case strings.HasPrefix(tl, "<![CDATA[") && !strings.Contains(tl[9:], "]]>"):
				c.html = 5
			case html4Re.MatchString(line) && !strings.Contains(tl[3:], ">"):
				c.html = 4
			}
		}
	}

	switch {
	case indent <= 3 && strings.HasPrefix(tl, "<"):
		c.lastHTML = true
	case c.html == 0 && c.gmFence == 0 && !blank && col0 && c.prevBlank && !linkRefDefRe.MatchString(line):
		c.lastHTML = false
	}
	switch {
	case indent <= 3 && strings.HasPrefix(tl, ":"):
		c.lastDef = true
	case !blank && col0 && c.prevBlank && !linkRefDefRe.MatchString(line) && !paraLike(line):
		c.lastDef = false
	}
	if !blank && col0 && c.gmFence == 0 {
		m := bulletMarker(line)
		ordered := orderedRe.MatchString(line)
		switch {
		case (m != 0 || ordered) && strings.Trim(listContent(line), " \t") == "":
			c.top = '?'
		case m != 0:
			c.top = m
		case ordered:
			c.top = '1'
		case c.prevBlank:
			c.top = 0
		}
	}
	if !blank && !linkRefDefRe.MatchString(line) {
		// Glamour draws the margin rows of a rule or a heading in that block's
		// style and merges them with the next block's margin, which the stub
		// above a chunk rendered alone cannot reproduce. setextRe also matches
		// the underline of a setext heading, and a rule written as dashes.
		c.lastStyled = isRule(line) || atxHeadingRe.MatchString(line) || setextRe.MatchString(line)
	}
	c.prevBlank = blank
}

// htmlEnds reports whether line closes the open HTML block.
func (c *chunkState) htmlEnds(line string) bool {
	switch c.html {
	case 1:
		return html1EndRe.MatchString(line)
	case 2:
		return strings.Contains(line, "-->")
	case 3:
		return strings.Contains(line, "?>")
	case 4:
		return strings.Contains(line, ">")
	case 5:
		return strings.Contains(line, "]]>")
	}
	return false
}
