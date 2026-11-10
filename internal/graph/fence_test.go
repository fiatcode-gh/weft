package graph

import "testing"

func TestParseBodySkipsTildeFences(t *testing.T) {
	// arrange / act
	links := ExtractLinks("~~~\ncode [[InsideCode]]\n~~~\n- after [[Real]]\n")

	// assert
	if len(links) != 1 || links[0].Target != "Real" {
		t.Fatalf("links = %+v, want only Real", links)
	}
}

func TestParseBodyHandlesBulletPrefixedFences(t *testing.T) {
	// arrange / act — Logseq puts fences inside bullets; the indented
	// closer must not INVERT state and swallow the rest of the page
	links := ExtractLinks("- ```\n  code [[InsideCode]]\n  ```\n- after [[Real]]\n")

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

func TestFenceStateGrammar(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  []bool
	}{
		{"inline triple backticks is not an opener",
			[]string{"```inline``` is code", "- TODO later"},
			[]bool{false, false}},
		{"tilde opener may hold backticks",
			[]string{"~~~ a`b", "code", "~~~", "text"},
			[]bool{true, true, true, false}},
		{"closer with an info string stays content",
			[]string{"```", "```js", "code", "```", "text"},
			[]bool{true, true, true, true, false}},
		{"longer opener needs an equal or longer closer",
			[]string{"````", "```", "code", "```` ", "text"},
			[]bool{true, true, true, true, false}},
		{"longer closer closes",
			[]string{"```", "code", "`````", "text"},
			[]bool{true, true, true, false}},
		{"bullet fence round trip",
			[]string{"- ```js", "  code", "  ```", "- text"},
			[]bool{true, true, true, false}},
		{"crlf closer",
			[]string{"```\r", "code\r", "```\r", "text\r"},
			[]bool{true, true, true, false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var f FenceState
			for i, l := range tc.lines {
				if got := f.Step(l); got != tc.want[i] {
					t.Fatalf("step %d (%q) = %v, want %v", i, l, got, tc.want[i])
				}
			}
		})
	}
}

func TestParseBodyInlineTripleBacktickKeepsRestOfPage(t *testing.T) {
	// arrange
	body := "```inline``` is code\n- TODO later [[Real]]\n"

	// act
	links, todos := ExtractLinks(body), ExtractTodos(body)

	// assert
	if len(links) != 1 || links[0].Target != "Real" {
		t.Fatalf("links = %+v, want Real", links)
	}
	if len(todos) != 1 {
		t.Fatalf("todos = %+v, want 1", todos)
	}
}

func TestParseBodyInfoStringLineDoesNotCloseFence(t *testing.T) {
	// arrange — "```js" is content inside an open fence, so the links
	// between it and the real closer stay hidden
	body := "```\n[[A]]\n```js\n[[B]]\n```\n- [[Real]]\n"

	// act
	links := ExtractLinks(body)

	// assert
	if len(links) != 1 || links[0].Target != "Real" {
		t.Fatalf("links = %+v, want only Real", links)
	}
}
