package graph

import "testing"

func TestResolveLink(t *testing.T) {
	idx := buildTempIndex(t, map[string]string{
		"Alpha.md":         "## Summary\n\n## The **Big** [[Idea|idea]]\n\nSetext one\n----\n",
		"Alpha#summary.md": "x\n",
		"C# notes.md":      "# Intro\n",
	})
	cases := []struct {
		target   string
		ok       bool
		page     string
		fragment string
	}{
		{"Alpha#summary", true, "Alpha#summary", ""},
		{"Alpha#the big idea", true, "Alpha", "the big idea"},
		{"Alpha#setext one", true, "Alpha", "setext one"},
		{"Alpha#Missing", false, "", ""},
		{"C#", false, "", ""},
		{"#x", false, "", ""},
		{"Lab #inner", false, "", ""},
		{"C# notes#Intro", true, "C# notes", "Intro"},
		{"alpha#THE BIG IDEA", true, "Alpha", "THE BIG IDEA"},
		{"alpha#Summary ", true, "Alpha", "Summary"},
		{"Alpha#", false, "", ""},
		{"Alpha# ", false, "", ""},
	}
	for _, tc := range cases {
		got, ok := idx.ResolveLink(tc.target)
		if ok != tc.ok {
			t.Errorf("ResolveLink(%q) ok = %v, want %v", tc.target, ok, tc.ok)
			continue
		}
		if ok && (got.Page.Name != tc.page || got.Heading != tc.fragment) {
			t.Errorf("ResolveLink(%q) = {%s %q}, want {%s %q}", tc.target, got.Page.Name, got.Heading, tc.page, tc.fragment)
		}
	}
}

// The index and the read view key a heading with the same function, even for
// markup that one pass of displayText does not fully strip.
func TestResolveLinkKeysHeadingsWithHeadingKey(t *testing.T) {
	idx := buildTempIndex(t, map[string]string{"Nest.md": "## [[[[a]]]]\n"})
	hs := Headings("## [[[[a]]]]\n")
	key := HeadingKey(hs[0].Text)
	if _, ok := idx.ResolveLink("Nest#" + key); !ok {
		t.Errorf("ResolveLink(Nest#%s) unresolved, but HeadingKey(%q) = %q", key, hs[0].Text, key)
	}
}
