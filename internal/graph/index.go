package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Index is the read-only in-memory view of a Logseq graph.
type Index struct {
	GraphPath  string
	Pages      []PageMeta
	ByName     map[string]*PageMeta // case-preserving (filename-derived)
	ByNameFold map[string]*PageMeta // case-folded (lowercase key) — for [[ALPHA]] → Alpha
	Backlinks  map[string][]Ref
	Todos      []TodoBullet
	Journals   []string // journal page names, sorted ascending
}

// BuildIndex walks <graphPath>/pages and <graphPath>/journals once and returns
// the populated Index. Unreadable files are logged to stderr and skipped.
func BuildIndex(graphPath string) (*Index, error) {
	idx := &Index{
		GraphPath:  graphPath,
		ByName:     make(map[string]*PageMeta),
		ByNameFold: make(map[string]*PageMeta),
		Backlinks:  make(map[string][]Ref),
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
				fmt.Fprintf(os.Stderr, "weft: skipping subdirectory %s (weft does not recurse — use the ___ namespace convention instead)\n", filepath.Join(dir, e.Name()))
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
				fmt.Fprintf(os.Stderr, "weft: ambiguous page name %q vs %q (case-insensitive); [[%s]] resolves to %q\n",
					name, existing.Name, fold, existing.Name)
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
			idx.Journals = append(idx.Journals, p.Name)
		}
	}
	sort.Strings(idx.Journals)

	// Second pass: parse bodies for links + todos.
	for _, p := range idx.Pages {
		body, err := os.ReadFile(p.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "weft: skipping %s: %v\n", p.Path, err)
			continue
		}
		lines, links, todos := parseBody(string(body))
		for _, lh := range links {
			idx.Backlinks[lh.Target] = append(idx.Backlinks[lh.Target], Ref{
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
