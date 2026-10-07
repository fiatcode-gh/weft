package views

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// clashPrompt is the state behind the confirmingClash mode.
type clashPrompt struct {
	theirs     edit.Snapshot
	reloadable bool // theirs exists and loads into the buffer unchanged
	exitAfter  bool // the prompt came from save-and-exit
}

type editorMode int

const (
	editing editorMode = iota
	confirmingExit
	confirmingClash
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
	path          string        // target file (may not exist yet)
	pageName      string        // logical page name, for the status line
	baseline      string        // content as last loaded/saved
	disk          edit.Snapshot // the file as weft last read or wrote it: the base for the save guard
	saved         bool          // at least one successful save this session
	mode          editorMode
	errMsg        string // non-empty while a save error is pending display
	notice        string // transient status line text: the merged-save notice
	clash         clashPrompt
	width, height int
	completer     *linkCompleter
	loadDiverged  bool // priming the textarea altered content — see LoadDiverged
}

// EditorResult is what EditorView.Update reports to the App.
type EditorResult struct {
	Save      bool // App runs the guarded save (saveEditor): plain write, merge, or clash prompt
	Overwrite bool // App writes Content() over the clash snapshot (prompt "o")
	Exit      bool // App tears down the editor and returns to the read view
}

// NewEditorView builds an editor for page `name` targeting `path`, primed
// with `content` (empty for a not-yet-created page). isNew seeds disk.Exists:
// whether the file existed at open time. idx is the graph index used for link
// completion; pass nil to disable completion (e.g. in tests that don't
// exercise it). anchorLine is the 0-based line of `content` to open the
// cursor on, clamped into the buffer.
func NewEditorView(idx *graph.Index, name, path, content string, isNew bool, width, height, anchorLine int) *EditorView {
	ta := newEditorTextarea()
	ta.SetValue(content)
	e := &EditorView{
		ta:        ta,
		path:      path,
		pageName:  name,
		baseline:  content,
		disk:      edit.Snapshot{Content: content, Exists: !isNew},
		width:     width,
		height:    height,
		completer: newLinkCompleter(idx),
	}
	e.SetSize(width, height)
	_ = e.ta.Focus() // blink cmd not needed here; the App calls Focus() again when it mounts the editor
	// SetValue leaves the cursor at the end of the buffer. Move it to the
	// anchor line, column 0. Clamped, so a stale anchor (file changed since the read view rendered) degrades instead of panicking.
	anchorLine = clampInt(anchorLine, 0, e.ta.LineCount()-1)
	e.moveCursorTo(anchorLine, 0)
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
	e.loadDiverged = e.Content() != normalizeContent(content)
	return e
}

// newEditorTextarea is the textarea configuration shared by the live editor
// and loadsFaithfully, so the probe sanitizes exactly as the editor does.
func newEditorTextarea() textarea.Model {
	ta := textarea.New()
	ta.CharLimit = 0 // no length cap
	ta.MaxHeight = 0 // the default 99 blocks Enter once the buffer reaches 99 lines (a separate hard 10000-line insert cap remains — see loadDiverged)
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	// The empty prompt is still rendered through the prompt STYLE, which by
	// default carries a foreground color — emitting an ANSI escape at the start
	// of every row. tintView treats any row containing an escape as the cursor
	// row and leaves it untinted, so a styled empty prompt would suppress all
	// tinting. Neutralize the prompt style so non-cursor rows stay escape-free.
	styles := textarea.DefaultDarkStyles()
	styles.Focused.Prompt = lipgloss.NewStyle()
	styles.Blurred.Prompt = lipgloss.NewStyle()
	styles.Cursor.Color = nil // plain reverse-video cursor, as in Bubbles v1
	ta.SetStyles(styles)
	ta.KeyMap = editorKeyMap()
	return ta
}

// editorKeyMap is the Bubbles v2 textarea keymap cut back to the v1
// bindings: v2's selection, select-all, copy, ctrl+arrow word moves,
// ctrl+backspace/delete and its own paging stay off until editor-core.
func editorKeyMap() textarea.KeyMap {
	km := textarea.DefaultKeyMap()
	km.WordForward.SetKeys("alt+right", "alt+f")
	km.WordBackward.SetKeys("alt+left", "alt+b")
	km.DeleteWordBackward.SetKeys("alt+backspace", "ctrl+w")
	km.DeleteWordForward.SetKeys("alt+delete", "alt+d")
	km.PageUp.SetEnabled(false)
	km.PageDown.SetEnabled(false)
	km.SelectCharacterForward.SetEnabled(false)
	km.SelectCharacterBackward.SetEnabled(false)
	km.SelectWordForward.SetEnabled(false)
	km.SelectWordBackward.SetEnabled(false)
	km.SelectLineUp.SetEnabled(false)
	km.SelectLineDown.SetEnabled(false)
	km.SelectAll.SetEnabled(false)
	km.CopySelection.SetEnabled(false)
	return km
}

// Paste inserts bracketed-paste text while editing. The clash and exit
// prompts ignore it, as they ignored a pasted key under Bubble Tea v1.
func (e *EditorView) Paste(msg tea.PasteMsg) tea.Cmd {
	e.notice = ""
	if e.mode != editing {
		return nil
	}
	return e.forward(msg)
}

// forward hands msg to the textarea and refreshes completion; only a
// message that changed the buffer may open the strip.
func (e *EditorView) forward(msg tea.Msg) tea.Cmd {
	prev := e.ta.Value()
	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(msg)
	e.refreshCompleter(e.ta.Value() != prev)
	return cmd
}

// moveCursorTo puts the cursor on logical line row (clamped) at column col
// (clamped) in one pass over the buffer. Bubbles v2 repositions the viewport
// on every CursorUp/CursorDown at a cost that grows with the cursor's row, so
// walking the cursor line by line is quadratic in the buffer length. Instead
// the buffer is rebuilt from row down, with the lines above it inserted at
// the top: InsertString leaves the cursor at the start of row. The text is
// the textarea's own (already sanitized and within the line cap), so the
// rebuild leaves the content unchanged. SetValue resets the viewport to the
// top; callers sync it.
func (e *EditorView) moveCursorTo(row, col int) {
	lines := strings.Split(e.ta.Value(), "\n")
	row = clampInt(row, 0, len(lines)-1)
	e.ta.SetValue(strings.Join(lines[row:], "\n"))
	e.ta.MoveToBegin()
	if row > 0 {
		e.ta.InsertString(strings.Join(lines[:row], "\n") + "\n")
	}
	e.ta.SetCursorColumn(col)
}

// textWidth is the width the textarea is laid out at: the editor width less
// the left inset.
func (e *EditorView) textWidth() int { return max(1, e.width-editorInset) }

// scrollAnchorToTop leaves the cursor at column 0 of anchorLine with that
// line's first visual row at the top of the textarea window (or as high as
// the buffer's end allows). The textarea only ever scrolls minimally to keep
// the cursor visible, so left alone the anchor would park on the bottom row.
// A probe textarea holding just the window's lines, at the same width, finds
// how far down the cursor can park (a window's height of visual rows, or the
// buffer's end). The real cursor goes there and the viewport syncs once; then
// PageUp snaps the cursor to the window's top row in one move, or, when the
// buffer ends inside the window, a few CursorUps walk back to the anchor.
func (e *EditorView) scrollAnchorToTop(anchorLine int) {
	h := e.ta.Height()
	lines := strings.Split(e.ta.Value(), "\n")
	probe := newEditorTextarea()
	probe.SetWidth(e.textWidth())
	probe.SetValue(strings.Join(lines[anchorLine:min(len(lines), anchorLine+h)], "\n"))
	probe.MoveToBegin()
	steps := 0
	for range h - 1 {
		line, row := probe.Line(), probe.LineInfo().RowOffset
		probe.CursorDown()
		if probe.Line() == line && probe.LineInfo().RowOffset == row {
			break // end of buffer: no further visual row to move to
		}
		steps++
	}
	e.moveCursorTo(anchorLine+probe.Line(), probe.Column())
	e.syncViewport()
	if steps > 0 && steps == h-1 {
		// The anchor's first row is now the window's top row: snap to it in one move.
		// (A one-row window has no rows to climb: the cursor is on the top row
		// already, and PageUp from there would move a whole page up.)
		e.ta.PageUp()
	} else {
		// The buffer ends less than a window below the anchor (or the window is
		// one row): at most h-2 moves.
		for e.ta.Line() > anchorLine {
			e.ta.CursorUp()
		}
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
			e.ta, _ = e.ta.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
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
		e.ta, _ = e.ta.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	e.ta.InsertString(newText)
	e.ta.SetCursorColumn(newCol)
	e.syncViewport()
}

// syncViewport repositions the textarea viewport onto the cursor after an
// intercept mutates the buffer without routing a message through ta.Update.
// InsertString and SetCursorColumn never reposition; only Update, SetHeight and
// cursor moves do. repositionMsg is the content-neutral message layout()
// already uses for the same purpose.
func (e *EditorView) syncViewport() { e.ta, _ = e.ta.Update(repositionMsg{}) }

// repositionMsg is a content-neutral message handed to the textarea purely to
// trigger its viewport reposition. The textarea ignores message types it
// doesn't recognize, so sending this changes no buffer state; it just
// re-centers the viewport on the cursor.
type repositionMsg struct{}

// layout sizes the textarea, reserving one row for the status line plus the
// completion strip's rows while it is active.
func (e *EditorView) layout() {
	e.ta.SetWidth(e.textWidth())
	// Cap the strip so it can't push the textarea/status off a short terminal:
	// reserve the box chrome (4 rows), the status line (1), and ≥1 textarea row.
	e.completer.maxVisible = clampInt(e.height-6-editorTopMargin, 1, maxCompleterRows)
	h := e.height - 1 - editorTopMargin - e.completer.rows()
	e.ta.SetHeight(max(1, h))
	// Bubbles v2's SetWidth does not reposition the viewport, and the
	// viewport still holds the old width's wrapped content, which SetHeight's
	// own reposition clamps against. After a resize that wraps more lines
	// the cursor line can sit below the window until the next keystroke.
	// Poke Update with a content-neutral message to render at the new width
	// and reposition; it's a no-op when the cursor is already visible
	// (TestEditorResizeNarrowKeepsCursorVisible).
	e.ta, _ = e.ta.Update(repositionMsg{})
}

// Content is the buffer normalized to end in exactly one newline.
func (e *EditorView) Content() string {
	return normalizeContent(e.ta.Value())
}

// normalizeContent makes s end in exactly one newline.
func normalizeContent(s string) string {
	return strings.TrimRight(s, "\n") + "\n"
}

// loadsFaithfully reports whether the editor's textarea holds s unaltered
// (modulo trailing newlines): no tabs, CRLF, invalid UTF-8 or over-long text.
func loadsFaithfully(s string) bool {
	ta := newEditorTextarea()
	ta.SetValue(s)
	return normalizeContent(ta.Value()) == normalizeContent(s)
}

// LoadDiverged reports whether priming the textarea altered the loaded
// content; a diverged buffer must never be written back over the file.
func (e *EditorView) LoadDiverged() bool { return e.loadDiverged }

// MarkSaved records a successful save: the given content becomes the new
// clean baseline and the file now exists.
func (e *EditorView) MarkSaved(content string) {
	e.baseline = content
	e.disk = edit.Snapshot{Content: content, Exists: true}
	e.saved = true
	e.errMsg = ""
}

// replaceBuffer swaps the whole buffer for content and puts the cursor on
// logical line `line` (clamped by the textarea) at absolute column col. It is
// the shared buffer-swap for merge-on-save and reload; it does not touch
// baseline or disk.
func (e *EditorView) replaceBuffer(content string, line, col int) {
	e.ta.SetValue(content)
	e.moveCursorTo(line, col)
	e.syncViewport()
	e.refreshCompleter(false)
}

// applyMerge installs a merged text that was already written to disk: the
// cursor follows its mine-line through the merge, and the merged text becomes
// the saved baseline.
func (e *EditorView) applyMerge(text string, mineLine []int) {
	row := clampInt(e.ta.Line(), 0, len(mineLine)-1)
	_, col := e.cursorRowCol()
	e.replaceBuffer(text, mineLine[row], col)
	e.MarkSaved(text)
	e.notice = mergedNotice
}

// cursorRowCol returns the cursor's buffer row and absolute column.
func (e *EditorView) cursorRowCol() (row, col int) {
	li := e.ta.LineInfo()
	return e.ta.Line(), li.StartColumn + li.ColumnOffset
}

// showClash opens the clash prompt for a disk state that cannot be merged.
func (e *EditorView) showClash(theirs edit.Snapshot, exitAfter bool) {
	e.clash = clashPrompt{
		theirs:     theirs,
		reloadable: theirs.Exists && loadsFaithfully(theirs.Content),
		exitAfter:  exitAfter,
	}
	e.errMsg = "" // the clash prompt supersedes any earlier save error
	e.mode = confirmingClash
	e.completer.dismiss() // the prompt owns the keys
	e.layout()
}

func (e *EditorView) View() string {
	v := tintView(e.ta.View())
	if strip := e.completer.View(e.textWidth()); strip != "" {
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
func (e *EditorView) Update(msg tea.KeyPressMsg) (EditorResult, tea.Cmd) {
	e.notice = ""
	if e.mode == confirmingClash {
		switch msg.String() {
		case "o":
			e.mode = editing
			return EditorResult{Overwrite: true, Exit: e.clash.exitAfter}, nil
		case "r":
			if e.clash.reloadable {
				row, col := e.cursorRowCol()
				e.replaceBuffer(e.clash.theirs.Content, row, col)
				e.baseline = e.Content()
				e.disk = e.clash.theirs
				e.errMsg = ""
				e.mode = editing
			}
		case "k", keyEsc, "ctrl+c":
			e.mode = editing
		}
		return EditorResult{}, nil // ignore everything else
	}
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

	return EditorResult{}, e.forward(msg)
}

// scrollPage moves the cursor by one viewport-height of visual rows (h-1, at
// least one) with the textarea's own CursorDown/CursorUp. weft pages that way
// so paging moves exactly as before, not through the textarea's PageUp/PageDown.
// The moves are made directly, not as key messages: Bubbles v2 renders the
// whole buffer on every Update, which made one page cost h full renders. A
// direct move still repositions the viewport on its own, and View renders the
// buffer itself, so nothing needs syncing afterwards.
func (e *EditorView) scrollPage(dir int) {
	for range max(1, e.ta.Height()-1) {
		if dir < 0 {
			e.ta.CursorUp()
		} else {
			e.ta.CursorDown()
		}
	}
}

func (e *EditorView) statusLine() string {
	if e.mode == confirmingExit {
		return styleFaint.Render("Save changes?  ") +
			styleTitle.Render("[s]") + styleFaint.Render("ave · ") +
			styleTitle.Render("[d]") + styleFaint.Render("iscard · ") +
			styleTitle.Render("[c]") + styleFaint.Render("ancel")
	}
	if e.mode == confirmingClash {
		reason := "Changed on disk."
		switch {
		case !e.clash.theirs.Exists:
			reason = "Deleted on disk."
		case !e.clash.reloadable:
			reason = "Changed on disk (can't load it here)."
		}
		s := styleFaint.Render(reason) + "  " +
			styleTitle.Render("[o]") + styleFaint.Render("verwrite · ")
		if e.clash.reloadable {
			s += styleTitle.Render("[r]") + styleFaint.Render("eload · ")
		}
		return s + styleTitle.Render("[k]") + styleFaint.Render("eep editing")
	}
	mark := ""
	if e.dirty() {
		mark = " ●"
	}
	left := styleTitle.Render(e.pageName) + styleFaint.Render(" [edit]"+mark)
	right := styleFaint.Render("^S save · esc exit")
	if e.errMsg != "" {
		right = styleTitle.Render("save failed: " + e.errMsg)
	} else if e.notice != "" {
		right = styleTitle.Render(e.notice)
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
