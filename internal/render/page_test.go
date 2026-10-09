package render

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
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
		if !strings.Contains(ansi.Strip(span), "the second") {
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

func TestLogbookBlockContainingFenceLineIsFullyStripped(t *testing.T) {
	// arrange — a fence delimiter inside :LOGBOOK: metadata must not
	// flip fence state for the rest of the page
	body := "- item\n  :LOGBOOK:\n  ```\n  :END:\n- [[After]]\n"

	// act
	res, err := Render(body, 80)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Styled, ":END:") {
		t.Fatal(":LOGBOOK: block leaked into the rendered page")
	}
	if len(res.Links) != 1 || res.Links[0].Target != "After" {
		t.Fatalf("links = %+v, want [[After]]", res.Links)
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

// A task marker renders as its literal text (WAITING = 7 cells, CANCELLED
// = 9) but its sentinel is ~3 cells, so Glamour under-reserves and the
// restored line can overflow the wrap width — spurious soft-wraps and
// ScrollToTask row drift. The sentinel must pad to the marker's width like
// the wiki-link and emphasis sentinels do.
func TestRenderTaskMarkerWrapsAtRightMargin(t *testing.T) {
	for _, marker := range []string{"WAITING", "CANCELLED"} {
		body := "- " + marker + " alpha beta gamma delta epsilon zeta eta theta iota\n"
		res := mustRender(t, body, 40)
		for _, line := range strings.Split(res.Styled, "\n") {
			if w := lipgloss.Width(line); w > 40 {
				t.Errorf("%s: line is %d cells wide, want <= 40: %q", marker, w, line)
			}
		}
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
	// Logseq has no [[page#fragment]] syntax and allows "#" in page names,
	// so "#" and everything after it must stay part of the target.
	// arrange / act
	out := mustRender(t, "see [[Alpha#summary]] for the upshot\n", 80)

	// assert
	if len(out.Links) != 1 {
		t.Fatalf("want 1 link, got %d (%+v)", len(out.Links), out.Links)
	}
	if out.Links[0].Target != "Alpha#summary" {
		t.Errorf("target = %q, want Alpha#summary", out.Links[0].Target)
	}
}

func TestRenderPageBlockRefWithAlias(t *testing.T) {
	// arrange / act
	out := mustRender(t, "see [[Alpha#summary|the summary]] for context\n", 80)

	// assert
	if len(out.Links) != 1 {
		t.Fatalf("want 1 link, got %d (%+v)", len(out.Links), out.Links)
	}
	if out.Links[0].Target != "Alpha#summary" {
		t.Errorf("target = %q, want Alpha#summary", out.Links[0].Target)
	}
	if out.Links[0].Display != "the summary" {
		t.Errorf("display = %q, want 'the summary'", out.Links[0].Display)
	}
}

func TestRenderLinkTargetKeepsHash(t *testing.T) {
	// arrange / act
	res := mustRender(t, "- [[C#]]\n", 80)

	// assert
	if len(res.Links) != 1 || res.Links[0].Target != "C#" {
		t.Fatalf("links = %+v, want target C#", res.Links)
	}
}

func TestRenderPageHashOnlyTargetIsADanglingLink(t *testing.T) {
	// [[#summary]] has no [[page#fragment]] meaning in Logseq, so it renders
	// as a normal (likely dangling) link named "#summary", not as literal
	// text.
	// arrange / act
	out := mustRender(t, "anchor: [[#summary]] here\n", 80)

	// assert
	if len(out.Links) != 1 {
		t.Fatalf("want 1 link for [[#summary]], got %d (%+v)", len(out.Links), out.Links)
	}
	if out.Links[0].Target != "#summary" {
		t.Errorf("target = %q, want #summary", out.Links[0].Target)
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

func TestRenderSingleLineQueryOrEmbedDropsOnlyItsLine(t *testing.T) {
	// arrange: single-line embed and query, with real content after them.
	body := "{{embed [[Other]]}}\n\n- TODO one\n- TODO two\n\n{{query (todo)}}\nSee [[Somewhere]] for details.\n"

	// act
	out := mustRender(t, body, 80)

	// assert
	plain := plainText(out)
	if strings.Contains(plain, "{{embed") || strings.Contains(plain, "{{query") {
		t.Errorf("marker line leaked into output:\n%s", plain)
	}
	for _, want := range []string{"one", "two", "Somewhere"} {
		if !strings.Contains(plain, want) {
			t.Errorf("content after a single-line block was dropped (missing %q):\n%s", want, plain)
		}
	}
	if len(out.Tasks) != 2 {
		t.Errorf("Tasks = %d, want 2 (todos-dashboard deep-link ordinals depend on this)", len(out.Tasks))
	}
	if len(out.Links) != 1 {
		t.Errorf("Links = %d, want 1 ([[Somewhere]])", len(out.Links))
	}
}

func TestRenderSingleLineQueryOrEmbedWithTrailingWhitespace(t *testing.T) {
	// arrange: single-line marker with trailing whitespace after }}.
	body := "{{embed [[Other]]}}  \t\n- TODO item\nSee [[Link]].\n"

	// act
	out := mustRender(t, body, 80)

	// assert
	plain := plainText(out)
	if strings.Contains(plain, "{{embed") {
		t.Errorf("marker line with trailing whitespace leaked into output:\n%s", plain)
	}
	if !strings.Contains(plain, "item") || !strings.Contains(plain, "Link") {
		t.Errorf("content after whitespace-trailing marker was dropped:\n%s", plain)
	}
	if len(out.Tasks) != 1 {
		t.Errorf("Tasks = %d, want 1", len(out.Tasks))
	}
	if len(out.Links) != 1 {
		t.Errorf("Links = %d, want 1", len(out.Links))
	}
}

func TestRenderSingleLineQueryOrEmbedWithCRLF(t *testing.T) {
	// arrange: single-line marker in CRLF body (Windows line endings).
	// This tests the trim semantics: TrimSpace must remove \r, not just spaces/tabs.
	body := "{{embed [[Other]]}}\r\n- TODO item\r\nSee [[Link]].\r\n"

	// act
	out := mustRender(t, body, 80)

	// assert
	plain := plainText(out)
	if strings.Contains(plain, "{{embed") {
		t.Errorf("CRLF marker line leaked into output:\n%s", plain)
	}
	if !strings.Contains(plain, "item") || !strings.Contains(plain, "Link") {
		t.Errorf("content after CRLF marker was dropped:\n%s", plain)
	}
	if len(out.Tasks) != 1 {
		t.Errorf("Tasks = %d, want 1 (CRLF line ending handling)", len(out.Tasks))
	}
	if len(out.Links) != 1 {
		t.Errorf("Links = %d, want 1 (CRLF line ending handling)", len(out.Links))
	}
}

func TestRenderSingleLineQueryOrEmbedWithTrailingText(t *testing.T) {
	// arrange: opener and closer on the same line, but with trailing prose
	// after the closing }} — not just whitespace. Without a same-line
	// scan for }} anywhere after the opener, this used to be mistaken for
	// an unclosed block and swallow the rest of the page.
	body := "{{embed [[X]]}} trailing words\n- TODO item\nSee [[Link]].\n"

	// act
	out := mustRender(t, body, 80)

	// assert
	plain := plainText(out)
	if strings.Contains(plain, "{{embed") {
		t.Errorf("marker line with trailing text leaked into output:\n%s", plain)
	}
	if !strings.Contains(plain, "item") || !strings.Contains(plain, "Link") {
		t.Errorf("content after a same-line embed with trailing text was dropped:\n%s", plain)
	}
	if len(out.Tasks) != 1 {
		t.Errorf("Tasks = %d, want 1", len(out.Tasks))
	}
	if len(out.Links) != 1 {
		t.Errorf("Links = %d, want 1", len(out.Links))
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

func TestHideMarkdownLinkURLsBalancedParens(t *testing.T) {
	got := hideMarkdownLinkURLs("see [Go](https://en.wikipedia.org/wiki/Go_(programming_language)) now\n")
	want := "see [Go](#) now\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHideMarkdownLinkURLsWithCodeSpanInText(t *testing.T) {
	// arrange — the link text contains a code span, but the whole thing is
	// still a real link: splitting the line on backticks first would sever
	// it and leave the URL exposed.
	in := "see [the `go` docs](https://go.dev/doc) now\n"
	want := "see [the `go` docs](#) now\n"

	// act
	got := hideMarkdownLinkURLs(in)

	// assert
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHideMarkdownLinkURLsLeavesLiteralExampleInCode(t *testing.T) {
	// arrange — a markdown-link-shaped example fully inside a code span is
	// literal text and must stay untouched.
	in := "type `[x](http://y)` literally\n"

	// act
	got := hideMarkdownLinkURLs(in)

	// assert
	if got != in {
		t.Fatalf("literal example inside inline code was rewritten: %q", got)
	}
}

func TestRenderMarkdownLinkHidesURL(t *testing.T) {
	// End-to-end: the rendered read view shows the link text but not the URL.
	out := mustRender(t, "see [PR 17](https://github.com/fiatcode-gh/weft/pulls/17) done\n", 80)

	plain := plainText(out)
	if !strings.Contains(plain, "PR 17") {
		t.Errorf("expected link text \"PR 17\" in output, got:\n%s", plain)
	}
	if strings.Contains(plain, "https://") {
		t.Errorf("expected URL hidden, but it appears in output:\n%s", plain)
	}
}

func TestPreprocessTaskMarkersUsesGraphOpenTaskPredicate(t *testing.T) {
	// arrange
	lines := []string{
		"- TODO buy milk",
		"  - WAITING [#B] vendor",
		"- DONE shipped",
		"- TODO: not a task",
		"- LATER   ",
	}

	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			// act
			_, markers := preprocessTaskMarkers(line, 0)

			// assert
			if len(markers) != 1 {
				t.Fatalf("marker count = %d, want 1", len(markers))
			}
			if got, want := markers[0].open, graph.IsOpenTask(line); got != want {
				t.Errorf("render open = %v, graph open = %v", got, want)
			}
		})
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

func TestRenderWithEmphasisNumericTermKeepsSentinels(t *testing.T) {
	// arrange / act — searching a bare number must not corrupt the
	// digit-encoded ids inside wiki/task sentinels
	res, err := RenderWithEmphasis("- TODO check [[Foo]] version 0\n", 80, "0")

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 || res.Links[0].Target != "Foo" {
		t.Fatalf("links = %+v, want [[Foo]] intact", res.Links)
	}
	if len(res.Tasks) != 1 {
		t.Fatalf("tasks = %v, want 1", res.Tasks)
	}
	if len(res.Finds) != 1 {
		t.Fatalf("finds = %v, want exactly the bare 0", res.Finds)
	}
	for _, r := range res.Styled {
		if r >= '' && r <= '' {
			t.Fatalf("raw PUA rune %U leaked into styled output", r)
		}
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

func TestRenderWithEmphasisRejectsMissingLeftWordBoundary(t *testing.T) {
	// arrange
	body := "xAlpha Alpha\n"

	// act
	out := mustRenderEmphasis(t, body, 80, "Alpha")

	// assert
	if len(out.Finds) != 1 {
		t.Fatalf("want only the standalone Alpha emphasized, got %d finds", len(out.Finds))
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

func TestRenderSkipsTildeAndBulletFences(t *testing.T) {
	for name, body := range map[string]string{
		"tilde":  "~~~\nsee [[Foo]] here\n~~~\n",
		"bullet": "- ```\n  see [[Foo]] here\n  ```\n",
	} {
		t.Run(name, func(t *testing.T) {
			res, err := Render(body, 80)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Links) != 0 {
				t.Fatalf("fenced [[Foo]] became a link: %+v", res.Links)
			}
		})
	}
}

// Alignment: the same corpus must be fenced identically for the index
// (graph.ExtractLinks) and the read view (Render). This is the
// guard against the two grammars drifting again.
func TestFenceGrammarAlignsWithGraph(t *testing.T) {
	corpus := "- ```\n  [[A]]\n  ```\n~~~\n[[B]]\n~~~\n- [[C]] `[[D]]` text\n"
	res, err := Render(corpus, 80)
	if err != nil {
		t.Fatal(err)
	}
	var rendered []string
	for _, l := range res.Links {
		rendered = append(rendered, l.Target)
	}
	var indexed []string
	for _, h := range graph.ExtractLinks(corpus) {
		indexed = append(indexed, h.Target)
	}
	if !reflect.DeepEqual(rendered, indexed) {
		t.Fatalf("render links %v != graph links %v", rendered, indexed)
	}
}

// corpusLinks is Corpus.md's links in document order (target, display).
var corpusLinks = [][2]string{
	{"kitchen", "#kitchen"}, {"Kitchen", "#Kitchen"}, {"s", "#s"},
	{"kb/notes", "#kb/notes"}, {"Book Club", "#Book Club"}, {"Book Club", "#the club"},
	{"café", "#café"}, {"日本", "#日本"}, {"add", "#add"}, {"cafe", "#cafe"},
	{"bad", "#bad"}, {"abc12", "#abc12"}, {"a1b2c3d4e", "#a1b2c3d4e"},
	{"C#", "C#"}, {"Lab #inner", "Lab #inner"}, {"inheading", "#inheading"},
	{"lead", "#lead"},
}

func readCorpus(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/fixture-graph/pages/Corpus.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRenderTags(t *testing.T) {
	res := mustRender(t, readCorpus(t), 100)

	var got [][2]string
	for _, l := range res.Links {
		got = append(got, [2]string{l.Target, l.Display})
		if span := ansi.Strip(res.Styled[l.Start:l.End]); span != l.Display {
			t.Errorf("link %q: styled span = %q", l.Target, span)
		}
	}
	if !reflect.DeepEqual(got, corpusLinks) {
		t.Fatalf("links = %q\nwant    %q", got, corpusLinks)
	}
	out := ansi.Strip(res.Styled)
	if strings.Contains(out, "#[[") {
		t.Errorf("bracket tag left raw in output:\n%s", out)
	}
	for _, lit := range []string{"C#", "repo#12", "#18", "#FAF3E7", "#incode", "#fenced"} {
		if !strings.Contains(out, lit) {
			t.Errorf("literal non-tag %q missing from output:\n%s", lit, out)
		}
	}
}

// Alignment: the index (graph.ExtractLinks) and the read view (Render) must
// find the same links, tags included.
func TestLinkGrammarAgreesAcrossConsumers(t *testing.T) {
	var corpusTargets []string
	for _, l := range corpusLinks {
		corpusTargets = append(corpusTargets, l[0])
	}
	cases := []struct {
		name, body string
		want       []string
	}{
		{"Corpus", readCorpus(t), corpusTargets},
		{"inline", "- #a [[B]] #[[C|c]] `#d` (#e) x#f\n", []string{"a", "B", "C", "e"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rendered, indexed []string
			for _, l := range mustRender(t, tc.body, 100).Links {
				rendered = append(rendered, l.Target)
			}
			for _, h := range graph.ExtractLinks(tc.body) {
				indexed = append(indexed, h.Target)
			}
			if !reflect.DeepEqual(rendered, tc.want) || !reflect.DeepEqual(indexed, tc.want) {
				t.Fatalf("render %q, index %q, want %q", rendered, indexed, tc.want)
			}

			// The source painter is the fourth consumer: per line outside
			// fences, its '[' spans are the wiki links and its '#' spans the
			// simple tags and bracket-tag hashes.
			var fence graph.FenceState
			for n, line := range strings.Split(tc.body, "\n") {
				if fence.Step(line) {
					continue
				}
				_, spans := maskLinks(line)
				var brackets, hashes, wantHashes []int
				for _, s := range spans {
					switch line[s.start] {
					case '[':
						brackets = append(brackets, s.start)
					case '#':
						hashes = append(hashes, s.start)
					}
				}
				simple := 0
				for _, tg := range graph.FindTags(line) {
					wantHashes = append(wantHashes, tg.Start)
					if !tg.Bracket {
						simple++
					}
				}
				if want := len(graph.ExtractLinks(line)) - simple; len(brackets) != want {
					t.Errorf("line %d %q: painter has %d wiki spans, index %d", n+1, line, len(brackets), want)
				}
				if !reflect.DeepEqual(hashes, wantHashes) {
					t.Errorf("line %d %q: painter tag starts %v, FindTags %v", n+1, line, hashes, wantHashes)
				}
			}
		})
	}
}

func TestRenderTagsWrapAtWidth(t *testing.T) {
	res := mustRender(t, "- "+strings.Repeat("word ", 8)+"#topic", 20)

	for i, row := range strings.Split(res.Styled, "\n") {
		if w := lipgloss.Width(row); w > 20 {
			t.Errorf("row %d is %d cells wide: %q", i, w, ansi.Strip(row))
		}
	}
}

// A Glamour failure falls back to un-styled text — that must be visible to
// the caller (FallbackErr) so it can skip its cache: a transient failure
// cached by mtime becomes a sticky unstyled page.
func TestRenderReportsGlamourFallback(t *testing.T) {
	// arrange — no input reliably makes Glamour fail, so swap the seam
	orig := glamourRender
	t.Cleanup(func() { glamourRender = orig })
	glamourRender = func(*glamour.TermRenderer, string) (string, error) {
		return "", errors.New("boom")
	}

	// act
	res, err := Render("- TODO hello [[Alpha]]\n", 40)

	// assert
	if err != nil {
		t.Fatalf("fallback must not be an error: %v", err)
	}
	if res.FallbackErr == nil {
		t.Fatal("expected FallbackErr to record the Glamour failure")
	}
	if !strings.Contains(res.Styled, "hello") {
		t.Fatalf("fallback should still carry the page text, got:\n%s", res.Styled)
	}
}

func TestRenderStripsHyperlinks(t *testing.T) {
	body := "- see [the docs][d] and <https://auto.example.com>\n\n[d]: https://example.com/docs\n"

	res := mustRender(t, body, 80)

	if strings.Contains(res.Styled, "\x1b]8;") {
		t.Errorf("Styled contains an OSC 8 hyperlink: %q", res.Styled)
	}
	plain := ansi.Strip(res.Styled)
	for _, want := range []string{"the docs", "https://auto.example.com"} {
		if !strings.Contains(plain, want) {
			t.Errorf("text %q missing from %q", want, plain)
		}
	}
	if got, want := len(sourceRowsOnly(body, 80, "")), strings.Count(res.Styled, "\n")+1; got != want {
		t.Errorf("SourceRows = %d, styled rows = %d", got, want)
	}
}

func TestRenderOverWideSentinelLeavesNoPadRunes(t *testing.T) {
	const longName = "A very long page name that is longer than the column width"
	cases := []struct {
		name     string
		body     string
		width    int
		emphasis string
		check    func(t *testing.T, res Result)
	}{
		{"wiki", "- see [[" + longName + "]] ok\n", 30, "", func(t *testing.T, res Result) {
			if len(res.Links) != 1 {
				t.Fatalf("links = %d, want 1", len(res.Links))
			}
			l := res.Links[0]
			if l.Display != longName {
				t.Errorf("Display = %q, want %q", l.Display, longName)
			}
			if got := ansi.Strip(res.Styled[l.Start:l.End]); got != longName {
				t.Errorf("styled span = %q, want %q", got, longName)
			}
		}},
		{"tag", "- see #" + strings.Repeat("abcdefghij", 4) + " ok\n", 30, "", func(t *testing.T, res Result) {
			name := strings.Repeat("abcdefghij", 4)
			if len(res.Links) != 1 {
				t.Fatalf("links = %d, want 1", len(res.Links))
			}
			l := res.Links[0]
			if l.Display != "#"+name {
				t.Errorf("Display = %q, want %q", l.Display, "#"+name)
			}
			if got := ansi.Strip(res.Styled[l.Start:l.End]); got != l.Display {
				t.Errorf("styled span = %q, want %q", got, l.Display)
			}
		}},
		{"emphasis", "- averyveryveryveryveryverylongword here\n", 20, "averyveryveryveryveryverylongword", func(t *testing.T, res Result) {
			if len(res.Finds) != 1 {
				t.Errorf("finds = %d, want 1", len(res.Finds))
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := mustRenderEmphasis(t, tc.body, tc.width, tc.emphasis)

			if i := strings.IndexAny(res.Styled, "\ue004\ue007\ue008"); i >= 0 {
				t.Errorf("pad rune at byte %d in %q", i, res.Styled)
			}
			if got, want := len(sourceRowsOnly(tc.body, tc.width, tc.emphasis)), strings.Count(res.Styled, "\n")+1; got != want {
				t.Errorf("SourceRows = %d, styled rows = %d", got, want)
			}
			tc.check(t, res)
		})
	}
}

func TestRenderStylesPriority(t *testing.T) {
	theme := mustTheme(t)
	out := mustRender(t, "- LATER [#A] prio\n", 80)
	if want := theme.Priority["A"].Render("[#A]"); !strings.Contains(out.Styled, want) {
		t.Errorf("priority not styled: want %q in %q", want, out.Styled)
	}
	if len(out.Tasks) != 1 {
		t.Errorf("Tasks = %d, want 1", len(out.Tasks))
	}

	// the text is unchanged: the marker and priority stay in the stripped text
	if got := strings.TrimSpace(plainText(out)); got != "• LATER [#A] prio" {
		t.Errorf("stripped text = %q", got)
	}

	tab := mustRender(t, "- LATER\t[#A] x\n", 80)
	if strings.Contains(tab.Styled, theme.Priority["A"].Render("[#A]")) {
		t.Errorf("tab-separated priority must stay unstyled: %q", tab.Styled)
	}
}

func TestRenderStylesStampLines(t *testing.T) {
	theme := mustTheme(t)
	const sched = "SCHEDULED: <2026-05-25 Mon 09:30 .+1w>"
	const dead = "DEADLINE: <2026-05-27 Wed>"
	body := "- TODO x\n  " + sched + "\n  " + dead + "\n"
	out := mustRender(t, body, 80)
	for _, want := range []string{theme.Scheduled.Render(sched), theme.Deadline.Render(dead)} {
		if !strings.Contains(out.Styled, want) {
			t.Errorf("stamp not styled: want %q in %q", want, out.Styled)
		}
	}
	if !strings.Contains(plainText(out), sched) || !strings.Contains(plainText(out), dead) {
		t.Errorf("stamp text changed: %q", plainText(out))
	}
	if len(out.Tasks) != 1 {
		t.Errorf("Tasks = %d, want 1", len(out.Tasks))
	}

	crlf := mustRender(t, strings.ReplaceAll(body, "\n", "\r\n"), 80)
	if !strings.Contains(crlf.Styled, theme.Scheduled.Render(sched)) {
		t.Errorf("CRLF stamp not styled: %q", crlf.Styled)
	}

	for name, doc := range map[string]string{
		"malformed": "- TODO x\n  SCHEDULED: <2026-02-30>\n",
		"fence":     "```\nSCHEDULED: <2026-05-25 Mon>\n```\n",
	} {
		got := mustRender(t, doc, 80)
		if strings.Contains(got.Styled, theme.Scheduled.Render("SCHEDULED: <2026-05-25 Mon>")) ||
			strings.Contains(got.Styled, theme.Scheduled.Render("SCHEDULED: <2026-02-30>")) {
			t.Errorf("%s: stamp styled: %q", name, got.Styled)
		}
	}

	log := mustRender(t, "- TODO x\n  :LOGBOOK:\n  SCHEDULED: <2026-05-25 Mon>\n  :END:\n", 80)
	if strings.Contains(plainText(log), "SCHEDULED") {
		t.Errorf("logbook stamp leaked: %q", plainText(log))
	}
}
