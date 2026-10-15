package views

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/fiatcode-gh/weft/v3/internal/graph"
)

func TestTodosDashboardAllFilter(t *testing.T) {
	quietTerm(t)
	td := NewTodos(loadFixture(t), 100, 30)
	teatest.RequireEqualOutput(t, []byte(plain(td.View())))
}

func TestTodosDashboardLaterFilter(t *testing.T) {
	quietTerm(t)
	td := NewTodos(loadFixture(t), 100, 30)
	td.Update("t") // → TODO
	td.Update("t") // → LATER
	teatest.RequireEqualOutput(t, []byte(plain(td.View())))
}

func TestTodosUpDownBounds(t *testing.T) {
	quietTerm(t)
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
	quietTerm(t)
	td := NewTodos(loadFixture(t), 100, 30)
	if len(td.visible) == 0 {
		t.Fatal("setup: no visible todos")
	}
	want := td.visible[0].Page
	res := td.Update("enter")
	if res.kind != overlayResultOpenTask || res.page != want || res.taskOrdinal != td.visible[0].Ordinal {
		t.Errorf("enter result: want open task %q/%d, got %+v", want, td.visible[0].Ordinal, res)
	}
}

func TestTodosFilterCycleEndsAtAll(t *testing.T) {
	quietTerm(t)
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
	quietTerm(t)
	for _, k := range []string{"esc", "q"} {
		td := NewTodos(loadFixture(t), 100, 30)
		res := td.Update(k)
		if res.kind != overlayResultCancel {
			t.Errorf("%s: want cancel, got %+v", k, res)
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
	quietTerm(t)
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

// TestTodosScrollWindowKeepsSelectionVisible asserts that the computed
// window always contains t.sel, even when sel sits at the far ends of
// a list much larger than the terminal-row budget.
func TestTodosScrollWindowKeepsSelectionVisible(t *testing.T) {
	quietTerm(t)
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
	quietTerm(t)
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
	mid := plain(td.View())
	if !strings.Contains(mid, "more above") {
		t.Errorf("mid selection: want \"more above\" hint; got:\n%s", mid)
	}
	if !strings.Contains(mid, "more below") {
		t.Errorf("mid selection: want \"more below\" hint; got:\n%s", mid)
	}

	td.sel = 0
	top := plain(td.View())
	if strings.Contains(top, "more above") {
		t.Errorf("top selection: should not show \"more above\"; got:\n%s", top)
	}
	if !strings.Contains(top, "more below") {
		t.Errorf("top selection: want \"more below\" hint; got:\n%s", top)
	}
}

// TestTodosLongRowStaysOneLine guards against layout shift from unclamped
// long todos: without clamping, a long bullet wraps inside the panel —
// continuation lines have no hanging indent (they look like sibling
// bullets) and a wrapped selected row carries the ▶ marker only on its
// first visible line.
func TestTodosLongRowStaysOneLine(t *testing.T) {
	quietTerm(t)

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
	if got := countTodoBlockLines(plain(td.View()), "LongCase"); got != 1 {
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

// todosAt builds the dashboard over the fixture, narrowed to filter ("" for
// none), with the row for text selected.
func todosAt(t *testing.T, filter, text string) (*Todos, *graph.Index) {
	t.Helper()
	quietTerm(t)
	idx := loadFixture(t)
	td := NewTodos(idx, 100, 30)
	for td.filter != filter {
		td.Update("t")
	}
	for i, b := range td.visible {
		if b.Text == text {
			td.sel = i
			return td, idx
		}
	}
	t.Fatalf("no row %q under filter %q", text, filter)
	return nil, nil
}

// withoutTask is a copy of idx whose open todos lack the task at page+line, as
// after a reindex that follows marking it DONE.
func withoutTask(idx *graph.Index, page string, line int) *graph.Index {
	cp := *idx
	cp.Todos = nil
	for _, b := range idx.Todos {
		if b.Page == page && b.LineNumber == line {
			continue
		}
		cp.Todos = append(cp.Todos, b)
	}
	return &cp
}

func TestTodosRowShowsDates(t *testing.T) {
	td, _ := todosAt(t, "LATER", "Plane the walnut slab")
	if v := plain(td.View()); !strings.Contains(v, "Plane the walnut slab") ||
		!strings.Contains(v, "scheduled 2026-05-28 · deadline 2026-05-27") {
		t.Errorf("dated row missing its label:\n%s", v)
	}
}

func TestTodosXReturnsMarkTask(t *testing.T) {
	td, _ := todosAt(t, "", "Replace the dust collector filter")
	b := td.visible[td.sel]

	res := td.Update("x")

	want := taskMark{page: b.Page, line: b.LineNumber, from: "TODO", to: "DONE", tail: "[#B] Replace the dust collector filter"}
	if res.kind != overlayResultMarkTask || res.mark != want {
		t.Errorf("x result = %+v, want mark %+v", res, want)
	}
}

func TestTodosMarkedRowStaysStruck(t *testing.T) {
	td, idx := todosAt(t, "LATER", "Plane the walnut slab")
	b := td.visible[td.sel]
	m := td.Update("x").mark

	td.marked(m, b.LineNumber, withoutTask(idx, b.Page, b.LineNumber))

	var row string
	for _, l := range strings.Split(td.View(), "\n") {
		if strings.Contains(plain(l), "DONE Plane the walnut slab") {
			row = l
		}
	}
	if row == "" {
		t.Fatalf("done row missing:\n%s", plain(td.View()))
	}
	if !regexp.MustCompile(`\x1b\[(?:[0-9]+;)*9(?:;[0-9]+)*m`).MatchString(row) {
		t.Errorf("done row has no strikethrough SGR: %q", row)
	}
	if res := td.Update("enter"); res.kind != overlayResultOpen || res.page != b.Page {
		t.Errorf("enter on a done row = %+v, want open %q", res, b.Page)
	}
	back := td.Update("x").mark
	if back.from != "DONE" || back.to != "LATER" || back.line != b.LineNumber {
		t.Errorf("undo mark = %+v, want DONE→LATER at line %d", back, b.LineNumber)
	}
}

func TestTodosRebuildKeepsSelectionAndFilter(t *testing.T) {
	td, idx := todosAt(t, "LATER", "Plane the walnut slab")
	b := td.visible[td.sel]
	m := td.Update("x").mark

	// The rebuilt index no longer lists the task; selection stays on its done row.
	td.marked(m, b.LineNumber, withoutTask(idx, b.Page, b.LineNumber))

	if td.filter != "LATER" {
		t.Errorf("filter = %q, want LATER", td.filter)
	}
	got := td.visible[td.sel]
	if got.Page != b.Page || got.LineNumber != b.LineNumber {
		t.Errorf("selection = %s:%d, want %s:%d", got.Page, got.LineNumber, b.Page, b.LineNumber)
	}
}

func TestTodosMarkedRelocatedFollowsNewLine(t *testing.T) {
	td, idx := todosAt(t, "LATER", "Plane the walnut slab")
	b := td.visible[td.sel]
	m := td.Update("x").mark
	shifted := withoutTask(idx, b.Page, b.LineNumber)
	shifted.Todos = slices.Clone(shifted.Todos)
	for i := range shifted.Todos {
		if shifted.Todos[i].Page == b.Page {
			shifted.Todos[i].LineNumber++
		}
	}

	td.marked(m, b.LineNumber+1, shifted)

	got := td.visible[td.sel]
	if got.Page != b.Page || got.LineNumber != b.LineNumber+1 || got.Text != b.Text {
		t.Fatalf("selection = %s:%d %q, want the done row at %s:%d", got.Page, got.LineNumber, got.Text, b.Page, b.LineNumber+1)
	}
	if _, ok := td.done[taskKey{b.Page, b.LineNumber + 1}]; !ok {
		t.Errorf("done key did not follow the line: %v", td.done)
	}
	if back := td.Update("x").mark; back.line != b.LineNumber+1 || back.from != "DONE" {
		t.Errorf("x on the done row = %+v, want DONE at line %d", back, b.LineNumber+1)
	}
}

func TestTodosOpenTaskOnStaleDoneKeyStaysVisible(t *testing.T) {
	td, idx := todosAt(t, "", "Plane the walnut slab")
	b := td.visible[td.sel]
	td.done[taskKey{b.Page, b.LineNumber}] = b
	other := *idx
	other.Todos = []graph.TodoBullet{{Page: b.Page, LineNumber: b.LineNumber, Marker: "TODO", Text: "a different task"}}
	td.idx = &other

	td.recompute()

	var seen []string
	for _, v := range td.visible {
		seen = append(seen, v.Text)
	}
	if !slices.Contains(seen, "a different task") {
		t.Errorf("open task hidden by a stale done row; visible = %v", seen)
	}
	if slices.Contains(seen, b.Text) {
		t.Errorf("stale done row still listed; visible = %v", seen)
	}
}

// todosMarkedDone marks the task whose text contains text on page as DONE the
// way the App does: the file changes, then marked gets the new index.
func todosMarkedDone(t *testing.T, td *Todos, page, rel, text string) {
	t.Helper()
	var row graph.TodoBullet
	for _, b := range td.visible {
		if b.Page == page && strings.Contains(b.Text, text) {
			row = b
		}
	}
	if row.Page == "" {
		t.Fatalf("no task %q on %s", text, page)
	}
	body := strings.Replace(fixtureBody(t, rel), "- "+row.Marker+" ", "- DONE ", 1)
	td.marked(taskMark{page: page, line: row.LineNumber, from: row.Marker, to: "DONE"}, row.LineNumber,
		fixtureIndexWith(t, map[string]string{rel: body}))
}

func todosRowOf(t *testing.T, td *Todos, text string) int {
	t.Helper()
	for i, b := range td.visible {
		if strings.Contains(b.Text, text) {
			return i
		}
	}
	t.Fatalf("no row %q", text)
	return -1
}

func TestTodosRefreshKeepsSelectionAndDoneRows(t *testing.T) {
	quietTerm(t)
	td := NewTodos(loadFixture(t), 100, 30)
	todosMarkedDone(t, td, "Alpha", "pages/Alpha.md", "Buy milk")
	td.sel = todosRowOf(t, td, "Replace the bench grinder belt")
	old := td.sel
	key, _ := td.selectedKey()

	idx := fixtureIndexWith(t, map[string]string{
		"pages/Alpha.md": strings.Replace(fixtureBody(t, "pages/Alpha.md"), "- TODO Buy milk", "- DONE Buy milk", 1),
		"pages/Aaa.md":   "- TODO Added elsewhere\n",
	})
	td.refresh(idx)

	if td.sel == old {
		t.Fatalf("row index stayed %d; the added task should shift it", old)
	}
	if got, _ := td.selectedKey(); got != key {
		t.Errorf("selected = %+v, want %+v", got, key)
	}
	if len(td.done) != 1 {
		t.Fatalf("done rows = %d, want 1", len(td.done))
	}
	for k := range td.done {
		if !slices.ContainsFunc(td.visible, func(b graph.TodoBullet) bool { return (taskKey{b.Page, b.LineNumber}) == k }) {
			t.Errorf("done row %+v no longer listed", k)
		}
	}
}

func TestTodosRefreshNearestWhenGone(t *testing.T) {
	quietTerm(t)
	td := NewTodos(loadFixture(t), 100, 30)
	td.sel = todosRowOf(t, td, "Tidy up the hub")
	old := td.sel
	key, _ := td.selectedKey()

	td.refresh(fixtureIndexWith(t, map[string]string{"pages/Hub.md": "title:: Hub\n"}))

	if want := min(old, len(td.visible)-1); td.sel != want {
		t.Errorf("sel = %d, want %d", td.sel, want)
	}
	if got, _ := td.selectedKey(); got == key {
		t.Errorf("gone task %+v still selected", got)
	}
}

func TestTodosClearDoneDropsStruckRows(t *testing.T) {
	quietTerm(t)
	td := NewTodos(loadFixture(t), 100, 30)
	todosMarkedDone(t, td, "Alpha", "pages/Alpha.md", "Buy milk")
	if len(td.done) != 1 {
		t.Fatalf("setup: done rows = %d, want 1", len(td.done))
	}
	struck := td.done

	td.clearDone()

	if len(td.done) != 0 {
		t.Errorf("done = %v, want empty", td.done)
	}
	for k := range struck {
		for _, b := range td.visible {
			if (taskKey{b.Page, b.LineNumber}) == k {
				t.Errorf("struck row %+v still listed as %q", k, b.Marker)
			}
		}
	}
}
