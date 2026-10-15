package views

import (
	"math"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
)

// vimMode is the modal state of a vim-mode editor.
type vimMode int

const (
	vimNormal vimMode = iota
	vimInsert
	vimVisual
	vimVisualLine
)

// vimState is the modal state of an editor opened with vim editing; a
// modeless editor has none (EditorView.vim is nil).
type vimState struct {
	mode    vimMode
	pending []string   // tokens of the command being typed
	insMark int        // undo depth before the change that entered insert mode
	anchor  buffer.Pos // visual mode: the end of the selection that stays put
	ex      *string    // the ex line's text; nil while it is closed

	// Dot repeat (LD12): last is the change `.` repeats, rec the change whose
	// insert session is open, replaying is set while `.` or a counted insert
	// re-runs input, insCount the times the session's input is applied in all.
	last      *vimChange
	rec       *vimChange
	replaying bool
	insCount  int

	// lastFind is the last f F t T, which ; and , repeat.
	lastFind struct {
		kind byte
		char string
	}
}

// vimAlias maps the arrow and home/end keys to the motions they stand for in
// normal mode.
var vimAlias = map[string]string{
	"left": "h", "right": "l", "up": "k", "down": "j", "home": "0", "end": "$",
}

// vimPassthrough is the set of weft chords normal mode hands to the modeless
// key handler, beside every extendKeys chord.
var vimPassthrough = map[string]bool{
	"ctrl+s": true, "ctrl+f": true, "alt+s": true, "alt+e": true,
	"ctrl+z": true, "ctrl+y": true, "ctrl+t": true, "alt+p": true,
	"alt+up": true, "alt+down": true, "ctrl+a": true, "ctrl+c": true,
	"ctrl+x": true, "ctrl+v": true, "pgup": true, "pgdown": true,
}

// vimToken is the parser's name for a key: the typed text when it is plain
// text, else the key's name.
func vimToken(msg tea.KeyPressMsg) string {
	if msg.Text != "" && !strings.ContainsFunc(msg.Text, unicode.IsControl) {
		return msg.Text
	}
	return msg.String()
}

// enableVim switches the editor to vim modal editing, in normal mode.
func (e *EditorView) enableVim() {
	e.vim = &vimState{mode: vimNormal}
	e.vimSettle()
}

// vimUpdate handles one key of a vim-mode editor.
func (e *EditorView) vimUpdate(msg tea.KeyPressMsg) (EditorResult, tea.Cmd) {
	if e.vim.ex != nil {
		return e.vimExKey(msg)
	}
	if e.prompt != nil || e.find != nil {
		res, cmd := e.editKey(msg)
		if e.prompt == nil && e.find == nil && e.vim.mode != vimInsert {
			e.vimSettle()
		}
		return res, cmd
	}
	if e.vim.mode == vimInsert {
		return e.vimInsertKey(msg)
	}
	if e.vimVisual() {
		return e.vimVisualKey(msg)
	}
	return e.vimNormalKey(msg)
}

// vimNormalKey handles one key in normal mode.
func (e *EditorView) vimNormalKey(msg tea.KeyPressMsg) (EditorResult, tea.Cmd) {
	v := e.vim
	key := msg.String()
	if key == keyEsc {
		if len(v.pending) > 0 {
			v.pending = nil
			return EditorResult{}, nil
		}
		return e.editKey(msg)
	}
	if _, extend := extendKeys[key]; extend || vimPassthrough[key] {
		v.pending = nil
		res, cmd := e.editKey(msg)
		if e.mode == editing && e.prompt == nil && e.find == nil {
			e.vimSettle()
		}
		return res, cmd
	}
	tok := vimToken(msg)
	if alias, ok := vimAlias[tok]; ok {
		tok = alias
	}
	v.pending = append(v.pending, tok)
	var res EditorResult
	var cmd tea.Cmd
	switch c, st := parseVim(v.pending, false); st {
	case vimDone:
		v.pending = nil
		res, cmd = e.vimRun(c)
	case vimBad:
		v.pending = nil
	}
	e.vimSettle()
	return res, cmd
}

// vimRun executes one parsed normal-mode command and records it for `.`.
func (e *EditorView) vimRun(c vimCmd) (EditorResult, tea.Cmd) {
	if c.key == "." && c.op == "" {
		return EditorResult{}, e.vimDot(c.count)
	}
	v := e.vim
	change := vimIsChange(c)
	before := e.buf.Version()
	res, cmd := e.vimExec(c)
	if v.mode == vimInsert {
		v.insCount = 1
		if c.op == "" && slices.Contains(vimCountedInserts, c.key) {
			v.insCount = max(1, c.count)
		}
		if !v.replaying {
			v.rec = &vimChange{cmd: c}
		}
	} else if change && !v.replaying && e.buf.Version() != before {
		v.last = &vimChange{cmd: c}
	}
	return res, cmd
}

// vimExec runs one parsed normal-mode command, the one executor of live and
// replayed commands.
func (e *EditorView) vimExec(c vimCmd) (EditorResult, tea.Cmd) {
	if alias, ok := vimShortcuts[c.key]; ok && c.op == "" {
		c = vimCmd{count: c.count, op: alias[0], key: alias[1]}
	}
	if c.op != "" {
		return EditorResult{}, e.vimOperate(c)
	}
	b := e.buf
	cur := b.Cursor()
	switch c.key {
	case "j", "k":
		dir := 1
		if c.key == "k" {
			dir = -1
		}
		for range max(c.count, 1) {
			before := b.Cursor()
			e.moveRows(dir, 1, false)
			if b.Cursor() == before {
				break
			}
		}
	case "ctrl+d", "ctrl+u":
		rows, dir := c.count, 1
		if rows == 0 {
			rows = max(1, e.textHeight()/2)
		}
		if c.key == "ctrl+u" {
			dir = -1
		}
		sr := e.cursorScreenRow()
		e.moveRows(dir, rows, false)
		e.scrollCursorTo(sr)
	case "i", "a", "I", "A", "o", "O":
		mark := b.UndoMark()
		switch c.key {
		case "a":
			q, _ := b.StepRight(cur, 1)
			b.MoveTo(q, false)
		case "I":
			b.MoveTo(b.FirstNonBlank(cur), false)
		case "A":
			b.MoveTo(b.LineEnd(cur), false)
		case "o":
			b.OpenLine(false)
		case "O":
			b.OpenLine(true)
		}
		e.vimEnterInsert(mark)
	case ":":
		e.vimExOpen()
	case "ZZ":
		return e.vimExLine("x")
	case "ZQ":
		return e.vimExLine("q!")
	case "v":
		e.vimStartVisual(vimVisual)
	case "V":
		e.vimStartVisual(vimVisualLine)
	case "r":
		e.vimReplace(c)
	case "J":
		e.vimJoin(c)
	case "p", "P":
		e.vimPut(c)
	case "u":
		e.history(b.Undo(), "nothing to undo")
	case "ctrl+r":
		e.history(b.Redo(), "nothing to redo")
	default:
		if q, _, ok := e.vimMotion(c, false); ok {
			b.MoveTo(q, false)
			e.goalOK = false
			if c.key == "$" {
				e.goalX, e.goalOK = math.MaxInt32, true
			}
		}
	}
	return EditorResult{}, nil
}

// vimEnterInsert starts an insert session whose undo step began at mark.
func (e *EditorView) vimEnterInsert(mark int) {
	e.vim.mode = vimInsert
	e.vim.insMark = mark
	e.goalOK = false
}

// vimInsertKey handles one key in insert mode: everything is the modeless
// editor's, except leaving and the undo keys.
func (e *EditorView) vimInsertKey(msg tea.KeyPressMsg) (EditorResult, tea.Cmd) {
	key := msg.String()
	switch key {
	case keyEsc:
		e.vimLeaveInsert()
		return EditorResult{}, nil
	case "ctrl+z", "ctrl+y", "ctrl+r":
		before := e.buf.Version()
		e.outsideInsertStep(func() {
			if key == "ctrl+z" {
				e.history(e.buf.Undo(), "nothing to undo")
			} else {
				e.history(e.buf.Redo(), "nothing to redo")
			}
		})
		e.afterKey(e.buf.Version() != before)
		e.findSync()
		return EditorResult{}, nil
	}
	if v := e.vim; !v.replaying && v.rec != nil && !vimUnrecorded[key] {
		v.rec.inputs = append(v.rec.inputs, vimInput{key: msg})
	}
	return e.editKey(msg)
}

// vimLeaveInsert ends the insert session as one undo step and returns to
// normal mode, one grapheme left within the line.
func (e *EditorView) vimLeaveInsert() {
	if e.completer.active {
		e.completer.dismiss()
	}
	v := e.vim
	if v.insCount > 1 && v.rec != nil {
		e.vimReplayInsert(v.rec, v.insCount-1)
	}
	e.buf.Squash(v.insMark)
	if v.rec != nil {
		v.last, v.rec = v.rec, nil
	}
	q, _ := e.buf.StepLeft(e.buf.Cursor(), 1)
	e.buf.MoveTo(q, false)
	e.vim.mode = vimNormal
	e.goalOK = false
	e.vimSettle()
}

// outsideInsertStep runs fn, a buffer change that is not part of the open
// insert session (undo, merge, reload), as its own undo step: the session so
// far is squashed first and a new one starts after. Outside insert mode it
// just runs fn.
func (e *EditorView) outsideInsertStep(fn func()) {
	if e.vim == nil || e.vim.mode != vimInsert {
		fn()
		return
	}
	e.buf.Squash(e.vim.insMark)
	fn()
	e.vim.insMark = e.buf.UndoMark()
}

// vimSettle brings the editor in line with normal-mode rules after a key: no
// completion strip, the cursor on a character (unless a modeless selection is
// open or insert mode has just begun), the view up to date.
func (e *EditorView) vimSettle() {
	if e.completer.active {
		e.completer.dismiss()
	}
	b := e.buf
	if _, sel := b.Selection(); !sel && e.vim.mode != vimInsert {
		cur := b.Cursor()
		p := cur
		p.Line = min(p.Line, b.LastLine())
		if last := b.LineLast(p); p.Col > last.Col {
			p = last
		}
		if p != cur {
			b.MoveTo(p, false)
		}
	}
	e.afterKey(false)
	e.findSync()
}

// vimTag is the mode name shown in the status line; normal mode has none.
func (e *EditorView) vimTag() string {
	switch e.vim.mode {
	case vimInsert:
		return "-- INSERT --"
	case vimVisual:
		return "-- VISUAL --"
	case vimVisualLine:
		return "-- VISUAL LINE --"
	}
	return ""
}

// vimPendingText is the command typed so far, as the status line shows it.
func (e *EditorView) vimPendingText() string { return strings.Join(e.vim.pending, "") }

// vimHints are the status-line key hints of the current mode.
func (e *EditorView) vimHints() string {
	if e.vim.mode == vimInsert {
		return "esc normal · ^S save"
	}
	other := "source"
	if e.source {
		other = "preview"
	}
	return ":w save · :q quit · :preview " + other
}
