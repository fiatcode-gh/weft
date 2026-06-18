package graph

import (
	"fmt"
	"testing"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/search"
)

func TestFilterUnlinked(t *testing.T) {
	// arrange
	target := "Alpha"
	targetPath := "/g/pages/Alpha.md"
	read := mapReader(map[string]string{
		"/g/pages/Note.md":  "mentions Alpha here\nlinked [[Alpha]] already\n```\nAlpha in fence\n```\n",
		"/g/pages/Alpha.md": "Alpha self mention\n",
	})
	hits := []search.Hit{
		{FilePath: "/g/pages/Note.md", Line: 1, Context: "mentions Alpha here", Matches: []search.Span{{Start: 9, End: 14}}},      // keep
		{FilePath: "/g/pages/Note.md", Line: 2, Context: "linked [[Alpha]] already", Matches: []search.Span{{Start: 9, End: 14}}}, // drop: inside [[ ]]
		{FilePath: "/g/pages/Note.md", Line: 4, Context: "Alpha in fence", Matches: []search.Span{{Start: 0, End: 5}}},            // drop: fenced
		{FilePath: "/g/pages/Alpha.md", Line: 1, Context: "Alpha self mention", Matches: []search.Span{{Start: 0, End: 5}}},       // drop: own file
	}

	// act
	got := FilterUnlinked(hits, target, targetPath, read)

	// assert
	if len(got) != 1 {
		t.Fatalf("want 1 unlinked ref, got %d: %+v", len(got), got)
	}
	if got[0].Line != 1 || got[0].PageName != "Note" || got[0].FilePath != "/g/pages/Note.md" {
		t.Errorf("unexpected ref: %+v", got[0])
	}
}

func TestFilterUnlinkedUnreadableFileDropped(t *testing.T) {
	// arrange — the reader always fails, so every hit's source is unreadable.
	read := func(string) (string, error) { return "", fmt.Errorf("boom") }
	hits := []search.Hit{
		{FilePath: "/g/pages/Note.md", Line: 1, Context: "Alpha here", Matches: []search.Span{{Start: 0, End: 5}}},
	}

	// act
	got := FilterUnlinked(hits, "Alpha", "/g/pages/Alpha.md", read)

	// assert
	if len(got) != 0 {
		t.Errorf("unreadable file should drop its hits, got %+v", got)
	}
}
