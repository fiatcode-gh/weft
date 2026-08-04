package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Index is the read-only in-memory view of a Logseq graph.
type Index struct {
	GraphPath  string
	Pages      []PageMeta
	ByName     map[string]*PageMeta // case-preserving (filename-derived)
	ByNameFold map[string]*PageMeta // case-folded (lowercase key) — for [[ALPHA]] → Alpha
	Todos      []TodoBullet

	backlinks   map[string][]Ref  // keyed by strings.ToLower(target) — resolution is case-insensitive
	targetSpell map[string]string // folded target → first-seen original spelling (walk order)
	journals    []string          // journal page names, sorted ascending

	// Warnings collects non-fatal problems found during the walk (skipped
	// subdirectories, case-fold collisions, unreadable pages, stat
	// failures). The TUI logs and hints them; BuildIndex never writes to
	// stderr — under the alt-screen nobody would see it.
	Warnings []string
}

// warnf records a formatted, non-fatal problem found during the walk onto
// Warnings. See the Warnings field doc for what the TUI does with these.
func (idx *Index) warnf(format string, args ...any) {
	idx.Warnings = append(idx.Warnings, fmt.Sprintf(format, args...))
}

// BuildIndex walks <graphPath>/pages and <graphPath>/journals once and returns
// the populated Index. Unreadable files are skipped and recorded in
// Index.Warnings.
func BuildIndex(graphPath string) (*Index, error) {
	idx := &Index{
		GraphPath:   graphPath,
		ByName:      make(map[string]*PageMeta),
		ByNameFold:  make(map[string]*PageMeta),
		backlinks:   make(map[string][]Ref),
		targetSpell: make(map[string]string),
	}

	for _, sub := range []string{"pages", "journals"} {
		dir := filepath.Join(graphPath, sub)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				idx.warnf("skipping subdirectory %s (weft does not recurse — use the ___ namespace convention instead)", filepath.Join(dir, e.Name()))
				continue
			}
			if filepath.Ext(e.Name()) != ".md" {
				continue
			}
			path := filepath.Join(dir, e.Name())
			meta := PageMeta{
				Name:      PageNameFromFilename(e.Name()),
				Path:      path,
				IsJournal: sub == "journals",
			}
			if info, err := e.Info(); err == nil {
				meta.ModTime = info.ModTime()
			} else {
				idx.warnf("cannot stat %s: %v — page will sort last in the picker", path, err)
			}
			idx.Pages = append(idx.Pages, meta)
		}
	}

	// ByName / ByNameFold must be built after Pages is final (so pointers are
	// stable). Keep the first page walked for each (folded) key so resolution is
	// deterministic — os.ReadDir returns entries sorted by filename — and warn on
	// a genuine case-fold collision rather than silently letting the last win.
	for i := range idx.Pages {
		name := idx.Pages[i].Name
		fold := strings.ToLower(name)
		if existing, ok := idx.ByNameFold[fold]; ok {
			if existing.Name != name {
				idx.warnf("ambiguous page name %q vs %q (case-insensitive); [[%s]] resolves to %q",
					name, existing.Name, fold, existing.Name)
				// The folded key stays first-wins, but an exact-name lookup
				// has no ambiguity — it must still find this page.
				if _, dup := idx.ByName[name]; !dup {
					idx.ByName[name] = &idx.Pages[i]
				}
			}
			continue
		}
		idx.ByNameFold[fold] = &idx.Pages[i]
		idx.ByName[name] = &idx.Pages[i]
	}

	// Collect journal page names sorted ascending. Names are YYYY-MM-DD so
	// lexical order matches chronological order.
	for _, p := range idx.Pages {
		if p.IsJournal {
			idx.journals = append(idx.journals, p.Name)
		}
	}
	sort.Strings(idx.journals)

	// Second pass: parse bodies for links + todos.
	for _, p := range idx.Pages {
		body, err := os.ReadFile(p.Path)
		if err != nil {
			idx.warnf("skipping %s: %v", p.Path, err)
			continue
		}
		lines, links, todos := parseBody(string(body))
		for _, lh := range links {
			key := strings.ToLower(lh.Target)
			if _, seen := idx.targetSpell[key]; !seen {
				idx.targetSpell[key] = lh.Target
			}
			idx.backlinks[key] = append(idx.backlinks[key], Ref{
				FromPage:   p.Name,
				LineNumber: lh.Line,
				Context:    lineContext(lines, lh.Line),
			})
		}
		for k, th := range todos {
			idx.Todos = append(idx.Todos, TodoBullet{
				Page:       p.Name,
				LineNumber: th.Line,
				Marker:     th.Marker,
				Priority:   th.Priority,
				Text:       th.Text,
				Ordinal:    k,
			})
		}
	}
	return idx, nil
}

// BacklinksTo returns a snapshot of references to name using the index's case-insensitive key.
func (idx *Index) BacklinksTo(name string) []Ref {
	return slices.Clone(idx.backlinks[strings.ToLower(name)])
}

// JournalNeighbor returns the nearest indexed journal in dir (-1 or +1).
// Phantom journal-shaped dates use their insertion point in the sorted index.
func (idx *Index) JournalNeighbor(current string, dir int) (string, bool) {
	if !IsJournalPageName(current) {
		return "", false
	}
	i := sort.SearchStrings(idx.journals, current)
	j := i
	if i < len(idx.journals) && idx.journals[i] == current {
		j = i + dir
	} else if dir < 0 {
		j = i - 1
	}
	if j < 0 || j >= len(idx.journals) {
		return "", false
	}
	return idx.journals[j], true
}

// lineContext returns the n-th 1-based line from a pre-split body with any
// trailing carriage return removed, or "" when n is out of range.
//
// The result is cloned so it owns its bytes: strings.Split lines are substrings
// sharing the whole body's backing array, and a retained Ref.Context would
// otherwise pin the entire file body in memory for the life of the index.
func lineContext(lines []string, n int) string {
	if n < 1 || n > len(lines) {
		return ""
	}
	return strings.Clone(strings.TrimRight(lines[n-1], "\r"))
}
