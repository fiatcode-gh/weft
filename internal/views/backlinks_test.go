package views

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
	"git.fiatcode.dev/fiatcode/weft/v2/internal/search"
)

func TestNewBacklinksLoadsRefs(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)

	// Hub is referenced from kb/notes and journal 2026-05-25. Hub's own
	// page also contains `[[Hub]]` but NewBacklinks filters self-refs so
	// the view doesn't show them — count must be exactly 2.
	b := NewBacklinks(idx, "Hub", nil, 80, 30)
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
	quietTerm(t)
	idx := loadFixture(t)

	// The underlying index records the self-reference; the view filters it.
	raw := idx.Backlinks[strings.ToLower("Hub")]
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

	b := NewBacklinks(idx, "Hub", nil, 80, 30)
	for _, r := range b.refs {
		if r.FromPage == "Hub" {
			t.Errorf("self-reference leaked into NewBacklinks output: %+v", r)
		}
	}
}

func TestBacklinksSetSize(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", nil, 80, 30)
	b.SetSize(120, 99)
	if b.width != 120 {
		t.Errorf("after SetSize: want width 120, got %d", b.width)
	}
}

func TestBacklinksNavigation(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", nil, 80, 30)
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
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", nil, 80, 30)
	if len(b.refs) == 0 {
		t.Skip("no Hub backlinks in fixture — skipping enter test")
	}
	b.sel = 0
	res := b.Update("enter")
	if !res.Accept || res.Cancel {
		t.Errorf("enter: want accept=true cancel=false, got %v/%v", res.Accept, res.Cancel)
	}
	if res.Selected != b.refs[0].FromPage {
		t.Errorf("returned page: want %q, got %q", b.refs[0].FromPage, res.Selected)
	}
}

func TestBacklinksEscAndBCancel(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", nil, 80, 30)
	for _, k := range []string{"esc", "b"} {
		res := b.Update(k)
		if res.Selected != "" || res.Accept || !res.Cancel {
			t.Errorf("%s: want cancel only, got (%q,%v,%v)", k, res.Selected, res.Accept, res.Cancel)
		}
	}
}

func TestBacklinksNoRefsEnterNoop(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Orphan", nil, 80, 30)
	if len(b.refs) != 0 {
		t.Fatalf("Orphan should have 0 backlinks, got %d", len(b.refs))
	}
	res := b.Update("enter")
	if res.Selected != "" || res.Accept || res.Cancel {
		t.Errorf("enter on empty refs: want zero-valued return, got (%q,%v,%v)",
			res.Selected, res.Accept, res.Cancel)
	}
}

func TestBacklinksInnerWidthClamps(t *testing.T) {
	b := &Backlinks{listBox: listBox{width: 10}}
	if got := b.innerWidth(); got != listInnerWidthMin {
		t.Errorf("narrow term: want %d, got %d", listInnerWidthMin, got)
	}
	b.width = 300
	if got := b.innerWidth(); got != listInnerWidthMax {
		t.Errorf("wide term: want %d, got %d", listInnerWidthMax, got)
	}
}

func TestBacklinksViewWithRefsGolden(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", nil, 100, 30)
	teatest.RequireEqualOutput(t, []byte(b.View()))
}

func TestBacklinksViewNoRefsGolden(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Orphan", nil, 100, 30)
	out := b.View()
	if !strings.Contains(out, "no backlinks") {
		t.Errorf("view should announce empty refs; got:\n%s", out)
	}
	teatest.RequireEqualOutput(t, []byte(out))
}

// TestBacklinksScrollWindowBoundsSelection asserts the scroll window
// always contains b.sel regardless of selection position in a large ref
// list. Without scrollWindow, the overlay rendered every ref and could
// overflow the terminal vertically — e.g. the user's [[OpenClaw]] page
// has 54 inbound references.
func TestBacklinksScrollWindowBoundsSelection(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	for i := 0; i < 50; i++ {
		idx.Backlinks[strings.ToLower("Alpha")] = append(idx.Backlinks[strings.ToLower("Alpha")], graph.Ref{
			FromPage:   fmt.Sprintf("Page-%02d", i),
			LineNumber: i + 1,
			Context:    fmt.Sprintf("- ref %d to [[Alpha]]", i),
		})
	}

	b := NewBacklinks(idx, "Alpha", nil, 80, 24)

	for _, sel := range []int{0, 25, len(b.refs) - 1} {
		b.sel = sel
		start, end := scrollWindow(b.sel, len(b.refs), b.visibleRows())
		if sel < start || sel >= end {
			t.Errorf("sel %d should be in [%d,%d)", sel, start, end)
		}
		if end-start > b.visibleRows() {
			t.Errorf("window size %d exceeds visibleRows %d", end-start, b.visibleRows())
		}
	}
}

// TestBacklinksScrollHintsAppear asserts the "↑ N more above" / "↓ N more
// below" chrome appears in the rendered output when refs fall outside
// the scroll window.
func TestBacklinksScrollHintsAppear(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	for i := 0; i < 30; i++ {
		idx.Backlinks[strings.ToLower("Alpha")] = append(idx.Backlinks[strings.ToLower("Alpha")], graph.Ref{
			FromPage:   fmt.Sprintf("Page-%02d", i),
			LineNumber: i + 1,
			Context:    "- ref",
		})
	}
	b := NewBacklinks(idx, "Alpha", nil, 80, 24)

	b.sel = len(b.refs) / 2
	mid := b.View()
	if !strings.Contains(mid, "more above") {
		t.Errorf("mid selection: want \"more above\" hint; got:\n%s", mid)
	}
	if !strings.Contains(mid, "more below") {
		t.Errorf("mid selection: want \"more below\" hint; got:\n%s", mid)
	}

	b.sel = 0
	top := b.View()
	if strings.Contains(top, "more above") {
		t.Errorf("top selection: should not show \"more above\"; got:\n%s", top)
	}
	if !strings.Contains(top, "more below") {
		t.Errorf("top selection: want \"more below\" hint; got:\n%s", top)
	}
}

func unlinkedFixture() []graph.UnlinkedRef {
	return []graph.UnlinkedRef{
		{FilePath: "/g/pages/Beta.md", PageName: "Beta", Line: 3, Context: "a bare Hub mention", Match: search.Span{Start: 7, End: 10}},
		{FilePath: "/g/journals/2026_05_24.md", PageName: "2026-05-24", Line: 9, Context: "Hub came up today", Match: search.Span{Start: 0, End: 3}},
	}
}

func TestBacklinksUnlinkedNavigation(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	for i := 0; i < 50; i++ {
		b.Update(keyDown)
	}
	res := b.Update(keyEnter)
	if !res.Accept || res.Selected != "2026-05-24" {
		t.Errorf("enter on last unlinked row should open its page; got %+v", res)
	}
}

func TestBacklinksUnlinkedSkipsHeaderOnNav(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	for i := 0; i < 50; i++ {
		b.Update(keyDown)
		if b.sel < 0 || b.sel >= len(b.rows) || b.rows[b.sel].header {
			t.Fatalf("selection landed on a non-selectable row: sel=%d", b.sel)
		}
	}
}

func TestBacklinksViewWithUnlinkedGolden(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 100, 30)
	teatest.RequireEqualOutput(t, []byte(b.View()))
}

func TestBacklinksUnlinkedRefHighlights(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	for i := 0; i < 50; i++ {
		b.Update(keyDown) // reach the last (unlinked) row
	}
	res := b.Update(keyEnter)
	if !res.Accept || res.HighlightText != "Hub" {
		t.Errorf("unlinked-ref enter should set HighlightText=Hub; got %+v", res)
	}
	if res.FocusLinkTo != "" {
		t.Errorf("unlinked-ref enter must not set FocusLinkTo; got %q", res.FocusLinkTo)
	}
}

func TestBacklinksLinkedRefFocusesBacklink(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	res := b.Update(keyEnter) // first selectable row is a linked backlink
	if !res.Accept || res.FocusLinkTo != "Hub" {
		t.Errorf("linked-ref enter should set FocusLinkTo=Hub; got %+v", res)
	}
	for i := 0; i < 50; i++ {
		b.Update(keyDown) // move to an unlinked row
	}
	res = b.Update(keyEnter)
	if !res.Accept || res.FocusLinkTo != "" {
		t.Errorf("unlinked-ref enter must NOT set FocusLinkTo; got %+v", res)
	}
}

// selectFirstUnlinked moves b's selection to its first unlinked-reference row,
// failing the test if there is none — avoids an unbounded moveSel loop (moveSel
// clamps at the ends, so a missing unlinked row would otherwise spin forever).
func selectFirstUnlinked(t *testing.T, b *Backlinks) {
	t.Helper()
	for i := 0; i < len(b.rows); i++ {
		if b.rows[b.sel].unlinkedRow() {
			return
		}
		b.moveSel(+1)
	}
	t.Fatal("fixture has no unlinked row to select")
}

func TestBacklinksLOnUnlinkedEntersConfirm(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	// Move to the first unlinked row (past the linked refs + the section header).
	selectFirstUnlinked(t, b)
	if res := b.Update("l"); res.Linkify != nil {
		t.Fatalf("first l should only open the confirm, not request linkify: %+v", res)
	}
	if !b.confirming {
		t.Fatal("l on an unlinked row should enter the confirm sub-state")
	}
}

func TestBacklinksLOnLinkedRowIsNoop(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	// sel starts on the first selectable row, which is a linked ref.
	if b.rows[b.sel].unlinkedRow() {
		t.Skip("fixture's first selectable row is unexpectedly unlinked")
	}
	b.Update("l")
	if b.confirming {
		t.Error("l on a linked row must not enter the confirm sub-state")
	}
}

func TestBacklinksConfirmYesReturnsLinkify(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	selectFirstUnlinked(t, b)
	b.Update("l")
	res := b.Update("y")
	if res.Linkify == nil {
		t.Fatal("y in confirm should return a Linkify request")
	}
	if res.LinkifyTarget != "Hub" {
		t.Errorf("LinkifyTarget = %q, want \"Hub\"", res.LinkifyTarget)
	}
	if b.confirming {
		t.Error("confirming should be cleared after y")
	}
}

func TestBacklinksConfirmEscCancels(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	selectFirstUnlinked(t, b)
	b.Update("l")
	res := b.Update(keyEsc)
	if res.Cancel {
		t.Error("esc in confirm should cancel the confirm, not the whole panel")
	}
	if res.Linkify != nil {
		t.Error("esc in confirm must not request linkify")
	}
	if b.confirming {
		t.Error("esc in confirm should clear confirming")
	}
}

func TestBacklinksSetLinkifyErrorClearsConfirm(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	selectFirstUnlinked(t, b)
	b.Update("l")
	b.SetLinkifyError("mention no longer found in Beta")
	if b.confirming {
		t.Error("SetLinkifyError should drop the confirm sub-state")
	}
	if b.errMsg == "" {
		t.Error("SetLinkifyError should record the message")
	}
}

func TestBacklinksConfirmViewGolden(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 100, 30)
	selectFirstUnlinked(t, b)
	b.Update("l")
	teatest.RequireEqualOutput(t, []byte(b.View()))
}

func TestBacklinksConfirmViewShowsBeforeAfter(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 100, 30)
	selectFirstUnlinked(t, b)
	b.Update("l")
	out := b.View()
	if !strings.Contains(out, "before:") || !strings.Contains(out, "after:") {
		t.Errorf("confirm view should show before/after lines:\n%s", out)
	}
	if !strings.Contains(out, "[[Hub]]") {
		t.Errorf("confirm 'after' line should show the wrapped mention:\n%s", out)
	}
	if !strings.Contains(out, "y confirm") {
		t.Errorf("confirm view should show the y/n hint:\n%s", out)
	}
}

func TestBacklinksErrorLineRendered(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 100, 30)
	b.SetLinkifyError("mention no longer found in Beta")
	if !strings.Contains(b.View(), "mention no longer found in Beta") {
		t.Errorf("error message should render in the panel:\n%s", b.View())
	}
}

func TestBacklinksHintShowsLinkifyOnUnlinkedRow(t *testing.T) {
	quietTerm(t)
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 100, 30)
	selectFirstUnlinked(t, b)
	if !strings.Contains(b.View(), "l linkify") {
		t.Errorf("hint should advertise linkify when an unlinked row is selected:\n%s", b.View())
	}
}
