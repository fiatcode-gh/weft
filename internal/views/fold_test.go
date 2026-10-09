package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// foldApp boots an app on page P (with a second page Q) at 80x24 and the
// notty look the suite runs in.
func foldApp(t *testing.T, p string) *App {
	t.Helper()
	quietTerm(t)
	a := newApp(t, map[string]string{"pages/P.md": p, "pages/Q.md": "- q [[P]]\n"}, 80, 24)
	a.navigate("P")
	return a
}

func frame(a *App) string { return a.View().Content }

func tab(a *App) { a.Update(keyTabK) }

// down moves the row cursor n non-blank rows.
func down(a *App, n int) {
	for range n {
		a.Update(key("j"))
	}
}

// markerRow is the frame row carrying a fold marker, "" when there is none.
func markerRow(a *App) string {
	for _, l := range frameLines(a) {
		if strings.Contains(l, "▸") {
			return strings.TrimRight(l, " ")
		}
	}
	return ""
}

func wantAbsent(t *testing.T, a *App, parts ...string) {
	t.Helper()
	f := appText(a)
	for _, s := range parts {
		if strings.Contains(f, s) {
			t.Errorf("frame still shows %q:\n%s", s, f)
		}
	}
}

func wantPresent(t *testing.T, a *App, parts ...string) {
	t.Helper()
	f := appText(a)
	for _, s := range parts {
		if !strings.Contains(f, s) {
			t.Errorf("frame lacks %q:\n%s", s, f)
		}
	}
}

func TestTabFoldsATXSection(t *testing.T) {
	a := foldApp(t, "## A\ntext a\n### A1\ntext a1\n## B\ntext b\n")
	before := frame(a)

	tab(a)

	wantAbsent(t, a, "text a", "A1", "text a1")
	wantPresent(t, a, "B", "text b")
	if m := markerRow(a); !strings.Contains(m, "A") || !strings.HasSuffix(m, "▸ 3 lines") {
		t.Errorf("marker row = %q, want the A row ending in %q", m, "▸ 3 lines")
	}
	tab(a)
	if got := frame(a); got != before {
		t.Errorf("unfolding did not restore the frame:\n%s\nwant\n%s", plain(got), plain(before))
	}
}

func TestTabFoldsSetextSection(t *testing.T) {
	a := foldApp(t, "Title\n=====\ntext\n\nSub\n---\nmore\n")
	tab(a)
	wantAbsent(t, a, "text", "Sub", "more")
	wantPresent(t, a, "Title")
	if m := markerRow(a); !strings.HasSuffix(m, "▸ 5 lines") {
		t.Errorf("marker row = %q, want ▸ 5 lines", m)
	}
}

func TestTabFoldsBulletChildren(t *testing.T) {
	a := foldApp(t, "- a\n  - a1\n- b\n  - b1\n")
	tab(a)
	wantAbsent(t, a, "a1")
	wantPresent(t, a, "a", "b", "b1")
	if m := markerRow(a); !strings.HasSuffix(m, "▸ 1 line") {
		t.Errorf("marker row = %q, want ▸ 1 line", m)
	}
}

func TestTabOnLeafBulletAndParagraphSaysSo(t *testing.T) {
	a := foldApp(t, "- leaf\n- other\n\nplain paragraph\n")
	before := frame(a)
	for _, moves := range []int{0, 2} {
		down(a, moves)
		a.hint = ""
		tab(a)
		if a.hint != "nothing to fold here" {
			t.Errorf("after %d moves hint = %q, want %q", moves, a.hint, "nothing to fold here")
		}
	}
	a.Update(key("g"))
	if got := frame(a); got != before {
		t.Errorf("a refused fold changed the frame:\n%s\nwant\n%s", plain(got), plain(before))
	}
}

func TestFoldsNest(t *testing.T) {
	a := foldApp(t, "## A\ntext a\n### A1\ntext a1\n## B\ntext b\n")
	down(a, 2) // A1
	tab(a)
	wantAbsent(t, a, "text a1")
	a.Update(key("g"))
	tab(a)
	wantAbsent(t, a, "text a", "A1")
	tab(a)
	wantPresent(t, a, "text a", "A1")
	wantAbsent(t, a, "text a1")
}

func TestZCyclesLevels(t *testing.T) {
	a := foldApp(t, "# H1\n- top1\n  - child1\n- top2\n  - child2\n\n## H2\ntext\n")
	rows := func() string { l := frameLines(a); return strings.Join(l[:len(l)-1], "\n") } // the status line carries the hint
	all := rows()

	a.Update(key("z"))
	if a.hint != "fold: top-level bullets" {
		t.Errorf("hint = %q", a.hint)
	}
	wantAbsent(t, a, "child1", "child2")
	wantPresent(t, a, "top1", "top2", "H2", "text")
	if m := markerRow(a); !strings.HasSuffix(m, "▸ 1 line") {
		t.Errorf("marker row = %q", m)
	}

	tab(a) // the cursor is on H1: Tab folds it on top of level 1
	wantAbsent(t, a, "top1", "H2")
	tab(a)
	wantAbsent(t, a, "child1")
	wantPresent(t, a, "top1")

	a.Update(key("z"))
	if a.hint != "fold: headings only" {
		t.Errorf("hint = %q", a.hint)
	}
	wantAbsent(t, a, "top1", "H2", "text")
	wantPresent(t, a, "H1")

	a.Update(key("z"))
	if a.hint != "fold: all shown" {
		t.Errorf("hint = %q", a.hint)
	}
	if got := rows(); got != all {
		t.Errorf("level 0 differs from the unfolded page:\n%s\nwant\n%s", got, all)
	}

	tab(a) // a single Tab fold, then z replaces it
	wantAbsent(t, a, "top1")
	a.Update(key("z"))
	wantPresent(t, a, "top1", "H2")
	wantAbsent(t, a, "child1")
}

func TestFoldMarkerSingular(t *testing.T) {
	a := foldApp(t, "- a\n  - a1\n- b\n")
	tab(a)
	if m := markerRow(a); !strings.HasSuffix(m, "▸ 1 line") || strings.Contains(m, "lines") {
		t.Errorf("marker row = %q", m)
	}
}

func TestFoldMarkerFaintUnderNoColor(t *testing.T) {
	a := foldApp(t, "- a\n  - a1\n- b\n")
	tab(a)
	down(a, 1) // off the folded row, so its marker is not in the cursor style
	if !strings.Contains(frame(a), "\x1b[2m ▸ 1 line") {
		t.Errorf("marker is not faint under NO_COLOR:\n%q", frame(a))
	}
}

func TestNSkipsFoldedLinks(t *testing.T) {
	a := foldApp(t, "## A\n[[Q]] one\n## B\n[[Q]] two\n")
	tab(a)
	a.Update(key("n"))
	if got := a.page.Cursor(); got != 1 {
		t.Errorf("n selected link %d, want the visible link 1", got)
	}
	a.Update(key("n"))
	a.Update(key("N"))
	if got := a.page.Cursor(); got != 1 {
		t.Errorf("link cursor = %d, want 1 (the only visible link)", got)
	}
}

func TestFoldHidingSelectedLinkDeselects(t *testing.T) {
	a := foldApp(t, "## A\n[[Q]] one\n## B\ntext\n")
	a.Update(key("n"))
	if a.page.Cursor() != 0 {
		t.Fatalf("setup: link cursor = %d", a.page.Cursor())
	}
	a.Update(key("k"))
	tab(a)
	if got := a.page.Cursor(); got != -1 {
		t.Errorf("link cursor = %d after its row was folded away, want -1", got)
	}
}

func TestTaskJumpUnfolds(t *testing.T) {
	a := foldApp(t, "## A\n- TODO task one\n## B\ntext\n")
	tab(a)
	wantAbsent(t, a, "task one")
	a.navigateToTask("P", 0)
	wantPresent(t, a, "task one")
	if m := markerRow(a); m != "" {
		t.Errorf("fold kept over the jump target: %q", m)
	}
}

func TestBacklinkJumpUnfolds(t *testing.T) {
	a := foldApp(t, "## A\nsee [[Q]] here\n## B\ntext\n")
	tab(a)
	wantAbsent(t, a, "see")
	a.navigateFocusingLink("P", "Q")
	wantPresent(t, a, "see")
	if a.page.Cursor() != 0 {
		t.Errorf("link cursor = %d, want 0", a.page.Cursor())
	}
}

func TestFindJumpUnfolds(t *testing.T) {
	a := foldApp(t, "## A\nthe needle sits here\n## B\ntext\n")
	tab(a)
	wantAbsent(t, a, "needle")
	a.navigateHighlighting("P", "needle")
	wantPresent(t, a, "needle")
}

func TestEscOnHiddenLineUnfolds(t *testing.T) {
	a := foldApp(t, "## A\nhidden text\n## B\nb text\n## C\nc text\n")
	tab(a)
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	a.Update(esc)
	if a.editor != nil {
		t.Fatal("editor did not close")
	}
	wantPresent(t, a, "hidden text")
	if got := a.page.rowText(a.page.CursorRow()); !strings.Contains(plain(got), "hidden text") {
		t.Errorf("cursor row = %q, want the revealed line", plain(got))
	}
}

func TestEscOnHiddenLineKeepsOtherFolds(t *testing.T) {
	a := foldApp(t, "## A\nhidden text\n## B\nb text\n## C\nc text\n")
	down(a, 4)
	tab(a) // fold C
	a.Update(key("g"))
	tab(a) // fold A
	a.Update(key("e"))
	a.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	a.Update(esc)
	wantPresent(t, a, "hidden text")
	wantAbsent(t, a, "c text")
}

func TestSearchHitKeepsFoldsAndOpensAtTop(t *testing.T) {
	a := foldApp(t, "## A\nhidden text\n## B\nb text\n")
	tab(a)
	a.navigate("Q")
	a.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	typeText(a, "P")
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.page.Page() != "P" {
		t.Fatalf("picker did not open P: %q", a.page.Page())
	}
	if a.page.Offset() != 0 || a.page.CursorRow() != a.page.snap(0, +1) {
		t.Errorf("opened at top %d row %d, want top 0 on the first non-blank row", a.page.Offset(), a.page.CursorRow())
	}
	wantAbsent(t, a, "hidden text")
	if markerRow(a) == "" {
		t.Error("the fold was dropped by opening the page")
	}
}

func TestFoldsSurviveNavigationHistoryReindexAndEditor(t *testing.T) {
	quietTerm(t)
	orig := sourceRowsFor
	t.Cleanup(func() { sourceRowsFor = orig })
	calls := 0
	sourceRowsFor = func(body string, w int, e string) ([]int, []bool) {
		calls++
		return orig(body, w, e)
	}
	a := foldApp(t, "## A\nhidden text\n## B\nb text\n")
	folded := func(step string) {
		t.Helper()
		if markerRow(a) == "" {
			t.Errorf("%s: the fold is gone:\n%s", step, appText(a))
		}
		wantAbsent(t, a, "hidden text")
	}
	tab(a)
	folded("fold")
	a.navigate("Q")
	a.navigate("P")
	folded("navigate away and back")
	a.Update(key("["))
	a.Update(key("]"))
	folded("history")
	a.Update(a.buildIndexCmd()())
	folded("reindex")
	a.Update(key("e"))
	a.Update(esc)
	folded("editor round trip")
	if calls != 1 {
		t.Errorf("row map built %d times, want once for the page", calls)
	}
	a.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	folded("width change")
	if calls != 2 {
		t.Errorf("row map built %d times after a width change, want 2", calls)
	}
}

func TestFoldsClearedOnTextChange(t *testing.T) {
	const body = "## A\n- TODO hidden task\n## B\nb text\n"
	shown := func(t *testing.T, a *App) {
		t.Helper()
		wantPresent(t, a, "hidden task")
		if m := markerRow(a); m != "" {
			t.Errorf("fold survived a text change: %q", m)
		}
	}
	t.Run("editor save", func(t *testing.T) {
		a := foldApp(t, body)
		tab(a)
		a.Update(key("e"))
		a.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
		a.Update(key("x"))
		a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		a.Update(esc)
		a.Update(a.buildIndexCmd()())
		shown(t, a)
	})
	t.Run("x from the todos overlay", func(t *testing.T) {
		a := foldApp(t, body)
		tab(a)
		a.Update(key("T"))
		a.Update(key("x"))
		a.Update(esc)
		shown(t, a)
	})
	t.Run("capture into today's journal", func(t *testing.T) {
		quietTerm(t)
		a := bootApp(t, bootConfig{
			now:   capNow,
			files: map[string]string{capJournal: body, "pages/Anchor.md": "anchor\n"},
		})
		if a.page.Page() != "2026-05-25" {
			t.Fatalf("boot page = %q", a.page.Page())
		}
		tab(a)
		wantAbsent(t, a, "hidden task")
		a.Update(key("c"))
		typeText(a, "captured")
		a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		a.Update(esc)
		shown(t, a)
	})
	t.Run("outside write and reindex", func(t *testing.T) {
		a := foldApp(t, body)
		tab(a)
		writeOutside(t, a.graphPath+"/pages/P.md", body+"- more\n")
		a.Update(a.buildIndexCmd()())
		shown(t, a)
	})
}

func TestEFromFoldedPageKeepsLineAndScreenRow(t *testing.T) {
	a := foldApp(t, "## A\n- t1\n- t2\n## B\n- b1\n- b2\n- b3\n")
	tab(a)
	down(a, 3) // b2
	sr := -1
	for i, l := range frameLines(a) {
		if strings.Contains(l, "b2") {
			sr = i
		}
	}
	if sr < 0 {
		t.Fatalf("setup: b2 not on screen:\n%s", appText(a))
	}
	before := frame(a)
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	at := a.editor.ExitAnchor()
	if at.Line != 5 || at.ScreenRow != sr {
		t.Errorf("editor opened on line %d screen row %d, want line 5 row %d", at.Line, at.ScreenRow, sr)
	}
	a.Update(esc)
	if got := frame(a); got != before {
		t.Errorf("Esc did not restore the folded frame:\n%s\nwant\n%s", plain(got), plain(before))
	}
}
