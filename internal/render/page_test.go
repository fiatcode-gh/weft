package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderPageReturnsLinksWithTargets(t *testing.T) {
	body := "- See [[Alpha]] and [[Beta|the second]]."
	out, err := Render(body, 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	gotTargets := make([]string, 0, len(out.Links))
	for _, l := range out.Links {
		gotTargets = append(gotTargets, l.Target)
	}
	want := []string{"Alpha", "Beta"}
	if strings.Join(gotTargets, ",") != strings.Join(want, ",") {
		t.Errorf("link targets: want %v, got %v", want, gotTargets)
	}
	if out.Styled == "" {
		t.Error("Styled output empty")
	}
	if len(out.Links) >= 2 {
		span := out.Styled[out.Links[1].Start:out.Links[1].End]
		if !strings.Contains(span, "the second") {
			t.Errorf("aliased link span: want substring %q, got %q", "the second", span)
		}
	}
}

func TestRenderPageHandlesEmptyBody(t *testing.T) {
	out, err := Render("", 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(out.Links) != 0 {
		t.Errorf("empty body should have no links, got %v", out.Links)
	}
}

func TestRenderStripsLogbookBlocks(t *testing.T) {
	body := strings.Join([]string{
		"- a normal bullet",
		"  :LOGBOOK:",
		"  CLOCK: [2026-05-21 Thu 15:38:56]--[2026-05-21 Thu 15:56:09] =>  00:17:13",
		"  :END:",
		"- another bullet",
	}, "\n")
	out, err := Render(body, 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out.Styled)
	plain = strings.Join(strings.Fields(plain), " ") // collapse Glamour's word-by-word spacing
	for _, s := range []string{":LOGBOOK:", ":END:", "CLOCK:"} {
		if strings.Contains(plain, s) {
			t.Errorf("Styled still contains %q: %q", s, plain)
		}
	}
	for _, s := range []string{"a normal bullet", "another bullet"} {
		if !strings.Contains(plain, s) {
			t.Errorf("missing %q in stripped output: %q", s, plain)
		}
	}
}

func TestRenderWikiLinkWrapsAtRightMargin(t *testing.T) {
	// A line whose wiki-link sits near the right margin: the link's rendered
	// display is much wider than the raw `<id>` sentinel, so without
	// width-padding Glamour wraps based on the sentinel and the substituted
	// link overflows the column budget.
	body := "- Some text leading up to [[VeryLongPageNameRightAtTheEnd]]"
	out, err := Render(body, 40)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out.Styled)
	maxLine := 0
	for _, line := range strings.Split(plain, "\n") {
		// strip trailing spaces lipgloss adds to pad to width
		line = strings.TrimRight(line, " ")
		if w := len(line); w > maxLine {
			maxLine = w
		}
	}
	if maxLine > 40 {
		t.Errorf("rendered line wider than wrap width 40: max=%d, output=%q", maxLine, plain)
	}
}

func TestRenderHangingIndentOnWrappedBullets(t *testing.T) {
	body := "- This bullet has enough text that Glamour will wrap it across two lines for sure."
	out, err := Render(body, 40)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out.Styled)
	lines := strings.Split(plain, "\n")
	var bullet, continuation int = -1, -1
	for i, line := range lines {
		if strings.Contains(line, "• ") && bullet < 0 {
			bullet = i
			continue
		}
		if bullet >= 0 && continuation < 0 && strings.TrimSpace(line) != "" {
			continuation = i
			break
		}
	}
	if bullet < 0 || continuation < 0 {
		t.Fatalf("expected a bullet line followed by a continuation line, got:\n%s", plain)
	}
	bulletIndent := leadingSpaceCount(lines[bullet])
	contentIndent := bulletIndent + 2 // bullet glyph + space
	if got := leadingSpaceCount(lines[continuation]); got != contentIndent {
		t.Errorf("continuation indent: want %d, got %d. lines:\n%s", contentIndent, got, plain)
	}
}

func leadingSpaceCount(s string) int {
	n := 0
	for _, r := range s {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}

func TestRenderTaskMarkersSurviveStyling(t *testing.T) {
	body := strings.Join([]string{
		"- TODO Buy milk",
		"- DOING Write the parser",
		"- DONE Old item",
	}, "\n")
	out, err := Render(body, 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out.Styled)
	for _, marker := range []string{"TODO", "DOING", "DONE"} {
		if !strings.Contains(plain, marker) {
			t.Errorf("missing marker %q in stripped output: %q", marker, plain)
		}
	}
}
