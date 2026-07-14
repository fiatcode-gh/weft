package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// mustRender renders body at the given width and fails the test on error. It is
// the shared arrange step for the many single-scenario render tests.
func mustRender(t *testing.T, body string, width int) Result {
	t.Helper()
	out, err := Render(body, width)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out
}

// mustRenderEmphasis renders body with an emphasis term and fails on error.
func mustRenderEmphasis(t *testing.T, body string, width int, term string) Result {
	t.Helper()
	out, err := RenderWithEmphasis(body, width, term)
	if err != nil {
		t.Fatalf("RenderWithEmphasis: %v", err)
	}
	return out
}

// plainText strips ANSI styling so assertions can match on visible text.
func plainText(out Result) string {
	return ansi.Strip(out.Styled)
}

// linkTargets pulls the Target field out of each recorded link, in order.
func linkTargets(out Result) []string {
	targets := make([]string, 0, len(out.Links))
	for _, l := range out.Links {
		targets = append(targets, l.Target)
	}
	return targets
}

// rowOf returns the zero-based line number that a Styled byte offset lands on.
func rowOf(out Result, off int) int {
	return strings.Count(out.Styled[:off], "\n")
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

func TestRenderPageReturnsLinksWithTargets(t *testing.T) {
	// arrange
	body := "- See [[Alpha]] and [[Beta|the second]]."

	// act
	out := mustRender(t, body, 80)

	// assert
	want := []string{"Alpha", "Beta"}
	if got := linkTargets(out); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("link targets: want %v, got %v", want, got)
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
	// arrange / act
	out := mustRender(t, "", 80)

	// assert
	if len(out.Links) != 0 {
		t.Errorf("empty body should have no links, got %v", out.Links)
	}
}

func TestRenderStripsLogbookBlocks(t *testing.T) {
	// arrange
	body := strings.Join([]string{
		"- a normal bullet",
		"  :LOGBOOK:",
		"  CLOCK: [2026-05-21 Thu 15:38:56]--[2026-05-21 Thu 15:56:09] =>  00:17:13",
		"  :END:",
		"- another bullet",
	}, "\n")

	// act
	out := mustRender(t, body, 80)

	// assert
	plain := strings.Join(strings.Fields(plainText(out)), " ") // collapse Glamour's word-by-word spacing
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
	// arrange — a line whose wiki-link sits near the right margin: the link's
	// rendered display is much wider than the raw `<id>` sentinel, so without
	// width-padding Glamour wraps based on the sentinel and the substituted
	// link overflows the column budget.
	body := "- Some text leading up to [[VeryLongPageNameRightAtTheEnd]]"

	// act
	out := mustRender(t, body, 40)

	// assert
	plain := plainText(out)
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
	// arrange
	body := "- This bullet has enough text that Glamour will wrap it across two lines for sure."

	// act
	out := mustRender(t, body, 40)

	// assert
	plain := plainText(out)
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

func TestRenderPageBlockRefBecomesLink(t *testing.T) {
	// arrange / act
	out := mustRender(t, "see [[Alpha#summary]] for the upshot\n", 80)

	// assert
	if len(out.Links) != 1 {
		t.Fatalf("want 1 link, got %d (%+v)", len(out.Links), out.Links)
	}
	if out.Links[0].Target != "Alpha" {
		t.Errorf("target = %q, want Alpha", out.Links[0].Target)
	}
}

func TestRenderPageBlockRefWithAlias(t *testing.T) {
	// arrange / act
	out := mustRender(t, "see [[Alpha#summary|the summary]] for context\n", 80)

	// assert
	if len(out.Links) != 1 {
		t.Fatalf("want 1 link, got %d (%+v)", len(out.Links), out.Links)
	}
	if out.Links[0].Target != "Alpha" {
		t.Errorf("target = %q, want Alpha", out.Links[0].Target)
	}
	if out.Links[0].Display != "the summary" {
		t.Errorf("display = %q, want 'the summary'", out.Links[0].Display)
	}
}

func TestRenderPageEmptyBlockFragmentIsNotALink(t *testing.T) {
	// arrange / act
	out := mustRender(t, "anchor: [[#summary]] here\n", 80)

	// assert
	if len(out.Links) != 0 {
		t.Errorf("want 0 links for [[#anchor]], got %d (%+v)", len(out.Links), out.Links)
	}
}

func TestRenderPageStripsQueryAndEmbedBlocks(t *testing.T) {
	// arrange
	body := "before\n{{query (and [[tag]] )}}\nstill query\n}}\nafter\n{{embed [[Other]]}}\n"

	// act
	out := mustRender(t, body, 80)

	// assert
	plain := plainText(out)
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
	// arrange
	body := strings.Join([]string{
		"- TODO Buy milk",
		"- DOING Write the parser",
		"- DONE Old item",
	}, "\n")

	// act
	out := mustRender(t, body, 80)

	// assert
	plain := plainText(out)
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
	// wiki link. weft's preprocessor previously matched `[[...]]` inside
	// backticks; this test pins the fix.
	out := mustRender(t, "see `[[Foo]]` for the literal text\n", 80)

	if len(out.Links) != 0 {
		t.Errorf("want 0 links (wiki link inside backticks should be literal), got %d (%+v)", len(out.Links), out.Links)
	}
	if plain := plainText(out); !strings.Contains(plain, "[[Foo]]") {
		t.Errorf("expected literal [[Foo]] in output, got:\n%s", plain)
	}
}

func TestRenderPageMixedBacktickAndPlainLinks(t *testing.T) {
	// On a line that mixes backtick-wrapped and plain wiki links, only the
	// plain one should be preprocessed. The backtick-wrapped one stays
	// literal in the output.
	out := mustRender(t, "code `[[Fake]]` and real [[Real]] end\n", 80)

	if len(out.Links) != 1 {
		t.Fatalf("want 1 link (only the plain one), got %d (%+v)", len(out.Links), out.Links)
	}
	if out.Links[0].Target != "Real" {
		t.Errorf("link target = %q, want Real", out.Links[0].Target)
	}
	if plain := plainText(out); !strings.Contains(plain, "[[Fake]]") {
		t.Errorf("expected literal [[Fake]] to survive, got:\n%s", plain)
	}
}

func TestRenderPageMultipleInlineCodeSpansOnOneLine(t *testing.T) {
	// `code1` ... `code2` alternation: both code spans should be left
	// literal, and the plain wiki link between them preprocessed.
	out := mustRender(t, "first `[[A]]` middle [[B]] last `[[C]]` end\n", 80)

	if len(out.Links) != 1 {
		t.Fatalf("want 1 link (only [[B]]), got %d (%+v)", len(out.Links), out.Links)
	}
	if out.Links[0].Target != "B" {
		t.Errorf("link target = %q, want B", out.Links[0].Target)
	}
}

func TestHideMarkdownLinkURLs(t *testing.T) {
	// Rewrites [text](url) -> [text](#) so Glamour renders only the styled
	// link text; images, wiki links, and code-span examples are left alone.
	cases := []struct {
		name, in, want string
	}{
		{"basic link", "see [PR 17](https://x/y) done", "see [PR 17](#) done"},
		{"multiple links", "[a](u1) and [b](u2)", "[a](#) and [b](#)"},
		{"leaves image", "![alt](http://img/x.png)", "![alt](http://img/x.png)"},
		{"leaves wiki link", "a [[Wiki]] link", "a [[Wiki]] link"},
		{"inline-code example is literal", "`[x](http://y)` stays", "`[x](http://y)` stays"},
		{"plain text unchanged", "no links here", "no links here"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hideMarkdownLinkURLs(tc.in); got != tc.want {
				t.Errorf("hideMarkdownLinkURLs(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHideMarkdownLinkURLsSkipsFencedCode(t *testing.T) {
	// A markdown link inside a fenced code block is a literal example and must
	// keep its URL verbatim.
	body := "```\nsee [x](http://y) in code\n```\n"
	if got := hideMarkdownLinkURLs(body); !strings.Contains(got, "[x](http://y)") {
		t.Errorf("expected fenced markdown link to survive, got:\n%s", got)
	}
}

func TestRenderMarkdownLinkHidesURL(t *testing.T) {
	// End-to-end: the rendered read view shows the link text but not the URL.
	out := mustRender(t, "see [PR 17](https://git.fiatcode.dev/fiatcode/weft/pulls/17) done\n", 80)

	plain := plainText(out)
	if !strings.Contains(plain, "PR 17") {
		t.Errorf("expected link text \"PR 17\" in output, got:\n%s", plain)
	}
	if strings.Contains(plain, "https://") {
		t.Errorf("expected URL hidden, but it appears in output:\n%s", plain)
	}
}

func TestRenderRecordsOpenTaskPositions(t *testing.T) {
	// arrange
	body := strings.Join([]string{
		"- DONE finished thing",
		"- TODO first open",
		"- some note",
		"- LATER second open",
	}, "\n")

	// act
	out := mustRender(t, body, 80)

	// assert — DONE is excluded; the two open markers are recorded in document order.
	if len(out.Tasks) != 2 {
		t.Fatalf("open task count: want 2, got %d (%v)", len(out.Tasks), out.Tasks)
	}
	r0, r1 := rowOf(out, out.Tasks[0]), rowOf(out, out.Tasks[1])
	if r0 >= r1 {
		t.Errorf("task rows not ascending: %d, %d", r0, r1)
	}
	lines := strings.Split(plainText(out), "\n")
	if r0 >= len(lines) || !strings.Contains(lines[r0], "first open") {
		t.Errorf("task 0 offset lands on wrong row %d: %q", r0, lines)
	}
	if r1 >= len(lines) || !strings.Contains(lines[r1], "second open") {
		t.Errorf("task 1 offset lands on wrong row %d: %q", r1, lines)
	}
}

func TestRenderOpenTaskExcludesPunctuationAdjacentMarker(t *testing.T) {
	// arrange — "- TODO: x" is NOT a todo per graph.ExtractTodos (it requires
	// whitespace after the marker), so it must not be recorded in Tasks —
	// otherwise the deep-link ordinal misaligns. Only "- TODO buy milk" counts.
	body := strings.Join([]string{
		"- TODO: not a real todo",
		"- TODO buy milk",
	}, "\n")

	// act
	out := mustRender(t, body, 80)

	// assert
	if len(out.Tasks) != 1 {
		t.Fatalf("open task count: want 1 (only the whitespace-separated todo), got %d", len(out.Tasks))
	}
	row := rowOf(out, out.Tasks[0])
	lines := strings.Split(plainText(out), "\n")
	if row >= len(lines) || !strings.Contains(lines[row], "buy milk") {
		t.Errorf("recorded task offset lands on wrong row %d: %q", row, lines)
	}
}

func TestRenderOpenTaskOrdinalAlignmentWithInterleavedAndPriority(t *testing.T) {
	// arrange — open-todo offsets are recorded in document order, skipping DONE
	// even when it sits between two open todos, and a priority marker is still
	// counted — keeping render's Tasks index aligned with graph's ordinal.
	body := strings.Join([]string{
		"- TODO [#A] first with priority",
		"- DONE done in the middle",
		"- LATER third open",
	}, "\n")

	// act
	out := mustRender(t, body, 80)

	// assert
	if len(out.Tasks) != 2 {
		t.Fatalf("open task count: want 2 (DONE skipped), got %d", len(out.Tasks))
	}
	lines := strings.Split(plainText(out), "\n")
	rowText := func(off int) string {
		r := rowOf(out, off)
		if r >= len(lines) {
			return ""
		}
		return lines[r]
	}
	if !strings.Contains(rowText(out.Tasks[0]), "first with priority") {
		t.Errorf("task 0 lands on wrong row: %q", rowText(out.Tasks[0]))
	}
	if !strings.Contains(rowText(out.Tasks[1]), "third open") {
		t.Errorf("task 1 lands on wrong row: %q", rowText(out.Tasks[1]))
	}
}

func TestRenderWithEmphasisRecordsFinds(t *testing.T) {
	// arrange / act
	out := mustRenderEmphasis(t, "see Alpha here\n", 80, "Alpha")

	// assert
	if len(out.Finds) != 1 {
		t.Fatalf("want 1 find, got %d", len(out.Finds))
	}
	if !strings.Contains(out.Styled, "Alpha") {
		t.Errorf("emphasised term should still appear in output")
	}
}

func TestRenderNoEmphasisNoFinds(t *testing.T) {
	// arrange / act
	out := mustRender(t, "see Alpha here\n", 80)

	// assert
	if len(out.Finds) != 0 {
		t.Errorf("Render should record no finds; got %d", len(out.Finds))
	}
}

func TestRenderWithEmphasisSkipsLinkCodeFence(t *testing.T) {
	// arrange
	body := "bare Alpha here\n" +
		"a [[Alpha]] link\n" +
		"inline `Alpha` code\n" +
		"```\nAlpha in fence\n```\n"

	// act
	out := mustRenderEmphasis(t, body, 80, "Alpha")

	// assert
	if len(out.Finds) != 1 {
		t.Fatalf("only the bare mention should be highlighted; got %d finds", len(out.Finds))
	}
}

func TestRenderWithEmphasisWholeWordCasePreserved(t *testing.T) {
	// arrange / act
	out := mustRenderEmphasis(t, "an alpha and Alphabet\n", 80, "Alpha")

	// assert
	if len(out.Finds) != 1 {
		t.Fatalf("want 1 find (alpha, not Alphabet); got %d", len(out.Finds))
	}
	if !strings.Contains(out.Styled, "alpha") {
		t.Errorf("original casing 'alpha' must be preserved in output")
	}
	if !strings.Contains(out.Styled, "Alphabet") {
		t.Errorf("Alphabet must remain in output untouched")
	}
}

func TestRenderWithEmphasisUnicodeAndPunctuationBoundaries(t *testing.T) {
	cases := []struct {
		name, body, term string
	}{
		{"punctuation-bounded", "I use C++ daily\n", "C++"},
		{"dotted", "built on .NET here\n", ".NET"},
		{"accented", "visited Über today\n", "Über"},
		{"cjk", "studied 日本 now\n", "日本"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := mustRenderEmphasis(t, tc.body, 80, tc.term)
			if len(out.Finds) != 1 {
				t.Errorf("term %q should highlight once (matching ripgrep -w); got %d finds", tc.term, len(out.Finds))
			}
		})
	}
}

func TestRenderWithEmphasisStillWholeWord(t *testing.T) {
	// Whole-word still holds: substring occurrences don't match.
	out := mustRenderEmphasis(t, "Alphabet and alpha\n", 80, "Alpha")

	if len(out.Finds) != 1 { // only the standalone "alpha"
		t.Fatalf("want 1 find (whole-word), got %d", len(out.Finds))
	}
}

func TestRenderWithEmphasisRespectsWrapWidth(t *testing.T) {
	// arrange — the emphasised term sits near the wrap boundary; without
	// sentinel padding Glamour wraps on the short sentinel and the restored
	// term overflows the right margin.
	body := "This is some padding text: Remarkable end\n"

	// act
	out := mustRenderEmphasis(t, body, 40, "Remarkable")

	// assert
	maxw := 0
	for _, line := range strings.Split(out.Styled, "\n") {
		if w := lipgloss.Width(line); w > maxw {
			maxw = w
		}
	}
	if maxw > 40 {
		t.Errorf("emphasis render overflows wrap width: max line %d > 40", maxw)
	}
}
