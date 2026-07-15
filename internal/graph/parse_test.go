package graph

import (
	"reflect"
	"testing"
)

func TestExtractWikiLinks(t *testing.T) {
	// arrange
	body := "- See [[Alpha]] and [[Beta|the second]].\n" +
		"```\n[[InsideFence]]\n```\n" +
		"- Another [[proj/nested]] ref."

	// act
	got := ExtractWikiLinks(body)

	// assert
	want := []LinkHit{
		{Target: "Alpha", Line: 1},
		{Target: "Beta", Line: 1},
		{Target: "proj/nested", Line: 5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractWikiLinks mismatch:\nwant %#v\ngot  %#v", want, got)
	}
}

func TestExtractWikiLinksKeepsHashInTarget(t *testing.T) {
	// Logseq has no [[page#fragment]] syntax and allows "#" in page names,
	// so "#" and everything after it must stay part of the target.
	body := "intro\n[[Alpha#summary]]\n[[proj/nested#intro]]\n"
	hits := ExtractWikiLinks(body)
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d (%+v)", len(hits), hits)
	}
	if hits[0].Target != "Alpha#summary" || hits[0].Line != 2 {
		t.Errorf("hit 0 = %+v, want {Alpha#summary 2}", hits[0])
	}
	if hits[1].Target != "proj/nested#intro" || hits[1].Line != 3 {
		t.Errorf("hit 1 = %+v, want {proj/nested#intro 3}", hits[1])
	}
}

func TestWikiLinkTargetKeepsHash(t *testing.T) {
	// arrange / act
	links := ExtractWikiLinks("- [[C#]] and [[F#|fsharp]]\n")

	// assert
	if len(links) != 2 || links[0].Target != "C#" || links[1].Target != "F#" {
		t.Fatalf("links = %+v, want C# and F#", links)
	}
}

func TestExtractWikiLinksSkipsInlineCode(t *testing.T) {
	// [[Literal]] inside backticks is a code example, not a real link;
	// only [[Real]] outside code counts. Mirrors the renderer.
	body := "see [[Real]] and `[[Literal]]` here\n"
	got := ExtractWikiLinks(body)
	if len(got) != 1 || got[0].Target != "Real" {
		t.Fatalf("inline-code link must be skipped; got %+v", got)
	}
	if got[0].Line != 1 {
		t.Errorf("line number should be 1; got %d", got[0].Line)
	}
}

func TestExtractWikiLinksInlineCodeMidDocument(t *testing.T) {
	// A line whose ONLY [[..]] is inside backticks yields nothing.
	body := "intro line\n" +
		"- `[[Backticked]]` is a literal example\n" +
		"- a real [[Link]] here\n"
	got := ExtractWikiLinks(body)
	if len(got) != 1 || got[0].Target != "Link" || got[0].Line != 3 {
		t.Fatalf("only the real link on line 3 should count; got %+v", got)
	}
}

func TestExtractTodos(t *testing.T) {
	// arrange
	body := "- TODO Buy milk\n" +
		"- LATER [#A] Review the doc\n" +
		"- DONE Should be ignored\n" +
		"- regular bullet\n" +
		"- DOING Write the parser\n" +
		"- WAITING [#B] Vendor response\n" +
		"```\n" +
		"- TODO Inside fence should be ignored\n" +
		"```\n" +
		"- TODO After fence"

	// act
	got := ExtractTodos(body)

	// assert
	want := []TodoHit{
		{Marker: "TODO", Priority: "", Text: "Buy milk", Line: 1},
		{Marker: "LATER", Priority: "A", Text: "Review the doc", Line: 2},
		{Marker: "DOING", Priority: "", Text: "Write the parser", Line: 5},
		{Marker: "WAITING", Priority: "B", Text: "Vendor response", Line: 6},
		{Marker: "TODO", Priority: "", Text: "After fence", Line: 10},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractTodos mismatch:\nwant %#v\ngot  %#v", want, got)
	}
}
