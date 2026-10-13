package graph

import "time"

// PageMeta is the indexed metadata for one .md file in the graph.
type PageMeta struct {
	Name      string    // filename stem; for pages "___" reads as "/" (e.g. "proj/nested"); for journals the stem as is ("2026_05_24")
	Key       string    // link target that opens exactly this page: Name, or a clash loser's shortest resolving path suffix
	Path      string    // filesystem path; absolute only when the supplied graph path is absolute
	Rel       string    // graph-relative path, slash-separated, with ".md"
	IsJournal bool      // matches journal_file_pattern under journals_dir
	Date      time.Time // journal date (civil, UTC); zero for pages
	ModTime   time.Time // file modification time; zero if stat failed
}

// Ref points from one page's body to another page name.
type Ref struct {
	FromPage   string
	LineNumber int    // 1-based
	Context    string // the source line, trimmed
}

// TodoBullet is one open task bullet (TODO/LATER/DOING/WAITING).
type TodoBullet struct {
	Page       string
	LineNumber int    // 1-based
	Marker     string // TODO | LATER | DOING | WAITING
	Priority   string // "" | "A" | "B" | "C"
	Text       string // remainder after marker (and priority), trimmed
	Ordinal    int    // 0-based index among open todos on the same page (document order)

	Scheduled, Deadline Stamp // zero when the task has none
}
