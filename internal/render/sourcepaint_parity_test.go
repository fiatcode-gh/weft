package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
)

// parityDoc has every element the painter colours, each in its own block.
const parityDoc = "# Title\n" +
	"\n" +
	"## Second level\n" +
	"\n" +
	"### Third level\n" +
	"\n" +
	"plain words **strong** and *slanted* and ~~struck~~ and `code` and [label](http://x.y) end\n" +
	"\n" +
	"- TODO a\n" +
	"- DOING b\n" +
	"- LATER c\n" +
	"- WAITING d\n" +
	"- DONE e\n" +
	"- CANCELED f\n" +
	"- NOW [#A] g\n" +
	"  SCHEDULED: <2026-05-25 Mon>\n" +
	"  DEADLINE: <2026-05-27 Wed>\n" +
	"- see [[Target]] and [[Real|Alias]] here\n" +
	"- tags #topic and #[[Two Words]] here\n" +
	"  - child words\n" +
	"\n" +
	"> quoted words\n" +
	"\n" +
	"| ca | cb |\n" +
	"|---|---|\n" +
	"| c1 | c2 |\n" +
	"\n" +
	"---\n" +
	"\n" +
	"```go\n" +
	"func main() { s := \"lit\" }\n" +
	"```\n"

type paritySample struct {
	name   string
	needle string // text located uniquely in both screens
}

// paritySamples is the full list; bodyOnly names the samples that stay valid
// in a named style, where the read view loses the body colour after a
// wiki-link or workflow marker (PLAN §12 item 2).
var paritySamples = []paritySample{
	{"h1", "Title"}, {"h2", "Second level"}, {"h3", "Third level"},
	{"body", "plain words"}, {"strong", "strong"}, {"emph", "slanted"},
	{"strike", "struck"}, {"code", "code"}, {"link", "label"},
	{"TODO", "TODO"}, {"DOING", "DOING"}, {"LATER", "LATER"}, {"WAITING", "WAITING"},
	{"DONE", "DONE"}, {"CANCELED", "CANCELED"}, {"NOW", "NOW"},
	{"priority", "[#A]"}, {"scheduled", "SCHEDULED"}, {"deadline", "DEADLINE"},
	{"wiki target", "Target"}, {"wiki alias", "Alias"},
	{"tag", "#topic"}, {"bracket tag", "Two Words"},
	{"child", "child words"}, {"quote", "quoted words"},
	{"table header", "ca"}, {"table cell", "c1"},
	{"fence func", "func"}, {"fence name", "main"}, {"fence string", "\"lit\""},
}

// screenOf draws styled text into cells, one row per line.
func screenOf(styled string) (cells [][]uv.Cell) {
	h := strings.Count(styled, "\n") + 1
	buf := uv.NewScreenBuffer(200, h)
	uv.NewStyledString(styled).Draw(buf, buf.Bounds())
	for y := range h {
		row := make([]uv.Cell, 200)
		for x := range row {
			if c := buf.CellAt(x, y); c != nil {
				row[x] = *c
			}
		}
		cells = append(cells, row)
	}
	return cells
}

func cellText(c uv.Cell) string {
	if c.Content == "" {
		return " "
	}
	return c.Content
}

func rowText(row []uv.Cell) string {
	var b strings.Builder
	for _, c := range row {
		b.WriteString(cellText(c))
	}
	return b.String()
}

// styleRun returns the styles of the cells spelling needle. The needle must
// occur once on the screen.
func styleRun(t *testing.T, scr [][]uv.Cell, side, needle string) []uv.Style {
	t.Helper()
	var found []uv.Style
	n := 0
	for _, row := range scr {
		for x := 0; x+len([]rune(needle)) <= len(row); x++ {
			var styles []uv.Style
			ok := true
			for i, r := range []rune(needle) {
				if cellText(row[x+i]) != string(r) {
					ok = false
					break
				}
				styles = append(styles, row[x+i].Style)
			}
			if ok {
				n++
				found = styles
			}
		}
	}
	if n != 1 {
		t.Fatalf("%s: %q found %d times, want once", side, needle, n)
	}
	return found
}

// firstGlyph is the first non-space cell of the row that contains needle.
func firstGlyph(t *testing.T, scr [][]uv.Cell, side, needle string) uv.Cell {
	t.Helper()
	for _, row := range scr {
		if strings.Contains(rowText(row), needle) {
			for _, c := range row {
				if cellText(c) != " " {
					return c
				}
			}
		}
	}
	t.Fatalf("%s: no row contains %q", side, needle)
	return uv.Cell{}
}

// ruleCell is the first cell of the row made only of dashes.
func ruleCell(t *testing.T, scr [][]uv.Cell, side string) uv.Cell {
	t.Helper()
	for _, row := range scr {
		s := strings.TrimSpace(rowText(row))
		if len(s) >= 3 && strings.Trim(s, "-") == "" {
			for _, c := range row {
				if cellText(c) != " " {
					return c
				}
			}
		}
	}
	t.Fatalf("%s: no rule row", side)
	return uv.Cell{}
}

type parityScreens struct {
	read, source [][]uv.Cell
}

func buildParity(t *testing.T) parityScreens {
	t.Helper()
	th, err := CurrentTheme()
	if err != nil {
		t.Fatal(err)
	}
	res, err := RenderWithEmphasis(parityDoc, 80, "")
	if err != nil {
		t.Fatal(err)
	}
	src := strLines(strings.Split(strings.TrimSuffix(parityDoc, "\n"), "\n"))
	sc, g, p := NewScanner(), NewGeometry(th, 80), NewPainter(th)
	var rows []string
	for i := range src {
		line := src[i]
		info := sc.Info(src, i)
		spans := p.Paint(src, i, info, sc)
		for _, r := range g.Rows(line, info) {
			rows = append(rows, strings.Repeat(" ", r.Col)+DrawRow(line, info, r, spans))
		}
	}
	return parityScreens{read: screenOf(res.Styled), source: screenOf(strings.Join(rows, "\n"))}
}

func (s parityScreens) compare(t *testing.T, samples []paritySample, conv func(uv.Style) uv.Style) {
	t.Helper()
	for _, sm := range samples {
		read := styleRun(t, s.read, "read view", sm.needle)
		source := styleRun(t, s.source, "source", sm.needle)
		for i := range read {
			r, c := conv(read[i]), conv(source[i])
			if !r.Equal(&c) {
				t.Errorf("%s: %q cell %d: source %+v, read view %+v", sm.name, sm.needle, i, c, r)
				break
			}
		}
	}
	cmp := func(name string, read, source uv.Cell) {
		r, c := conv(read.Style), conv(source.Style)
		if !r.Equal(&c) {
			t.Errorf("%s: source %+v (%q), read view %+v (%q)", name, c, source.Content, r, read.Content)
		}
	}
	cmp("bullet", firstGlyph(t, s.read, "read view", "Target"), firstGlyph(t, s.source, "source", "Target"))
	cmp("rule", ruleCell(t, s.read, "read view"), ruleCell(t, s.source, "source"))
}

func identity(s uv.Style) uv.Style { return s }

func ascii(s uv.Style) uv.Style { return uv.ConvertStyle(s, colorprofile.Ascii) }

// bodyOnly drops the samples that sit after a wiki-link or marker in the same
// text run, where named styles show the read view's colour loss.
func bodyOnly(samples []paritySample) []paritySample {
	var out []paritySample
	for _, s := range samples {
		switch s.name {
		case "body", "child", "quote", "h1", "h2", "h3", "strong", "emph", "strike", "code", "link",
			"table header", "table cell", "fence func", "fence name", "fence string", "wiki target", "tag":
			out = append(out, s)
		}
	}
	return out
}

func TestPainterParityTerminal(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	buildParity(t).compare(t, paritySamples, identity)
}

func TestPainterParityDark(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": "dark"}) {
		return
	}
	buildParity(t).compare(t, bodyOnly(paritySamples), identity)
}

func TestPainterParityNoColor(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "1", "WEFT_STYLE": ""}) {
		return
	}
	s := buildParity(t)
	s.compare(t, paritySamples, identity)
	s.compare(t, paritySamples, ascii)

	// What weft displays under NO_COLOR: no colour anywhere, attributes kept.
	wantAttrs := map[string]struct {
		attrs uint8
		ul    bool
	}{
		"h1": {uv.AttrBold, false}, "h2": {uv.AttrBold, false}, "h3": {uv.AttrBold, false},
		"strong": {uv.AttrBold, false}, "emph": {uv.AttrItalic, false},
		"strike": {uv.AttrStrikethrough, false},
		"TODO":   {uv.AttrBold, false}, "CANCELED": {uv.AttrStrikethrough, false},
		"wiki target": {0, true}, "wiki alias": {0, true},
		"tag": {0, true}, "bracket tag": {0, true},
	}
	for _, sm := range paritySamples {
		for _, side := range []struct {
			name string
			scr  [][]uv.Cell
		}{{"read view", s.read}, {"source", s.source}} {
			for i, st := range styleRun(t, side.scr, side.name, sm.needle) {
				st = ascii(st)
				if st.Fg != nil || st.Bg != nil || st.UnderlineColor != nil {
					t.Errorf("%s %s cell %d keeps a colour: %+v", side.name, sm.name, i, st)
				}
				if want, ok := wantAttrs[sm.name]; ok {
					if st.Attrs&want.attrs != want.attrs {
						t.Errorf("%s %s cell %d attrs %#x, want %#x set", side.name, sm.name, i, st.Attrs, want.attrs)
					}
					if want.ul && st.Underline == uv.UnderlineNone {
						t.Errorf("%s %s cell %d is not underlined", side.name, sm.name, i)
					}
				}
			}
		}
	}
}
