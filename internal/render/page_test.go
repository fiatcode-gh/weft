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

func TestRenderPageBlockRefBecomesLink(t *testing.T) {
	res, err := Render("see [[Alpha#summary]] for the upshot\n", 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 {
		t.Fatalf("want 1 link, got %d (%+v)", len(res.Links), res.Links)
	}
	if res.Links[0].Target != "Alpha" {
		t.Errorf("target = %q, want Alpha", res.Links[0].Target)
	}
}

func TestRenderPageBlockRefWithAlias(t *testing.T) {
	res, err := Render("see [[Alpha#summary|the summary]] for context\n", 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 {
		t.Fatalf("want 1 link, got %d (%+v)", len(res.Links), res.Links)
	}
	if res.Links[0].Target != "Alpha" {
		t.Errorf("target = %q, want Alpha", res.Links[0].Target)
	}
	if res.Links[0].Display != "the summary" {
		t.Errorf("display = %q, want 'the summary'", res.Links[0].Display)
	}
}

func TestRenderPageEmptyBlockFragmentIsNotALink(t *testing.T) {
	res, err := Render("anchor: [[#summary]] here\n", 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 0 {
		t.Errorf("want 0 links for [[#anchor]], got %d (%+v)", len(res.Links), res.Links)
	}
}

func TestRenderPageStripsQueryAndEmbedBlocks(t *testing.T) {
	body := "before\n{{query (and [[tag]] )}}\nstill query\n}}\nafter\n{{embed [[Other]]}}\n"
	res, err := Render(body, 80)
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(res.Styled)
	if strings.Contains(plain, "{{query") {
		t.Errorf("query block leaked into output:\n%s", plain)
	}
	if strings.Contains(plain, "{{embed") {
		t.Errorf("embed block leaked into output:\n%s", plain)
	}
	if !strings.Contains(plain, "before") || !strings.Contains(plain, "after") {
		t.Errorf("surrounding text dropped:\n%s", plain)
	}
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

func TestWarmupDoesNotPanic(t *testing.T) {
	// Warmup is paid once at process start so the first page render inside
	// the TUI doesn't pay chroma's syntax-highlighter init cost. Calling it more
	// than once is safe and is a no-op against the renderer cache.
	Warmup()
	Warmup()
}

func TestRenderPageBacktickWrappedWikiLinkIsLiteral(t *testing.T) {
	// Markdown inline code (backticks) is literal text — the contents are
	// NOT processed for other markdown constructs. So `[[Foo]]` should
	// appear in the output as the literal "[[Foo]]" text, not as a styled
	// wiki link. peekseq's preprocessor previously matched `[[...]]` inside
	// backticks; this test pins the fix.
	body := "see `[[Foo]]` for the literal text\n"
	res, err := Render(body, 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 0 {
		t.Errorf("want 0 links (wiki link inside backticks should be literal), got %d (%+v)", len(res.Links), res.Links)
	}
	plain := ansi.Strip(res.Styled)
	if !strings.Contains(plain, "[[Foo]]") {
		t.Errorf("expected literal [[Foo]] in output, got:\n%s", plain)
	}
}

func TestRenderPageMixedBacktickAndPlainLinks(t *testing.T) {
	// On a line that mixes backtick-wrapped and plain wiki links, only the
	// plain one should be preprocessed. The backtick-wrapped one stays
	// literal in the output.
	body := "code `[[Fake]]` and real [[Real]] end\n"
	res, err := Render(body, 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 {
		t.Fatalf("want 1 link (only the plain one), got %d (%+v)", len(res.Links), res.Links)
	}
	if res.Links[0].Target != "Real" {
		t.Errorf("link target = %q, want Real", res.Links[0].Target)
	}
	plain := ansi.Strip(res.Styled)
	if !strings.Contains(plain, "[[Fake]]") {
		t.Errorf("expected literal [[Fake]] to survive, got:\n%s", plain)
	}
}

func TestRenderPageMultipleInlineCodeSpansOnOneLine(t *testing.T) {
	// `code1` ... `code2` alternation: both code spans should be left
	// literal, and the plain wiki link between them preprocessed.
	body := "first `[[A]]` middle [[B]] last `[[C]]` end\n"
	res, err := Render(body, 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 {
		t.Fatalf("want 1 link (only [[B]]), got %d (%+v)", len(res.Links), res.Links)
	}
	if res.Links[0].Target != "B" {
		t.Errorf("link target = %q, want B", res.Links[0].Target)
	}
}
