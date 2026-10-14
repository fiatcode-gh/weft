package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

var treeJournalDay = time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

func fixtureTreeApp(t *testing.T) *App {
	t.Helper()
	return bootApp(t, bootConfig{now: treeJournalDay})
}

func openTree(t *testing.T, a *App) *Tree {
	t.Helper()
	a.Update(key("F"))
	tr, ok := a.active.(*Tree)
	if !ok {
		t.Fatalf("active overlay = %T, want *Tree", a.active)
	}
	return tr
}

func rowLabels(tr *Tree) []string {
	var out []string
	for _, r := range tr.rows {
		out = append(out, r.node.Label)
	}
	return out
}

func selLabel(t *testing.T, tr *Tree) string {
	t.Helper()
	if tr.sel < 0 || tr.sel >= len(tr.rows) {
		t.Fatalf("sel %d outside %d rows", tr.sel, len(tr.rows))
	}
	return tr.rows[tr.sel].node.Label
}

func treePress(tr *Tree, keys ...string) OverlayResult {
	var res OverlayResult
	for _, k := range keys {
		res = tr.Update(k)
	}
	return res
}

func TestTreeGolden(t *testing.T) {
	quietTerm(t)
	tr := NewTree(loadFixture(t), "kb/notes", treeExpansion{}, 80, 24)
	teatest.RequireEqualOutput(t, []byte(plain(tr.View())))
}

func TestTreeStartsOnFirstRowForJournal(t *testing.T) {
	a := fixtureTreeApp(t)
	tr := openTree(t, a)
	if tr.sel != 0 {
		t.Fatalf("sel = %d, want 0", tr.sel)
	}
	if len(a.treeOpen) != 0 {
		t.Fatalf("treeOpen = %v, want empty", a.treeOpen)
	}
}

func TestTreeStartsOnCurrentPage(t *testing.T) {
	a := bootVault(t)
	a.navigate("b/Note")
	tr := openTree(t, a)
	if !a.treeOpen["b"] || a.treeOpen["a"] {
		t.Fatalf("treeOpen = %v, want only b", a.treeOpen)
	}
	row := tr.rows[tr.sel]
	if row.node.Page == nil || row.node.Page.Rel != "b/Note.md" || row.depth != 1 {
		t.Fatalf("selected %+v depth %d, want b/Note.md at depth 1", row.node, row.depth)
	}
}

func TestTreeMoves(t *testing.T) {
	tr := NewTree(loadFixture(t), "2026_05_24", treeExpansion{}, 80, 24)
	want := []string{"Alpha", "Beta", "Book Club", "Corpus", "Hub", "kb", "Kitchen", "Orphan", "proj", "Workbench"}
	if got := rowLabels(tr); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	treePress(tr, "k")
	if tr.sel != 0 {
		t.Fatalf("k at top: sel = %d", tr.sel)
	}
	treePress(tr, "j", "j", "j", "j", "j")
	if selLabel(t, tr) != "kb" || tr.sel != 5 {
		t.Fatalf("5×j: on %q (row %d)", selLabel(t, tr), tr.sel)
	}
	treePress(tr, "l")
	if tr.sel != 5 || !tr.open["kb"] {
		t.Fatalf("l on collapsed kb: sel %d open %v", tr.sel, tr.open)
	}
	treePress(tr, "l")
	if selLabel(t, tr) != "notes" || tr.sel != 6 || tr.rows[6].depth != 1 {
		t.Fatalf("l on expanded kb: on %q row %d", selLabel(t, tr), tr.sel)
	}
	treePress(tr, "l")
	if tr.sel != 6 {
		t.Fatalf("l on page-only notes moved to %d", tr.sel)
	}
	treePress(tr, "h")
	if selLabel(t, tr) != "kb" || !tr.open["kb"] {
		t.Fatalf("h on notes: on %q open %v", selLabel(t, tr), tr.open)
	}
	treePress(tr, "h")
	if tr.open["kb"] || tr.sel != 5 {
		t.Fatalf("h on expanded kb: sel %d open %v", tr.sel, tr.open)
	}
	treePress(tr, "h")
	if tr.sel != 5 {
		t.Fatalf("h on collapsed top-level kb moved to %d", tr.sel)
	}
	treePress(tr, "j", "j", "j", "j", "j", "j")
	if selLabel(t, tr) != "Workbench" {
		t.Fatalf("j past end: on %q", selLabel(t, tr))
	}
	treePress(tr, "up", "up")
	if selLabel(t, tr) != "Orphan" {
		t.Fatalf("up: on %q", selLabel(t, tr))
	}
	treePress(tr, "down")
	if selLabel(t, tr) != "proj" {
		t.Fatalf("down: on %q", selLabel(t, tr))
	}
	treePress(tr, "right")
	if !tr.open["proj"] {
		t.Fatalf("right did not expand proj: %v", tr.open)
	}
	treePress(tr, "left")
	if tr.open["proj"] {
		t.Fatalf("left did not collapse proj: %v", tr.open)
	}
}

func TestTreeEnter(t *testing.T) {
	tr := NewTree(loadFixture(t), "2026_05_24", treeExpansion{}, 80, 24)
	treePress(tr, "j", "j", "j", "j", "j")
	if res := tr.Update("enter"); res.kind != overlayResultNone || !tr.open["kb"] {
		t.Fatalf("enter on kb: kind %d open %v", res.kind, tr.open)
	}
	if res := tr.Update("enter"); res.kind != overlayResultNone || tr.open["kb"] {
		t.Fatalf("second enter on kb: kind %d open %v", res.kind, tr.open)
	}
	treePress(tr, "k", "k", "k", "k", "k")
	res := tr.Update("enter")
	if res.kind != overlayResultOpen || res.page != "Alpha" {
		t.Fatalf("enter on Alpha: kind %d page %q", res.kind, res.page)
	}
}

func TestTreePageAndGroupRow(t *testing.T) {
	a := bootApp(t, bootConfig{files: map[string]string{
		"pages/a.md":     "# a\n",
		"pages/a___b.md": "# b\n",
	}})
	tr := openTree(t, a)
	view := plain(tr.View())
	if !strings.Contains(view, "▸ a (1)") {
		t.Fatalf("want collapsed page+group row, got:\n%s", view)
	}
	treePress(tr, "l")
	if view = plain(tr.View()); !strings.Contains(view, "▾ a (1)") || !strings.Contains(view, "  b") {
		t.Fatalf("l did not expand a:\n%s", view)
	}
	treePress(tr, "h")
	if res := tr.Update("enter"); res.kind != overlayResultOpen || res.page != "a" {
		t.Fatalf("enter on a: kind %d page %q", res.kind, res.page)
	}
}

func TestTreeCloseAndCaptureKeys(t *testing.T) {
	for _, k := range []string{"esc", "q", "F"} {
		a := fixtureTreeApp(t)
		openTree(t, a)
		_, cmd := a.Update(key(k))
		if a.active != nil {
			t.Errorf("%q left overlay open", k)
		}
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Errorf("%q quit the app", k)
			}
		}
	}
	a := fixtureTreeApp(t)
	tr := openTree(t, a)
	a.Update(key("c"))
	if a.capture == nil || a.active != tr {
		t.Fatalf("c: capture %v, tree still active %v", a.capture != nil, a.active == tr)
	}
}

func TestTreeExpansionKeptAcrossOpenings(t *testing.T) {
	a := fixtureTreeApp(t)
	tr := openTree(t, a)
	treePress(tr, "j", "j", "j", "j", "j", "l")
	a.Update(key("esc"))
	tr = openTree(t, a)
	if !a.treeOpen["kb"] || len(tr.rows) <= 10 {
		t.Fatalf("kb not kept expanded: open %v rows %v", a.treeOpen, rowLabels(tr))
	}
}

var _ panelFeedback = (*Tree)(nil)

func nestedTreeApp(t *testing.T) *App {
	t.Helper()
	return bootApp(t, bootConfig{files: map[string]string{
		"pages/x.md":         "# x\n",
		"pages/x___a.md":     "# a\n",
		"pages/x___a___b.md": "# b\n",
	}})
}

func TestTreeCollapseKeepsDescendantsExpanded(t *testing.T) {
	a := nestedTreeApp(t)
	tr := openTree(t, a)
	treePress(tr, "l", "j", "l")
	if !tr.open["x"] || !tr.open["x/a"] {
		t.Fatalf("open = %v, want x and x/a", tr.open)
	}
	treePress(tr, "k", "h")
	if tr.open["x"] || !tr.open["x/a"] {
		t.Fatalf("after collapsing x: open = %v", tr.open)
	}
	treePress(tr, "l")
	if !tr.open["x"] || !tr.open["x/a"] {
		t.Fatalf("after reopening x: open = %v", tr.open)
	}
	if got := rowLabels(tr); fmt.Sprint(got) != "[x a b]" {
		t.Fatalf("rows = %v, want [x a b]", got)
	}
}

func TestTreeOpensAncestorsNotOwnGroup(t *testing.T) {
	a := nestedTreeApp(t)
	a.navigate("x/a")
	tr := openTree(t, a)
	if !a.treeOpen["x"] || a.treeOpen["x/a"] {
		t.Fatalf("treeOpen = %v, want only x", a.treeOpen)
	}
	if got := selLabel(t, tr); got != "a" {
		t.Fatalf("selected %q, want a", got)
	}
}

func TestVaultTreeOpensLoserFile(t *testing.T) {
	a := bootVault(t)
	tr := openTree(t, a)
	for range tr.rows {
		if selLabel(t, tr) == "b" {
			break
		}
		treePress(tr, "j")
	}
	if selLabel(t, tr) != "b" {
		t.Fatalf("b not among rows %v", rowLabels(tr))
	}
	treePress(tr, "l", "l")
	a.Update(key("enter"))
	if a.active != nil || a.page.Page() != "b/Note" {
		t.Fatalf("page = %q active %v, want b/Note", a.page.Page(), a.active)
	}
	if !strings.Contains(appText(a), "note in b") {
		t.Fatalf("loser file not shown:\n%s", appText(a))
	}
}

func TestTreeIsReadOnly(t *testing.T) {
	a := fixtureTreeApp(t)
	before := tree(t, a.graphPath)
	tr := openTree(t, a)
	treePress(tr, "j", "j", "j", "j", "j", "l", "l", "h", "h", "enter", "enter", "k")
	a.Update(key("enter"))
	a.Update(key("F"))
	a.Update(key("esc"))
	if after := tree(t, a.graphPath); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("graph changed:\nbefore %v\nafter  %v", before, after)
	}
}

func TestTreeScrolls(t *testing.T) {
	files := map[string]string{}
	for i := range 30 {
		files[fmt.Sprintf("pages/p%02d.md", i)] = "# p\n"
	}
	a := bootApp(t, bootConfig{files: files})
	tr := openTree(t, a)
	if v := plain(tr.View()); !strings.Contains(v, "more below") || strings.Contains(v, "more above") {
		t.Fatalf("top view:\n%s", v)
	}
	for range 25 {
		treePress(tr, "j")
	}
	v := plain(tr.View())
	if !strings.Contains(v, selLabel(t, tr)) || !strings.Contains(v, "more above") {
		t.Fatalf("scrolled view lacks %q or 'more above':\n%s", selLabel(t, tr), v)
	}
}

func TestTreeEmpty(t *testing.T) {
	a := bootApp(t, bootConfig{files: map[string]string{"journals/2026_05_24.md": "- x\n"}, now: treeJournalDay})
	tr := openTree(t, a)
	if !strings.Contains(plain(tr.View()), "no pages — journals are not listed here") {
		t.Fatalf("empty message missing:\n%s", plain(tr.View()))
	}
	for _, k := range []string{"enter", "j", "l"} {
		if res := tr.Update(k); res.kind != overlayResultNone {
			t.Fatalf("%q on empty tree: kind %d", k, res.kind)
		}
	}
	a.Update(key("esc"))
	if a.active != nil {
		t.Fatal("esc did not close empty tree")
	}
}

func treeRowOf(t *testing.T, tr *Tree, label string) int {
	t.Helper()
	for i, r := range tr.rows {
		if r.node.Label == label {
			return i
		}
	}
	t.Fatalf("no row %q in %v", label, rowLabels(tr))
	return -1
}

func TestTreeRefreshKeepsSelectedNode(t *testing.T) {
	quietTerm(t)
	tr := NewTree(loadFixture(t), "", treeExpansion{}, 80, 24)
	old := treeRowOf(t, tr, "Workbench")
	tr.sel = old
	path := tr.rows[old].node.Path

	tr.refresh(fixtureIndexWith(t, map[string]string{"pages/Aaa.md": "- new\n"}))

	if tr.sel == old {
		t.Fatalf("row index stayed %d; the added page should shift Workbench", old)
	}
	if got := tr.rows[tr.sel].node.Path; got != path {
		t.Errorf("selected node = %q, want %q", got, path)
	}
}

func TestTreeRefreshMovesToNearestRowWhenGone(t *testing.T) {
	quietTerm(t)
	tr := NewTree(loadFixture(t), "", treeExpansion{}, 80, 24)
	old := treeRowOf(t, tr, "Workbench")
	tr.sel = old
	path := tr.rows[old].node.Path

	tr.refresh(fixtureIndexWith(t, map[string]string{"pages/Workbench.md": ""}))

	if want := clampInt(old, 0, len(tr.rows)-1); tr.sel != want {
		t.Errorf("sel = %d, want %d", tr.sel, want)
	}
	if got := tr.rows[tr.sel].node.Path; got == path {
		t.Errorf("removed node %q still selected", got)
	}
}
