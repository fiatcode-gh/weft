package views

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPageEdgeKeys(t *testing.T) {
	a := bootApp(t)
	// Navigate to Alpha so we have real content that can scroll. Boot
	// page (today's journal) is typically empty in the fixture.
	a.navigate("Alpha")

	// Shrink the viewport so Alpha is scrollable.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5})

	// 'G' jumps to bottom.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	bottomOffset := a.page.Offset()
	if bottomOffset == 0 {
		t.Errorf("after G: expected non-zero offset, got 0")
	}

	// 'g' jumps back to top.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if got := a.page.Offset(); got != 0 {
		t.Errorf("after g: want offset 0, got %d", got)
	}
}

func TestAppStatusBarContainsScrollIndicator(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	// Shrink the viewport so Alpha is scrollable.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5})

	a.page.GotoBottom()

	bar := a.statusBar()
	if !strings.Contains(bar, "100%") {
		t.Errorf("status bar missing 100%% indicator; got:\n%s", bar)
	}
	if !strings.Contains(bar, "? help") {
		t.Errorf("status bar missing close hint; got:\n%s", bar)
	}
}

func TestAppStatusBarHidesIndicatorWhenFits(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	// Force a very tall viewport so Alpha is guaranteed to fit regardless
	// of future content changes.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 200})

	bar := a.statusBar()
	if strings.Contains(bar, "%") {
		t.Errorf("status bar should hide percentage when page fits; got:\n%s", bar)
	}
	if !strings.Contains(bar, "? help") {
		t.Errorf("status bar missing close hint; got:\n%s", bar)
	}
}

func TestAppLoadingSplashBeforePage(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/nonexistent/before/build", "test")
	v := a.View()
	if !strings.Contains(v, "peekseq") {
		t.Errorf("splash should contain title; got:\n%s", v)
	}
	if !strings.Contains(v, "Loading") {
		t.Errorf("splash should announce loading; got:\n%s", v)
	}
}

func TestAppErrorSplashOnIndexFailure(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/some/graph", "test")
	a.Update(indexLoadedMsg{err: errors.New("synthetic build failure")})
	v := a.View()
	if !strings.Contains(v, "failed to index") {
		t.Errorf("error splash should announce failure; got:\n%s", v)
	}
	if !strings.Contains(v, "R to retry") {
		t.Errorf("error splash should show retry hint; got:\n%s", v)
	}
}

func TestAppRetryFromErrorSplash(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/some/graph", "test")
	a.Update(indexLoadedMsg{err: errors.New("synthetic build failure")})
	if a.loadErr == nil {
		t.Fatalf("setup: loadErr should be set")
	}

	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if cmd == nil {
		t.Fatalf("R from error splash should return a retry cmd")
	}
	if a.loadErr != nil {
		t.Errorf("R should clear loadErr before rebuilding, got %v", a.loadErr)
	}
}

func TestAppQuitsBeforePage(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/no/such/path", "test")
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Errorf("q before page should return Quit cmd, got nil")
	}
}

func TestAppPageKeyDispatch(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5}) // scrollable

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if a.page.Cursor() < 0 {
		t.Errorf("n should advance cursor from -1, got %d", a.page.Cursor())
	}

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	afterJ := a.page.Offset()
	if afterJ == 0 {
		t.Fatalf("setup: j should advance offset, got 0")
	}

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if a.page.Offset() >= afterJ {
		t.Errorf("k should retreat from %d, got %d", afterJ, a.page.Offset())
	}

	a.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	afterCtrlD := a.page.Offset()
	if afterCtrlD == 0 {
		t.Fatalf("setup: ctrl+d should advance offset, got 0")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if a.page.Offset() >= afterCtrlD {
		t.Errorf("ctrl+u should retreat from %d, got %d", afterCtrlD, a.page.Offset())
	}
}

func TestAppEnterFollowsLink(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	a.page.CycleLink(+1)
	target := a.page.FollowCursor()
	if target == "" {
		t.Fatalf("setup: no link target available")
	}

	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.page.Page() != target {
		t.Errorf("enter should navigate to %q, got %q", target, a.page.Page())
	}
}

func TestAppReindexFromPage(t *testing.T) {
	a := bootApp(t)
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if cmd == nil {
		t.Errorf("R from page mode should return a reindex cmd")
	}
}

// TestStatusBarTruncatesLongLeft asserts that a wide page name + meta
// don't overflow the terminal width — the left segment is clamped with
// an ellipsis so the bar fits on one line.
func TestStatusBarTruncatesLongLeft(t *testing.T) {
	a := bootApp(t)
	// Tight terminal width — left ("Alpha · 5 links" ish) plus right
	// ("? help") still wants ~25 cells; shrink so the page name itself
	// can dominate the budget. Then navigate so the page is forced; we
	// already use Alpha which is short, so the page name itself fits.
	// To make the bar overflow we force a width narrower than the page
	// name's length-plus-right and verify the bar's total rendered
	// width stays bounded.
	a.navigate("Alpha")
	a.Update(tea.WindowSizeMsg{Width: 20, Height: 24})

	bar := a.statusBar()
	for _, line := range strings.Split(bar, "\n") {
		if w := runewidthLen(line); w > 20 {
			t.Errorf("status bar line exceeds width 20: w=%d, line=%q", w, line)
		}
	}
	// Even with the clamp, the right segment (which carries the help
	// hint) must survive — it's the user's only on-screen reminder of
	// how to open help.
	if !strings.Contains(bar, "? help") {
		t.Errorf("clamped bar dropped the close hint; got:\n%s", bar)
	}
}

// runewidthLen counts visible cells in a single line, ignoring nothing
// (NO_COLOR=1 in test setup keeps ANSI out of the way).
func runewidthLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func TestPeriodJumpsToTodayJournal(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))
	// Boot lands on today's journal (2026-05-23) per tryInitPage seeding.
	// Navigate elsewhere first so we can verify . actually moves us.
	a.navigate("Alpha")
	startHistLen := len(a.hist)

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})

	if got := a.page.Page(); got != "2026-05-23" {
		t.Errorf("after .: want page 2026-05-23, got %q", got)
	}
	if got := len(a.hist); got != startHistLen+1 {
		t.Errorf("history len: want %d, got %d", startHistLen+1, got)
	}
	if a.hint != "" {
		t.Errorf("hint should be empty on successful jump, got %q", a.hint)
	}
}

func TestPeriodOnAbsentTodayShowsHint(t *testing.T) {
	// 2026-06-15 has no journal in the fixture.
	a := bootAppAt(t, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	a.navigate("Alpha")
	startPage := a.page.Page()
	startHistLen := len(a.hist)

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})

	if got := a.page.Page(); got != startPage {
		t.Errorf("after . on absent today: page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("history grew on absent today: want %d, got %d", startHistLen, got)
	}
	if want := "no journal for 2026-06-15"; a.hint != want {
		t.Errorf("hint: want %q, got %q", want, a.hint)
	}

	// Status bar must surface the hint in place of "? help".
	bar := a.statusBar()
	if !strings.Contains(bar, "no journal for 2026-06-15") {
		t.Errorf("status bar missing hint; got:\n%s", bar)
	}
	if strings.Contains(bar, "? help") {
		t.Errorf("status bar should hide ? help while hint is set; got:\n%s", bar)
	}
}

func TestHintClearsOnNextKey(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if a.hint == "" {
		t.Fatal("setup: expected hint to be set by first .")
	}
	// Any subsequent key clears the hint at the top of Update.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if a.hint != "" {
		t.Errorf("hint should clear on next key, got %q", a.hint)
	}
}

func TestHintExpiresOnTick(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	a.navigate("Alpha")
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if a.hint == "" {
		t.Fatal("setup: expected . to set a hint")
	}
	gen := a.hintGen
	a.Update(hintExpireMsg{gen: gen})
	if a.hint != "" {
		t.Errorf("hint should clear after matching tick; got %q", a.hint)
	}
}

func TestStaleHintTickIgnored(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	a.navigate("Alpha")
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	staleGen := a.hintGen
	// Second . clears the hint via the top-of-KeyMsg sweep, then re-sets it
	// with a fresh generation. The tick scheduled by the first . is now stale.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if a.hint == "" {
		t.Fatal("setup: expected second . to set a new hint")
	}
	current := a.hint
	a.Update(hintExpireMsg{gen: staleGen})
	if a.hint != current {
		t.Errorf("stale tick should not clear current hint; want %q, got %q", current, a.hint)
	}
}

func TestPeriodIdempotentOnTodayJournal(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))
	// Boot already lands on today's journal.
	if got := a.page.Page(); got != "2026-05-23" {
		t.Fatalf("setup: want boot page 2026-05-23, got %q", got)
	}
	startHistLen := len(a.hist)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if got := a.page.Page(); got != "2026-05-23" {
		t.Errorf("after . on today: want 2026-05-23, got %q", got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("after . on today: history grew from %d to %d", startHistLen, got)
	}
}

func TestPrevJournalWalksBackwards(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC))
	// Boot lands on 2026-05-24.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")})
	if got := a.page.Page(); got != "2026-05-23" {
		t.Errorf("after <: want 2026-05-23, got %q", got)
	}
	if a.hint != "" {
		t.Errorf("hint should be empty on successful walk, got %q", a.hint)
	}
}

func TestNextJournalWalksForward(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})
	if got := a.page.Page(); got != "2026-05-24" {
		t.Errorf("after >: want 2026-05-24, got %q", got)
	}
}

func TestPrevJournalSkipsGapDays(t *testing.T) {
	// Fixture has 2026-04-20 then 2026-03-15 — large gap. < from 04-20 lands
	// on 03-15, skipping the missing calendar days in between.
	a := bootAppAt(t, time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")})
	if got := a.page.Page(); got != "2026-03-15" {
		t.Errorf("after < across gap: want 2026-03-15, got %q", got)
	}
}

func TestPrevJournalAtOldestShowsHint(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC))
	startPage := a.page.Page()
	startHistLen := len(a.hist)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")})
	if got := a.page.Page(); got != startPage {
		t.Errorf("< at oldest: page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("< at oldest grew history: want %d, got %d", startHistLen, got)
	}
	if a.hint != "no earlier journal" {
		t.Errorf("hint: want %q, got %q", "no earlier journal", a.hint)
	}
}

func TestNextJournalAtNewestShowsHint(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC))
	startPage := a.page.Page()
	startHistLen := len(a.hist)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})
	if got := a.page.Page(); got != startPage {
		t.Errorf("> at newest: page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("> at newest grew history: want %d, got %d", startHistLen, got)
	}
	if a.hint != "no later journal" {
		t.Errorf("hint: want %q, got %q", "no later journal", a.hint)
	}
}

func TestPrevJournalFromPhantomToday(t *testing.T) {
	// 2026-06-15 has no fixture journal; it's phantom-today. < should walk
	// to the newest existing fixture journal (2026-05-25).
	a := bootAppAt(t, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	if got := a.page.Page(); got != "2026-06-15" {
		t.Fatalf("setup: want boot page 2026-06-15, got %q", got)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")})
	if got := a.page.Page(); got != "2026-05-25" {
		t.Errorf("after < from phantom today: want 2026-05-25, got %q", got)
	}
	if a.hint != "" {
		t.Errorf("hint should be empty on successful walk, got %q", a.hint)
	}
}

func TestNextJournalFromPhantomTodayShowsHint(t *testing.T) {
	// 2026-06-15 is phantom-today; no fixture journal is later. > should
	// surface "no later journal" without navigating.
	a := bootAppAt(t, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	startPage := a.page.Page()
	startHistLen := len(a.hist)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})
	if got := a.page.Page(); got != startPage {
		t.Errorf("> from phantom today (no later): page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("> from phantom today: history grew from %d to %d", startHistLen, got)
	}
	if a.hint != "no later journal" {
		t.Errorf("hint: want %q, got %q", "no later journal", a.hint)
	}
}

func TestPrevNextInertOutsideJournalContext(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))
	a.navigate("Alpha")
	startPage := a.page.Page()
	startHistLen := len(a.hist)

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")})
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})

	if got := a.page.Page(); got != startPage {
		t.Errorf("<,> outside journal context: page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("<,> outside journal context grew history: want %d, got %d", startHistLen, got)
	}
	if a.hint != "" {
		t.Errorf("<,> outside journal context: hint should be empty, got %q", a.hint)
	}
}
