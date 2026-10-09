package graph

import (
	"reflect"
	"testing"
)

func TestUnresolvedLinksFindsPhantoms(t *testing.T) {
	// arrange
	idx := buildFixtureIndex(t)

	// act
	got := idx.UnresolvedLinks()

	// assert — [[DoesNotExist]] in Beta plus every Corpus tag without a page;
	// fenced [[ShouldNotMatch]] and [[NotALink]] never enter the index.
	var targets []string
	for _, u := range got {
		targets = append(targets, u.Target)
	}
	wantTargets := []string{
		"a1b2c3d4e", "abc12", "add", "bad", "C#", "cafe", "café", "DoesNotExist",
		"inheading", "Lab #inner", "lead", "s", "日本",
	}
	if !reflect.DeepEqual(targets, wantTargets) {
		t.Fatalf("unresolved targets = %v, want %v", targets, wantTargets)
	}
	var dne UnresolvedLink
	for _, u := range got {
		if u.Target == "DoesNotExist" {
			dne = u
		}
	}
	want := []Ref{{FromPage: "Beta", LineNumber: 3, Context: "- This link is dangling: [[DoesNotExist]]."}}
	if !reflect.DeepEqual(dne.Refs, want) {
		t.Errorf("refs = %+v, want %+v", dne.Refs, want)
	}
}

func TestUnresolvedLinksResolvesCaseFolded(t *testing.T) {
	// arrange — [[alpha]] resolves to Alpha.md case-insensitively
	idx := buildTempIndex(t, map[string]string{
		"Alpha.md": "- alpha page\n",
		"Note.md":  "- see [[alpha]]\n",
	})

	// act
	got := idx.UnresolvedLinks()

	// assert
	if len(got) != 0 {
		t.Fatalf("case-folded target must resolve, got %+v", got)
	}
}

func TestUnresolvedLinksSortsCaseInsensitively(t *testing.T) {
	// arrange
	idx := buildTempIndex(t, map[string]string{
		"Note.md": "- [[zeta]] then [[Alpha Phantom]] then [[beta phantom]]\n",
	})

	// act
	got := idx.UnresolvedLinks()

	// assert — folded order: "alpha phantom" < "beta phantom" < "zeta"
	var targets []string
	for _, u := range got {
		targets = append(targets, u.Target)
	}
	want := []string{"Alpha Phantom", "beta phantom", "zeta"}
	if !reflect.DeepEqual(targets, want) {
		t.Errorf("targets = %v, want %v", targets, want)
	}
}

func TestUnresolvedLinksKeepsFirstSeenSpelling(t *testing.T) {
	// arrange — two spellings share one folded key; walk order puts Note.md first
	idx := buildTempIndex(t, map[string]string{
		"Note.md":  "- [[Phantom]]\n",
		"Other.md": "- [[PHANTOM]]\n",
	})

	// act
	got := idx.UnresolvedLinks()

	// assert — one entry with the first-seen spelling and both refs
	if len(got) != 1 {
		t.Fatalf("want 1 unresolved link, got %d: %+v", len(got), got)
	}
	if got[0].Target != "Phantom" {
		t.Errorf("target = %q, want first-seen spelling Phantom", got[0].Target)
	}
	if len(got[0].Refs) != 2 || got[0].Refs[0].FromPage != "Note" || got[0].Refs[1].FromPage != "Other" {
		t.Errorf("refs = %+v, want Note then Other", got[0].Refs)
	}
}

func TestUnresolvedLinksSkipsHeadingLinks(t *testing.T) {
	idx := buildTempIndex(t, map[string]string{
		"Alpha.md": "## Summary\ntext\n",
		"Note.md":  headingLinkGraphNote,
	})

	var targets []string
	for _, u := range idx.UnresolvedLinks() {
		targets = append(targets, u.Target)
	}

	if !reflect.DeepEqual(targets, []string{"Alpha#Missing"}) {
		t.Fatalf("unresolved targets = %v, want [Alpha#Missing]", targets)
	}
}
