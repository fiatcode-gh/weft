package views

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

const onThisDayPreviewLines = 2

type onThisDayRow struct {
	graph.OnThisDayEntry
	preview []string
	readErr string
}

// OnThisDay is the overlay listing journals from a week ago, a month ago and
// this date in earlier years (graph.OnThisDay), each with a short preview.
type OnThisDay struct {
	listBox
	today   time.Time
	entries []onThisDayRow
	errMsg  string
}

// NewOnThisDay reads each entry's file under journals/ through read; it does
// not resolve names via idx.ByName, which prefers a same-named pages/ file.
func NewOnThisDay(idx *graph.Index, today time.Time, read func(string) (edit.Snapshot, error), width, height int) *OnThisDay {
	o := &OnThisDay{listBox: listBox{width: width, height: height}, today: today}
	for _, e := range graph.OnThisDay(today, idx.Journals()) {
		row := onThisDayRow{OnThisDayEntry: e}
		path := filepath.Join(idx.GraphPath, "journals", graph.FilenameFromPageName(e.Name))
		if snap, err := read(path); err != nil {
			row.readErr = "cannot read: " + err.Error()
		} else if snap.Exists {
			row.preview = previewLines(snap.Content, onThisDayPreviewLines)
		}
		o.entries = append(o.entries, row)
	}
	return o
}

// previewLines returns up to n lines of content that are not blank, each with
// trailing whitespace (and "\r") removed.
func previewLines(content string, n int) []string {
	var out []string
	for line := range strings.SplitSeq(content, "\n") {
		if len(out) == n {
			break
		}
		if line = strings.TrimRight(line, " \t\r"); strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// SetError shows msg inside the panel until the next key.
func (o *OnThisDay) SetError(msg string) { o.errMsg = msg }

func (o *OnThisDay) Update(key string) OverlayResult {
	o.errMsg = ""
	switch key {
	case keyEsc, keyQ:
		return overlayCancel()
	case "c":
		return overlayCapture()
	case keyUp, keyK:
		o.moveUp()
	case keyDown, keyJ:
		o.moveDown(len(o.entries))
	case keyEnter:
		if o.sel >= 0 && o.sel < len(o.entries) {
			return overlayOpen(o.entries[o.sel].Name)
		}
	}
	return OverlayResult{}
}

func (o *OnThisDay) visibleEntries() int {
	return max(1, clampInt(o.height-9, listVisibleRowsMin, 30)/3)
}

func (o *OnThisDay) View() string {
	inner := o.innerWidth()
	var sb strings.Builder
	sb.WriteString(styleTitle.Render("On this day"))
	sb.WriteString(styleFaint.Render(fmt.Sprintf("   · today %s   (%d)", o.today.Format("2006-01-02 Mon"), len(o.entries))))
	sb.WriteString("\n\n")
	sb.WriteString(styleFaint.Render(strings.Repeat("─", inner)))
	sb.WriteString("\n")
	if len(o.entries) == 0 {
		sb.WriteString(styleFaint.Render(clamp("  nothing on this day — no journal from a week ago, a month ago or this date in earlier years", inner)))
		sb.WriteString("\n")
	} else {
		start, end := scrollWindow(o.sel, len(o.entries), o.visibleEntries())
		if start > 0 {
			sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↑ %d more above", start)))
			sb.WriteString("\n")
		}
		for i := start; i < end; i++ {
			o.writeEntry(&sb, i, inner)
		}
		if below := len(o.entries) - end; below > 0 {
			sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↓ %d more below", below)))
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")
	if o.errMsg != "" {
		sb.WriteString(styleTitle.Render(clamp(o.errMsg, inner)))
		sb.WriteString("\n")
	}
	sb.WriteString(styleFaint.Render(clamp("↑/↓ select · enter open · c capture · esc back", inner)))
	return renderBordered(inner+4, sb.String())
}

func (o *OnThisDay) writeEntry(sb *strings.Builder, i, inner int) {
	e := o.entries[i]
	date, _ := graph.JournalDate(e.Name)
	head := clamp(e.Label+" · "+date.Format("2006-01-02 Mon"), inner-3)
	if i == o.sel {
		sb.WriteString(styleSel.Render(" ▶ ") + styleSel.Render(head))
	} else {
		sb.WriteString("   " + head)
	}
	sb.WriteString("\n")
	const indent = "     "
	switch {
	case e.readErr != "":
		sb.WriteString(indent + styleFaint.Render(clamp("("+e.readErr+")", inner-5)) + "\n")
	case len(e.preview) == 0:
		sb.WriteString(indent + styleFaint.Render("(empty)") + "\n")
	default:
		for _, l := range e.preview {
			sb.WriteString(indent + styleFaint.Render(clamp(l, inner-5)) + "\n")
		}
	}
}
