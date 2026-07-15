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

// FilenameFromPageName is the inverse of PageNameFromFilename for the
// cases the App needs to construct a path for: journal page names
// (YYYY-MM-DD → YYYY_MM_DD.md) and namespace pages (proj/nested →
// proj___nested.md). Returns "" for any page name whose filename
// shape isn't covered here — callers should fall back to a more
// permissive resolution (or treat it as a programmer error) in that
// case. Used by App.Update's "." handler to derive the journal-file
// path when creating today's journal on demand.
func FilenameFromPageName(name string) string {
	if m := journalPageNameRe.FindStringSubmatch(name); m != nil {
		// Reuse journalRe's capture groups by hand: the page name is
		// YYYY-MM-DD; emit YYYY_MM_DD.md.
		return strings.ReplaceAll(name, "-", "_") + ".md"
	}
	if strings.Contains(name, "/") {
		return strings.ReplaceAll(name, "/", "___") + ".md"
	}
	if name == "" {
		return ""
	}
	return name + ".md"
}

// IsJournalPageName reports whether name is a journal page name (YYYY-MM-DD).
// This is the form PageNameFromFilename produces; it answers "is this page
// a journal?" without requiring the file to exist on disk.
func IsJournalPageName(name string) bool {
	return journalPageNameRe.MatchString(name)
}

// Resolve returns the page whose name matches name exactly, or whose
// case-folded form matches name's lower-case. The case-preserving
// PageMeta is always returned, so callers see the on-disk name.
func (idx *Index) Resolve(name string) (*PageMeta, bool) {
	if p, ok := idx.ByName[name]; ok {
		return p, true
	}
	if p, ok := idx.ByNameFold[strings.ToLower(name)]; ok {
		return p, true
	}
	return nil, false
}
