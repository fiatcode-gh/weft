package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

type editorMode int

const (
	editing editorMode = iota
	confirmingExit
)

// editorInset is the left margin (in columns) applied to the whole editor
// view so its text occupies the same horizontal box as the Glamour-rendered
// read view, whose document margin is 2 columns. Without this,
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
	loadDiverged  bool // priming the textarea altered content — see LoadDiverged
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
// exercise it). anchorLine is the 0-based line of `content` to open the
// cursor on, clamped into the buffer.
func NewEditorView(idx *graph.Index, name, path, content string, isNew bool, width, height, anchorLine int) *EditorView {
	ta := textarea.New()
	ta.CharLimit = 0 // no length cap
	ta.MaxHeight = 0 // lift textarea's default 99-line height cap (a separate hard 10000-line insert cap remains — see loadDiverged)
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
	// SetValue leaves the cursor at the end of the buffer. Walk it up to the
	// anchor line — clamped, so a stale anchor (file changed since the read
	// view rendered) degrades instead of panicking — and to column 0.
	anchorLine = clampInt(anchorLine, 0, e.ta.LineCount()-1)
	for e.ta.Line() > anchorLine {
		e.ta.CursorUp()
	}
	e.ta.CursorStart()
	if anchorLine > 0 {
		e.scrollAnchorToTop(anchorLine)
	}
	e.baseline = e.Content()  // normalize so open-time dirty() is accurate
	e.refreshCompleter(false) // opening a file must not pop the strip
	// The textarea's input sanitizer can silently alter content on load —
	// CRLF becomes doubled newlines, tabs become spaces, invalid UTF-8 is
	// dropped, and a hard 10000-line cap truncates. baseline was captured
	// post-mutation, so dirty() can't warn; saving would corrupt the file.
	// Record the divergence so the App can refuse in-app editing.
	e.loadDiverged = e.Content() != strings.TrimRight(content, "\n")+"\n"
	return e
}

// scrollAnchorToTop leaves the cursor at column 0 of anchorLine with that
// line's first visual row at the top of the textarea window (or as high as
// the buffer's end allows). The textarea only ever scrolls minimally to keep
// the cursor visible, so left alone the anchor would park on the bottom row.
// Parking the cursor a window's height further down first, syncing the
// viewport, then walking back up gives the same result as a top-aligned
// scroll. The textarea's viewport also has no lines until its first View(),
// hence the render before the sync.
func (e *EditorView) scrollAnchorToTop(anchorLine int) {
	for range e.ta.Height() - 1 {
		line, row := e.ta.Line(), e.ta.LineInfo().RowOffset
		e.ta.CursorDown()
		if e.ta.Line() == line && e.ta.LineInfo().RowOffset == row {
			break // end of buffer: no further visual row to move to
		}
	}
	_ = e.ta.View()
	e.syncViewport()
	for e.ta.Line() > anchorLine {
		e.ta.CursorUp()
	}
	e.ta.CursorStart()
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

// replaceCurrentLine rewrites the logical line the cursor is on: it deletes the
// line's existing runes (backward from line end, exactly its rune length so the
// trailing newline is untouched), inserts newText, then places the cursor at
// column newCol. Used for empty-bullet termination and marker cycling.
func (e *EditorView) replaceCurrentLine(newText string, newCol int) {
	before, after := e.cursorLineSplit()
	oldLen := len([]rune(before + after))
	e.ta.CursorEnd()
	for i := 0; i < oldLen; i++ {
		e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	e.ta.InsertString(newText)
	e.ta.SetCursor(newCol)
	e.syncViewport()
}

// syncViewport repositions the textarea viewport onto the cursor after an
// intercept mutates the buffer without routing a message through ta.Update
// (the only place the textarea calls repositionView). repositionMsg is the
// content-neutral message layout() already uses for the same purpose.
func (e *EditorView) syncViewport() { e.ta, _ = e.ta.Update(repositionMsg{}) }

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
	// SetHeight/SetWidth never reposition the textarea viewport (bubbles
	// quirk: repositioning happens only inside Update), so after ANY resize —
	// whether or not the completion strip is involved — the cursor line can
	// sit outside the visible window until the next keystroke. Poke Update
	// with a content-neutral message to force a reposition; it's a no-op
	// when the cursor is already visible.
	e.ta, _ = e.ta.Update(repositionMsg{})
}

// Content is the buffer normalized to end in exactly one newline.
func (e *EditorView) Content() string {
	return strings.TrimRight(e.ta.Value(), "\n") + "\n"
}

// LoadDiverged reports whether priming the textarea altered the loaded
// content; a diverged buffer must never be written back over the file.
func (e *EditorView) LoadDiverged() bool { return e.loadDiverged }

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
// pgup/pgdown, the markdown helpers enter/ctrl+t/tab/shift+tab, and the
// completion-strip keys while it is open) is forwarded to the textarea. The
// returned tea.Cmd is the textarea's own (cursor blink) command, which the
// App must propagate.
func (e *EditorView) Update(msg tea.KeyMsg) (EditorResult, tea.Cmd) {
	if e.mode == confirmingExit {
		switch msg.String() {
		case "s":
			// Reset to editing before returning: if the App's write fails it
			// keeps the editor open and calls SetError, and statusLine only
			// renders errMsg in editing mode (it shows the confirm prompt in
			// confirmingExit). On success the App tears the editor down, so
			// this reset is harmless.
			e.mode = editing
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
	case "enter":
		before, after := e.cursorLineSplit()
		line := before + after
		if isEmptyBullet(line) {
			e.replaceCurrentLine("", 0)
			e.refreshCompleter(false)
			return EditorResult{}, nil
		}
		if prefix, ok := bulletPrefix(line); ok {
			e.ta.InsertString("\n" + prefix)
			e.syncViewport()
			e.refreshCompleter(false)
			return EditorResult{}, nil
		}
		// Non-bullet: do not return — fall past the switch so the textarea
		// inserts a normal newline below.
	case "ctrl+t":
		before, after := e.cursorLineSplit()
		oldCol := len([]rune(before))
		if newLine, newCol, ok := cycleMarkerLine(before+after, oldCol); ok {
			e.replaceCurrentLine(newLine, newCol)
			e.refreshCompleter(false)
		}
		return EditorResult{}, nil
	case "tab":
		before, after := e.cursorLineSplit()
		oldCol := len([]rune(before))
		e.replaceCurrentLine(indentLine(before+after), oldCol+2)
		e.refreshCompleter(false)
		return EditorResult{}, nil
	case "shift+tab":
		before, after := e.cursorLineSplit()
		oldCol := len([]rune(before))
		newLine, removed := dedentLine(before + after)
		newCol := oldCol - removed
		if newCol < 0 {
			newCol = 0
		}
		e.replaceCurrentLine(newLine, newCol)
		e.refreshCompleter(false)
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
