package graph

import (
	"path/filepath"
	"regexp"
	"strings"
)

var journalRe = regexp.MustCompile(`^(\d{4})_(\d{2})_(\d{2})\.md$`)

// PageNameFromFilename returns the logical page name for a Logseq filename.
// Journals (YYYY_MM_DD.md) become YYYY-MM-DD; namespace separator "___" becomes "/".
func PageNameFromFilename(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), ".md")
	if m := journalRe.FindStringSubmatch(filepath.Base(name)); m != nil {
		return m[1] + "-" + m[2] + "-" + m[3]
	}
	return strings.ReplaceAll(base, "___", "/")
}

// IsJournalFilename reports whether the filename matches Logseq's journal pattern.
func IsJournalFilename(name string) bool {
	return journalRe.MatchString(filepath.Base(name))
}
