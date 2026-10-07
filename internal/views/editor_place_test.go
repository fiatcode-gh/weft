package views

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestEditorCursorPlacementIsLinear(t *testing.T) {
	quietTerm(t)
	content := strings.Repeat("- some line of text\n", 6000)

	t0 := time.Now()
	e := NewEditorView(nil, "P", "/tmp/p.md", content, false, 80, 24, 0)
	open := time.Since(t0)

	t1 := time.Now()
	e.replaceBuffer(content, 0, 0)
	replace := time.Since(t1)

	const bound = 5 * time.Second
	if open > bound || replace > bound {
		t.Fatalf("cursor placement is not linear: open %v, replaceBuffer %v (bound %v each)", open, replace, bound)
	}
}

func TestEditorReplaceBufferPutsCursorLineOnBottomRow(t *testing.T) {
	quietTerm(t)
	var b strings.Builder
	for i := range 300 {
		fmt.Fprintf(&b, "- line %d\n", i)
	}
	content := b.String()
	e := NewEditorView(nil, "P", "/tmp/p.md", content, false, 40, 10, 10)
	_ = e.View()
	e.replaceBuffer(content+"- extra\n", 150, 3)

	rows := strings.Split(plain(e.View()), "\n")
	if got := strings.TrimSpace(rows[1]); got != "- line 143" {
		t.Errorf("top row = %q, want %q", got, "- line 143")
	}
	if got := strings.TrimSpace(rows[8]); got != "- line 150" {
		t.Errorf("bottom row = %q, want %q", got, "- line 150")
	}
	if r, c := e.cursorRowCol(); r != 150 || c != 3 {
		t.Errorf("cursor = (%d,%d), want (150,3)", r, c)
	}
}

func TestEditorOpenKeepsContentAndCap(t *testing.T) {
	quietTerm(t)
	tests := []struct {
		name     string
		input    string
		anchor   int
		lines    int // 0 = don't check
		diverged bool
		content  bool // Content() must equal normalizeContent(input)
	}{
		{"10000 lines", strings.Repeat("- l\n", 9999) + "- last", 5000, 10000, false, true},
		{"10001 lines", strings.Repeat("- l\n", 10000) + "- last", 9999, 10000, true, false},
		{"CRLF", strings.Repeat("- l\r\n", 50), 20, 0, true, false},
		{"tabs", strings.Repeat("\t- l\n", 50), 20, 0, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEditorView(nil, "P", "/tmp/p.md", tc.input, false, 80, 24, tc.anchor)
			if r, c := e.cursorRowCol(); r != tc.anchor || c != 0 {
				t.Errorf("cursor = (%d,%d), want (%d,0)", r, c, tc.anchor)
			}
			if tc.lines != 0 && e.ta.LineCount() != tc.lines {
				t.Errorf("LineCount = %d, want %d", e.ta.LineCount(), tc.lines)
			}
			if e.LoadDiverged() != tc.diverged {
				t.Errorf("LoadDiverged = %v, want %v", e.LoadDiverged(), tc.diverged)
			}
			if tc.content && e.Content() != normalizeContent(tc.input) {
				t.Errorf("Content differs from normalizeContent(input)")
			}
		})
	}
}
