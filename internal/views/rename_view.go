package views

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

const renameLegend = "enter preview · esc cancel"

// RenamePrompt is the one-line prompt that asks for a page's new name. Like
// CapturePrompt it is not an Overlay: App draws it over the screen and gives
// it every key while it is open.
type RenamePrompt struct {
	page          graph.PageMeta // copy of the page being renamed
	input         string
	errMsg        string
	width, height int
}

func newRenamePrompt(page graph.PageMeta, width, height int) *RenamePrompt {
	p := &RenamePrompt{page: page, input: page.Name}
	p.SetSize(width, height)
	return p
}

// SetSize records the terminal size.
func (p *RenamePrompt) SetSize(w, h int) { p.width, p.height = w, h }

// SetError shows msg on the prompt's second row until the next key.
func (p *RenamePrompt) SetError(msg string) { p.errMsg = msg }

// Update handles one key. Input is edited only at its end.
func (p *RenamePrompt) Update(msg tea.KeyPressMsg) captureAction {
	p.errMsg = ""
	switch msg.String() {
	case keyEsc:
		return captureCancel
	case keyEnter:
		return captureSubmit
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
	return captureNone
}

// View renders the prompt box. The cursor is the terminal cursor's cell
// within the returned block.
func (p *RenamePrompt) View() (block string, cursorX, cursorY int) {
	room := max(10, p.width-2) - 4 // box content width

	label := "Rename " + p.page.Key + " to: "
	lw := ansi.StringWidth(label)
	line := tailFit(render.DisplayText(p.input), max(2, room-lw))
	row1 := styleTitle.Render(label) + line

	var row2 string
	if p.errMsg != "" {
		row2 = styleTitle.Render(clamp(p.errMsg, room))
	} else {
		row2 = styleFaint.Render(clamp(renameLegend, room))
	}

	block = renderBordered(max(10, p.width-2), row1+"\n"+row2)
	return block, 3 + lw + ansi.StringWidth(line), 2
}

// RenamePreview is the overlay that shows what a rename will write, asks for
// confirmation, and reports where a confirmed rename stopped.
type RenamePreview struct {
	rp            *renamePlan
	stop          *renameStop
	reindexErr    error
	errMsg        string
	off           int
	width, height int
}

// NewRenamePreview shows the preview of p.
func NewRenamePreview(p *renamePlan, width, height int) *RenamePreview {
	return &RenamePreview{rp: p, width: width, height: height}
}

// SetSize records the terminal size.
func (r *RenamePreview) SetSize(w, h int) { r.width, r.height = w, h; r.clampOff() }

// SetError shows msg above the footer until the next key.
func (r *RenamePreview) SetError(msg string) { r.errMsg = msg }

// SetStopped turns the panel into the stopped report.
func (r *RenamePreview) SetStopped(stop renameStop, reindexErr error) {
	r.stop, r.reindexErr, r.off = &stop, reindexErr, 0
}

func (r *RenamePreview) innerWidth() int {
	return clampInt(r.width-2-4-4, listInnerWidthMin, listInnerWidthMax)
}

// listRows is the scrollable content: one row per file (preview) or the
// rewritten/not-rewritten sections (stopped).
func (r *RenamePreview) listRows() []string {
	inner := r.innerWidth()
	var rows []string
	if r.stop == nil {
		for _, f := range r.rp.files {
			n := fmt.Sprint(f.links)
			rel := clamp(f.rel, inner-len(n)-2)
			gap := max(2, inner-ansi.StringWidth(rel)-len(n))
			rows = append(rows, rel+strings.Repeat(" ", gap)+n)
		}
		return rows
	}
	section := func(title string, rels []string) {
		if len(rels) == 0 {
			return
		}
		rows = append(rows, styleFaint.Render(fmt.Sprintf("%s (%d)", title, len(rels))))
		for _, rel := range rels {
			rows = append(rows, "  "+clamp(rel, inner-2))
		}
	}
	section("rewritten", r.stop.done)
	section("not rewritten", r.stop.pending)
	return rows
}

// head is the lines above the list; foot is the lines below it, indicator
// and error rows included so the panel height never depends on scrolling.
func (r *RenamePreview) head() []string {
	inner := r.innerWidth()
	if r.stop != nil {
		lines := []string{styleTitle.Render("Rename stopped")}
		lines = append(lines, strings.Split(ansi.Wrap(r.stop.reason, inner, ""), "\n")...)
		if r.reindexErr != nil {
			lines = append(lines, strings.Split(ansi.Wrap("reindex failed: "+r.reindexErr.Error(), inner, ""), "\n")...)
		}
		return append(lines, "")
	}
	p := r.rp
	summary := "no links to rewrite"
	if p.links > 0 {
		summary = fmt.Sprintf("%s in %s", plural(p.links, "link"), plural(len(p.files), "file"))
	}
	return []string{
		styleTitle.Render("Rename page"),
		clamp(fmt.Sprintf("name  %s → %s", p.page.Key, p.to.Key), inner),
		clamp(fmt.Sprintf("file  %s → %s", p.page.Rel, p.to.Rel), inner),
		"",
		summary,
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// window is the number of list rows that fit; unbounded until the terminal
// height is known.
func (r *RenamePreview) window(nHead, nRows int) int {
	if r.height <= 0 {
		return max(1, nRows)
	}
	// border+padding (4), indicator line, error line, footer line.
	return max(1, r.height-4-3-nHead)
}

func (r *RenamePreview) clampOff() {
	rows := r.listRows()
	r.off = clampInt(r.off, 0, max(0, len(rows)-r.window(len(r.head()), len(rows))))
}

// Update handles one key.
func (r *RenamePreview) Update(key string) OverlayResult {
	r.errMsg = ""
	switch key {
	case keyEsc, "n":
		return overlayCancel()
	case "q":
		if r.stop != nil {
			return overlayCancel()
		}
	case "y":
		if r.stop == nil {
			return overlayRename()
		}
	case keyUp, "k":
		r.off--
		r.clampOff()
	case keyDown, "j":
		r.off++
		r.clampOff()
	}
	return OverlayResult{}
}

func (r *RenamePreview) View() string {
	inner := r.innerWidth()
	head := r.head()
	rows := r.listRows()
	win := r.window(len(head), len(rows))
	start := clampInt(r.off, 0, max(0, len(rows)-win))
	end := min(len(rows), start+win)

	var sb strings.Builder
	sb.WriteString(strings.Join(head, "\n"))
	for _, row := range rows[start:end] {
		sb.WriteString("\n" + row)
	}
	sb.WriteString("\n")
	overflow := len(rows) > win
	if overflow {
		noun := "files"
		if r.stop != nil {
			noun = "rows"
		}
		sb.WriteString(styleFaint.Render(fmt.Sprintf("%s %d–%d of %d", noun, start+1, end, len(rows))))
	}
	sb.WriteString("\n")
	if r.errMsg != "" {
		sb.WriteString(styleTitle.Render(clamp(r.errMsg, inner)))
	}
	sb.WriteString("\n")
	var footer string
	switch {
	case r.stop != nil:
		footer = "F2 with the same name finishes the rename · esc close"
	case overflow:
		footer = "y rename · n/esc cancel · ↑/↓ scroll"
	default:
		footer = "y rename · n/esc cancel"
	}
	sb.WriteString(styleFaint.Render(clamp(footer, inner)))
	return renderBordered(inner+4, sb.String())
}

// openRename opens the rename prompt for the page on screen.
func (a *App) openRename() tea.Cmd {
	if cmd, blocked := a.blockIfSyncing(); blocked {
		return cmd
	}
	meta, ok := a.idx.Resolve(a.page.Page())
	if !ok {
		return a.setHint("no file to rename yet")
	}
	if meta.IsJournal {
		return a.setHint("journals cannot be renamed")
	}
	a.rename = newRenamePrompt(*meta, a.width, a.height)
	return nil
}

// submitRename checks the typed name and, when it is acceptable, swaps the
// prompt for the preview. A refusal stays in the prompt with its text.
func (a *App) submitRename() tea.Cmd {
	p := a.rename
	page, ok := a.idx.PageAt(p.page.Path)
	if !ok {
		p.SetError(p.page.Rel + " no longer exists")
		return nil
	}
	plan, err := a.planRename(page, p.input)
	if err != nil {
		p.SetError(err.Error())
		return nil
	}
	a.rename = nil
	a.active = NewRenamePreview(plan, a.width, a.height)
	return nil
}

// confirmRename carries out the previewed rename.
func (a *App) confirmRename() tea.Cmd {
	if cmd, blocked := a.blockIfSyncing(); blocked {
		return cmd
	}
	pv := a.active.(*RenamePreview)
	res, err := a.applyRename(pv.rp)
	if err != nil {
		pv.SetError(err.Error())
		return nil
	}
	if res.stop != nil {
		pv.SetStopped(*res.stop, res.reindexErr)
		return a.statusProbeCmd()
	}
	p := pv.rp
	a.active = nil
	hint := fmt.Sprintf("renamed %s → %s · %s in %s", p.page.Key, p.to.Key, plural(p.links, "link"), plural(len(p.files), "file"))
	if res.reindexErr != nil {
		hint += " — reindex failed: " + res.reindexErr.Error()
	}
	return tea.Batch(a.setHint(hint), a.statusProbeCmd())
}
