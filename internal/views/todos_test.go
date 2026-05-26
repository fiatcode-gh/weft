package views

import (
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
)

func TestTodosDashboardAllFilter(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100)
	teatest.RequireEqualOutput(t, []byte(td.View()))
}

func TestTodosDashboardLaterFilter(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100)
	td.Update("t") // → TODO
	td.Update("t") // → LATER
	teatest.RequireEqualOutput(t, []byte(td.View()))
}

func TestTodosUpDownBounds(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100)
	if len(td.visible) < 2 {
		t.Fatalf("setup: need >=2 visible todos, got %d", len(td.visible))
	}
	td.Update("up")
	if td.sel != 0 {
		t.Errorf("up at top: want sel 0, got %d", td.sel)
	}
	td.Update("down")
	if td.sel != 1 {
		t.Errorf("after down: want sel 1, got %d", td.sel)
	}
	prev := td.sel
	td.Update("j")
	if td.sel <= prev {
		t.Errorf("after j: sel should advance from %d, got %d", prev, td.sel)
	}
	td.Update("k")
	if td.sel != prev {
		t.Errorf("after k: sel should restore to %d, got %d", prev, td.sel)
	}
}

func TestTodosEnterReturnsPage(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100)
	if len(td.visible) == 0 {
		t.Fatal("setup: no visible todos")
	}
	want := td.visible[0].Page
	page, accept, cancel := td.Update("enter")
	if !accept || cancel {
		t.Errorf("enter: want accept=true cancel=false, got %v/%v", accept, cancel)
	}
	if page != want {
		t.Errorf("returned page: want %q, got %q", want, page)
	}
}

func TestTodosFilterCycleEndsAtAll(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100)
	if td.filter != "" {
		t.Fatalf("initial filter: want \"\", got %q", td.filter)
	}
	expected := []string{"TODO", "LATER", "DOING", "WAITING", ""}
	for i, want := range expected {
		td.Update("t")
		if td.filter != want {
			t.Errorf("step %d: want filter %q, got %q", i, want, td.filter)
		}
	}
}

func TestTodosEscAndQCancel(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	for _, k := range []string{"esc", "q"} {
		td := NewTodos(loadFixture(t), 100)
		page, accept, cancel := td.Update(k)
		if page != "" || accept || !cancel {
			t.Errorf("%s: want cancel only, got (%q,%v,%v)", k, page, accept, cancel)
		}
	}
}

func TestTodosSetSize(t *testing.T) {
	td := &Todos{width: 80}
	td.SetSize(120, 99)
	if td.width != 120 {
		t.Errorf("SetSize: want width 120, got %d", td.width)
	}
}

func TestTodosSelClampedOnFilter(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100)
	if len(td.visible) < 2 {
		t.Skip("not enough bullets to clamp test")
	}
	td.sel = len(td.visible) - 1
	td.Update("t") // → TODO (almost certainly fewer bullets)
	if td.sel < 0 || td.sel >= len(td.visible) {
		t.Errorf("after filter: sel %d out of range [0,%d)", td.sel, len(td.visible))
	}
}
