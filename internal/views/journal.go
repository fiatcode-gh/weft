package views

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// journalTemplateName is the page whose file content seeds a new journal file.
const journalTemplateName = "Journal Template"

// journalTemplate returns the template page's file content as it is on disk
// now, or "" when no page resolves to journalTemplateName or its file is gone.
// A read failure is returned ("cannot read Journal Template: …").
func (a *App) journalTemplate() (string, error) {
	meta, ok := a.idx.Resolve(journalTemplateName)
	if !ok {
		return "", nil
	}
	snap, err := a.readSnapshot(meta.Path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", journalTemplateName, err)
	}
	return snap.Content, nil
}

// journalPath is where journal name's file lives: journals/<YYYY_MM_DD>.md.
func (a *App) journalPath(name string) string {
	return filepath.Join(a.graphPath, "journals", graph.FilenameFromPageName(name))
}

// createJournal creates journal name's file from the journal template, or
// empty when there is none, and never touches an existing file.
func (a *App) createJournal(name string) (created bool, err error) {
	path := a.journalPath(name)
	tmpl, err := a.journalTemplate()
	if err != nil {
		return false, err
	}
	if tmpl == "" {
		created, err = edit.EnsureFile(path)
	} else {
		err = edit.WriteFileIfUnchanged(path, edit.Snapshot{}, []byte(tmpl))
		switch {
		case err == nil:
			created = true
		case errors.Is(err, edit.ErrChanged):
			err = nil // the file appeared meanwhile: leave it alone
		}
	}
	if err != nil {
		return false, fmt.Errorf("cannot create journal: %w", err)
	}
	return created, nil
}

// createJournalAndReindex creates the on-disk file for journal page `name`
// (from the journal template, or empty without one), rebuilds the index
// synchronously, and rebinds the current PageView to it. Shared by the `.`
// and `E` handlers when they land on a today's-journal page whose file
// doesn't exist yet. Returns an error whose message is ready for setHint.
func (a *App) createJournalAndReindex(name string) error {
	if _, err := a.createJournal(name); err != nil {
		return err
	}
	if err := a.reindex(); err != nil {
		return fmt.Errorf("reindex failed: %w", err)
	}
	return nil
}
