package views

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/sahilm/fuzzy"
)

// paletteKeyCol is the width of the key column on the right of a palette row.
const paletteKeyCol = 6

// Palette is the command palette (`:`): every palette action in help order,
// fuzzy-filtered on its key and title as the user types. Enter asks the App to
// close it and run the action exactly as its key would.
type Palette struct {
	listBox
	input   textinput.Model
	matches []fuzzy.Match // Index into paletteActions
}

// NewPalette opens the palette with every action listed.
func NewPalette(width, height int) *Palette {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 200
	p := &Palette{listBox: listBox{width: width, height: height}, input: ti}
	p.search("")
	return p
}

func (p *Palette) search(q string) {
	if strings.TrimSpace(q) == "" {
		p.matches = p.matches[:0]
		for i := range paletteActions {
			p.matches = append(p.matches, fuzzy.Match{Str: paletteLabels[i], Index: i})
		}
	} else {
		p.matches = fuzzy.Find(q, paletteLabels)
	}
	p.sel = 0
}

// Update handles a key and reports the result to the App.
func (p *Palette) Update(key string) OverlayResult {
	switch key {
	case keyEsc:
		return overlayCancel()
	case keyEnter:
		if p.sel >= 0 && p.sel < len(p.matches) {
			return overlayRun(paletteActions[p.matches[p.sel].Index])
		}
		return OverlayResult{}
	case keyUp, keyCtrlK:
		p.moveUp()
		return OverlayResult{}
	case keyDown, keyCtrlJ:
		p.moveDown(len(p.matches))
		return OverlayResult{}
	}
	p.input, _ = consumeKey(p.input, key)
	p.search(p.input.Value())
	return OverlayResult{}
}

// visibleRows returns how many match rows the palette renders at once; the
// chrome is the picker's.
func (p *Palette) visibleRows() int {
	const chrome = 10
	return clampInt(p.height-chrome, listVisibleRowsMin, pickerVisibleRowsMax)
}

func (p *Palette) View() string {
	inner := p.innerWidth()
	budget := inner - paletteKeyCol - 2 - 3 // -3 = the " ▶ "/"   " marker cells every row carries

	var b strings.Builder
	p.drawn.reset()
	b.WriteString(styleTitle.Render("Commands"))
	b.WriteString("\n\n")
	b.WriteString(styleFaint.Render("> "))
	b.WriteString(clamp(p.input.Value(), inner-3))
	b.WriteString("\n")
	b.WriteString(styleFaint.Render(strings.Repeat("─", inner)))
	b.WriteString("\n")
	if len(p.matches) == 0 {
		b.WriteString(styleFaint.Render("  no matches"))
		b.WriteString("\n")
	}

	start, end := scrollWindow(p.sel, len(p.matches), p.visibleRows())
	if start > 0 {
		b.WriteString(styleFaint.Render(fmt.Sprintf("  ↑ %d more above", start)))
		b.WriteString("\n")
	}
	for i := start; i < end; i++ {
		from := b.Len()
		act := paletteActions[p.matches[i].Index]
		title := clamp(act.title, budget)
		marker := "   "
		var row string
		if i == p.sel {
			marker = styleSel.Render(" ▶ ")
			row = styleSel.Render(layoutPickerRow(title, act.key, budget))
		} else {
			row = padTo(title, budget) + "  " + styleFaint.Render(act.key)
		}
		b.WriteString(marker)
		b.WriteString(row)
		b.WriteString("\n")
		p.drawn.span(&b, from, i)
	}
	if end < len(p.matches) {
		b.WriteString(styleFaint.Render(fmt.Sprintf("  ↓ %d more below", len(p.matches)-end)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(styleFaint.Render(clamp("↑/↓ select · enter run · esc cancel", inner)))
	return renderBordered(inner+4, b.String())
}
