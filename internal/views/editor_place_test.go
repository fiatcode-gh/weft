package views

import (
	"fmt"
	"strings"
	"testing"
)

// A whole-buffer swap (merge-on-save, clash reload) keeps the cursor where it
// was on screen. The v1 test pinned the cursor line to the bottom row, a
// side effect of the old minimal scrolling that the editor no longer has.
func TestEditorReplaceKeepsCursorScreenRow(t *testing.T) {
	quietTerm(t)
	var b strings.Builder
	for i := range 300 {
		fmt.Fprintf(&b, "- line %d\n", i)
	}
	content := b.String()
	e := NewEditorView(nil, "P", "/tmp/p.md", content, false, 40, 10, Anchor{Line: 150, ScreenRow: 4}, nil)
	rows := strings.Split(plain(e.View()), "\n")
	if got := strings.TrimSpace(rows[4]); got != "- line 150" {
		t.Fatalf("setup: row 4 = %q, want %q", got, "- line 150")
	}

	e.replaceBuffer("- extra\n"+content, 151, 3)

	rows = strings.Split(plain(e.View()), "\n")
	if got := strings.TrimSpace(rows[4]); got != "- line 150" {
		t.Errorf("row 4 = %q, want the cursor line %q", got, "- line 150")
	}
	if r, c := cursorRowCol(e); r != 151 || c != 3 {
		t.Errorf("cursor = (%d,%d), want (151,3)", r, c)
	}
}
