package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type editorMode int

const (
	editing editorMode = iota
	confirmingExit
)

// EditorView is peekseq's in-app raw-markdown editor: a full-screen mode
// (not a centered overlay) that wraps bubbles/textarea. It owns the
// edit/save/exit state machine; the App performs the actual disk write so
// internal/edit stays the only writer.
type EditorView struct {
	ta            textarea.Model
	path          string // target file (may not exist yet)
	pageName      string // logical page name, for the status line
	baseline      string // content as last loaded/saved
	isNew         bool
	saved         bool // at least one successful save this session
	mode          editorMode
	errMsg        string // non-empty while a save error is pending display
	width, height int
}

// EditorResult is what EditorView.Update reports to the App.
type EditorResult struct {
	Save bool // App writes Content() to path
	Exit bool // App tears down the editor and returns to the read view
}

// NewEditorView builds an editor for page `name` targeting `path`, primed
// with `content` (empty for a not-yet-created page). isNew records whether
// the file existed at open time.
func NewEditorView(name, path, content string, isNew bool, width, height int) *EditorView {
	ta := textarea.New()
	ta.CharLimit = 0 // no length cap
	ta.MaxHeight = 0 // no line cap — pages can exceed textarea's default 99
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.SetValue(content)
	e := &EditorView{
		ta:       ta,
		path:     path,
		pageName: name,
		baseline: content,
		isNew:    isNew,
		width:    width,
		height:   height,
	}
	e.SetSize(width, height)
	_ = e.ta.Focus() // blink cmd not needed here; the App calls Focus() again when it mounts the editor
	e.baseline = e.Content() // normalize so open-time dirty() is accurate
	return e
}

// SetError records a message (e.g. a failed save) to show in the status
// line until the next successful save. Surfacing it here is load-bearing:
// while the editor is open, App.View renders EditorView.View(), not the
// app status bar, so a save error routed through the app hint is invisible.
func (e *EditorView) SetError(msg string) { e.errMsg = msg }

// Focus focuses the textarea and returns its (cursor-blink) command.
func (e *EditorView) Focus() tea.Cmd { return e.ta.Focus() }

// SetSize resizes the textarea, reserving one row for the status line.
func (e *EditorView) SetSize(w, h int) {
	e.width, e.height = w, h
	e.ta.SetWidth(w)
	e.ta.SetHeight(max(1, h-1))
}

// dirty reports whether the buffer differs from the last loaded/saved
// content. Both sides are compared in normalized form (Content()) so a
// buffer that lacks a trailing newline doesn't read as dirty right after
// a save, which writes the normalized form.
func (e *EditorView) dirty() bool { return e.Content() != e.baseline }

// Content is the buffer normalized to end in exactly one newline.
func (e *EditorView) Content() string {
	return strings.TrimRight(e.ta.Value(), "\n") + "\n"
}

// MarkSaved records a successful save: the given content becomes the new
// clean baseline and the file now exists.
func (e *EditorView) MarkSaved(content string) {
	e.baseline = content
	e.saved = true
	e.isNew = false
	e.errMsg = ""
}

func (e *EditorView) View() string {
	return e.ta.View() + "\n" + e.statusLine()
}

// Update handles one key and reports whether the App should save/exit.
// In editing mode every key except the intercepts (ctrl+s, esc/ctrl+c,
// pgup/pgdown) is forwarded to the textarea. The returned tea.Cmd is the
// textarea's own (cursor blink) command, which the App must propagate.
func (e *EditorView) Update(msg tea.KeyMsg) (EditorResult, tea.Cmd) {
	if e.mode == confirmingExit {
		switch msg.String() {
		case "s":
			return EditorResult{Save: true, Exit: true}, nil
		case "d":
			return EditorResult{Exit: true}, nil
		case "c", keyEsc, "ctrl+c":
			e.mode = editing
		}
		return EditorResult{}, nil // ignore everything else
	}

	switch msg.String() {
	case "ctrl+s":
		return EditorResult{Save: true}, nil
	case keyEsc, "ctrl+c":
		if e.dirty() {
			e.mode = confirmingExit
			return EditorResult{}, nil
		}
		return EditorResult{Exit: true}, nil
	case "pgup":
		e.scrollPage(-1)
		return EditorResult{}, nil
	case "pgdown":
		e.scrollPage(+1)
		return EditorResult{}, nil
	}

	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(msg)
	return EditorResult{}, cmd
}

// scrollPage moves the cursor by one viewport-height of lines by feeding the
// textarea that many up/down keys, reusing its built-in line navigation and
// viewport tracking. v1.0.0 textarea has no PageUp/PageDown of its own.
func (e *EditorView) scrollPage(dir int) {
	steps := max(1, e.ta.Height()-1)
	k := tea.KeyMsg{Type: tea.KeyDown}
	if dir < 0 {
		k = tea.KeyMsg{Type: tea.KeyUp}
	}
	for i := 0; i < steps; i++ {
		e.ta, _ = e.ta.Update(k)
	}
}

func (e *EditorView) statusLine() string {
	if e.mode == confirmingExit {
		return styleFaint.Render("Save changes?  ") +
			styleTitle.Render("[s]") + styleFaint.Render("ave · ") +
			styleTitle.Render("[d]") + styleFaint.Render("iscard · ") +
			styleTitle.Render("[c]") + styleFaint.Render("ancel")
	}
	mark := ""
	if e.dirty() {
		mark = " ●"
	}
	left := styleTitle.Render(e.pageName) + styleFaint.Render(" [edit]"+mark)
	right := styleFaint.Render("^S save · esc exit")
	if e.errMsg != "" {
		right = styleTitle.Render("save failed: " + e.errMsg)
	}
	return left + "  " + right
}
