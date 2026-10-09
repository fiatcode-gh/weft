package render

import (
	"regexp"
	"strings"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// SourceLines is the read-only view of a document the source layout needs.
// *buffer.Buffer satisfies it; render never imports the buffer.
type SourceLines interface {
	Len() int
	Line(i int) string
}

// BlockKind is the block a source line belongs to.
type BlockKind uint8

const (
	KindBlank BlockKind = iota
	KindText
	KindHeading
	KindSetextHeading
	KindSetextUnderline
	KindBullet
	KindOrdered
	KindQuote
	KindFence
	KindCode
	KindTable
	KindRule
	KindHidden
)

// LineInfo is a source line's block context.
type LineInfo struct {
	Kind   BlockKind
	Level  int    // heading level; list depth (0 = top) for Bullet/Ordered and InList text
	Indent int    // display cells of leading whitespace (tab → next multiple of 4)
	Marker int    // byte offset where the line's own text starts after its block marker
	InList bool   // non-bullet line continuing a list item
	Fence  int    // Fence/Code: index of the opening delimiter line
	Lang   string // Fence/Code: first word of the opener's info string
	Strip  int    // Code: display column of the fence characters on the opener line

	// Live preview (see chunkState): Start marks a line a chunk may begin at;
	// Bullet is the marker when that line is a top-level bullet item; Subst
	// marks a line the read view's wiki-link and task-marker passes see.
	Start  bool
	Bullet byte
	Subst  bool
}

var (
	atxHeadingRe = regexp.MustCompile(`^ {0,3}(#{1,6})(?:\s+|$)`)
	setextRe     = regexp.MustCompile(`^ {0,3}(=+|-+) *$`)
	bulletRe     = regexp.MustCompile(`^\s*[-*+](?:\s+|$)`)
	orderedRe    = regexp.MustCompile(`^\s*\d{1,9}[.)](?:\s+|$)`)
	quoteRe      = regexp.MustCompile(`^\s*> ?`)
	tableDelimRe = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
)

type hiddenState uint8

const (
	hiddenNone hiddenState = iota
	hiddenLogbook
	hiddenQuery
)

type tableState uint8

const (
	tableNone   tableState = iota
	tableHeader            // the header row was seen; the next line is the delimiter row
	tableBody
)

// scanState is everything the forward pass carries between lines. listStack is
// copy-on-write: a stored snapshot is never mutated.
type scanState struct {
	fence     graph.FenceState
	opener    int
	lang      string
	strip     int
	listStack []int
	hidden    hiddenState
	setext    int // level of the setext heading just classified; 0 = none
	table     tableState
	chunk     chunkState
}

// Scanner classifies source lines in one forward pass and caches the result,
// with a state snapshot before every line so a scan resumes where an edit
// invalidated it.
type Scanner struct {
	infos   []LineInfo
	states  []scanState // states[i] is the state before line i
	scanned int
}

func NewScanner() *Scanner { return &Scanner{states: []scanState{{}}} }

// Invalidate drops everything from line from on. Line from-1 goes too: setext
// and table classification look one line ahead.
func (s *Scanner) Invalidate(from int) {
	from = max(0, from-1)
	if from < len(s.infos) {
		s.infos = s.infos[:from]
		s.states = s.states[:from+1]
	}
}

// Scanned counts the line classifications performed since creation.
func (s *Scanner) Scanned() int { return s.scanned }

// Info returns line i's classification, scanning forward as needed. A line
// outside the document gets the zero LineInfo.
func (s *Scanner) Info(src SourceLines, i int) LineInfo {
	if i < 0 || i >= src.Len() {
		return LineInfo{}
	}
	for len(s.infos) <= i {
		n := len(s.infos)
		next := ""
		if n+1 < src.Len() {
			next = src.Line(n + 1)
		}
		st := s.states[n]
		info := classify(&st, n, src.Line(n), next, n+1 < src.Len())
		info.Start, info.Bullet, info.Subst = st.chunk.step(src.Line(n))
		s.infos = append(s.infos, info)
		s.states = append(s.states, st)
		s.scanned++
	}
	return s.infos[i]
}

// leadingSpace returns the display width of line's leading whitespace and its
// byte length.
func leadingSpace(line string) (cells, bytes int) {
	for bytes < len(line) {
		switch line[bytes] {
		case ' ':
			cells++
		case '\t':
			cells += 4 - cells%4
		default:
			return cells, bytes
		}
		bytes++
	}
	return cells, bytes
}

func isRule(line string) bool {
	_, n := leadingSpace(line)
	rest := line[n:]
	if n > 3 || rest == "" {
		return false
	}
	c := rest[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	count := 0
	for i := range len(rest) {
		switch rest[i] {
		case c:
			count++
		case ' ':
		default:
			return false
		}
	}
	return count >= 3
}

// classify classifies one line, advancing st past it.
func classify(st *scanState, idx int, line, next string, hasNext bool) LineInfo {
	indent, ws := leadingSpace(line)
	info := LineInfo{Indent: indent}

	switch st.hidden {
	case hiddenLogbook:
		if graph.LogbookEndRe.MatchString(line) {
			st.hidden = hiddenNone
		}
		info.Kind = KindHidden
		return info
	case hiddenQuery:
		if strings.TrimSpace(line) == "}}" {
			st.hidden = hiddenNone
		}
		info.Kind = KindHidden
		return info
	}

	wasOpen := st.fence.Open()
	if st.fence.Step(line) {
		info.Indent = indent
		switch {
		case !wasOpen:
			m := graph.FenceDelimiterRe.FindString(line)
			st.opener = idx
			st.strip = 0
			for _, c := range displayCells(line[:len(m)-3]) {
				st.strip += c.w
			}
			if f := strings.Fields(line[len(m):]); len(f) > 0 {
				st.lang = f[0]
			} else {
				st.lang = ""
			}
			info.Kind = KindFence
		case !st.fence.Open():
			info.Kind = KindFence
		default:
			info.Kind = KindCode
		}
		info.Fence, info.Lang, info.Strip = st.opener, st.lang, st.strip
		return info
	}
	if graph.LogbookStartRe.MatchString(line) {
		st.hidden = hiddenLogbook
		info.Kind = KindHidden
		return info
	}
	if loc := graph.QueryOrEmbedRe.FindStringIndex(line); loc != nil {
		if !strings.Contains(line[loc[1]:], "}}") {
			st.hidden = hiddenQuery
		}
		info.Kind = KindHidden
		return info
	}

	setext := st.setext
	st.setext = 0
	table := st.table
	st.table = tableNone

	if setext > 0 {
		info.Kind, info.Level = KindSetextUnderline, setext
		return info
	}
	if strings.TrimSpace(line) == "" {
		info.Kind = KindBlank
		return info
	}
	if table == tableHeader {
		st.table = tableBody
		info.Kind = KindTable
		return info
	}
	if table == tableBody && strings.Contains(line, "|") {
		st.table = tableBody
		info.Kind = KindTable
		return info
	}

	switch {
	case atxHeadingRe.MatchString(line):
		m := atxHeadingRe.FindStringSubmatch(line)
		info.Kind, info.Level, info.Marker = KindHeading, len(m[1]), len(m[0])
		st.clearList(indent)
	case isRule(line):
		info.Kind = KindRule
		st.clearList(indent)
	case bulletRe.MatchString(line), orderedRe.MatchString(line):
		re, kind := bulletRe, KindBullet
		if !bulletRe.MatchString(line) {
			re, kind = orderedRe, KindOrdered
		}
		info.Kind, info.Marker = kind, len(re.FindString(line))
		info.Level = st.pushList(indent)
	case quoteRe.MatchString(line):
		info.Kind, info.Marker = KindQuote, len(quoteRe.FindString(line))
		st.clearList(indent)
	case strings.Contains(line, "|") && hasNext &&
		strings.Contains(next, "|") && tableDelimRe.MatchString(next):
		info.Kind, info.Marker = KindTable, ws
		st.table = tableHeader
		st.clearList(indent)
	default:
		info.Kind, info.Marker = KindText, ws
		if indent > 0 && len(st.listStack) > 0 {
			info.InList = true
			info.Level = len(st.listStack) - 1
		} else {
			st.clearList(indent)
			if m := setextRe.FindStringSubmatch(next); hasNext && m != nil {
				info.Kind, info.Level = KindSetextHeading, 1
				if m[1][0] == '-' {
					info.Level = 2
				}
				st.setext = info.Level
			}
		}
	}
	return info
}

// clearList empties the list stack when a non-list line starts at column 0.
func (st *scanState) clearList(indent int) {
	if indent == 0 {
		st.listStack = nil
	}
}

// pushList records a bullet at indent and returns its depth: bullets at the
// same or deeper indent are popped first.
func (st *scanState) pushList(indent int) int {
	n := len(st.listStack)
	for n > 0 && st.listStack[n-1] >= indent {
		n--
	}
	st.listStack = append(st.listStack[:n:n], indent)
	return n
}
