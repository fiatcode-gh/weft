package graph

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	journalRe         = regexp.MustCompile(`^(\d{4})_(\d{2})_(\d{2})\.md$`)
	journalPageNameRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// PageNameFromFilename returns the logical page name for a Logseq filename.
// Journals (YYYY_MM_DD.md) become YYYY-MM-DD; namespace separator "___" becomes "/".
func PageNameFromFilename(name string) string {
	base := filepath.Base(name)
	if m := journalRe.FindStringSubmatch(base); m != nil {
		return m[1] + "-" + m[2] + "-" + m[3]
	}
	return strings.ReplaceAll(strings.TrimSuffix(base, ".md"), "___", "/")
}

// IsJournalFilename reports whether the filename matches Logseq's journal pattern.
func IsJournalFilename(name string) bool {
	return journalRe.MatchString(filepath.Base(name))
}

// IsJournalPageName reports whether name is a journal page name (YYYY-MM-DD).
// This is the form PageNameFromFilename produces; it answers "is this page
// a journal?" without requiring the file to exist on disk.
func IsJournalPageName(name string) bool {
	return journalPageNameRe.MatchString(name)
}
