package graph

import "time"

// PageMeta is the indexed metadata for one .md file in the graph.
type PageMeta struct {
	Name      string    // logical page name (e.g., "proj/nested" or "2026-05-24")
	Path      string    // absolute filesystem path
	IsJournal bool      //
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
}
