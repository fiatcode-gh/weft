package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

func TestExtractTagPartial(t *testing.T) {
	cases := []struct {
		name          string
		before, after string
		want          string
		wantOK        bool
	}{
		{"one letter", "#k", "", "k", true},
		{"after text", "see #kit", "", "kit", true},
		{"after paren", "(#kit", "", "kit", true},
		{"nested path after tab", "\t#kb/no", "", "kb/no", true},
		{"unicode", "#café", "", "café", true},
		{"combining marks", "#हिन्", "", "हिन्", true},
		{"mark cannot start", "#\u0301k", "", "", false},
		{"hex-shaped is still a query", "#f2e", "", "f2e", true},
		{"bare hash", "#", "", "", false},
		{"heading", "# ", "", "", false},
		{"h2", "## x", "", "", false},
		{"digit first", "#1", "", "", false},
		{"plus", "#+B", "", "", false},
		{"bang", "#!", "", "", false},
		{"brace", "#{", "", "", false},
		{"glued to a word", "C#k", "", "", false},
		{"glued to a url", "repo#k", "", "", false},
		{"open code span", "`x #k", "", "", false},
		{"link destination", "[x](#k", ")", "", false},
		{"mid-word", "#ki", "t", "", false},
		{"inside closed wiki link", "[[Lab #in", "]]", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractTagPartial(tc.before, tc.after)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("extractTagPartial(%q, %q) = (%q, %v), want (%q, %v)", tc.before, tc.after, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestTagTextRoundTrips(t *testing.T) {
	for _, n := range []string{"Kitchen", "kb/notes", "Book Club", "2026-05-24", "f2e309", "café"} {
		tags := graph.FindTags(tagText(n))
		if len(tags) != 1 {
			t.Errorf("tagText(%q) = %q: %d tags, want 1", n, tagText(n), len(tags))
			continue
		}
		tg := tags[0]
		line := tagText(n)
		target := tg.Name
		if tg.Bracket {
			target = line[tg.Start+1+2 : tg.End-2]
		}
		if target != n {
			t.Errorf("tagText(%q) = %q resolves to %q", n, line, target)
		}
	}
	for n, want := range map[string]string{
		"Kitchen":   "#Kitchen",
		"Book Club": "#[[Book Club]]",
		"f2e309":    "#[[f2e309]]",
	} {
		if got := tagText(n); got != want {
			t.Errorf("tagText(%q) = %q, want %q", n, got, want)
		}
	}
}

func TestLinkCompleterTagTrigger(t *testing.T) {
	quietTerm(t)
	c := newLinkCompleter(loadFixture(t))

	c.refresh("see #kit", "", true, true)
	if !c.active || !c.tag {
		t.Fatalf("see #kit: active=%v tag=%v, want both", c.active, c.tag)
	}
	if c.cands[0].name != "Kitchen" {
		t.Errorf("first candidate = %q, want Kitchen", c.cands[0].name)
	}

	c = newLinkCompleter(loadFixture(t))
	c.refresh("see #kit", "", true, false)
	if c.active {
		t.Errorf("tagOK=false must not open the strip")
	}

	c = newLinkCompleter(loadFixture(t))
	c.refresh("[[Lab #kit", "", true, true)
	if !c.active || c.tag || c.partial != "Lab #kit" {
		t.Errorf("[[ must win: active=%v tag=%v partial=%q", c.active, c.tag, c.partial)
	}
}

func enter(e *EditorView) { e.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) }

func TestEditorTagCompletionInsertsSimpleTag(t *testing.T) {
	quietTerm(t)
	e := editorAt(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 16, 0)
	typeRunes(e, "see #kit")
	if !e.completer.active {
		t.Fatalf("strip should be open after #kit")
	}
	enter(e)
	if got := text(e); got != "see #Kitchen" {
		t.Errorf("text = %q, want %q", got, "see #Kitchen")
	}
	if e.completer.active {
		t.Errorf("strip should be closed after accepting")
	}
	typeRunes(e, "s")
	if !e.completer.active || e.completer.partial != "Kitchens" {
		t.Errorf("typing on should reopen: active=%v partial=%q", e.completer.active, e.completer.partial)
	}
}

func TestEditorTagCompletionInsertsBracketTag(t *testing.T) {
	quietTerm(t)
	e := editorAt(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 16, 0)
	typeRunes(e, "see #boo")
	enter(e)
	if got := text(e); got != "see #[[Book Club]]" {
		t.Errorf("text = %q, want %q", got, "see #[[Book Club]]")
	}
	if e.completer.active {
		t.Errorf("strip should be closed after accepting")
	}
}

func TestEditorTagCompletionCreateRow(t *testing.T) {
	quietTerm(t)
	e := editorAt(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 16, 0)
	typeRunes(e, "#zzq")
	if !e.completer.active {
		t.Fatalf("create row should show")
	}
	v := e.buf.Version()
	enter(e)
	if got := text(e); got != "#zzq" {
		t.Errorf("text = %q, want #zzq", got)
	}
	if e.completer.active {
		t.Errorf("strip should be closed")
	}
	if e.buf.Version() != v {
		t.Errorf("accepting an unchanged tag edited the buffer (version %d -> %d)", v, e.buf.Version())
	}

	e = editorAt(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 16, 0)
	typeRunes(e, "#f2e")
	enter(e)
	if got := text(e); got != "#[[f2e]]" {
		t.Errorf("text = %q, want #[[f2e]]", got)
	}
}

func TestEditorTagCompletionNeverOpens(t *testing.T) {
	quietTerm(t)
	for _, in := range []string{"# x", "## x", "C#k", "#18", "`#kit", "`x #kit"} {
		e := editorAt(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 16, 0)
		typeRunes(e, in)
		if e.completer.active {
			t.Errorf("%q opened the strip", in)
		}
	}
	e := editorAt(loadFixture(t), "Note", "/tmp/n.md", "```\n\n```", true, 80, 16, 1)
	typeRunes(e, "#kit")
	if e.completer.active {
		t.Errorf("#kit inside a fence opened the strip")
	}
}

func TestEditorHashBracketUsesLinkCompletion(t *testing.T) {
	quietTerm(t)
	e := editorAt(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 16, 0)
	typeRunes(e, "#[[Boo")
	enter(e)
	if got := text(e); got != "#[[Book Club]]" {
		t.Errorf("text = %q, want #[[Book Club]]", got)
	}
}
