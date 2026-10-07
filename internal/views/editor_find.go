package views

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// findBar is the state of the open find/replace bar (PLAN §7).
type findBar struct {
	query, repl  string
	replaceShown bool       // tab pressed once or repl non-empty
	field        int        // 0 find, 1 replace
	origin       buffer.Pos // cursor when the bar opened
	matches      []buffer.Range
	cur          int // index of the current match, -1 none
	version      int // buffer version the matches belong to
}

const (
	findField = iota
	replaceField
)

// Match styles stay visible under NO_COLOR, which strips colour but keeps
// attributes: others are underlined, the current match bold and underlined
// (fg 0 on bg 11, the read view's link-cursor colours).
var (
	matchStyle        = uv.Style{Bg: ansi.IndexedColor(8), Underline: uv.UnderlineSingle}
	currentMatchStyle = uv.Style{Fg: ansi.IndexedColor(0), Bg: ansi.IndexedColor(11), Underline: uv.UnderlineSingle, Attrs: uv.AttrBold}
)

// findRows is the number of rows the find bar takes from the text window.
func (e *EditorView) findRows() int {
	if e.find == nil {
		return 0
	}
	return 1
}

// openFind opens the bar seeded from a one-line selection, else from the
// session's last query. The completion strip is dismissed and the selection
// cleared.
func (e *EditorView) openFind() {
	b := e.buf
	b.Break()
	f := &findBar{query: e.lastQuery, origin: b.Cursor(), cur: -1}
	if sel, ok := b.Selection(); ok {
		f.origin = sel.Start
		if sel.Start.Line == sel.End.Line {
			f.query = fieldInput(b.Text(sel))
		}
		b.MoveTo(sel.Start, false)
	}
	e.find = f
	e.completer.dismiss()
	e.findRun(f.origin, true)
}

// closeFind closes the bar, leaving the cursor where the current match
// started (or at the origin) with no selection.
func (e *EditorView) closeFind() {
	if f := e.find; f != nil && f.query != "" {
		e.lastQuery = f.query
	}
	e.find = nil
	e.goalOK = false
	e.refreshCompleter(false)
}

// findRun recomputes the matches and makes current the first at or after
// from, wrapping. With move the cursor goes to it (or back to the origin when
// nothing matches) and is scrolled into view.
func (e *EditorView) findRun(from buffer.Pos, move bool) {
	f := e.find
	f.matches = e.buf.Find(f.query)
	f.version = e.buf.Version()
	f.cur = -1
	if len(f.matches) > 0 {
		f.cur = sort.Search(len(f.matches), func(i int) bool { return !f.matches[i].Start.Less(from) })
		if f.cur == len(f.matches) {
			f.cur = 0
		}
	}
	if move {
		e.findShow()
	}
	e.ensureVisible()
}

// findShow puts the cursor on the current match, or at the origin without one.
func (e *EditorView) findShow() {
	f := e.find
	if f.cur >= 0 {
		e.buf.MoveTo(f.matches[f.cur].Start, false)
	} else {
		e.buf.MoveTo(f.origin, false)
	}
	e.goalOK = false
	e.ensureVisible()
}

// findSync re-finds when the buffer changed under the matches (undo, redo, a
// merged save). The cursor stays; the current match is the first at or after it.
func (e *EditorView) findSync() {
	if f := e.find; f != nil && f.version != e.buf.Version() {
		e.findRun(e.buf.Cursor(), false)
	}
}

// findStep moves to the next (dir > 0) or previous match, wrapping.
func (e *EditorView) findStep(dir int) {
	f := e.find
	n := len(f.matches)
	if n == 0 {
		return
	}
	next := f.cur + dir
	if next < 0 || next >= n {
		e.notice = "search wrapped"
	}
	f.cur = (next + n) % n
	e.findShow()
}

// findReplaceOne replaces the current match and moves to the first match at
// or after the end of the replacement.
func (e *EditorView) findReplaceOne() {
	f := e.find
	if f.cur < 0 {
		return
	}
	e.buf.ReplaceRanges([]buffer.Range{f.matches[f.cur]}, f.repl)
	e.goalOK = false
	e.findRun(e.buf.Cursor(), true)
}

// findReplaceAll replaces every match as one undo step.
func (e *EditorView) findReplaceAll() {
	f := e.find
	n := e.buf.ReplaceRanges(f.matches, f.repl)
	if n == 0 {
		return
	}
	e.goalOK = false
	e.findRun(e.buf.Cursor(), false)
	e.notice = "replaced " + strconv.Itoa(n)
	e.ensureVisible()
}

// fieldText returns the focused field's text.
func (f *findBar) fieldText() *string {
	if f.field == replaceField {
		return &f.repl
	}
	return &f.query
}

// findEdited re-runs the search after the find field changed, from the origin.
func (e *EditorView) findEdited() {
	if e.find.field == findField {
		e.findRun(e.find.origin, true)
	}
}

// updateFind handles one key while the bar is open. handled is false for the
// keys that act on the buffer (ctrl+s, ctrl+z, ctrl+y); the caller runs them
// and then calls findSync.
func (e *EditorView) updateFind(msg tea.KeyPressMsg) (res EditorResult, handled bool) {
	f := e.find
	e.findSync()
	key := msg.String()
	switch key {
	case "ctrl+s", "ctrl+z", "ctrl+y":
		return EditorResult{}, false
	case keyEsc:
		e.closeFind()
	case "ctrl+f":
		f.field = findField
	case "tab", "shift+tab":
		if !f.replaceShown && f.repl == "" {
			f.replaceShown, f.field = true, replaceField
		} else {
			f.field = 1 - f.field
		}
	case keyEnter:
		if f.field == replaceField {
			e.findReplaceOne()
		} else {
			e.findStep(+1)
		}
	case keyDown, "ctrl+n":
		e.findStep(+1)
	case keyUp, "ctrl+p":
		e.findStep(-1)
	case "ctrl+a":
		if f.field == replaceField {
			e.findReplaceAll()
		}
	case "ctrl+u":
		*f.fieldText() = ""
		e.findEdited()
	case keyBackspace, "ctrl+h":
		s := f.fieldText()
		*s = dropLastGrapheme(*s)
		e.findEdited()
	default:
		if t := msg.Text; t != "" && !strings.ContainsFunc(t, unicode.IsControl) {
			*f.fieldText() += t
			e.findEdited()
		}
	}
	return EditorResult{}, true
}

// pasteFind puts the first line of a paste, C0 characters dropped, into the
// focused field.
func (e *EditorView) pasteFind(content string) {
	f := e.find
	line, _, _ := strings.Cut(cleanPaste(content), "\n")
	*f.fieldText() += fieldInput(line)
	e.findEdited()
}

// fieldInput is s as a find or replace field may hold it: invalid UTF-8 and
// control characters dropped, so the bar never prints a raw byte of the file.
func fieldInput(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}

// dropLastGrapheme removes the last user-perceived character of s.
func dropLastGrapheme(s string) string {
	g := uniseg.NewGraphemes(s)
	last := 0
	for g.Next() {
		last, _ = g.Positions()
	}
	return s[:last]
}

// findOverlay draws the matches of line into spans. The current match, when
// on this line, is returned so the caller can draw it above the selection.
func (e *EditorView) findOverlay(spans []render.Span, line int) ([]render.Span, *buffer.Range) {
	f := e.find
	if f == nil || f.version != e.buf.Version() {
		return spans, nil
	}
	var cur *buffer.Range
	i := sort.Search(len(f.matches), func(i int) bool { return f.matches[i].Start.Line >= line })
	for ; i < len(f.matches) && f.matches[i].Start.Line == line; i++ {
		m := f.matches[i]
		spans = render.Overlay(spans, m.Start.Col, m.End.Col, matchStyle)
		if i == f.cur {
			cur = &f.matches[i]
		}
	}
	return spans, cur
}

// hints is the status line's right side while the bar is open.
func (f *findBar) hints() string {
	if f.field == replaceField {
		return "↵ replace · ^A all · tab find · esc close"
	}
	return "↵ next · ↑ prev · tab replace · esc close"
}

// barView renders the bar's row (without the left margin) and the cell
// column of the terminal cursor within it. avail is the room in cells.
func (f *findBar) barView(avail int) (string, int) {
	count := ""
	switch {
	case f.query == "":
	case len(f.matches) == 0:
		count = "  no matches"
	default:
		count = fmt.Sprintf("  %d/%d", f.cur+1, len(f.matches))
	}
	show := f.replaceShown || f.repl != ""
	const findLabel, replLabel = "Find: ", "Replace: "
	fixed := ansi.StringWidth(findLabel) + ansi.StringWidth(count)
	if show {
		fixed += 2 + ansi.StringWidth(replLabel)
	}
	room := max(2, avail-fixed)
	query, repl := render.DisplayText(f.query), render.DisplayText(f.repl)
	fw, rw := ansi.StringWidth(query), ansi.StringWidth(repl)
	fRoom, rRoom := fw, rw
	if !show {
		fRoom = room
	} else if fw+rw > room {
		switch {
		case fw <= room/2:
			rRoom = room - fw
		case rw <= room/2:
			fRoom = room - rw
		default:
			fRoom, rRoom = room/2, room-room/2
		}
	}
	label := func(s string, focused bool) string {
		if focused {
			return styleTitle.Render(s)
		}
		return styleFaint.Render(s)
	}
	q := tailFit(query, fRoom)
	var b strings.Builder
	b.WriteString(label(findLabel, f.field == findField))
	b.WriteString(q)
	x := ansi.StringWidth(findLabel) + ansi.StringWidth(q)
	b.WriteString(styleFaint.Render(count))
	if !show {
		return b.String(), x
	}
	r := tailFit(repl, rRoom)
	b.WriteString("  ")
	b.WriteString(label(replLabel, f.field == replaceField))
	b.WriteString(r)
	if f.field == replaceField {
		x = ansi.StringWidth(findLabel) + ansi.StringWidth(q) + ansi.StringWidth(count) + 2 + ansi.StringWidth(replLabel) + ansi.StringWidth(r)
	}
	return b.String(), x
}

// tailFit keeps the end of s (where typing happens) within room cells,
// marking a cut with a leading ellipsis.
func tailFit(s string, room int) string {
	w := ansi.StringWidth(s)
	if w <= room {
		return s
	}
	if room <= 1 {
		return "…"
	}
	return "…" + ansi.TruncateLeft(s, w-(room-1), "")
}
