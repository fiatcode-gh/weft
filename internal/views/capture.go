package views

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/graph"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// syncBusyMsg is the refusal shown when a write is attempted mid-sync.
const syncBusyMsg = "sync in progress — retry when it finishes"

const captureLegend = "enter add · tab TODO on/off · [[ link · # tag · esc cancel"

type captureAction uint8

const (
	captureNone captureAction = iota
	captureCancel
	captureSubmit
)

// CapturePrompt is the one-line prompt that appends a bullet to a journal.
// It is not an Overlay: App draws it over whatever is on screen and gives it
// every key while it is open.
type CapturePrompt struct {
	target        string // journal page name the line goes to, fixed at open
	input         string
	todo          bool
	errMsg        string
	completer     *linkCompleter
	width, height int
}

func newCapturePrompt(idx *graph.Index, target string, width, height int) *CapturePrompt {
	p := &CapturePrompt{target: target, completer: newLinkCompleter(idx)}
	p.SetSize(width, height)
	return p
}

// SetSize records the terminal size and caps the completion strip so the
// prompt keeps room above it.
func (p *CapturePrompt) SetSize(w, h int) {
	p.width, p.height = w, h
	p.completer.maxVisible = clampInt(h-10, 1, maxCompleterRows)
}

func (p *CapturePrompt) prefix() string {
	if p.todo {
		return "- TODO "
	}
	return "- "
}

// Line is the bullet as it will be written, without the final newline.
func (p *CapturePrompt) Line() string {
	return p.prefix() + strings.TrimSpace(p.input)
}

// SetError shows msg on the prompt's second row until the next key.
func (p *CapturePrompt) SetError(msg string) { p.errMsg = msg }

// Update handles one key. Input is edited only at its end.
func (p *CapturePrompt) Update(msg tea.KeyPressMsg) captureAction {
	p.errMsg = ""
	k := msg.String()
	if p.completer.active {
		switch k {
		case keyUp:
			p.completer.moveUp()
			return captureNone
		case keyDown:
			p.completer.moveDown()
			return captureNone
		case keyEnter, "tab":
			p.acceptCompletion()
			return captureNone
		case keyEsc:
			p.completer.dismiss()
			return captureNone
		}
	}
	switch k {
	case keyEsc:
		return captureCancel
	case keyEnter:
		if strings.TrimSpace(p.input) != "" {
			return captureSubmit
		}
		return captureNone
	case "tab":
		p.todo = !p.todo
		return captureNone
	case keyBackspace, "ctrl+h":
		p.input = dropLastGrapheme(p.input)
	case "ctrl+u":
		p.input = ""
	default:
		t := msg.Text
		if t == "" || strings.ContainsFunc(t, unicode.IsControl) {
			return captureNone
		}
		p.input += t
	}
	p.refreshCompleter(true)
	return captureNone
}

func (p *CapturePrompt) refreshCompleter(allowOpen bool) {
	p.completer.refresh(p.prefix()+p.input, "", allowOpen, strings.IndexByte(p.input, '#') >= 0)
}

// acceptCompletion splices the selected candidate onto the end of the input.
func (p *CapturePrompt) acceptCompletion() {
	del, ins, ok := p.completer.completion()
	if !ok {
		return
	}
	p.input = p.input[:max(0, len(p.input)-del)] + ins
	if p.completer.tag {
		p.completer.dismiss()
	}
	p.refreshCompleter(false)
}

// View renders the completion strip (when open) above the prompt box. The
// cursor is the terminal cursor's cell within the returned block.
func (p *CapturePrompt) View() (block string, cursorX, cursorY int) {
	strip := p.completer.View(p.width)
	room := max(10, p.width-2) - 4 // box content width

	label := "Capture to " + p.target + ": "
	lw := ansi.StringWidth(label)
	line := tailFit(render.DisplayText(p.prefix()+p.input), max(2, room-lw))
	row1 := styleTitle.Render(label) + line

	var row2 string
	if p.errMsg != "" {
		row2 = styleTitle.Render(clamp(p.errMsg, room))
	} else {
		row2 = styleFaint.Render(clamp(captureLegend, room))
	}

	block = renderBordered(max(10, p.width-2), row1+"\n"+row2)
	if strip != "" {
		block = strip + "\n" + block
	}
	return block, 3 + lw + ansi.StringWidth(line), p.completer.rows() + 2
}

// spliceBottom puts block over the last lines of base, keeping the frame at
// height lines.
func spliceBottom(base, block string, height int) string {
	if height <= 0 {
		return base + "\n" + block
	}
	bl := strings.Split(block, "\n")
	if len(bl) >= height {
		return strings.Join(bl[len(bl)-height:], "\n")
	}
	lines := strings.Split(base, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	copy(lines[height-len(bl):], bl)
	return strings.Join(lines, "\n")
}

// panelFeedback is an overlay with an in-panel message line: the status bar
// is hidden behind an overlay, so a capture's result goes there too.
type panelFeedback interface{ SetError(msg string) }

func (a *App) openCapture() {
	a.capture = newCapturePrompt(a.idx, a.todayJournalName(), a.width, a.height)
}

// appendToJournal adds line to journal name's file in one guarded write,
// seeding a missing file from the journal template. A file that changes
// between read and write is re-read once; nothing is written on failure.
func (a *App) appendToJournal(name, line string) (created bool, err error) {
	path := a.journalPath(name)
	for range 2 {
		snap, err := a.readSnapshot(path)
		if err != nil {
			return false, fmt.Errorf("cannot read %s: %w", name, err)
		}
		base := snap.Content
		if !snap.Exists {
			if base, err = a.journalTemplate(); err != nil {
				return false, err
			}
		}
		err = edit.WriteFileIfUnchanged(path, snap, []byte(graph.AppendLine(base, line)))
		if err == nil {
			return !snap.Exists, nil
		}
		if !errors.Is(err, edit.ErrChanged) {
			return false, err
		}
	}
	return false, edit.ErrChanged
}

// submitCapture writes the prompt's line; on failure the prompt stays open
// with its text and says why.
func (a *App) submitCapture() tea.Cmd {
	p := a.capture
	if a.syncing {
		p.SetError(syncBusyMsg)
		return nil
	}
	created, err := a.appendToJournal(p.target, p.Line())
	switch {
	case errors.Is(err, edit.ErrChanged):
		p.SetError(p.target + " kept changing on disk — nothing written; enter to retry")
		return nil
	case err != nil:
		p.SetError("not captured: " + err.Error())
		return nil
	}
	a.capture = nil
	msg := "captured to " + p.target
	if created {
		msg += " (new journal)"
	}
	if err := a.reindex(); err != nil {
		msg += " — reindex failed: " + err.Error()
	}
	if o, ok := a.active.(panelFeedback); ok {
		o.SetError(msg)
	}
	return tea.Batch(a.setHint(msg), a.statusProbeCmd())
}
