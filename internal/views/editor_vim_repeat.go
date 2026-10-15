package views

import (
	"slices"

	tea "charm.land/bubbletea/v2"
)

// vimChange is a recorded change command and the input of the insert
// session it opened, which `.` runs again.
type vimChange struct {
	cmd    vimCmd
	inputs []vimInput
}

// vimInput is one input of an insert session: a key, or a bracketed paste.
type vimInput struct {
	key     tea.KeyPressMsg
	paste   string
	isPaste bool
}

// vimCountedInserts are the commands whose count repeats the typed text
// instead of scaling a motion.
var vimCountedInserts = []string{"i", "a", "I", "A", "o", "O"}

// vimChangeKeys are the commands without an operator that `.` repeats.
var vimChangeKeys = []string{"x", "X", "s", "S", "r", "J", "p", "P", "D", "C", "o", "O", "a", "A", "i", "I"}

// vimIsChange reports whether c is a change `.` repeats: every operator but
// y, and the change commands (LD12).
func vimIsChange(c vimCmd) bool {
	if c.op != "" {
		return c.op != "y"
	}
	return slices.Contains(vimChangeKeys, c.key)
}

// vimUnrecorded are the insert-mode chords a repeat never replays: saving,
// finding, dating, undo and the clipboard.
var vimUnrecorded = map[string]bool{
	"ctrl+s": true, "ctrl+f": true, "alt+s": true, "alt+e": true,
	"ctrl+z": true, "ctrl+y": true, "ctrl+r": true, "ctrl+c": true, "ctrl+x": true,
}

// vimFeed applies one recorded input through the editor's own key and paste
// handling.
func (e *EditorView) vimFeed(in vimInput) {
	if in.isPaste {
		e.pasteText(in.paste)
		return
	}
	e.editKey(in.key)
}

// vimReplayInsert applies the inputs of rec n more times; o and O open a new
// line before each.
func (e *EditorView) vimReplayInsert(rec *vimChange, n int) {
	v := e.vim
	was := v.replaying
	v.replaying = true
	for range n {
		if rec.cmd.op == "" && (rec.cmd.key == "o" || rec.cmd.key == "O") {
			e.buf.OpenLine(rec.cmd.key == "O")
		}
		for _, in := range rec.inputs {
			e.vimFeed(in)
		}
	}
	v.replaying = was
}

// vimDot is `.`: the last change again, as one undo step. A count replaces
// the stored one.
func (e *EditorView) vimDot(count int) tea.Cmd {
	v := e.vim
	last := v.last
	if last == nil {
		return nil
	}
	if count > 0 {
		last.cmd.count = count
	}
	mark := e.buf.UndoMark()
	v.replaying = true
	_, cmd := e.vimRun(last.cmd)
	if v.mode == vimInsert {
		v.rec = last
		for _, in := range last.inputs {
			e.vimFeed(in)
		}
		e.vimLeaveInsert()
	}
	v.replaying = false
	e.buf.Squash(mark)
	return cmd
}
