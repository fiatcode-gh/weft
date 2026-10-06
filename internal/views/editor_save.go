package views

import (
	"errors"
	"strings"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/merge"
)

const (
	mergedNotice          = "saved — merged changes made outside weft"
	changedWhileSavingMsg = "file changed on disk while saving — press Ctrl+S again"
)

type saveOutcome int

const (
	saveBlocked saveOutcome = iota // clash prompt shown or error set; editor stays
	saveWritten
	saveMerged
)

// saveEditor saves the editor buffer without ever replacing content that
// changed on disk since weft last read or wrote it: unchanged → plain write;
// changed in non-touching lines → merged write plus buffer update; anything
// else → clash prompt with nothing written.
func (a *App) saveEditor() saveOutcome {
	e := a.editor
	theirs, err := a.readSnapshot(e.path)
	if err != nil {
		e.SetError(err.Error())
		return saveBlocked
	}
	mine := e.Content()
	if theirs == e.disk {
		if !a.writeEditor(theirs, mine) {
			return saveBlocked
		}
		e.MarkSaved(mine)
		return saveWritten
	}
	if !e.disk.Exists || !theirs.Exists {
		e.showClash(theirs)
		return saveBlocked
	}
	r := merge.Lines(mergeInput(e.disk.Content), mergeInput(mine), mergeInput(theirs.Content))
	if r.Conflict {
		e.showClash(theirs)
		return saveBlocked
	}
	text := normalizeContent(r.Text)
	if !loadsFaithfully(text) {
		e.showClash(theirs)
		return saveBlocked
	}
	// Write before touching the buffer so a failed write leaves it intact.
	if !a.writeEditor(theirs, text) {
		return saveBlocked
	}
	e.applyMerge(text, r.MineLine)
	return saveMerged
}

// writeEditor writes content only if the file still equals seen. On failure
// it surfaces the error on the editor and reports false.
func (a *App) writeEditor(seen edit.Snapshot, content string) bool {
	err := edit.WriteFileIfUnchanged(a.editor.path, seen, []byte(content))
	switch {
	case err == nil:
		return true
	case errors.Is(err, edit.ErrChanged):
		a.editor.SetError(changedWhileSavingMsg)
	default:
		a.editor.SetError(err.Error())
	}
	return false
}

// mergeInput normalizes a merge input: empty, or ending in exactly one
// newline. Unlike normalizeContent a newline-only text is empty (zero lines),
// so an empty file and an empty buffer agree.
func mergeInput(s string) string {
	t := strings.TrimRight(s, "\n")
	if t == "" {
		return ""
	}
	return t + "\n"
}
