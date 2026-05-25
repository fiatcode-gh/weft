package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Index is the read-only in-memory view of a Logseq graph.
type Index struct {
	GraphPath string
	Pages     []PageMeta
	ByName    map[string]*PageMeta
	Backlinks map[string][]Ref
	Todos     []TodoBullet
}

// BuildIndex walks <graphPath>/pages and <graphPath>/journals once and returns
// the populated Index. Unreadable files are logged to stderr and skipped.
func BuildIndex(graphPath string) (*Index, error) {
	idx := &Index{
		GraphPath: graphPath,
		ByName:    make(map[string]*PageMeta),
		Backlinks: make(map[string][]Ref),
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
			if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
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

	// ByName must be built after Pages is final (so pointers are stable).
	for i := range idx.Pages {
		idx.ByName[idx.Pages[i].Name] = &idx.Pages[i]
	}

	// Second pass: parse bodies for links + todos.
	for _, p := range idx.Pages {
		body, err := os.ReadFile(p.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lstui: skipping %s: %v\n", p.Path, err)
			continue
		}
		s := string(body)
		for _, lh := range ExtractWikiLinks(s) {
			idx.Backlinks[lh.Target] = append(idx.Backlinks[lh.Target], Ref{
				FromPage:   p.Name,
				LineNumber: lh.Line,
				Context:    lineAt(s, lh.Line),
			})
		}
		for _, th := range ExtractTodos(s) {
			idx.Todos = append(idx.Todos, TodoBullet{
				Page:       p.Name,
				LineNumber: th.Line,
				Marker:     th.Marker,
				Priority:   th.Priority,
				Text:       th.Text,
			})
		}
	}
	return idx, nil
}

func lineAt(body string, n int) string {
	i := 1
	start := 0
	for j := 0; j < len(body); j++ {
		if i == n {
			end := j
			for end < len(body) && body[end] != '\n' {
				end++
			}
			return strings.TrimRight(body[start:end], "\r")
		}
		if body[j] == '\n' {
			i++
			start = j + 1
		}
	}
	return ""
}
