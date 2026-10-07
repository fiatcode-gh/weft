package render

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

func paintAt(t *testing.T, lines []string, i int, needle string) uv.Style {
	t.Helper()
	th, err := CurrentTheme()
	if err != nil {
		t.Fatal(err)
	}
	src := strLines(lines)
	sc := NewScanner()
	spans := NewPainter(th).Paint(src, i, sc.Info(src, i), sc)
	return styleAtByte(spans, strings.Index(lines[i], needle))
}

// A leaf heading inside a list item draws on the list's stack, as the read
// view does: stack List, then the heading's own rules.
func TestPainterHeadingInListItemCascadesFromList(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": "dark"}) {
		return
	}
	th, _ := CurrentTheme()
	c := th.Config
	rules := cascade(c.Heading.StylePrimitive, c.H2.StylePrimitive)
	want := toUV(cascade(cascade(cascade(c.Document.StylePrimitive, c.List.StylePrimitive), rules), c.Text))

	got := paintAt(t, []string{"- ## Title"}, 0, "Title")
	if !got.Equal(&want) {
		t.Errorf("heading text in a list item = %+v, want %+v", got, want)
	}
	plain := paintAt(t, []string{"## Title"}, 0, "Title")
	if got.Equal(&plain) && !want.Equal(&plain) {
		t.Errorf("list heading painted like a top-level heading: %+v", got)
	}
}

// Nested emphasis lets the outer style win, as Glamour's StyleOverrideRender
// cascades it: in ***x*** the outer emphasis overrides the inner one.
func TestPainterNestedEmphasisAccumulatesBoldAndItalic(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	got := paintAt(t, []string{"***both***"}, 0, "both")
	if got.Attrs&uv.AttrBold == 0 || got.Attrs&uv.AttrItalic == 0 {
		t.Errorf("***both*** attrs = %#x, want bold and italic", got.Attrs)
	}
}
