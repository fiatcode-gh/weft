package graph

import (
	"errors"
	"testing"

	"github.com/fiatcode-gh/weft/v2/internal/search"
)

// linkifyOK runs LinkifyMention and fails the test if it returns an error,
// returning the rewritten body and span for the caller to assert on.
func linkifyOK(t *testing.T, body string, line int, target string) (string, search.Span) {
	t.Helper()
	got, span, err := LinkifyMention(body, line, target)
	if err != nil {
		t.Fatalf("LinkifyMention(%q, %d, %q): unexpected error: %v", body, line, target, err)
	}
	return got, span
}

// assertMentionNotFound fails the test unless LinkifyMention reports
// ErrMentionNotFound for the given input.
func assertMentionNotFound(t *testing.T, body string, line int, target string) {
	t.Helper()
	if _, _, err := LinkifyMention(body, line, target); !errors.Is(err, ErrMentionNotFound) {
		t.Errorf("LinkifyMention(%q, %d, %q): err = %v, want ErrMentionNotFound", body, line, target, err)
	}
}

func TestLinkifyMentionHappyPath(t *testing.T) {
	body := "line one\nthe Alpha ship date\nline three\n"
	got, span := linkifyOK(t, body, 2, "Alpha")

	if want := "line one\nthe [[Alpha]] ship date\nline three\n"; got != want {
		t.Errorf("body =\n%q\nwant\n%q", got, want)
	}
	if body[span.Start:span.End] != "Alpha" {
		t.Errorf("span %v points at %q, want \"Alpha\"", span, body[span.Start:span.End])
	}
}

func TestLinkifyMentionPreservesCasing(t *testing.T) {
	got, _ := linkifyOK(t, "a bare alpha here\n", 1, "Alpha")
	if want := "a bare [[alpha]] here\n"; got != want {
		t.Errorf("got %q, want %q (author's casing must be preserved)", got, want)
	}
}

func TestLinkifyMentionPreservesCRLF(t *testing.T) {
	got, _ := linkifyOK(t, "intro\r\nthe Alpha ship\r\nend\r\n", 2, "Alpha")
	if want := "intro\r\nthe [[Alpha]] ship\r\nend\r\n"; got != want {
		t.Errorf("got %q, want %q (CRLF must survive)", got, want)
	}
}

func TestLinkifyMentionNoTrailingNewline(t *testing.T) {
	got, _ := linkifyOK(t, "only Alpha", 1, "Alpha")
	if want := "only [[Alpha]]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLinkifyMentionFirstOccurrenceWins(t *testing.T) {
	got, _ := linkifyOK(t, "Alpha and Alpha again\n", 1, "Alpha")
	if want := "[[Alpha]] and Alpha again\n"; got != want {
		t.Errorf("got %q, want %q (only the first occurrence is wrapped)", got, want)
	}
}

func TestLinkifyMentionSkipsAlreadyLinked(t *testing.T) {
	assertMentionNotFound(t, "see [[Alpha]] there\n", 1, "Alpha") // must not double-wrap
}

func TestLinkifyMentionSkipsLinkedTakesNextBare(t *testing.T) {
	got, _ := linkifyOK(t, "[[Alpha]] then bare Alpha\n", 1, "Alpha")
	if want := "[[Alpha]] then bare [[Alpha]]\n"; got != want {
		t.Errorf("got %q, want %q (skip the linked one, wrap the bare one)", got, want)
	}
}

func TestLinkifyMentionWholeWordOnly(t *testing.T) {
	assertMentionNotFound(t, "the Alphabet song\n", 1, "Alpha") // substring must not match
}

func TestLinkifyMentionUnicodeWord(t *testing.T) {
	got, _ := linkifyOK(t, "schön Über alles\n", 1, "Über")
	if want := "schön [[Über]] alles\n"; got != want {
		t.Errorf("got %q, want %q (Unicode word boundary)", got, want)
	}
}

func TestLinkifyMentionLineOutOfRange(t *testing.T) {
	assertMentionNotFound(t, "just one line\n", 9, "Alpha") // out-of-range line
}

func TestLinkifyMentionMentionGone(t *testing.T) {
	assertMentionNotFound(t, "nothing relevant here\n", 1, "Alpha") // line lacks the mention
}

func TestLinkifyMentionRegexMetaTarget(t *testing.T) {
	got, _ := linkifyOK(t, "about C++ today\n", 1, "C++")
	if want := "about [[C++]] today\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLinkifyMentionSkipsInlineCode(t *testing.T) {
	// arrange — first occurrence is in code, second is bare
	body := "- run `alpha deploy` then alpha again\n"

	// act
	got, span, err := LinkifyMention(body, 1, "alpha")

	// assert — the BARE mention gets wrapped, the code span is untouched
	if err != nil {
		t.Fatal(err)
	}
	want := "- run `alpha deploy` then [[alpha]] again\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	_ = span
}

func TestLinkifyMentionOnlyCodeOccurrenceIsNotFound(t *testing.T) {
	_, _, err := LinkifyMention("- run `alpha deploy` now\n", 1, "alpha")
	if !errors.Is(err, ErrMentionNotFound) {
		t.Fatalf("err = %v, want ErrMentionNotFound", err)
	}
}

func TestLinkifyMentionSkipsTags(t *testing.T) {
	got, _ := linkifyOK(t, "- #kitchen and kitchen\n", 1, "kitchen")
	if want := "- #kitchen and [[kitchen]]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	assertMentionNotFound(t, "- #kitchen only\n", 1, "kitchen")
	assertMentionNotFound(t, "- #kb/notes\n", 1, "notes")
}
