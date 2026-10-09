package views

import (
	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

type overlayResultKind uint8

const (
	overlayResultNone overlayResultKind = iota
	overlayResultCancel
	overlayResultCommand
	overlayResultOpen
	overlayResultCreate
	overlayResultOpenTask
	overlayResultFocusLink
	overlayResultHighlight
	overlayResultLinkify
	overlayResultMarkTask
	overlayResultCapture
)

// OverlayResult is the tagged outcome that an overlay's Update reports to the
// App. Its zero value is a no-action result.
type OverlayResult struct {
	kind        overlayResultKind
	page        string
	taskOrdinal int
	target      string
	ref         *graph.UnlinkedRef
	cmd         tea.Cmd
	mark        taskMark
}

func overlayCancel() OverlayResult { return OverlayResult{kind: overlayResultCancel} }

// overlayCapture asks the App to open the capture prompt over the overlay.
func overlayCapture() OverlayResult { return OverlayResult{kind: overlayResultCapture} }

func overlayCommand(cmd tea.Cmd) OverlayResult {
	return OverlayResult{kind: overlayResultCommand, cmd: cmd}
}

func overlayOpen(name string) OverlayResult {
	return OverlayResult{kind: overlayResultOpen, page: name}
}

func overlayCreate(name string) OverlayResult {
	return OverlayResult{kind: overlayResultCreate, page: name}
}

func overlayOpenTask(name string, ordinal int) OverlayResult {
	return OverlayResult{kind: overlayResultOpenTask, page: name, taskOrdinal: ordinal}
}

func overlayFocusLink(name, target string) OverlayResult {
	return OverlayResult{kind: overlayResultFocusLink, page: name, target: target}
}

func overlayHighlight(name, text string) OverlayResult {
	return OverlayResult{kind: overlayResultHighlight, page: name, target: text}
}

func overlayLinkify(ref *graph.UnlinkedRef, target string) OverlayResult {
	return OverlayResult{kind: overlayResultLinkify, ref: ref, target: target}
}

func overlayMarkTask(m taskMark) OverlayResult {
	return OverlayResult{kind: overlayResultMarkTask, mark: m}
}

// Overlay is a modal view layered over the page. App routes keys to the active
// overlay and handles the tagged outcome reported by Update; only closing or
// navigation outcomes clear the overlay.
type Overlay interface {
	Update(key string) OverlayResult
	View() string
	SetSize(w, h int)
}
