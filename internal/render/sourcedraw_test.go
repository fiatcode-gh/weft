package render

import (
	"reflect"
	"strings"
	"testing"

	chromastyles "github.com/alecthomas/chroma/v2/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

var (
	styleRed  = uv.Style{Fg: ansi.Red}
	styleBold = uv.Style{Attrs: uv.AttrBold}
	styleUl   = uv.Style{Underline: uv.UnderlineStyleSingle}
)

func TestDrawRowNeverEmitsRawControls(t *testing.T) {
	line := "a\tb\x1b[31mc\x7fd\u009be\xff"
	rows := NewGeometry(Theme{}, 80).Rows(line, LineInfo{Kind: KindText})
	var got strings.Builder
	for _, r := range rows {
		got.WriteString(DrawRow(line, LineInfo{Kind: KindText}, r, nil))
	}
	want := "a   b^[[31mc^?d\uFFFDe\uFFFD"
	if got.String() != want {
		t.Errorf("DrawRow = %q, want %q", got.String(), want)
	}
	for _, c := range got.String() {
		if c < 0x20 || c == 0x7f {
			t.Errorf("raw control %#x in %q", c, got.String())
		}
	}
}

func TestDrawRowEmitsOnlyItsOwnSGR(t *testing.T) {
	line := "x\x1b]0;title\x07y"
	spans := []Span{{0, len(line), styleRed}}
	got := DrawRow(line, LineInfo{Kind: KindText}, Row{0, len(line), 0}, spans)
	if strings.Count(got, "\x1b") != 2 { // one SGR set, one reset
		t.Errorf("DrawRow = %q, want exactly the generated SGR set and reset", got)
	}
	if ansi.Strip(got) != "x^[]0;title^Gy" {
		t.Errorf("text = %q", ansi.Strip(got))
	}
}

func TestDrawRowMergesEqualAdjacentStyles(t *testing.T) {
	line := "aabbcc"
	spans := []Span{{0, 2, styleRed}, {2, 4, styleRed}, {4, 6, styleBold}}
	got := DrawRow(line, LineInfo{Kind: KindText}, Row{0, 6, 0}, spans)
	want := styleRed.Styled("aabb") + styleBold.Styled("cc")
	if got != want {
		t.Errorf("DrawRow = %q, want %q", got, want)
	}
}

func TestDrawRowDrawsOnlyItsRowAndKeepsGapsPlain(t *testing.T) {
	line := "one two three"
	spans := []Span{{4, 7, styleRed}}
	got := DrawRow(line, LineInfo{Kind: KindText}, Row{4, 13, 0}, spans)
	want := styleRed.Styled("two") + " three"
	if got != want {
		t.Errorf("DrawRow = %q, want %q", got, want)
	}
}

func TestDrawRowStylesAClusterByItsFirstByte(t *testing.T) {
	line := "e\u0301x"
	spans := []Span{{0, 1, styleRed}, {1, 4, styleBold}}
	got := DrawRow(line, LineInfo{Kind: KindText}, Row{0, 4, 0}, spans)
	want := styleRed.Styled("e\u0301") + styleBold.Styled("x")
	if got != want {
		t.Errorf("DrawRow = %q, want %q", got, want)
	}
}

func TestOverlay(t *testing.T) {
	base := []Span{{0, 4, styleRed}, {6, 10, styleBold}}
	tests := []struct {
		name       string
		start, end int
		want       []Span
	}{
		{"inside one span splits it", 1, 3, []Span{{0, 1, styleRed}, {1, 3, styleUl}, {3, 4, styleRed}, {6, 10, styleBold}}},
		{"across a gap fills it", 2, 8, []Span{{0, 2, styleRed}, {2, 8, styleUl}, {8, 10, styleBold}}},
		{"over a gap only", 4, 6, []Span{{0, 4, styleRed}, {4, 6, styleUl}, {6, 10, styleBold}}},
		{"covering everything", 0, 12, []Span{{0, 12, styleUl}}},
		{"empty range changes nothing", 3, 3, base},
		{"past the end appends", 12, 14, []Span{{0, 4, styleRed}, {6, 10, styleBold}, {12, 14, styleUl}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := append([]Span(nil), base...)
			got := Overlay(in, tt.start, tt.end, styleUl)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Overlay = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(in, base) {
				t.Errorf("Overlay mutated its input: %v", in)
			}
		})
	}
}

func TestPainterFenceFallbackOnTextMismatch(t *testing.T) {
	th := Theme{Config: terminalStyleConfig, Formatter: "terminal16"}
	p := NewPainter(th)
	p.highlight = func(code, lang string) (string, error) { return "not the code at all", nil }
	doc := strLines{"```go", "x := 1", "```"}
	sc := NewScanner()
	info := sc.Info(doc, 1)

	spans := p.Paint(doc, 1, info, sc)

	want := toUV(cascade(terminalStyleConfig.Document.StylePrimitive, terminalStyleConfig.CodeBlock.StylePrimitive))
	var wantSpans []Span
	if !want.IsZero() {
		wantSpans = []Span{{0, len("x := 1"), want}}
	}
	if !reflect.DeepEqual(spans, wantSpans) {
		t.Errorf("spans = %v, want %v (the fallback style)", spans, wantSpans)
	}
}

func TestPainterFenceFallbackOnHighlightError(t *testing.T) {
	th := Theme{Config: terminalStyleConfig, Formatter: "terminal16"}
	p := NewPainter(th)
	p.highlight = func(code, lang string) (string, error) { return "", errTestHighlight }
	doc := strLines{"```", "x", "```"}
	sc := NewScanner()
	_ = p.Paint(doc, 1, sc.Info(doc, 1), sc) // must not panic
}

type testErr string

func (e testErr) Error() string { return string(e) }

const errTestHighlight = testErr("highlight failed")

func TestPainterChromaRegistered(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	th, err := CurrentTheme()
	if err != nil {
		t.Fatal(err)
	}
	// No read-view render has happened in this process: NewPainter must
	// register Glamour's "charm" chroma style itself.
	if _, ok := chromastyles.Registry["charm"]; ok {
		t.Fatal("charm chroma style registered before NewPainter; the test no longer proves anything")
	}
	p := NewPainter(th)
	if _, ok := chromastyles.Registry["charm"]; !ok {
		t.Fatal("NewPainter did not register Glamour's charm chroma style")
	}
	doc := strLines{"```go", "func main() {}", "```"}
	sc := NewScanner()
	spans := p.Paint(doc, 1, sc.Info(doc, 1), sc)
	seen := map[string]bool{}
	for _, s := range spans {
		seen[s.Style.String()] = true
	}
	if len(seen) < 2 {
		t.Errorf("fence painted with %d distinct styles, want syntax highlighting: %v", len(seen), spans)
	}
}
