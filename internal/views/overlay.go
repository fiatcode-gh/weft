package views

import (
	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/weft/internal/graph"
)

// OverlayResult is what an overlay's Update reports back to the App.
type OverlayResult struct {
	// Selected is the page to open when Accept is true. Empty means "accept
	// but navigate nowhere" (e.g. a search hit whose file isn't indexed).
	Selected string
	// TaskOrdinal is the 0-based open-todo index on the selected page to
	// scroll to, honoured only when DeepLink is true. Only the Todos overlay
	// sets these; other overlays leave DeepLink false and the page opens at
	// the top.
	TaskOrdinal int
	DeepLink    bool
	Accept      bool    // user chose Selected; App navigates (if non-empty) and closes
	Cancel      bool    // user dismissed; App closes the overlay
	// Create, when Accept is true, means "create the page named Selected"
	// rather than open an existing one. Only the Picker sets it.
	Create bool
	// FocusLinkTo, when Accept is true and non-empty, asks the App to position
	// the destination page's link cursor on the first link back to this page
	// (so a backlink jump lands on — and highlights — the referencing link).
	// Only the Backlinks overlay sets it, and only for linked refs.
	FocusLinkTo string
	// HighlightText, when Accept is true and non-empty, asks the App to navigate
	// to Selected and highlight occurrences of this term on the destination
	// (scrolling to the first). Only the Backlinks overlay sets it, for unlinked
	// refs.
	HighlightText string
	// Linkify, when non-nil, asks the App to wrap LinkifyTarget as a [[link]]
	// at this unlinked reference's location in its source file, then reindex
	// and refresh the panel. Only the Backlinks overlay sets it.
	Linkify *graph.UnlinkedRef
	// LinkifyTarget is the page name to wrap, honoured only when Linkify is set.
	LinkifyTarget string
	Cmd           tea.Cmd // optional async work to run (search launches rg here)
}

// Overlay is a modal view layered over the page. App routes keys to the active
// overlay and closes it when Update reports Accept or Cancel.
type Overlay interface {
	Update(key string) OverlayResult
	View() string
	SetSize(w, h int)
}
