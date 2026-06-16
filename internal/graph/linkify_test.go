package graph

import (
	"errors"
	"testing"
)

func TestLinkifyMentionHappyPath(t *testing.T) {
	body := "line one\nthe Alpha ship date\nline three\n"
	got, span, err := LinkifyMention(body, 2, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line one\nthe [[Alpha]] ship date\nline three\n"
	if got != want {
		t.Errorf("body =\n%q\nwant\n%q", got, want)
	}
	if body[span.Start:span.End] != "Alpha" {
		t.Errorf("span %v points at %q, want \"Alpha\"", span, body[span.Start:span.End])
	}
}

func TestLinkifyMentionPreservesCasing(t *testing.T) {
	got, _, err := LinkifyMention("a bare alpha here\n", 1, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "a bare [[alpha]] here\n"; got != want {
		t.Errorf("got %q, want %q (author's casing must be preserved)", got, want)
	}
}

func TestLinkifyMentionPreservesCRLF(t *testing.T) {
	body := "intro\r\nthe Alpha ship\r\nend\r\n"
	got, _, err := LinkifyMention(body, 2, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "intro\r\nthe [[Alpha]] ship\r\nend\r\n"; got != want {
		t.Errorf("got %q, want %q (CRLF must survive)", got, want)
	}
}

func TestLinkifyMentionNoTrailingNewline(t *testing.T) {
	got, _, err := LinkifyMention("only Alpha", 1, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "only [[Alpha]]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLinkifyMentionFirstOccurrenceWins(t *testing.T) {
	got, _, err := LinkifyMention("Alpha and Alpha again\n", 1, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[[Alpha]] and Alpha again\n"; got != want {
		t.Errorf("got %q, want %q (only the first occurrence is wrapped)", got, want)
	}
}

func TestLinkifyMentionSkipsAlreadyLinked(t *testing.T) {
	_, _, err := LinkifyMention("see [[Alpha]] there\n", 1, "Alpha")
	if !errors.Is(err, ErrMentionNotFound) {
		t.Errorf("err = %v, want ErrMentionNotFound (must not double-wrap)", err)
	}
}

func TestLinkifyMentionSkipsLinkedTakesNextBare(t *testing.T) {
	got, _, err := LinkifyMention("[[Alpha]] then bare Alpha\n", 1, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[[Alpha]] then bare [[Alpha]]\n"; got != want {
		t.Errorf("got %q, want %q (skip the linked one, wrap the bare one)", got, want)
	}
}

func TestLinkifyMentionWholeWordOnly(t *testing.T) {
	_, _, err := LinkifyMention("the Alphabet song\n", 1, "Alpha")
	if !errors.Is(err, ErrMentionNotFound) {
		t.Errorf("err = %v, want ErrMentionNotFound (substring must not match)", err)
	}
}

func TestLinkifyMentionUnicodeWord(t *testing.T) {
	got, _, err := LinkifyMention("schön Über alles\n", 1, "Über")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "schön [[Über]] alles\n"; got != want {
		t.Errorf("got %q, want %q (Unicode word boundary)", got, want)
	}
}

func TestLinkifyMentionLineOutOfRange(t *testing.T) {
	_, _, err := LinkifyMention("just one line\n", 9, "Alpha")
	if !errors.Is(err, ErrMentionNotFound) {
		t.Errorf("err = %v, want ErrMentionNotFound for out-of-range line", err)
	}
}

func TestLinkifyMentionMentionGone(t *testing.T) {
	_, _, err := LinkifyMention("nothing relevant here\n", 1, "Alpha")
	if !errors.Is(err, ErrMentionNotFound) {
		t.Errorf("err = %v, want ErrMentionNotFound when line lacks the mention", err)
	}
}

func TestLinkifyMentionRegexMetaTarget(t *testing.T) {
	got, _, err := LinkifyMention("about C++ today\n", 1, "C++")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "about [[C++]] today\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
