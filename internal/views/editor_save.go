package views

import (
	"errors"

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
// else → clash prompt with nothing written. exitAfter records that the save
// came from save-and-exit, so the prompt's overwrite can finish the exit.
func (a *App) saveEditor(exitAfter bool) saveOutcome {
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
		e.showClash(theirs, exitAfter)
		return saveBlocked
	}
	r := merge.Text(e.disk.Content, mine, theirs.Content)
	if r.Conflict {
		e.showClash(theirs, exitAfter)
		return saveBlocked
	}
	// Write before touching the buffer so a failed write leaves it intact.
	if !a.writeEditor(theirs, r.Text) {
		return saveBlocked
	}
	e.applyMerge(r.Text, r.MineLine)
	return saveMerged
}

// overwriteEditor writes the buffer over the clash snapshot, guarded so a
// further outside change is refused rather than clobbered.
func (a *App) overwriteEditor() bool {
	mine := a.editor.Content()
	if !a.writeEditor(a.editor.clash.theirs, mine) {
		return false
	}
	a.editor.MarkSaved(mine)
	return true
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
