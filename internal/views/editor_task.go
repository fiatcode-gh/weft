package views

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// datePrompt is the one-row prompt behind alt+s (SCHEDULED) and alt+e (DEADLINE).
type datePrompt struct {
	kind                   graph.StampKind
	input, current, errMsg string
}

const datePromptHint = "YYYY-MM-DD · today · tomorrow · +3d · +2w · mon…sun · empty clears"

// promptRows is the number of rows the date prompt takes from the text window.
func (e *EditorView) promptRows() int {
	if e.prompt == nil {
		return 0
	}
	return 1
}

// openDatePrompt opens the prompt for the task under the cursor; elsewhere it
// only says so. The completion strip is dismissed.
func (e *EditorView) openDatePrompt(kind graph.StampKind) {
	e.buf.Break()
	date, ok := e.buf.TaskStamp(kind)
	if !ok {
		e.notice = "not on a task"
		return
	}
	e.completer.dismiss()
	e.prompt = &datePrompt{kind: kind, current: date}
	e.ensureVisible()
}

// updatePrompt handles one key while the prompt is open; it owns every key.
func (e *EditorView) updatePrompt(msg tea.KeyPressMsg) {
	p := e.prompt
	p.errMsg = ""
	switch msg.String() {
	case keyEsc:
		e.prompt = nil
		e.ensureVisible()
	case keyEnter:
		e.applyPrompt()
	case keyBackspace, "ctrl+h":
		p.input = dropLastGrapheme(p.input)
	case "ctrl+u":
		p.input = ""
	default:
		if t := msg.Text; t != "" && !strings.ContainsFunc(t, unicode.IsControl) {
			p.input += t
		}
	}
}

// applyPrompt sets (or, with empty input, clears) the stamp. An invalid date
// keeps the prompt open.
func (e *EditorView) applyPrompt() {
	p := e.prompt
	date := ""
	if strings.TrimSpace(p.input) != "" {
		d, ok := graph.ParseDateInput(p.input, e.now())
		if !ok {
			p.errMsg = "not a date: " + p.input
			return
		}
		date = d
	}
	changed, _ := e.buf.SetTaskStamp(p.kind, date)
	if !changed && date == "" {
		e.notice = "no scheduled date to clear"
		if p.kind == graph.StampDeadline {
			e.notice = "no deadline to clear"
		}
	}
	e.prompt = nil
	e.goalOK = false
	e.afterKey(changed)
	e.ensureVisible()
}

// promptView renders the prompt's row (without the left margin) and the cell
// column of the terminal cursor within it. avail is the room in cells.
func (p *datePrompt) promptView(avail int) (string, int) {
	label := "Scheduled: "
	if p.kind == graph.StampDeadline {
		label = "Deadline: "
	}
	hint := datePromptHint
	if p.current != "" {
		hint = "now " + p.current + " · " + hint
	}
	lw := ansi.StringWidth(label)
	input := render.DisplayText(p.input)
	input = tailFit(input, max(2, avail-lw-1))
	x := lw + ansi.StringWidth(input)
	room := avail - x - 2
	var b strings.Builder
	b.WriteString(styleTitle.Render(label))
	b.WriteString(input)
	if room > 1 {
		b.WriteString(styleFaint.Render("  " + ansi.Truncate(hint, room, "…")))
	}
	return b.String(), x
}
