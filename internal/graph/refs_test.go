package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiatcode-gh/weft/v2/internal/search"
)

func TestFilterUnlinked(t *testing.T) {
	// arrange
	idx := buildTempIndex(t, map[string]string{
		"Note.md":  "mentions Alpha here\nlinked [[Alpha]] already\n```\nAlpha in fence\n```\n",
		"Alpha.md": "Alpha self mention\n",
	})
	note, targetPath := pagePath(idx, "Note.md"), pagePath(idx, "Alpha.md")
	read := fileReader()
	hits := []search.Hit{
		{FilePath: note, Line: 1, Context: "mentions Alpha here", Matches: []search.Span{{Start: 9, End: 14}}},      // keep
		{FilePath: note, Line: 2, Context: "linked [[Alpha]] already", Matches: []search.Span{{Start: 9, End: 14}}}, // drop: inside [[ ]]
		{FilePath: note, Line: 4, Context: "Alpha in fence", Matches: []search.Span{{Start: 0, End: 5}}},            // drop: fenced
		{FilePath: targetPath, Line: 1, Context: "Alpha self mention", Matches: []search.Span{{Start: 0, End: 5}}},  // drop: own file
	}

	// act
	got := idx.FilterUnlinked(hits, targetPath, read)

	// assert
	if len(got) != 1 {
		t.Fatalf("want 1 unlinked ref, got %d: %+v", len(got), got)
	}
	if got[0].Line != 1 || got[0].PageName != "Note" || got[0].FilePath != note {
		t.Errorf("unexpected ref: %+v", got[0])
	}
}

func TestFilterUnlinkedUnreadableFileDropped(t *testing.T) {
	// arrange — the reader always fails, so every hit's source is unreadable.
	idx := buildTempIndex(t, map[string]string{"Note.md": "Alpha here\n"})
	read := func(string) (string, error) { return "", fmt.Errorf("boom") }
	hits := []search.Hit{
		{FilePath: pagePath(idx, "Note.md"), Line: 1, Context: "Alpha here", Matches: []search.Span{{Start: 0, End: 5}}},
	}

	// act
	got := idx.FilterUnlinked(hits, "", read)

	// assert
	if len(got) != 0 {
		t.Errorf("unreadable file should drop its hits, got %+v", got)
	}
}

func TestFirstUnlinkedMatchSkipsInlineCode(t *testing.T) {
	// arrange — the only match sits inside `alpha deploy`
	line := "- run `alpha deploy` now"
	matches := []search.Span{{Start: strings.Index(line, "alpha"), End: strings.Index(line, "alpha") + 5}}

	// act
	_, ok := firstUnlinkedMatch(line, matches)

	// assert
	if ok {
		t.Fatal("match inside inline code offered as unlinked ref")
	}
}

func TestInlineCodeSpansUnclosedBacktickRunsToEndOfLine(t *testing.T) {
	// arrange — a single, unpaired backtick: parse.go's appendLinks and
	// render's replaceLinksOutsideInlineCode both split on backticks and
	// treat every odd-indexed segment as inline code, including a trailing
	// segment with no closing backtick (strings.Split("a `code", "`") ==
	// ["a ", "code"], and "code" sits at odd index 1). InlineCodeSpans must
	// mirror that exactly, so an unclosed backtick still hides the rest of
	// the line from unlinked-ref detection.
	line := "a `code"

	// act
	spans := InlineCodeSpans(line)

	// assert
	if len(spans) != 1 {
		t.Fatalf("spans = %+v, want exactly one span covering the unclosed code run", spans)
	}
	got := line[spans[0].Start:spans[0].End]
	if want := "`code"; got != want {
		t.Fatalf("span covers %q, want %q", got, want)
	}
}

func TestFilterUnlinkedSkipsTags(t *testing.T) {
	body := "- #kitchen and (#Kitchen)\n- the kitchen sink\n- #kb/notes\n"
	idx := buildTempIndex(t, map[string]string{"Note.md": body})
	note := pagePath(idx, "Note.md")
	read := fileReader()
	span := func(line, sub string) search.Span {
		i := strings.Index(line, sub)
		return search.Span{Start: i, End: i + len(sub)}
	}
	lines := strings.Split(body, "\n")
	hit := func(n int, sub string) search.Hit {
		return search.Hit{FilePath: note, Line: n, Context: lines[n-1], Matches: []search.Span{span(lines[n-1], sub)}}
	}
	// ripgrep reports every match on a line; line 1 carries both tags.
	line1 := search.Hit{FilePath: note, Line: 1, Context: lines[0], Matches: []search.Span{span(lines[0], "kitchen"), {Start: 17, End: 24}}}

	got := idx.FilterUnlinked([]search.Hit{line1, hit(2, "kitchen")}, "", read)
	if len(got) != 1 || got[0].Line != 2 {
		t.Errorf("kitchen: want only line 2, got %+v", got)
	}
	got = idx.FilterUnlinked([]search.Hit{hit(3, "notes")}, "", read)
	if len(got) != 0 {
		t.Errorf("notes inside #kb/notes must be dropped, got %+v", got)
	}
}

func TestFilterUnlinkedMapsByPath(t *testing.T) {
	idx := buildTempIndex(t, map[string]string{
		"kb___notes.md": "Alpha in namespaced page\n",
		"Note.md":       "Alpha in note\n",
	})
	ns, note := pagePath(idx, "kb___notes.md"), pagePath(idx, "Note.md")
	hit := func(p string) search.Hit {
		return search.Hit{FilePath: p, Line: 1, Context: "Alpha in x", Matches: []search.Span{{Start: 0, End: 5}}}
	}
	hits := []search.Hit{hit(ns), hit("other/x.md"), hit(note)}

	got := idx.FilterUnlinked(hits, note, fileReader())

	if len(got) != 1 || got[0].PageName != "kb/notes" || got[0].FilePath != ns {
		t.Fatalf("want only the kb/notes hit, got %+v", got)
	}
}

func TestPageAtAndFiles(t *testing.T) {
	idx := buildFixtureIndex(t)

	files := idx.Files()
	if len(files) != len(idx.Pages) {
		t.Fatalf("Files len = %d, want %d", len(files), len(idx.Pages))
	}
	for i, p := range idx.Pages {
		if files[i] != p.Path {
			t.Errorf("Files[%d] = %q, want %q", i, files[i], p.Path)
		}
		got, ok := idx.PageAt(p.Path)
		if !ok || got.Path != p.Path || got.Name != p.Name {
			t.Errorf("PageAt(%q) = %+v, %v", p.Path, got, ok)
		}
	}
	files[0] = "mutated"
	if idx.Files()[0] == "mutated" {
		t.Error("Files must return a copy")
	}
	if _, ok := idx.PageAt("/nope/x.md"); ok {
		t.Error("PageAt of an unindexed path must miss")
	}
}

// pagePath returns the on-disk path of a page file in a buildTempIndex graph.
func pagePath(idx *Index, file string) string {
	return filepath.Join(idx.GraphPath, "pages", file)
}

// fileReader reads real files, as the production callers do.
func fileReader() func(string) (string, error) {
	return func(p string) (string, error) {
		b, err := os.ReadFile(p)
		return string(b), err
	}
}
