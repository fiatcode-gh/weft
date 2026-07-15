package graph

import "testing"

func TestParseBodySkipsTildeFences(t *testing.T) {
	// arrange / act
	links := ExtractWikiLinks("~~~\ncode [[InsideCode]]\n~~~\n- after [[Real]]\n")

	// assert
	if len(links) != 1 || links[0].Target != "Real" {
		t.Fatalf("links = %+v, want only Real", links)
	}
}

func TestParseBodyHandlesBulletPrefixedFences(t *testing.T) {
	// arrange / act — Logseq puts fences inside bullets; the indented
	// closer must not INVERT state and swallow the rest of the page
	links := ExtractWikiLinks("- ```\n  code [[InsideCode]]\n  ```\n- after [[Real]]\n")

	// assert
	if len(links) != 1 || links[0].Target != "Real" {
		t.Fatalf("links = %+v, want only Real", links)
	}
}

func TestFenceStateMixedMarkers(t *testing.T) {
	// a ``` line inside an open ~~~ fence is content, not a toggle
	var f FenceState
	steps := []struct {
		line string
		want bool
	}{
		{"~~~", true},
		{"```", true}, // content of the tilde fence
		{"still code", true},
		{"~~~", true}, // closes
		{"- plain [[X]]", false},
	}
	for i, s := range steps {
		if got := f.Step(s.line); got != s.want {
			t.Fatalf("step %d (%q) = %v, want %v", i, s.line, got, s.want)
		}
	}
}
