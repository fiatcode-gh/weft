package views

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNavigateCanonicalizesCaseMismatchedTarget(t *testing.T) {
	// arrange: the file is Weft.md; a link elsewhere says [[weft]].
	a := bootApp(t, bootConfig{files: map[string]string{
		"pages/Weft.md": "- the real page\n",
	}})

	// act
	a.navigate("weft")

	// assert: the view adopts the indexed page's exact name; an
	// unindexed name still passes through untouched (lazy creation).
	if got := a.page.Page(); got != "Weft" {
		t.Errorf("page name = %q, want canonical %q", got, "Weft")
	}
	a.navigate("Brand New")
	if got := a.page.Page(); got != "Brand New" {
		t.Errorf("unindexed name = %q, want pass-through %q", got, "Brand New")
	}
}

func TestEnterEditorOnCaseMismatchedTargetOpensExistingFile(t *testing.T) {
	// arrange
	a := bootApp(t, bootConfig{files: map[string]string{
		"pages/Weft.md": "- existing content\n",
	}})
	a.navigate("weft")

	// act
	_ = a.enterEditor()

	// assert: the editor must target the existing file with its content,
	// not a fresh case-colliding pages/weft.md.
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	if got := filepath.Base(a.editor.path); got != "Weft.md" {
		t.Errorf("editor path = %q, want Weft.md", got)
	}
	if a.editor.isNew {
		t.Error("existing page treated as new")
	}
	if !strings.Contains(a.editor.Content(), "existing content") {
		t.Error("existing content not loaded into the editor")
	}
}

func TestEnterEditorRefusesFileTheTextareaWouldAlter(t *testing.T) {
	// arrange: a CRLF file — the sanitizer would double every line break.
	a := bootApp(t, bootConfig{files: map[string]string{
		"pages/Crlf.md": "a\r\nb\r\n",
	}})
	a.navigate("Crlf")

	// act
	_ = a.enterEditor()

	// assert: refused, with a hint pointing at the $EDITOR handoff.
	if a.editor != nil {
		t.Fatal("editor opened on a file it would corrupt")
	}
	if !strings.Contains(a.hint, "would alter") || !strings.Contains(a.hint, "edit externally") {
		t.Errorf("hint = %q, want a pointer to E / $EDITOR", a.hint)
	}
}
