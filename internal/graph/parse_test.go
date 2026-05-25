package graph

import (
	"reflect"
	"testing"
)

func TestExtractWikiLinks(t *testing.T) {
	body := "- See [[Alpha]] and [[Beta|the second]].\n" +
		"```\n[[InsideFence]]\n```\n" +
		"- Another [[proj/nested]] ref."
	got := ExtractWikiLinks(body)
	want := []LinkHit{
		{Target: "Alpha", Line: 1},
		{Target: "Beta", Line: 1},
		{Target: "proj/nested", Line: 5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractWikiLinks mismatch:\nwant %#v\ngot  %#v", want, got)
	}
}

func TestExtractTodos(t *testing.T) {
	body := "- TODO Buy milk\n" +
		"- LATER [#A] Review the doc\n" +
		"- DONE Should be ignored\n" +
		"- regular bullet\n" +
		"- DOING Write the parser\n" +
		"- WAITING [#B] Vendor response"
	got := ExtractTodos(body)
	want := []TodoHit{
		{Marker: "TODO", Priority: "", Text: "Buy milk", Line: 1},
		{Marker: "LATER", Priority: "A", Text: "Review the doc", Line: 2},
		{Marker: "DOING", Priority: "", Text: "Write the parser", Line: 5},
		{Marker: "WAITING", Priority: "B", Text: "Vendor response", Line: 6},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractTodos mismatch:\nwant %#v\ngot  %#v", want, got)
	}
}
