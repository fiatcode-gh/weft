package views

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestTintLineNormalUnchanged(t *testing.T) {
	in := "just a plain paragraph"
	if got := tintLine(in); got != in {
		t.Fatalf("normal line should be unchanged; got %q", got)
	}
}

func TestTintLineHeadingIsBold(t *testing.T) {
	got := tintLine("# Heading")
	want := lipgloss.NewStyle().Bold(true).Render("# Heading")
	if got != want {
		t.Fatalf("heading not bold:\n got %q\nwant %q", got, want)
	}
}

func TestTintLineBlockquoteIsFaint(t *testing.T) {
	got := tintLine("> a quote")
	want := lipgloss.NewStyle().Faint(true).Render("> a quote")
	if got != want {
		t.Fatalf("blockquote not faint:\n got %q\nwant %q", got, want)
	}
}

func TestTintLineCodeFenceIsFaint(t *testing.T) {
	got := tintLine("```go")
	want := lipgloss.NewStyle().Faint(true).Render("```go")
	if got != want {
		t.Fatalf("code fence not faint:\n got %q\nwant %q", got, want)
	}
}

func TestTintLineWikiLinkTinted(t *testing.T) {
	got := tintLine("see [[Foo Bar]] now")
	link := lipgloss.NewStyle().Foreground(colorHighlight).Render("[[Foo Bar]]")
	if !strings.Contains(got, link) {
		t.Fatalf("wiki link span not tinted:\n got %q\nwant substring %q", got, link)
	}
	if !strings.HasPrefix(got, "see ") || !strings.HasSuffix(got, " now") {
		t.Fatalf("text around link altered: %q", got)
	}
}

func TestTintLineTaskMarkerColored(t *testing.T) {
	got := tintLine("- TODO buy milk")
	marker := editorMarkerTint["TODO"].Render("TODO")
	if !strings.Contains(got, marker) {
		t.Fatalf("task marker not colored:\n got %q\nwant substring %q", got, marker)
	}
	if !strings.HasPrefix(got, "- ") {
		t.Fatalf("bullet prefix altered: %q", got)
	}
}

func TestTintLineHeadingWithLinkComposes(t *testing.T) {
	got := tintLine("# See [[Foo]]")
	// The link span must be both bold (heading base) and link-colored.
	want := lipgloss.NewStyle().Foreground(colorHighlight).Bold(true).Render("[[Foo]]")
	if !strings.Contains(got, want) {
		t.Fatalf("heading+link did not compose bold+color:\n got %q\nwant substring %q", got, want)
	}
}

func TestTintViewSkipsRowsWithSGR(t *testing.T) {
	cursorRow := lipgloss.NewStyle().Background(lipgloss.Color("0")).Render("# Heading on cursor row")
	in := "# normal heading\n" + cursorRow
	out := tintView(in)
	lines := strings.Split(out, "\n")
	if lines[1] != cursorRow {
		t.Fatalf("cursor row (pre-styled) must pass through verbatim:\n got %q\nwant %q", lines[1], cursorRow)
	}
	if lines[0] != lineToBold("# normal heading") {
		t.Fatalf("non-cursor heading should be tinted; got %q", lines[0])
	}
}

// lineToBold mirrors the expected heading render for the assertion above.
func lineToBold(s string) string { return lipgloss.NewStyle().Bold(true).Render(s) }
