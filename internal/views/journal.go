package views

import (
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v3/internal/edit"
	"github.com/fiatcode-gh/weft/v3/internal/graph"
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

// createJournal creates the file of the journal dated day (and any missing
// folders) from the journal template, or empty when there is none, and never
// touches an existing file.
func (a *App) createJournal(day time.Time) (created bool, err error) {
	path := a.idx.JournalPath(day)
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

// createJournalAndReindex creates the on-disk file for the journal dated day
// (from the journal template, or empty without one), rebuilds the index
// synchronously, and rebinds the current PageView to it. Shared by the `.`
// and `E` handlers when they land on a today's-journal page whose file
// doesn't exist yet. Returns an error whose message is ready for setHint.
func (a *App) createJournalAndReindex(day time.Time) error {
	if _, err := a.createJournal(day); err != nil {
		return err
	}
	if err := a.reindex(); err != nil {
		return fmt.Errorf("reindex failed: %w", err)
	}
	return nil
}

// gotoToday opens today's journal, creating its file first when it has none.
// Callers gate it on a running sync (unlessSyncing).
func (a *App) gotoToday() tea.Cmd {
	day := a.today()
	today := a.lay.JournalName(day)
	var probe tea.Cmd
	if _, ok := a.idx.JournalOn(day); !ok {
		if err := a.createJournalAndReindex(day); err != nil {
			return a.setHint(err.Error())
		}
		// Creating the journal wrote a new file (from the journal
		// template, or empty without one) — refresh the indicator.
		probe = a.statusProbeCmd()
	}
	if a.page.Page() != today {
		a.navigate(today)
	}
	return probe
}

// stepJournal moves to the neighbouring journal in direction dir; on a journal
// with none that way it says none, and on any other page it does nothing.
func (a *App) stepJournal(dir int, none string) tea.Cmd {
	page := a.page.Page()
	if name, ok := a.idx.JournalNeighbor(page, dir); ok {
		a.navigate(name)
	} else if _, ok := a.idx.JournalDateOf(page); ok {
		return a.setHint(none)
	}
	return nil
}

// isJournalName reports whether name is shaped like a journal under idx's
// layout.
func isJournalName(idx *graph.Index, name string) bool {
	_, ok := idx.Layout.ParseJournalName(name)
	return ok
}
