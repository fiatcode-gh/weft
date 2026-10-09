package graph

import (
	"reflect"
	"testing"
)

func TestHeadings(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []Heading
	}{
		{"atx levels", "# a\n## b\n### c\n#### d\n##### e\n###### f\n", []Heading{
			{Line: 0, Level: 1, Text: "a"}, {Line: 1, Level: 2, Text: "b"}, {Line: 2, Level: 3, Text: "c"},
			{Line: 3, Level: 4, Text: "d"}, {Line: 4, Level: 5, Text: "e"}, {Line: 5, Level: 6, Text: "f"},
		}},
		{"closing sequence", "## Title ##\n", []Heading{{Line: 0, Level: 2, Text: "Title"}}},
		{"empty atx", "#\n", []Heading{{Line: 0, Level: 1, Text: ""}}},
		{"four-space indent", "    # x\n", nil},
		{"indent two", "  ## x\n", []Heading{{Line: 0, Level: 2, Text: "x"}}},
		{"setext", "One\n===\n\nTwo\n---\n", []Heading{
			{Line: 0, Under: true, Level: 1, Text: "One"}, {Line: 3, Under: true, Level: 2, Text: "Two"},
		}},
		{"list continuation is not setext", "- item\n  text\n  ---\n", nil},
		{"table delimiter", "a | b\n---|---\n1 | 2\n", nil},
		{"fence", "```\n# no\n```\n# yes\n", []Heading{{Line: 3, Level: 1, Text: "yes"}}},
		{"logbook", "# a\n:LOGBOOK:\n# no\n:END:\n# b\n", []Heading{
			{Line: 0, Level: 1, Text: "a"}, {Line: 4, Level: 1, Text: "b"},
		}},
		{"query block", "{{query\n# no\n}}\n# b\n{{embed [[X]]}}\n# c\n", []Heading{
			{Line: 3, Level: 1, Text: "b"}, {Line: 5, Level: 1, Text: "c"},
		}},
		{"not headings", "#tag\n#!/bin/sh\n#+BEGIN_QUERY\n", nil},
		{"bullet marker line", "- ## x\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Headings(tc.body); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Headings(%q) = %+v, want %+v", tc.body, got, tc.want)
			}
		})
	}
}

func TestHeadingKey(t *testing.T) {
	cases := map[string]string{
		"[[Alpha|the A]] intro":      "the a intro",
		"**Bold** `code` _em_ ~~x~~": "bold code em x",
		"snake_case_name":            "snake_case_name",
		"#[[Book Club]]":             "#book club",
		"  Spaced   out ":            "spaced out",
		"[text](http://x)":           "text",
		"The **Big** [[Idea|idea]]":  "the big idea",
	}
	for in, want := range cases {
		if got := HeadingKey(in); got != want {
			t.Errorf("HeadingKey(%q) = %q, want %q", in, got, want)
		}
	}
}
