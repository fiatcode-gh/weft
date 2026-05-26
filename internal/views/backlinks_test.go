package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
)

func TestNewBacklinksLoadsRefs(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)

	// Hub is referenced from kb/notes and journal 2026-05-25. Hub's own
	// page also contains `[[Hub]]` but NewBacklinks filters self-refs so
	// the view doesn't show them — count must be exactly 2.
	b := NewBacklinks(idx, "Hub", 80)
	if got := len(b.refs); got != 2 {
		t.Errorf("Hub backlinks (post self-filter): want 2, got %d", got)
	}
	for _, r := range b.refs {
		if r.FromPage == "Hub" {
			t.Errorf("self-reference leaked into refs: %+v", r)
		}
	}
	if b.target != "Hub" {
		t.Errorf("target: want \"Hub\", got %q", b.target)
	}
	if b.width != 80 {
		t.Errorf("width: want 80, got %d", b.width)
	}
}

func TestNewBacklinksFiltersSelfRefs(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)

	// The underlying index records the self-reference; the view filters it.
	raw := idx.Backlinks["Hub"]
	var hasSelf bool
	for _, r := range raw {
		if r.FromPage == "Hub" {
			hasSelf = true
			break
		}
	}
	if !hasSelf {
		t.Fatalf("fixture precondition: Hub.md should contain a self-mention, raw refs=%+v", raw)
	}

	b := NewBacklinks(idx, "Hub", 80)
	for _, r := range b.refs {
		if r.FromPage == "Hub" {
			t.Errorf("self-reference leaked into NewBacklinks output: %+v", r)
		}
	}
}

func TestBacklinksSetSize(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", 80)
	b.SetSize(120, 99)
	if b.width != 120 {
		t.Errorf("after SetSize: want width 120, got %d", b.width)
	}
}

func TestBacklinksNavigation(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", 80)
	if len(b.refs) < 2 {
		t.Fatalf("need >=2 refs for this test, got %d", len(b.refs))
	}

	b.Update("up")
	if b.sel != 0 {
		t.Errorf("up at top: want sel 0, got %d", b.sel)
	}
	b.Update("down")
	if b.sel != 1 {
		t.Errorf("after down: want sel 1, got %d", b.sel)
	}
	if len(b.refs) > 2 {
		b.Update("j")
		if b.sel != 2 {
			t.Errorf("after j: want sel 2, got %d", b.sel)
		}
	}
	prev := b.sel
	b.Update("k")
	if b.sel >= prev {
		t.Errorf("after k: sel should decrement from %d, got %d", prev, b.sel)
	}
}

func TestBacklinksEnterReturnsFromPage(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", 80)
	if len(b.refs) == 0 {
		t.Skip("no Hub backlinks in fixture — skipping enter test")
	}
	b.sel = 0
	sel, accept, cancel := b.Update("enter")
	if !accept || cancel {
		t.Errorf("enter: want accept=true cancel=false, got %v/%v", accept, cancel)
	}
	if sel != b.refs[0].FromPage {
		t.Errorf("returned page: want %q, got %q", b.refs[0].FromPage, sel)
	}
}

func TestBacklinksEscAndBCancel(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", 80)
	for _, k := range []string{"esc", "b"} {
		sel, accept, cancel := b.Update(k)
		if sel != "" || accept || !cancel {
			t.Errorf("%s: want cancel only, got (%q,%v,%v)", k, sel, accept, cancel)
		}
	}
}

func TestBacklinksNoRefsEnterNoop(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Orphan", 80)
	if len(b.refs) != 0 {
		t.Fatalf("Orphan should have 0 backlinks, got %d", len(b.refs))
	}
	sel, accept, cancel := b.Update("enter")
	if sel != "" || accept || cancel {
		t.Errorf("enter on empty refs: want zero-valued return, got (%q,%v,%v)",
			sel, accept, cancel)
	}
}

func TestBacklinksInnerWidthClamps(t *testing.T) {
	b := &Backlinks{width: 10}
	if got := b.innerWidth(); got != blInnerWidthMin {
		t.Errorf("narrow term: want %d, got %d", blInnerWidthMin, got)
	}
	b.width = 300
	if got := b.innerWidth(); got != blInnerWidthMax {
		t.Errorf("wide term: want %d, got %d", blInnerWidthMax, got)
	}
}

func TestBacklinksViewWithRefsGolden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", 100)
	teatest.RequireEqualOutput(t, []byte(b.View()))
}

func TestBacklinksViewNoRefsGolden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Orphan", 100)
	out := b.View()
	if !strings.Contains(out, "no backlinks") {
		t.Errorf("view should announce empty refs; got:\n%s", out)
	}
	teatest.RequireEqualOutput(t, []byte(out))
}
