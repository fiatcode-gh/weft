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
		t.Skip("set WEFT_SIDE_BY_SIDE=<dir> to write read.ans, edit.ans and side.ans")
	}
	const width = 100
	a := newApp(t, map[string]string{"pages/Doc.md": parityDoc}, width, 60)
	a.navigate("Doc")
	read := strings.Split(a.View().Content, "\n")
	a.Update(key("e"))
	edit := strings.Split(a.View().Content, "\n")

	var side strings.Builder
	for i := range max(len(read), len(edit)) {
		var r, e string
		if i < len(read) {
			r = read[i]
		}
		if i < len(edit) {
			e = edit[i]
		}
		side.WriteString(r + strings.Repeat(" ", max(0, width-ansi.StringWidth(r))) + " │ " + e + "\n")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"read.ans": strings.Join(read, "\n") + "\n",
		"edit.ans": strings.Join(edit, "\n") + "\n",
		"side.ans": side.String(),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
