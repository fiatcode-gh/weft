package views

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)

func TestTodosDashboardAllFilter(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100, 30)
	teatest.RequireEqualOutput(t, []byte(td.View()))
}

func TestTodosDashboardLaterFilter(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100, 30)
	td.Update("t") // → TODO
	td.Update("t") // → LATER
	teatest.RequireEqualOutput(t, []byte(td.View()))
}

func TestTodosUpDownBounds(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100, 30)
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
	td := NewTodos(loadFixture(t), 100, 30)
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
	td := NewTodos(loadFixture(t), 100, 30)
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
		td := NewTodos(loadFixture(t), 100, 30)
		page, accept, cancel := td.Update(k)
		if page != "" || accept || !cancel {
			t.Errorf("%s: want cancel only, got (%q,%v,%v)", k, page, accept, cancel)
		}
	}
}

func TestTodosSetSize(t *testing.T) {
	td := &Todos{listBox: listBox{width: 80}}
	td.SetSize(120, 99)
	if td.width != 120 {
		t.Errorf("SetSize: want width 120, got %d", td.width)
	}
}

func TestTodosSelClampedOnFilter(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100, 30)
	if len(td.visible) < 2 {
		t.Skip("not enough bullets to clamp test")
	}
	td.sel = len(td.visible) - 1
	td.Update("t") // → TODO (almost certainly fewer bullets)
	if td.sel < 0 || td.sel >= len(td.visible) {
		t.Errorf("after filter: sel %d out of range [0,%d)", td.sel, len(td.visible))
	}
}

// TestTodosLongRowStaysOneLine guards against layout shift from unclamped
// long todos: without clamping, a long bullet wraps inside the panel —
// continuation lines have no hanging indent (they look like sibling
// bullets) and a wrapped selected row carries the ▶ marker only on its
// first visible line.
// TestTodosScrollWindowKeepsSelectionVisible asserts that the computed
// window always contains t.sel, even when sel sits at the far ends of
// a list much larger than the terminal-row budget.
func TestTodosScrollWindowKeepsSelectionVisible(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	// Replace fixture todos with 60 bullets across 12 groups so the
	// list is comfortably larger than the visibleRows budget at any
	// realistic terminal height.
	idx.Todos = idx.Todos[:0]
	for g := 0; g < 12; g++ {
		for i := 0; i < 5; i++ {
			idx.Todos = append(idx.Todos, graph.TodoBullet{
				Page:       fmt.Sprintf("Page-%02d", g),
				LineNumber: i + 1,
				Marker:     "TODO",
				Text:       fmt.Sprintf("item %d/%d", g, i),
			})
		}
	}

	td := NewTodos(idx, 80, 24)

	for _, sel := range []int{0, 25, 50, len(td.visible) - 1} {
		td.sel = sel
		start, end := td.computeWindow()
		if sel < start || sel >= end {
			t.Errorf("sel %d should be in [%d,%d)", sel, start, end)
		}
	}
}

// TestTodosScrollHintsAppear asserts the "more above" / "more below"
// chrome appears when bullets fall outside the rendered window.
func TestTodosScrollHintsAppear(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	idx.Todos = idx.Todos[:0]
	for g := 0; g < 8; g++ {
		for i := 0; i < 4; i++ {
			idx.Todos = append(idx.Todos, graph.TodoBullet{
				Page:       fmt.Sprintf("Page-%02d", g),
				LineNumber: i + 1,
				Marker:     "TODO",
				Text:       fmt.Sprintf("item %d/%d", g, i),
			})
		}
	}

	td := NewTodos(idx, 80, 24)

	td.sel = len(td.visible) / 2
	mid := td.View()
	if !strings.Contains(mid, "more above") {
		t.Errorf("mid selection: want \"more above\" hint; got:\n%s", mid)
	}
	if !strings.Contains(mid, "more below") {
		t.Errorf("mid selection: want \"more below\" hint; got:\n%s", mid)
	}

	td.sel = 0
	top := td.View()
	if strings.Contains(top, "more above") {
		t.Errorf("top selection: should not show \"more above\"; got:\n%s", top)
	}
	if !strings.Contains(top, "more below") {
		t.Errorf("top selection: want \"more below\" hint; got:\n%s", top)
	}
}

func TestTodosLongRowStaysOneLine(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")

	// Build an index with a single very long bullet so the scroll window
	// is guaranteed to include it.
	idx := loadFixture(t)
	idx.Todos = []graph.TodoBullet{{
		Page:       "LongCase",
		LineNumber: 1,
		Marker:     "TODO",
		Text:       strings.Repeat("really long text ", 30), // ~510 chars
	}}

	td := NewTodos(idx, 80, 30)
	td.recompute()
	if len(td.visible) != 1 {
		t.Fatalf("setup: want 1 visible bullet, got %d", len(td.visible))
	}

	// With clamp, the bullet renders on exactly one row whether selected
	// or not. Without clamp, lipgloss would wrap the row across multiple
	// lines and only the first carried the ▶ marker.
	td.sel = 0
	if got := countTodoBlockLines(td.View(), "LongCase"); got != 1 {
		t.Errorf("selected long row should occupy exactly one bullet line, got %d", got)
	}
}

// countTodoBlockLines counts how many rendered rows belong to the bullet
// list under pageHeader inside a todos View() snapshot. The block ends at
// the next blank line OR at a scroll-position hint ("more above" /
// "more below"), since those are chrome, not bullet rows.
func countTodoBlockLines(view, pageHeader string) int {
	lines := strings.Split(view, "\n")
	start := -1
	for i, l := range lines {
		if strings.Contains(l, pageHeader) && !strings.Contains(l, "▶") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return 0
	}
	n := 0
	for j := start; j < len(lines); j++ {
		inner := strings.TrimSuffix(strings.TrimPrefix(lines[j], "│"), "│")
		if strings.TrimSpace(inner) == "" {
			break
		}
		if strings.Contains(inner, "more above") || strings.Contains(inner, "more below") {
			break
		}
		n++
	}
	return n
}
