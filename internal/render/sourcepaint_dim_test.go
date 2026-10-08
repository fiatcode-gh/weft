package render

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// dimDoc marks, with « », the bytes the read view hides. The markers are
// stripped before painting.
var dimDoc = []string{
	"words «**»bold«**» «*»ital«*» «~~»gone«~~» «`»code«`» «[»label«](http://x.y)» end",
	"- see «[[»Target«]]» and «[[Real|»Alias«]]» here",
	"- tag #topic and #«[[»Two Words«]]» and #«[[Real|»Alias«]]» here",
	"- code «`»#x«`» C# #18",
	"## Two «##»",
	"Setext",
	"«======»",
	"a «\\»*b",
	"«```go»",
	"x := 1",
	"«```»",
	"«:LOGBOOK:»",
	"«:END:»",
}

func TestPainterDimsExactlyTheHiddenSyntax(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	th, err := CurrentTheme()
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	var faint [][]bool
	for _, raw := range dimDoc {
		var line strings.Builder
		var f []bool
		on := false
		for _, r := range raw {
			switch r {
			case '«':
				on = true
			case '»':
				on = false
			default:
				for range len(string(r)) {
					f = append(f, on)
				}
				line.WriteRune(r)
			}
		}
		lines, faint = append(lines, line.String()), append(faint, f)
	}

	src := strLines(lines)
	sc, p := NewScanner(), NewPainter(th)
	for i, line := range lines {
		spans := p.Paint(src, i, sc.Info(src, i), sc)
		got := make([]bool, len(line))
		for _, s := range spans {
			for b := s.Start; b < s.End; b++ {
				got[b] = s.Style.Attrs&uv.AttrFaint != 0
			}
		}
		for b := range line {
			if got[b] != faint[i][b] {
				t.Errorf("line %d %q byte %d (%q): faint = %v, want %v", i, line, b, line[b], got[b], faint[i][b])
			}
		}
	}
}

func TestPainterDimIsTheBaseStyleWithFaint(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	th, err := CurrentTheme()
	if err != nil {
		t.Fatal(err)
	}
	cfg := th.Config
	doc := strLines{"w **b**", "```", "x", "```"}
	sc, p := NewScanner(), NewPainter(th)

	paragraph := cascade(cfg.Document.StylePrimitive, cfg.Paragraph.StylePrimitive)
	wantDelim := toUV(cascade(paragraph, cfg.Text))
	wantDelim.Attrs |= uv.AttrFaint
	spans := p.Paint(doc, 0, sc.Info(doc, 0), sc)
	if got := styleAtByte(spans, 2); !got.Equal(&wantDelim) {
		t.Errorf("`**` style = %+v, want %+v", got, wantDelim)
	}

	wantFence := toUV(cfg.Document.StylePrimitive)
	wantFence.Attrs |= uv.AttrFaint
	spans = p.Paint(doc, 1, sc.Info(doc, 1), sc)
	if got := styleAtByte(spans, 0); !got.Equal(&wantFence) {
		t.Errorf("fence delimiter style = %+v, want %+v", got, wantFence)
	}
}

func styleAtByte(spans []Span, b int) uv.Style {
	for _, s := range spans {
		if b >= s.Start && b < s.End {
			return s.Style
		}
	}
	return uv.Style{}
}
