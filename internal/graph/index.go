package graph

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fiatcode-gh/weft/v2/internal/layout"
)

// Index is the read-only in-memory view of a Logseq graph.
type Index struct {
	Layout     layout.Layout
	GraphPath  string
	Pages      []PageMeta
	ByName     map[string]*PageMeta // exact name → clash winner
	ByNameFold map[string]*PageMeta // lower-cased name → clash winner — for [[ALPHA]] → Alpha
	Todos      []TodoBullet

	byPath map[string]*PageMeta // exact Path → page, pointers into Pages
	files  []string             // every PageMeta.Path in walk order

	linksTo     map[string][]Ref       // resolved links, keyed by the target page's Path
	unresolved  map[string][]Ref       // links no page opens, keyed by strings.ToLower(target)
	targetSpell map[string]string      // folded unresolved target → first-seen spelling (walk order)
	stems       map[string][]*PageMeta // lower-cased filename stem → pages, for path resolution
	journals    []PageMeta             // journal pages (copies), sorted ascending by Date

	// headingKeys holds each page's HeadingKey set, keyed by PageMeta.Path.
	headingKeys map[string]map[string]struct{}

	// Warnings collects non-fatal problems found during the walk (unreadable
	// subdirectories, case-fold collisions, unreadable pages, stat
	// failures). The TUI logs and hints them; BuildIndex never writes to
	// stderr — under the alt-screen nobody would see it.
	Warnings []string

	signature Signature // of the walk that built this index
}

// warnf records a formatted, non-fatal problem found during the walk onto
// Warnings. See the Warnings field doc for what the TUI does with these.
func (idx *Index) warnf(format string, args ...any) {
	idx.Warnings = append(idx.Warnings, fmt.Sprintf(format, args...))
}

// BuildIndex walks the layout's page and journal folders once, with the
// walkGraph rule set (recursively, in lay.Roots() order), and returns the
// populated Index. Only .md files are indexed; folders lay.SkipDir names are
// not entered. A missing root is skipped; unreadable files and
// subdirectories are skipped and recorded in Index.Warnings.
func BuildIndex(graphPath string, lay layout.Layout) (*Index, error) {
	idx := &Index{
		Layout:      lay,
		GraphPath:   graphPath,
		ByName:      make(map[string]*PageMeta),
		ByNameFold:  make(map[string]*PageMeta),
		linksTo:     make(map[string][]Ref),
		unresolved:  make(map[string][]Ref),
		targetSpell: make(map[string]string),
		headingKeys: make(map[string]map[string]struct{}),
		byPath:      make(map[string]*PageMeta),
	}

	var sb sigBuilder
	err := walkGraph(graphPath, lay, idx.warnf, func(rel string, d fs.DirEntry) {
		filePath := filepath.Join(graphPath, filepath.FromSlash(rel))
		stem := strings.TrimSuffix(d.Name(), ".md")
		meta := PageMeta{Name: pageName(stem), Path: filePath, Rel: rel}
		if day, ok := lay.Journal(rel); ok {
			meta.Name, meta.IsJournal, meta.Date = stem, true, day
		}
		info, err := d.Info()
		sb.add(rel, info, err)
		if err == nil {
			meta.ModTime = info.ModTime()
		} else {
			idx.warnf("cannot stat %s: %v — page will sort last in the picker", filePath, err)
		}
		idx.Pages = append(idx.Pages, meta)
	})
	if err != nil {
		return nil, err
	}
	idx.signature = sb.sum()

	// Names, clashes and keys need Pages final (so pointers are stable).
	for i := range idx.Pages {
		idx.byPath[idx.Pages[i].Path] = &idx.Pages[i]
		idx.files = append(idx.files, idx.Pages[i].Path)
	}
	idx.buildNames()

	// Journals are copied after the keys are set.
	for _, p := range idx.Pages {
		if p.IsJournal {
			idx.journals = append(idx.journals, p)
		}
	}
	slices.SortStableFunc(idx.journals, func(a, b PageMeta) int { return a.Date.Compare(b.Date) })

	// Second pass, phase A: parse every body for headings, links and todos.
	// Links stay pending until every page's headings are known.
	type pendingLink struct {
		page    string
		target  string
		line    int
		context string
	}
	var pending []pendingLink
	for _, p := range idx.Pages {
		body, err := os.ReadFile(p.Path)
		if err != nil {
			idx.warnf("skipping %s: %v", p.Path, err)
			continue
		}
		lines, links, todos := parseBody(string(body))
		if hs := Headings(string(body)); len(hs) > 0 {
			keys := make(map[string]struct{}, len(hs))
			for _, h := range hs {
				keys[HeadingKey(h.Text)] = struct{}{}
			}
			idx.headingKeys[p.Path] = keys
		}
		for _, lh := range links {
			pending = append(pending, pendingLink{p.Key, lh.target, lh.line, lineContext(lines, lh.line)})
		}
		for k, th := range todos {
			idx.Todos = append(idx.Todos, TodoBullet{
				Page:       p.Key,
				LineNumber: th.Line,
				Marker:     th.Marker,
				Priority:   th.Priority,
				Text:       th.Text,
				Ordinal:    k,
				Scheduled:  th.Scheduled,
				Deadline:   th.Deadline,
			})
		}
	}

	// Phase B, in walk order: a link that resolves is filed under its page's
	// Path; one that does not under its lower-cased target.
	for _, pl := range pending {
		ref := Ref{FromPage: pl.page, LineNumber: pl.line, Context: pl.context}
		if dest, ok := idx.ResolveLink(pl.target); ok {
			idx.linksTo[dest.Page.Path] = append(idx.linksTo[dest.Page.Path], ref)
			continue
		}
		key := strings.ToLower(pl.target)
		if _, seen := idx.targetSpell[key]; !seen {
			idx.targetSpell[key] = pl.target
		}
		idx.unresolved[key] = append(idx.unresolved[key], ref)
	}
	return idx, nil
}

// pageName is the page name a file stem stands for: "___" is a namespace
// separator.
func pageName(stem string) string { return strings.ReplaceAll(stem, "___", "/") }

// walkGraph is the one walk rule set shared by BuildIndex and ScanSignature.
// For every lay.Roots() folder, in order, it calls visit for each .md file
// under it, depth-first in lexical order, with the file's graph-relative
// slash path. A root that is missing is skipped; one that cannot be read is
// an error; a subdirectory that cannot be read is skipped with a warnf.
func walkGraph(graphPath string, lay layout.Layout, warnf func(string, ...any), visit func(rel string, d fs.DirEntry)) error {
	for _, root := range lay.Roots() {
		if err := walkRoot(graphPath, root, lay, warnf, visit); err != nil {
			return err
		}
	}
	return nil
}

func walkRoot(graphPath, root string, lay layout.Layout, warnf func(string, ...any), visit func(rel string, d fs.DirEntry)) error {
	dir := filepath.Join(graphPath, filepath.FromSlash(root))
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err == nil && !info.IsDir() {
		err = errors.New("not a directory")
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	// WalkDir does not follow a symlinked start directory.
	start, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}

	return filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == start {
				return fmt.Errorf("read %s: %w", dir, err)
			}
			warnf("skipping %s: %v", filepath.Join(dir, mustRel(start, p)), err)
			return nil
		}
		rel := root
		if sub := mustRel(start, p); sub != "." {
			rel = path.Join(root, filepath.ToSlash(sub))
		}
		if d.IsDir() {
			if p != start && lay.SkipDir(rel) {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}
		visit(rel, d)
		return nil
	})
}

// mustRel is filepath.Rel for a path WalkDir produced under base, where it
// cannot fail.
func mustRel(base, p string) string {
	r, err := filepath.Rel(base, p)
	if err != nil {
		return p
	}
	return r
}

// BacklinksTo returns a snapshot of references to the page key opens, or —
// when key opens no page — to that (case-insensitive) unresolved target.
func (idx *Index) BacklinksTo(key string) []Ref {
	if p, ok := idx.Resolve(key); ok {
		return slices.Clone(idx.linksTo[p.Path])
	}
	return slices.Clone(idx.unresolved[strings.ToLower(key)])
}

// NewPagePath returns the file path a new page called name is created at,
// per the layout, or the layout's "cannot place page" error.
func (idx *Index) NewPagePath(name string) (string, error) {
	rel, err := idx.Layout.NewPageRel(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(idx.GraphPath, filepath.FromSlash(rel)), nil
}

// PageAt returns the page whose file is exactly path.
func (idx *Index) PageAt(path string) (*PageMeta, bool) {
	p, ok := idx.byPath[path]
	return p, ok
}

// Files returns a copy of every indexed page's path, in walk order — exactly
// the set of files search and mention scans cover.
func (idx *Index) Files() []string { return slices.Clone(idx.files) }

// Journals returns the indexed journals, ascending by Date (a copy).
func (idx *Index) Journals() []PageMeta { return slices.Clone(idx.journals) }

// JournalOn returns the journal dated day.
func (idx *Index) JournalOn(day time.Time) (*PageMeta, bool) {
	i, ok := journalSearch(idx.journals, civilDay(day))
	if !ok {
		return nil, false
	}
	return &idx.journals[i], true
}

// JournalDateOf reports the date key names as a journal: a resolved page's
// own date when it is a journal (false for any other page); an unresolved
// key that parses as a journal name under the layout's pattern gives that
// date, so a not-yet-created journal still has one.
func (idx *Index) JournalDateOf(key string) (time.Time, bool) {
	if p, ok := idx.Resolve(key); ok {
		return p.Date, p.IsJournal
	}
	return idx.Layout.ParseJournalName(key)
}

// JournalPath is the file path a journal dated day lives at (or would be
// created at), per the layout.
func (idx *Index) JournalPath(day time.Time) string {
	return filepath.Join(idx.GraphPath, filepath.FromSlash(idx.Layout.JournalRel(day)))
}

// JournalNeighbor returns the name of the nearest indexed journal in dir (-1
// or +1) from the journal key names. A key that is not a journal has no
// neighbours; a journal-shaped date with no file uses its insertion point.
func (idx *Index) JournalNeighbor(key string, dir int) (string, bool) {
	day, ok := idx.JournalDateOf(key)
	if !ok {
		return "", false
	}
	i, found := journalSearch(idx.journals, day)
	j := i
	if found {
		j = i + dir
	} else if dir < 0 {
		j = i - 1
	}
	if j < 0 || j >= len(idx.journals) {
		return "", false
	}
	return idx.journals[j].Key, true
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
