package views

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var keyEnterK = tea.KeyPressMsg{Code: tea.KeyEnter}

// targetPage is page X: 60 filler bullets, the heading, 20 more bullets.
func targetPage(heading string) string {
	var b strings.Builder
	for n := range 60 {
		fmt.Fprintf(&b, "- filler %d\n", n)
	}
	b.WriteString(heading + "\n\n")
	for n := range 20 {
		fmt.Fprintf(&b, "- after %d\n", n)
	}
	return b.String()
}

// headingApp boots with page X and a page Note holding noteBody, on Note.
func headingApp(t *testing.T, x, noteBody string) *App {
	t.Helper()
	quietTerm(t)
	a := newApp(t, map[string]string{"pages/X.md": x, "pages/Note.md": noteBody}, 80, 24)
	a.navigate("Note")
	return a
}

func cursorText(a *App) string { return plain(a.page.rowText(a.page.CursorRow())) }

// follow selects the first link and presses Enter.
func followFirstLink(a *App) {
	a.Update(key("n"))
	a.Update(keyEnterK)
}

func wantAtHeading(t *testing.T, a *App, text string) {
	t.Helper()
	if a.page.Page() != "X" {
		t.Fatalf("page = %q, want X", a.page.Page())
	}
	if got := cursorText(a); !strings.Contains(got, text) {
		t.Errorf("cursor row = %q, want it to contain %q", got, text)
	}
	if d := a.page.CursorRow() - a.page.Offset(); d != 1 {
		t.Errorf("heading on screen row %d, want 1", d)
	}
}

func TestEnterOnHeadingLinkOpensAtHeading(t *testing.T) {
	a := headingApp(t, targetPage("## Target Section"), "- [[X#target section]]\n")
	followFirstLink(a)
	wantAtHeading(t, a, "Target Section")
}

func TestHeadingLinkAliasAndBracketTag(t *testing.T) {
	for _, body := range []string{"- [[X#Target Section|go]]\n", "- #[[X#Target Section]]\n"} {
		a := headingApp(t, targetPage("## Target Section"), body)
		if strings.HasPrefix(body, "- [[") {
			wantPresent(t, a, "go")
		} else {
			wantPresent(t, a, "#X#Target Section")
		}
		followFirstLink(a)
		wantAtHeading(t, a, "Target Section")
	}
}

func TestHeadingLinkMatchesDisplayText(t *testing.T) {
	a := headingApp(t, targetPage("## The **Big** [[Idea|idea]]"), "- [[X#the big idea]]\n")
	followFirstLink(a)
	wantAtHeading(t, a, "Big")
}

func TestHeadingLinkSetext(t *testing.T) {
	a := headingApp(t, targetPage("Target\n---"), "- [[X#target]]\n")
	followFirstLink(a)
	wantAtHeading(t, a, "Target")
}

func TestHeadingLinkFirstMatchWins(t *testing.T) {
	x := targetPage("## Dup") + "\n## Dup\n\n- second\n"
	a := headingApp(t, x, "- [[X#dup]]\n")
	followFirstLink(a)
	wantAtHeading(t, a, "Dup")
	if a.page.CursorRow() > 70 {
		t.Errorf("landed on the second heading (row %d)", a.page.CursorRow())
	}
}

func TestHeadingLinkUnfoldsTarget(t *testing.T) {
	x := "# Top\n\n" + targetPage("## Target Section")
	a := headingApp(t, x, "- [[X#target section]]\n")
	a.navigate("X")
	a.Update(key("z"))
	a.Update(key("z"))
	wantAbsent(t, a, "filler 0", "after 0")
	a.navigate("Note")
	followFirstLink(a)
	wantAtHeading(t, a, "Target Section")
	wantPresent(t, a, "after 0")
	if strings.Contains(cursorText(a), "▸") {
		t.Errorf("target section still folded: %q", cursorText(a))
	}
}

func TestHeadingLinkPageWins(t *testing.T) {
	quietTerm(t)
	a := newApp(t, map[string]string{
		"pages/X.md":      targetPage("## Y"),
		"pages/X#Y.md":    "- whole page\n",
		"pages/Source.md": "- [[X#Y]]\n",
	}, 80, 24)
	a.navigate("Source")
	followFirstLink(a)
	if a.page.Page() != "X#Y" || a.page.Offset() != 0 {
		t.Errorf("page = %q offset %d, want X#Y at top", a.page.Page(), a.page.Offset())
	}
}

func TestHeadingLinkMissingStaysAPageName(t *testing.T) {
	a := headingApp(t, targetPage("## Target Section"), "- [[X#Nope]]\n")
	followFirstLink(a)
	if a.page.Page() != "X#Nope" {
		t.Errorf("page = %q, want X#Nope", a.page.Page())
	}
}

func TestHeadingGoneSinceIndexHints(t *testing.T) {
	a := headingApp(t, targetPage("## Target Section"), "- [[X#target section]]\n")
	for _, m := range a.idx.Pages {
		if m.Name == "X" {
			if err := os.WriteFile(filepath.Clean(m.Path), []byte(targetPage("## Other")), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	followFirstLink(a)
	if a.page.Page() != "X" || a.page.Offset() != 0 {
		t.Errorf("page = %q offset %d, want X at top", a.page.Page(), a.page.Offset())
	}
	if !strings.HasPrefix(a.hint, "heading not found: ") {
		t.Errorf("hint = %q", a.hint)
	}
}

func TestHeadingLinkHistory(t *testing.T) {
	a := headingApp(t, targetPage("## Target Section"), "- [[X#target section]]\n")
	a.Update(key("n"))
	noteRow := a.page.CursorRow()
	a.Update(keyEnterK)
	a.Update(key("["))
	if a.page.Page() != "Note" || a.page.CursorRow() != noteRow {
		t.Errorf("back: page %q row %d, want Note row %d", a.page.Page(), a.page.CursorRow(), noteRow)
	}
	a.Update(key("]"))
	wantAtHeading(t, a, "Target Section")
}

func TestBacklinkFromHeadingLinkLandsOnReference(t *testing.T) {
	a := headingApp(t, targetPage("## Target Section"), "- [[X#target section]]\n")
	a.navigate("X")
	a.Update(key("b"))
	a.Update(keyEnterK)
	if a.page.Page() != "Note" {
		t.Fatalf("page = %q, want Note", a.page.Page())
	}
	if got := a.page.FollowCursor(); got != "X#target section" {
		t.Errorf("FollowCursor = %q", got)
	}
}

// Without a row map the heading cannot be placed, but it is not missing: the
// page opens at the top and says nothing false.
func TestHeadingLinkWithoutRowMapOpensAtTopSilently(t *testing.T) {
	a := headingApp(t, targetPage("## Target Section"), "- [[X#target section]]\n")
	orig := sourceRowsFor
	t.Cleanup(func() { sourceRowsFor = orig })
	sourceRowsFor = func(string, int, string) ([]int, []bool) { return nil, nil }
	followFirstLink(a)
	if a.page.Page() != "X" || a.page.Offset() != 0 {
		t.Errorf("page = %q offset %d, want X at top", a.page.Page(), a.page.Offset())
	}
	if a.hint != "" {
		t.Errorf("hint = %q, want none", a.hint)
	}
}
