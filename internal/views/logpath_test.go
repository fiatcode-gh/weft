package views

import (
	"path/filepath"
	"testing"
)

func TestDebugLogPathIsOutsideCwd(t *testing.T) {
	// arrange
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	// act
	p := DebugLogPath()

	// assert
	if !filepath.IsAbs(p) {
		t.Fatalf("DebugLogPath() = %q, want absolute cache-dir path", p)
	}
	if filepath.Base(filepath.Dir(p)) != "weft" {
		t.Fatalf("DebugLogPath() = %q, want .../weft/weft.log", p)
	}
	if filepath.Base(p) != "weft.log" {
		t.Fatalf("DebugLogPath() = %q, want file named weft.log", p)
	}
}
