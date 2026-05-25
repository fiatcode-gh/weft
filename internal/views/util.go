package views

import "github.com/charmbracelet/x/ansi"

// clamp truncates s to fit within max display columns, appending "…" when it
// has to chop. ANSI escape codes inside s are preserved; only visible cells
// count toward max. Returns "" for non-positive max.
func clamp(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= max {
		return s
	}
	return ansi.Truncate(s, max, "…")
}
