package views

import (
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// vimExOpen opens the ex line, empty.
func (e *EditorView) vimExOpen() {
	s := ""
	e.vim.ex = &s
}

// vimExKey handles one key while the ex line is open: it owns every key.
func (e *EditorView) vimExKey(msg tea.KeyPressMsg) (EditorResult, tea.Cmd) {
	v := e.vim
	var res EditorResult
	var cmd tea.Cmd
	switch key := msg.String(); key {
	case keyEsc:
		v.ex = nil
	case keyEnter:
		text := *v.ex
		v.ex = nil
		res, cmd = e.vimExLine(text)
	case keyBackspace:
		if *v.ex == "" {
			v.ex = nil
		} else {
			*v.ex = dropLastGrapheme(*v.ex)
		}
	case "ctrl+u":
		*v.ex = ""
	default:
		*v.ex += fieldInput(msg.Text)
	}
	e.vimSettle()
	return res, cmd
}

// vimExLine runs one ex command. Saving and leaving are only results for the
// App: the save guard, the merge and the clash and unsaved prompts stay
// where they are.
func (e *EditorView) vimExLine(text string) (EditorResult, tea.Cmd) {
	text = strings.TrimSpace(text)
	switch text {
	case "w":
		e.buf.Break()
		return EditorResult{Save: true}, nil
	case "q":
		e.buf.Break()
		if e.dirty() {
			e.mode = confirmingExit
			return EditorResult{}, nil
		}
		return EditorResult{Exit: true}, nil
	case "wq":
		e.buf.Break()
		return EditorResult{Save: true, Exit: true}, nil
	case "x":
		e.buf.Break()
		return EditorResult{Save: e.dirty(), Exit: true}, nil
	case "q!":
		return EditorResult{Exit: true}, nil
	case "preview":
		e.toggleSource()
		return EditorResult{}, nil
	}
	if n, ok := vimExLineNumber(text); ok {
		b := e.buf
		b.MoveTo(b.FirstNonBlank(buffer.Pos{Line: clampInt(n-1, 0, b.LastLine())}), false)
		e.goalOK = false
		return EditorResult{}, nil
	}
	e.notice = "not an editor command: " + text
	return EditorResult{}, nil
}

// vimExLineNumber reads text as a line number; a run of digits too long for
// an int is the largest one.
func vimExLineNumber(text string) (int, bool) {
	if text == "" {
		return 0, false
	}
	for i := range len(text) {
		if text[i] < '0' || text[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return math.MaxInt, true
	}
	return n, true
}

// vimExStatus is the status line while the ex line is open.
func (e *EditorView) vimExStatus() string {
	return styleTitle.Render(":") + render.DisplayText(*e.vim.ex)
}

// vimExCursor is the terminal cursor while the ex line is open: after the
// text, on the status row.
func (e *EditorView) vimExCursor() *tea.Cursor {
	x := e.geo.Margin + 1 + ansi.StringWidth(render.DisplayText(*e.vim.ex))
	return tea.NewCursor(min(x, max(0, e.width-1)), e.height-1)
}
