package views

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/sahilm/fuzzy"
)

func TestPickerFiltersOnQuery(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPicker(loadFixture(t), 80, 30)
	// Zero out mtimes so the relative-time hint doesn't drift with
	// checkout time. relativeTime returns "" for a zero mtime.
	for i := range p.choices {
		p.choices[i].mtime = time.Time{}
	}
	p.Update("a") // type 'a'
	p.Update("l")
	p.Update("p")
	teatest.RequireEqualOutput(t, []byte(p.View()))
}

func TestConsumeKeyAppendsRune(t *testing.T) {
	ti := textinput.New()
	ti, ok := consumeKey(ti, "x")
	if !ok || ti.Value() != "x" {
		t.Errorf("rune key: want \"x\"/true, got %q/%v", ti.Value(), ok)
	}
}

func TestConsumeKeyBackspace(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("abc")
	ti, ok := consumeKey(ti, "backspace")
	if !ok || ti.Value() != "ab" {
		t.Errorf("backspace: want \"ab\"/true, got %q/%v", ti.Value(), ok)
	}
	ti.SetValue("")
	ti, ok = consumeKey(ti, "backspace")
	if !ok || ti.Value() != "" {
		t.Errorf("backspace on empty: want \"\"/true, got %q/%v", ti.Value(), ok)
	}
}

func TestConsumeKeySpaceVariants(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("a")
	ti, ok := consumeKey(ti, " ")
	if !ok || ti.Value() != "a " {
		t.Errorf("literal space: want \"a \"/true, got %q/%v", ti.Value(), ok)
	}
	ti, ok = consumeKey(ti, "space")
	if !ok || ti.Value() != "a  " {
		t.Errorf("named space: want \"a  \"/true, got %q/%v", ti.Value(), ok)
	}
}

func TestConsumeKeyMultiCharNoop(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("a")
	_, ok := consumeKey(ti, "ctrl+x")
	if ok {
		t.Errorf("multi-char key should not be consumed")
	}
}

func TestPickerUpDownBounds(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPicker(loadFixture(t), 80, 30)
	if len(p.matches) < 2 {
		t.Fatalf("setup: need >=2 matches, got %d", len(p.matches))
	}

	p.Update("up")
	if p.sel != 0 {
		t.Errorf("up at top: want sel 0, got %d", p.sel)
	}
	p.Update("down")
	if p.sel != 1 {
		t.Errorf("after down: want sel 1, got %d", p.sel)
	}
	p.Update("ctrl+j")
	if p.sel != 2 && len(p.matches) > 2 {
		t.Errorf("after ctrl+j: want sel 2, got %d", p.sel)
	}
	p.Update("ctrl+k")
	if p.sel < 0 {
		t.Errorf("after ctrl+k: sel went negative, got %d", p.sel)
	}
}

func TestPickerEnterReturnsSelected(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPicker(loadFixture(t), 80, 30)
	if len(p.matches) == 0 {
		t.Fatal("setup: no matches")
	}
	want := p.matches[0].Str
	sel, accept, cancel := p.Update("enter")
	if !accept || cancel {
		t.Errorf("enter: want accept=true cancel=false, got %v/%v", accept, cancel)
	}
	if sel != want {
		t.Errorf("returned name: want %q, got %q", want, sel)
	}
}

func TestPickerEnterEmptyNoop(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPicker(loadFixture(t), 80, 30)
	for _, r := range "zzzzzzzzznotapage" {
		p.Update(string(r))
	}
	if len(p.matches) != 0 {
		t.Fatalf("setup: query should yield no matches, got %d", len(p.matches))
	}
	sel, accept, cancel := p.Update("enter")
	if sel != "" || accept || cancel {
		t.Errorf("enter with no matches: want zeros, got (%q,%v,%v)", sel, accept, cancel)
	}
}

func TestPickerEscCancels(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPicker(loadFixture(t), 80, 30)
	sel, accept, cancel := p.Update("esc")
	if sel != "" || accept || !cancel {
		t.Errorf("esc: want cancel only, got (%q,%v,%v)", sel, accept, cancel)
	}
}

func TestPickerScrollWindowAllFit(t *testing.T) {
	p := &Picker{height: 24}
	p.matches = make([]fuzzy.Match, 5)
	start, end := p.scrollWindow()
	if start != 0 || end != 5 {
		t.Errorf("all-fit: want [0,5), got [%d,%d)", start, end)
	}
}

func TestPickerScrollWindowAtBottom(t *testing.T) {
	p := &Picker{height: 16}
	p.matches = make([]fuzzy.Match, 20)
	p.sel = 19
	start, end := p.scrollWindow()
	if end != 20 || (end-start) != p.visibleRows() {
		t.Errorf("at bottom: want end=20 window=%d, got [%d,%d)",
			p.visibleRows(), start, end)
	}
	if p.sel < start || p.sel >= end {
		t.Errorf("sel %d should be in window [%d,%d)", p.sel, start, end)
	}
}

func TestPickerScrollWindowMiddle(t *testing.T) {
	p := &Picker{height: 24}
	p.matches = make([]fuzzy.Match, 30)
	p.sel = 15
	start, end := p.scrollWindow()
	if p.sel < start || p.sel >= end {
		t.Errorf("sel %d should be in window [%d,%d)", p.sel, start, end)
	}
}

func TestPickerSetSize(t *testing.T) {
	p := &Picker{width: 80, height: 24}
	p.SetSize(100, 30)
	if p.width != 100 || p.height != 30 {
		t.Errorf("SetSize: want 100x30, got %dx%d", p.width, p.height)
	}
}

func TestPadTo(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want string
	}{
		{"abc", 5, "abc  "},
		{"abc", 3, "abc"},
		{"abc", 2, "abc"},
		{"", 3, "   "},
	}
	for _, c := range cases {
		if got := padTo(c.in, c.w); got != c.want {
			t.Errorf("padTo(%q,%d): want %q, got %q", c.in, c.w, c.want, got)
		}
	}
}
