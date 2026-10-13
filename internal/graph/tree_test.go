package graph

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// render prints a forest as indented lines: "*" marks a page, "(N)" a group's count.
func render(nodes []*TreeNode) []string {
	var out []string
	var walk func(ns []*TreeNode, depth int)
	walk = func(ns []*TreeNode, depth int) {
		for _, n := range ns {
			line := strings.Repeat("  ", depth) + n.Label
			if n.Page != nil {
				line += " *"
			}
			if len(n.Children) > 0 {
				line += fmt.Sprintf(" (%d)", n.Count)
			}
			out = append(out, line)
			walk(n.Children, depth+1)
		}
	}
	walk(nodes, 0)
	return out
}

func labels(nodes []*TreeNode) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.Label)
	}
	return out
}

func child(t *testing.T, nodes []*TreeNode, label string) *TreeNode {
	t.Helper()
	for _, n := range nodes {
		if n.Label == label {
			return n
		}
	}
	t.Fatalf("no node %q in %v", label, labels(nodes))
	return nil
}

func wantLabels(t *testing.T, got []*TreeNode, want ...string) {
	t.Helper()
	if !slices.Equal(labels(got), want) {
		t.Fatalf("labels = %v, want %v", labels(got), want)
	}
}

func TestPageTreeFixtureGraph(t *testing.T) {
	tree := buildFixtureIndex(t).PageTree()
	wantLabels(t, tree, "Alpha", "Beta", "Book Club", "Corpus", "Hub", "kb", "Kitchen", "Orphan", "proj", "Workbench")
	kb := child(t, tree, "kb")
	if kb.Page != nil || kb.Count != 1 {
		t.Fatalf("kb page=%v count=%d, want group-only count 1", kb.Page, kb.Count)
	}
	wantLabels(t, kb.Children, "notes")
	if kb.Children[0].Page == nil || kb.Children[0].Page.Key != "kb/notes" {
		t.Fatalf("kb/notes page = %+v", kb.Children[0].Page)
	}
	wantLabels(t, child(t, tree, "proj").Children, "nested")
	journal := regexp.MustCompile(`^\d{4}_\d{2}_\d{2}$`)
	for _, line := range render(tree) {
		if journal.MatchString(strings.TrimSpace(strings.TrimSuffix(line, " *"))) {
			t.Fatalf("journal in tree: %q", line)
		}
	}
}

func TestPageTreeObsidianVault(t *testing.T) {
	tree := vaultIndex(t).PageTree()
	wantLabels(t, tree, "a", "b", "Daily", "Home", "Projects")
	a := child(t, child(t, tree, "a").Children, "Note")
	if a.Page.Rel != "a/Note.md" || a.Page.Key != "Note" {
		t.Fatalf("a/Note page = %+v", a.Page)
	}
	b := child(t, child(t, tree, "b").Children, "Note")
	if b.Page.Rel != "b/Note.md" || b.Page.Key != "b/Note" {
		t.Fatalf("b/Note page = %+v", b.Page)
	}
	daily := child(t, tree, "Daily")
	wantLabels(t, daily.Children, "Ideas")
	if daily.Count != 1 {
		t.Fatalf("Daily count = %d, want 1", daily.Count)
	}
	wantLabels(t, child(t, tree, "Projects").Children, "Alpha")
	for _, line := range render(tree) {
		if strings.Contains(line, ".trash") || strings.Contains(line, ".obsidian") {
			t.Fatalf("hidden dir in tree: %q", line)
		}
	}
}

func TestPageTreePageAndGroup(t *testing.T) {
	tree := buildPageTree([]PageMeta{
		{Name: "a", Key: "a", Rel: "pages/a.md"},
		{Name: "a/b", Key: "a/b", Rel: "pages/a___b.md"},
	}, "pages")
	wantLabels(t, tree, "a")
	a := tree[0]
	if a.Page == nil || a.Count != 1 {
		t.Fatalf("a page=%v count=%d", a.Page, a.Count)
	}
	wantLabels(t, a.Children, "b")
	if a.Children[0].Path != "a/b" {
		t.Fatalf("child path = %q", a.Children[0].Path)
	}
}

func TestPageTreeOutsidePagesDir(t *testing.T) {
	tree := buildPageTree([]PageMeta{
		{Name: "Alpha", Key: "Alpha", Rel: "Projects/Alpha.md"},
		{Name: "Notes", Key: "Notes", Rel: "journals/Notes.md"},
		{Name: "2026_05_24", Key: "2026_05_24", Rel: "journals/2026_05_24.md", IsJournal: true},
		{Name: "y", Key: "y", Rel: "pages/x/y.md"},
	}, "pages")
	wantLabels(t, tree, "journals", "Projects", "x")
	wantLabels(t, child(t, tree, "journals").Children, "Notes")
	wantLabels(t, child(t, tree, "x").Children, "y")
}

func TestPageTreeSortAndCase(t *testing.T) {
	tree := buildPageTree([]PageMeta{
		{Name: "beta", Rel: "beta.md"},
		{Name: "Alpha", Rel: "Alpha.md"},
		{Name: "alpha", Rel: "alpha.md"},
		{Name: "Gamma", Rel: "Gamma.md"},
		{Name: "one", Rel: "Kb/one.md"},
		{Name: "kb/two", Rel: "kb___two.md"},
	}, "")
	wantLabels(t, tree, "Alpha", "alpha", "beta", "Gamma", "Kb", "kb")
	wantLabels(t, child(t, tree, "Kb").Children, "one")
	wantLabels(t, child(t, tree, "kb").Children, "two")
}

func TestPageTreeCounts(t *testing.T) {
	tree := buildPageTree([]PageMeta{
		{Name: "a/b/c", Rel: "pages/a___b___c.md"},
		{Name: "a/d", Rel: "pages/a___d.md"},
		{Name: "a", Rel: "pages/a.md"},
	}, "pages")
	a := child(t, tree, "a")
	if a.Count != 2 {
		t.Fatalf("a.Count = %d, want 2", a.Count)
	}
	b := child(t, a.Children, "b")
	if b.Count != 1 || b.Page != nil || b.Path != "a/b" {
		t.Fatalf("a/b = count %d page %v path %q", b.Count, b.Page, b.Path)
	}
}

func TestPageTreeDuplicatePath(t *testing.T) {
	folder := PageMeta{Name: "b", Key: "b", Rel: "pages/a/b.md"}
	ns := PageMeta{Name: "a/b", Key: "a/b", Rel: "pages/a___b.md"}
	var first []string
	for _, in := range [][]PageMeta{{folder, ns}, {ns, folder}} {
		tree := buildPageTree(in, "pages")
		a := child(t, tree, "a")
		wantLabels(t, a.Children, "b", "b")
		if a.Children[0].Page.Rel != "pages/a/b.md" || a.Children[1].Page.Rel != "pages/a___b.md" {
			t.Fatalf("order = %s, %s", a.Children[0].Page.Rel, a.Children[1].Page.Rel)
		}
		if a.Count != 2 {
			t.Fatalf("a.Count = %d, want 2", a.Count)
		}
		got := render(tree)
		if first == nil {
			first = got
		} else if !slices.Equal(first, got) {
			t.Fatalf("input order changed output: %v vs %v", first, got)
		}
	}
}

func TestPageTreeEmptySegments(t *testing.T) {
	tree := buildPageTree([]PageMeta{
		{Name: "/x", Rel: "pages/___x.md"},
		{Name: "/", Rel: "pages/___.md"},
	}, "pages")
	wantLabels(t, tree, "___", "x")
	for _, n := range tree {
		if n.Page == nil || n.Label == "" {
			t.Fatalf("bad node %+v", n)
		}
	}
}

func TestPageTreeNoPages(t *testing.T) {
	tree := buildPageTree([]PageMeta{{Name: "2026_05_24", Rel: "journals/2026_05_24.md", IsJournal: true}}, "pages")
	if len(tree) != 0 {
		t.Fatalf("len = %d, want 0", len(tree))
	}
}
