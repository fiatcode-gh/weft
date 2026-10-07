package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Whatever is on disk opens unchanged: the buffer holds the file byte for
// byte, with the cursor on the anchor line.
func TestEditorOpenIsFaithful(t *testing.T) {
	quietTerm(t)
	tests := []struct {
		name   string
		input  string
		anchor int
		lines  int // 0 = don't check
	}{
		{"10000 lines", strings.Repeat("- l\n", 9999) + "- last", 5000, 10000},
		{"10001 lines", strings.Repeat("- l\n", 10000) + "- last", 9999, 10001},
		{"20000 lines", strings.Repeat("- l\n", 20000), 15000, 20001},
		{"CRLF", strings.Repeat("- l\r\n", 50), 20, 51},
		{"mixed endings", "a\r\nb\nc\r\nd", 2, 4},
		{"tabs", strings.Repeat("\t- l\n", 50), 20, 51},
		{"no final newline", "- a\n- b", 1, 2},
		{"invalid UTF-8 and NUL", "a\xff\x00b\n- c\n", 1, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := editorAt(nil, "P", "/tmp/p.md", tc.input, false, 80, 24, tc.anchor)
			if r, c := cursorRowCol(e); r != tc.anchor || c != 0 {
				t.Errorf("cursor = (%d,%d), want (%d,0)", r, c, tc.anchor)
			}
			if tc.lines != 0 && e.buf.Len() != tc.lines {
				t.Errorf("lines = %d, want %d", e.buf.Len(), tc.lines)
			}
			if e.Content() != tc.input {
				t.Errorf("Content differs from the input (len %d vs %d)", len(e.Content()), len(tc.input))
			}
			if e.dirty() {
				t.Error("a freshly opened buffer must be clean")
			}
			_ = e.View() // drawing never touches the text
			if e.Content() != tc.input {
				t.Error("View changed the content")
			}
		})
	}
}

const crlfTabsNoFinalNewline = "- one\r\n\t- two\r\n- three"

func TestEditorSaveRoundTripsUnedited(t *testing.T) {
	a, path := openEditor(t, map[string]string{"pages/Odd.md": crlfTabsNoFinalNewline}, "Odd")

	a.Update(ctrlS)

	if got := readFile(t, path); got != crlfTabsNoFinalNewline {
		t.Errorf("file = %q, want the original bytes %q", got, crlfTabsNoFinalNewline)
	}
}

func TestEditorEnterInCRLFFile(t *testing.T) {
	const original = "one\r\ntwo\r\nthree\r\n"
	a, path := openEditor(t, map[string]string{"pages/Crlf.md": original}, "Crlf")
	a.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	a.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	typeApp(a, "X")
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	a.Update(ctrlS)

	want := "one\r\ntwo\r\nX\r\nthree\r\n"
	if got := readFile(t, path); got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

// The in-app editor opens every file; v1 refused CRLF, tabs and long files.
func TestEnterEditorOpensAnyFile(t *testing.T) {
	files := map[string]string{
		"pages/Crlf.md": "a\r\nb\r\n",
		"pages/Tabs.md": "code:\n\tindented\n",
		"pages/Long.md": strings.Repeat("- x\n", 10001),
	}
	for page, name := range map[string]string{"Crlf": "pages/Crlf.md", "Tabs": "pages/Tabs.md", "Long": "pages/Long.md"} {
		t.Run(page, func(t *testing.T) {
			a := bootApp(t, bootConfig{files: files})
			a.navigate(page)

			cmd := a.enterEditor()

			if a.editor == nil {
				t.Fatalf("editor refused the file; hint %q", a.hint)
			}
			if cmd != nil {
				t.Errorf("enterEditor returned a command; the terminal cursor needs none")
			}
			if a.editor.Content() != files[name] {
				t.Error("Content differs from the file's bytes")
			}
		})
	}
}
