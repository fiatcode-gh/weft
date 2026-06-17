package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
)

type editorMode int

const (
	editing editorMode = iota
	confirmingExit
)

// editorInset is the left margin (in columns) applied to the whole editor
// view so its text occupies the same horizontal box as the Glamour-rendered
// read view, whose standard-style document margin is 2 columns. Without this,
// switching from read to edit jumps the text flush-left.
const editorInset = 2

// editorTopMargin is the number of blank rows above the editor's text, matching
// the leading blank line Glamour emits at the top of the read view. Without it,
// the first line sits flush at row 0 and jumps up by a row on entering the editor.
const editorTopMargin = 1

// EditorView is weft's in-app raw-markdown editor: a full-screen mode
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
	completer     *linkCompleter
}

// EditorResult is what EditorView.Update reports to the App.
type EditorResult struct {
	Save bool // App writes Content() to path
	Exit bool // App tears down the editor and returns to the read view
}

// NewEditorView builds an editor for page `name` targeting `path`, primed
// with `content` (empty for a not-yet-created page). isNew records whether
// the file existed at open time. idx is the graph index used for link
// completion; pass nil to disable completion (e.g. in tests that don't
// exercise it).
func NewEditorView(idx *graph.Index, name, path, content string, isNew bool, width, height int) *EditorView {
	ta := textarea.New()
	ta.CharLimit = 0 // no length cap
	ta.MaxHeight = 0 // no line cap — pages can exceed textarea's default 99
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	// The empty prompt is still rendered through the prompt STYLE, which by
	// default carries a foreground color — emitting an ANSI escape at the start
	// of every row. tintView treats any row containing an escape as the cursor
	// row and leaves it untinted, so a styled empty prompt would suppress all
	// tinting. Neutralize the prompt style so non-cursor rows stay escape-free.
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.BlurredStyle.Prompt = lipgloss.NewStyle()
	ta.SetValue(content)
	e := &EditorView{
		ta:        ta,
		path:      path,
		pageName:  name,
		baseline:  content,
		isNew:     isNew,
		width:     width,
		height:    height,
		completer: newLinkCompleter(idx),
	}
	e.SetSize(width, height)
	_ = e.ta.Focus() // blink cmd not needed here; the App calls Focus() again when it mounts the editor
	e.baseline = e.Content() // normalize so open-time dirty() is accurate
	e.refreshCompleter(false) // opening a file must not pop the strip
	return e
}

// SetError records a message (e.g. a failed save) to show in the status
// line until the next successful save. Surfacing it here is load-bearing:
// while the editor is open, App.View renders EditorView.View(), not the
// app status bar, so a save error routed through the app hint is invisible.
func (e *EditorView) SetError(msg string) { e.errMsg = msg }

// Focus focuses the textarea and returns its (cursor-blink) command.
func (e *EditorView) Focus() tea.Cmd { return e.ta.Focus() }

// SetSize resizes the textarea, reserving rows for the status line and the
// active completion strip.
func (e *EditorView) SetSize(w, h int) {
	e.width, e.height = w, h
	e.layout()
}

// dirty reports whether the buffer differs from the last loaded/saved
// content. Both sides are compared in normalized form (Content()) so a
// buffer that lacks a trailing newline doesn't read as dirty right after
// a save, which writes the normalized form.
func (e *EditorView) dirty() bool { return e.Content() != e.baseline }

// cursorLineSplit returns the current logical row's text split at the cursor.
// The textarea exposes no rune-offset getter, but Line() gives the row and
// LineInfo().StartColumn+ColumnOffset reconstructs the absolute rune column.
func (e *EditorView) cursorLineSplit() (before, after string) {
	lines := strings.Split(e.ta.Value(), "\n")
	row := e.ta.Line()
	if row < 0 || row >= len(lines) {
		return "", ""
	}
	li := e.ta.LineInfo()
	col := li.StartColumn + li.ColumnOffset
	runes := []rune(lines[row])
	if col > len(runes) {
		col = len(runes)
	}
	return string(runes[:col]), string(runes[col:])
}

// refreshCompleter re-derives completion state from the cursor position and
// re-lays-out the view so the strip fits. allowOpen reports whether the key
// that triggered this refresh edited the buffer; only an edit may open a closed
// strip (see linkCompleter.refresh).
func (e *EditorView) refreshCompleter(allowOpen bool) {
	before, after := e.cursorLineSplit()
	e.completer.refresh(before, after, allowOpen)
	e.layout()
}

// acceptCompletion splices the selected candidate into the buffer. For an
// existing page it deletes the typed partial (via backspaces, so the
// textarea's own cursor tracking stays correct) and inserts "Name]]". For the
// create row it keeps the typed name and only closes the link with "]]".
func (e *EditorView) acceptCompletion() {
	cand, ok := e.completer.selected()
	if !ok {
		return
	}
	if cand.create {
		e.ta.InsertString("]]")
	} else {
		for range []rune(e.completer.partial) {
			e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		}
		e.ta.InsertString(cand.name + "]]")
	}
	e.refreshCompleter(false) // accepting inserts "]]" which closes the link
}

// repositionMsg is a content-neutral message handed to the textarea purely to
// trigger its viewport reposition. The textarea repositions the viewport only
// inside Update (never on SetHeight), and it ignores message types it doesn't
// recognize — so sending this changes no buffer state, it just re-centers the
// viewport on the cursor after a resize.
type repositionMsg struct{}

// layout sizes the textarea, reserving one row for the status line plus the
// completion strip's rows while it is active.
func (e *EditorView) layout() {
	e.ta.SetWidth(max(1, e.width-editorInset))
	// Cap the strip so it can't push the textarea/status off a short terminal:
	// reserve the box chrome (4 rows), the status line (1), and ≥1 textarea row.
	e.completer.maxVisible = clampInt(e.height-6-editorTopMargin, 1, maxCompleterRows)
	h := e.height - 1 - editorTopMargin - e.completer.rows()
	e.ta.SetHeight(max(1, h))
	if e.completer.active {
		// SetHeight just shrank the textarea to make room for the strip, but it
		// doesn't reposition the viewport — so the line being edited can sit
		// below the new bottom edge, hidden behind the strip. Poke Update to
		// reposition the cursor back into view. Gated on active so the normal
		// editing/open-a-page viewport behaviour is untouched.
		e.ta, _ = e.ta.Update(repositionMsg{})
	}
}

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
	v := tintView(e.ta.View())
	if strip := e.completer.View(max(1, e.width-editorInset)); strip != "" {
		v += "\n" + strip
	}
	v += "\n" + e.statusLine()
	// A leading blank row matches the read view's top margin (see editorTopMargin).
	return strings.Repeat("\n", editorTopMargin) + indentBlock(v, editorInset)
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

	if e.completer.active {
		switch msg.String() {
		case keyUp:
			e.completer.moveUp()
			return EditorResult{}, nil
		case keyDown:
			e.completer.moveDown()
			return EditorResult{}, nil
		case keyEnter, "tab":
			e.acceptCompletion()
			return EditorResult{}, nil
		case keyEsc, "ctrl+c":
			e.completer.dismiss()
			e.layout()
			return EditorResult{}, nil
		}
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
		e.refreshCompleter(false) // scrolling is navigation, not an edit
		return EditorResult{}, nil
	case "pgdown":
		e.scrollPage(+1)
		e.refreshCompleter(false) // scrolling is navigation, not an edit
		return EditorResult{}, nil
	}

	var cmd tea.Cmd
	prev := e.ta.Value()
	e.ta, cmd = e.ta.Update(msg)
	// allowOpen only when the key actually edited the buffer — a bare caret move
	// must not pop the completion strip open (it stays a typing affordance).
	e.refreshCompleter(e.ta.Value() != prev)
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

// indentBlock prefixes every line of s with n spaces. Blank lines are left
// empty so the inset never adds trailing whitespace to padding rows.
func indentBlock(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if ln != "" {
			lines[i] = pad + ln
		}
	}
	return strings.Join(lines, "\n")
}
