package views

import (
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/graph"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// clashPrompt is the state behind the confirmingClash mode.
type clashPrompt struct {
	theirs     edit.Snapshot
	reloadable bool // theirs exists and can replace the buffer
	exitAfter  bool // the prompt came from save-and-exit
}

type editorMode int

const (
	editing editorMode = iota
	confirmingExit
	confirmingClash
)

// register is weft's copy register: what the last copy or cut put in it and
// whether it was taken as whole lines. The App owns one so it outlives editor
// sessions; an editor opened without one keeps a private register.
type register struct {
	text     string
	linewise bool
}

// EditorView is weft's in-app raw-markdown editor: a full-screen mode (not a
// centered overlay) that edits a buffer.Buffer and draws its source lines
// with the read view's styles. It owns the edit/save/exit state machine; the
// App performs the actual disk write so internal/edit stays the only writer.
type EditorView struct {
	buf           *buffer.Buffer
	reg           *register
	path          string        // target file (may not exist yet)
	pageName      string        // logical page name, for the status line
	baseline      string        // content as last loaded/saved, exactly
	disk          edit.Snapshot // the file as weft last read or wrote it: the base for the save guard
	saved         bool          // at least one successful save this session
	mode          editorMode
	errMsg        string // non-empty while a save error is pending display
	notice        string // transient status line text: the merged-save notice
	clash         clashPrompt
	width, height int
	completer     *linkCompleter
	find          *findBar         // nil while the find bar is closed
	lastQuery     string           // the last find query of this editor session
	prompt        *datePrompt      // nil while the date prompt is closed
	now           func() time.Time // the clock for relative dates

	theme    render.Theme
	geo      render.Geometry
	scanner  *render.Scanner
	painter  *render.Painter
	cache    map[int]*lineCache
	top      viewPos // the display row at screen row 0
	goalX    int     // remembered screen column for vertical moves
	goalOK   bool    // goalX is current; cleared by every non-vertical key
	dirtyVer int     // buffer version dirtyVal was computed at
	dirtyVal bool
	dirtyOK  bool
	stats    struct{ wrapped, painted int } // work counters for the perf tests

	source    bool               // draw the unit 3 source look instead of live preview
	preview   *render.Preview    // rendered rows of the lines away from the cursor
	shown     []lineSpan         // the lines drawn raw: the reveal set
	revealCur buffer.Pos         // the cursor at the last syncReveal
	previewed map[int]previewRow // preview rows by line, valid until the buffer changes
}

// EditorResult is what EditorView.Update reports to the App.
type EditorResult struct {
	Save      bool // App runs the guarded save (saveEditor): plain write, merge, or clash prompt
	Overwrite bool // App writes Content() over the clash snapshot (prompt "o")
	Exit      bool // App tears down the editor and returns to the read view
}

// NewEditorView builds an editor for page `name` targeting `path`, primed
// with `content`: the file as it is on disk, or for a new page (isNew) the
// text to start from, such as the journal template. isNew decides what disk
// holds: the zero Snapshot (no file) for a new page, else content. idx is
// the graph index used for link completion; pass nil to disable completion
// (e.g. in tests that don't exercise it). at says where the cursor opens: the
// row RowInLine of line at.Line (clamped into the buffer) is placed on screen
// row at.ScreenRow. reg is the App's copy register; nil gives the editor a
// private one. source draws the unit 3 source look; otherwise the lines away
// from the cursor are drawn as the read view draws them (live preview).
func NewEditorView(idx *graph.Index, name, path, content string, isNew bool, width, height int, at Anchor, reg *register, source bool) *EditorView {
	if reg == nil {
		reg = &register{}
	}
	disk := edit.Snapshot{}
	if !isNew {
		disk = edit.Snapshot{Content: content, Exists: true}
	}
	theme, _ := render.CurrentTheme() // on error: the notty theme it returned
	e := &EditorView{
		buf:       buffer.New(content),
		reg:       reg,
		path:      path,
		pageName:  name,
		baseline:  content,
		disk:      disk,
		completer: newLinkCompleter(idx),
		theme:     theme,
		scanner:   render.NewScanner(),
		painter:   render.NewPainter(theme),
		cache:     map[int]*lineCache{},
		source:    source,
		previewed: map[int]previewRow{},
		now:       time.Now,
	}
	e.width, e.height = width, height
	e.geo = render.NewGeometry(theme, width)
	e.preview = render.NewPreview(theme, width)
	e.completer.maxVisible = clampInt(height-7, 1, maxCompleterRows)
	e.place(at)
	e.refreshCompleter(false) // opening a file must not pop the strip
	return e
}

// Paste inserts bracketed-paste text while editing as one undo group: line
// breaks (CR, CRLF, LF) become the buffer's, other control characters are
// dropped. The clash and exit prompts ignore it.
func (e *EditorView) Paste(msg tea.PasteMsg) tea.Cmd {
	e.notice = ""
	if e.mode != editing {
		return nil
	}
	if e.prompt != nil {
		return nil
	}
	if e.find != nil {
		e.pasteFind(msg.Content)
		return nil
	}
	e.buf.Break()
	e.buf.Insert(cleanPaste(msg.Content))
	e.goalOK = false
	e.afterKey(true)
	return nil
}

// cleanPaste normalises pasted text: CRLF and CR become "\n", and every other
// C0 control character and DEL is dropped (tabs stay).
func cleanPaste(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || (!unicode.IsControl(r)) {
			return r
		}
		return -1
	}, s)
}

// SetError records a message (e.g. a failed save) to show in the status
// line until the next successful save. Surfacing it here is load-bearing:
// while the editor is open, App.View renders EditorView.View(), not the
// app status bar, so a save error routed through the app hint is invisible.
func (e *EditorView) SetError(msg string) { e.errMsg = msg }

// SetSize resizes the editor, reserving rows for the rule, the status line and
// the active completion strip.
func (e *EditorView) SetSize(w, h int) {
	if w != e.width {
		e.geo = render.NewGeometry(e.theme, w)
		e.preview = render.NewPreview(e.theme, w)
		clear(e.cache)
		clear(e.previewed)
	}
	e.width, e.height = w, h
	e.completer.maxVisible = clampInt(h-7, 1, maxCompleterRows)
	e.ensureVisible()
}

// dirty reports whether the buffer differs from the last loaded/saved
// content, byte for byte. The comparison is cached per buffer version.
func (e *EditorView) dirty() bool {
	if v := e.buf.Version(); !e.dirtyOK || v != e.dirtyVer {
		e.dirtyVer, e.dirtyVal, e.dirtyOK = v, !e.buf.EqualString(e.baseline), true
	}
	return e.dirtyVal
}

// cursorSplit returns the cursor line's text before and after the cursor.
func (e *EditorView) cursorSplit() (before, after string) {
	c := e.buf.Cursor()
	line := e.buf.Line(c.Line)
	return line[:c.Col], line[c.Col:]
}

// refreshCompleter re-derives completion state from the cursor position and
// re-lays-out the view so the strip fits. allowOpen reports whether the key
// that triggered this refresh edited the buffer; only an edit may open a closed
// strip (see linkCompleter.refresh).
func (e *EditorView) refreshCompleter(allowOpen bool) {
	if e.find != nil || e.prompt != nil { // the bar owns the keys; no strip over it
		e.ensureVisible()
		return
	}
	before, after := e.cursorSplit()
	tagOK := strings.IndexByte(before, '#') >= 0 && e.lineSubst()
	e.completer.refresh(before, after, allowOpen, tagOK)
	e.ensureVisible()
}

// acceptCompletion splices the selected candidate into the buffer as one
// edit. For an existing page it replaces the typed partial with "Name]]". For
// the create row it keeps the typed name and only closes the link with "]]".
// A tag completion replaces "#partial" with tagText(name), writing nothing
// when the text already matches.
func (e *EditorView) acceptCompletion() {
	cand, ok := e.completer.selected()
	if !ok {
		return
	}
	cur := e.buf.Cursor()
	switch {
	case e.completer.tag:
		start := buffer.Pos{Line: cur.Line, Col: cur.Col - len(e.completer.partial) - 1}
		text := tagText(cand.name)
		if e.buf.Line(cur.Line)[start.Col:cur.Col] != text {
			e.buf.ReplaceRange(buffer.Range{Start: start, End: cur}, text)
		}
		e.completer.dismiss()
	case cand.create:
		e.buf.ReplaceRange(buffer.Range{Start: cur, End: cur}, "]]")
	default:
		start := buffer.Pos{Line: cur.Line, Col: cur.Col - len(e.completer.partial)}
		e.buf.ReplaceRange(buffer.Range{Start: start, End: cur}, cand.name+"]]")
	}
	e.goalOK = false
	e.afterKey(false)
}

// lineSubst reports whether the read view substitutes the cursor line's links
// and tags (outside fences and hidden blocks).
func (e *EditorView) lineSubst() bool {
	e.syncBuffer()
	return e.scanner.Info(e.buf, e.buf.Cursor().Line).Subst
}

// afterKey brings the view up to date after a key changed the buffer or the
// cursor: the cursor stays on screen and completion follows it. edited
// reports whether the key changed the text; only an edit may open the strip.
func (e *EditorView) afterKey(edited bool) {
	e.syncReveal()
	e.refreshCompleter(edited)
}

// Content is the buffer's exact text.
func (e *EditorView) Content() string { return e.buf.String() }

// MarkSaved records a successful save: the given content becomes the new
// clean baseline and the file now exists.
func (e *EditorView) MarkSaved(content string) {
	e.baseline = content
	e.dirtyOK = false
	e.disk = edit.Snapshot{Content: content, Exists: true}
	e.saved = true
	e.errMsg = ""
}

// replaceBuffer swaps the whole buffer for content as one undo step and puts
// the cursor at (line, col), clamped. The cursor keeps its screen row. It is
// the shared buffer-swap for merge-on-save and reload; it does not touch
// baseline or disk.
func (e *EditorView) replaceBuffer(content string, line, col int) {
	sr := e.cursorScreenRow()
	e.buf.Break()
	e.buf.ReplaceAll(content)
	e.buf.MoveTo(buffer.Pos{Line: line, Col: col}, false)
	e.goalOK = false
	e.syncReveal()
	e.scrollCursorTo(sr)
	e.refreshCompleter(false)
}

// applyMerge installs a merged text that was already written to disk: the
// cursor follows its mine-line through the merge, and the merged text becomes
// the saved baseline.
func (e *EditorView) applyMerge(text string, mineLine []int) {
	cur := e.buf.Cursor()
	row := clampInt(cur.Line, 0, len(mineLine)-1)
	e.replaceBuffer(text, mineLine[row], cur.Col)
	e.MarkSaved(text)
	e.notice = mergedNotice
}

// showClash opens the clash prompt for a disk state that cannot be merged.
func (e *EditorView) showClash(theirs edit.Snapshot, exitAfter bool) {
	e.clash = clashPrompt{
		theirs:     theirs,
		reloadable: theirs.Exists,
		exitAfter:  exitAfter,
	}
	e.errMsg = "" // the clash prompt supersedes any earlier save error
	e.mode = confirmingClash
	e.completer.dismiss() // the prompt owns the keys
	e.ensureVisible()
}

// Update handles one key and reports whether the App should save/exit.
func (e *EditorView) Update(msg tea.KeyPressMsg) (EditorResult, tea.Cmd) {
	e.notice = ""
	if e.mode == confirmingClash {
		switch msg.String() {
		case "o":
			e.mode = editing
			return EditorResult{Overwrite: true, Exit: e.clash.exitAfter}, nil
		case "r":
			if e.clash.reloadable {
				cur := e.buf.Cursor()
				e.replaceBuffer(e.clash.theirs.Content, cur.Line, cur.Col)
				e.baseline = e.clash.theirs.Content
				e.dirtyOK = false
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

	key := msg.String()
	if key == "ctrl+r" {
		e.toggleSource()
		return EditorResult{}, nil
	}
	if e.prompt != nil {
		e.updatePrompt(msg)
		return EditorResult{}, nil
	}
	if e.find != nil {
		if res, handled := e.updateFind(msg); handled {
			return res, nil
		}
	}
	if e.completer.active {
		switch key {
		case keyUp:
			e.completer.moveUp()
			return EditorResult{}, nil
		case keyDown:
			e.completer.moveDown()
			return EditorResult{}, nil
		case keyEnter, "tab":
			e.buf.Break()
			e.acceptCompletion()
			return EditorResult{}, nil
		case keyEsc:
			e.completer.dismiss()
			e.ensureVisible()
			return EditorResult{}, nil
		}
	}

	switch key {
	case "ctrl+s":
		e.buf.Break()
		return EditorResult{Save: true}, nil
	case keyEsc:
		e.buf.Break()
		if e.dirty() {
			e.mode = confirmingExit
			return EditorResult{}, nil
		}
		return EditorResult{Exit: true}, nil
	case "ctrl+f":
		e.openFind()
		return EditorResult{}, nil
	case "alt+s":
		e.openDatePrompt(graph.StampScheduled)
		return EditorResult{}, nil
	case "alt+e":
		e.openDatePrompt(graph.StampDeadline)
		return EditorResult{}, nil
	}

	before := e.buf.Version()
	cmd := e.edit(key, msg.Text)
	e.afterKey(e.buf.Version() != before)
	e.findSync()
	return EditorResult{}, cmd
}

// toggleSource switches between live preview and the source look. The cursor's
// line stays on its screen row and nothing else about the session changes.
func (e *EditorView) toggleSource() {
	sr := e.cursorScreenRow()
	e.buf.Break()
	e.source = !e.source
	e.goalOK = false
	if !e.source {
		e.shown, e.revealCur = e.revealSpans(), e.buf.Cursor()
	}
	e.scrollCursorTo(sr)
}

// extendKeys maps each selection chord to the movement key it extends from.
var extendKeys = map[string]string{
	"shift+left":       "left",
	"shift+right":      "right",
	"shift+up":         keyUp,
	"shift+down":       keyDown,
	"shift+home":       "home",
	"shift+end":        "end",
	"shift+pgup":       "pgup",
	"shift+pgdown":     "pgdown",
	"alt+shift+left":   "alt+left",
	"alt+shift+right":  "alt+right",
	"alt+shift+b":      "alt+b",
	"alt+shift+f":      "alt+f",
	"ctrl+shift+left":  "ctrl+left",
	"ctrl+shift+right": "ctrl+right",
	"ctrl+shift+home":  "ctrl+home",
	"ctrl+shift+end":   "ctrl+end",
}

// edit applies one editing-mode key to the buffer and returns the command it
// needs run (the clipboard write of a copy or cut). Keys that are not typing,
// backspace or delete first close the undo group in progress; unbound chords
// change nothing.
func (e *EditorView) edit(key, text string) tea.Cmd {
	b := e.buf
	switch key {
	case keyBackspace, "ctrl+h":
		b.Backspace()
		e.goalOK = false
		return nil
	case "delete", "ctrl+d":
		b.Delete()
		e.goalOK = false
		return nil
	}
	if text != "" && !strings.ContainsFunc(text, unicode.IsControl) && key != keyEnter && key != "tab" {
		b.Type(text)
		e.goalOK = false
		return nil
	}
	b.Break()
	extend := false
	if base, ok := extendKeys[key]; ok {
		key, extend = base, true
	}
	cur := b.Cursor()
	horizontal := func(p buffer.Pos) { b.MoveTo(p, extend); e.goalOK = false }
	sel, hasSel := b.Selection()
	switch key {
	case keyEnter:
		b.Newline()
	case "left", "ctrl+b":
		if hasSel && !extend {
			horizontal(sel.Start)
		} else {
			horizontal(b.Left(cur))
		}
	case "right":
		if hasSel && !extend {
			horizontal(sel.End)
		} else {
			horizontal(b.Right(cur))
		}
	case keyUp, "ctrl+p":
		e.moveRows(-1, 1, extend)
		return nil
	case keyDown, "ctrl+n":
		e.moveRows(+1, 1, extend)
		return nil
	case "pgup":
		e.moveRows(-1, max(1, e.textHeight()-1), extend)
		return nil
	case "pgdown":
		e.moveRows(+1, max(1, e.textHeight()-1), extend)
		return nil
	case "home":
		horizontal(b.LineStart(cur))
	case "end", "ctrl+e":
		horizontal(b.LineEnd(cur))
	case "alt+left", "alt+b", "ctrl+left":
		horizontal(b.WordLeft(cur))
	case "alt+right", "alt+f", "ctrl+right":
		horizontal(b.WordRight(cur))
	case "ctrl+home", "alt+<":
		horizontal(buffer.Pos{})
	case "ctrl+end", "alt+>":
		horizontal(b.End())
	case "ctrl+a":
		b.SelectAll()
	case "ctrl+z":
		e.history(b.Undo(), "nothing to undo")
		return nil
	case "ctrl+y":
		e.history(b.Redo(), "nothing to redo")
		return nil
	case "ctrl+c":
		text, linewise := b.Copy()
		return e.toRegister(text, linewise, "copied")
	case "ctrl+x":
		text, linewise := b.Cut()
		e.goalOK = false
		return e.toRegister(text, linewise, "cut")
	case "ctrl+v":
		if e.reg.text == "" {
			e.notice = "nothing copied yet"
			return nil
		}
		b.Paste(e.reg.text, e.reg.linewise)
	case "alt+backspace", "ctrl+w":
		b.DeleteWordBackward()
	case "alt+delete", "alt+d":
		b.DeleteWordForward()
	case "ctrl+k":
		b.DeleteToLineEnd()
	case "ctrl+u":
		b.DeleteToLineStart()
	case "alt+c":
		b.CaseWord(buffer.CaseCapitalize)
	case "alt+l":
		b.CaseWord(buffer.CaseLower)
	case "alt+u":
		b.CaseWord(buffer.CaseUpper)
	case "ctrl+t":
		b.CycleMarker()
	case "alt+p":
		if !b.CyclePriority() {
			e.notice = "not on a task"
		}
	case "tab":
		b.Indent()
	case "shift+tab":
		b.Outdent()
	case "alt+up":
		b.MoveBlock(-1)
	case "alt+down":
		b.MoveBlock(+1)
	default:
		return nil // unbound chord: the group is closed, nothing else changes
	}
	e.goalOK = false
	return nil
}

// history finishes an undo or redo: a refusal becomes the status notice, and
// either way the cursor comes back on screen with a fresh goal column.
func (e *EditorView) history(done bool, refusal string) {
	if !done {
		e.notice = refusal
	}
	e.goalOK = false
	e.syncReveal()
	e.ensureVisible()
}

// toRegister stores a copy or cut in weft's register and hands the text to
// the terminal's clipboard (OSC 52).
func (e *EditorView) toRegister(text string, linewise bool, notice string) tea.Cmd {
	*e.reg = register{text: text, linewise: linewise}
	e.notice = notice
	return tea.SetClipboard(text)
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
	look, other := "preview", "source"
	if e.source {
		look, other = other, look
	}
	left := styleTitle.Render(e.pageName) + styleFaint.Render(" [edit · "+look+"]"+mark)
	right := styleFaint.Render("^S save · ^F find · ^R " + other + " · esc exit")
	if e.find != nil {
		right = styleFaint.Render(e.find.hints())
	}
	if p := e.prompt; p != nil {
		right = styleFaint.Render("↵ set · esc cancel")
		if p.errMsg != "" {
			right = styleTitle.Render(p.errMsg)
		}
	}
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
