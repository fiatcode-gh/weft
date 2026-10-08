package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestWriteSideBySide writes the colour-parity page as the read view and the
// editor draw it, and the two next to each other, for the smoke run to look at.
// It does nothing unless WEFT_SIDE_BY_SIDE names an output directory.
func TestWriteSideBySide(t *testing.T) {
	dir := os.Getenv("WEFT_SIDE_BY_SIDE")
	if dir == "" {
		t.Skip("set WEFT_SIDE_BY_SIDE=<dir> to write read.ans, edit.ans (source look), live.ans, side.ans and side-live.ans")
	}
	const width = 100
	a := newApp(t, map[string]string{"pages/Doc.md": parityDoc}, width, 60)
	a.navigate("Doc")
	read := strings.Split(a.View().Content, "\n")
	a.editorSource = true // source look
	a.Update(key("e"))
	edit := strings.Split(a.View().Content, "\n")
	a.Update(esc)
	a.editorSource = false
	a.Update(key("e"))
	live := strings.Split(a.View().Content, "\n")

	sideBySide := func(right []string) string {
		var side strings.Builder
		for i := range max(len(read), len(right)) {
			var r, e string
			if i < len(read) {
				r = read[i]
			}
			if i < len(right) {
				e = right[i]
			}
			side.WriteString(r + strings.Repeat(" ", max(0, width-ansi.StringWidth(r))) + " │ " + e + "\n")
		}
		return side.String()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"read.ans":      strings.Join(read, "\n") + "\n",
		"edit.ans":      strings.Join(edit, "\n") + "\n",
		"live.ans":      strings.Join(live, "\n") + "\n",
		"side.ans":      sideBySide(edit),
		"side-live.ans": sideBySide(live),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
