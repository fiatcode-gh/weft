package graph

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// RenameTarget is where a renamed page's file goes and the link target it gets.
type RenameTarget struct {
	Name string // the new name, trimmed
	Rel  string // new graph-relative path, with ".md"
	Path string // new file path
	Key  string // the page's Key once renamed: the text every rewritten link gets
}

// RenameTo checks newName for page (a page of idx) and returns where it
// would go, or the refusal as a user-facing error. It reads no file.
func (idx *Index) RenameTo(page *PageMeta, newName string) (RenameTarget, error) {
	if page.IsJournal {
		return RenameTarget{}, fmt.Errorf("journals cannot be renamed")
	}
	name := strings.TrimSpace(newName)
	if name == "" {
		return RenameTarget{}, fmt.Errorf("new name is empty")
	}
	for _, r := range name {
		if r == ']' || r == '|' || r == '`' || unicode.IsControl(r) {
			return RenameTarget{}, fmt.Errorf("a link cannot hold %q in a page name", string(r))
		}
	}
	rel, err := idx.Layout.RenameRel(page.Rel, name)
	if err != nil {
		return RenameTarget{}, err
	}
	if name == page.Name || rel == page.Rel {
		return RenameTarget{}, fmt.Errorf("name unchanged")
	}
	if _, ok := idx.Layout.Journal(rel); ok {
		return RenameTarget{}, fmt.Errorf("%q would be a journal", name)
	}
	newPath := filepath.Join(idx.GraphPath, filepath.FromSlash(rel))

	h, moved := idx.renamed(page, rel, newPath)
	if dest, ok := idx.ResolveLink(name); ok && dest.Page.Path != page.Path {
		return RenameTarget{}, fmt.Errorf("%q is already a page: %s", name, dest.Page.Key)
	}
	for i := range h.Pages {
		q := &h.Pages[i]
		if q.Path == newPath || !strings.EqualFold(q.Name, moved.Name) {
			continue
		}
		other := q.Name
		if o, ok := idx.PageAt(q.Path); ok {
			other = o.Key
		}
		return RenameTarget{}, fmt.Errorf("%q is already a page: %s", name, other)
	}
	// Defensive: unreachable today. The checks above leave no page whose
	// name is fold-equal to moved.Name, so ByName[moved.Name] is moved and
	// moved.Key is its Name, which Resolve returns first. Tried and refused
	// earlier: a "___" name equal to another page's path suffix, a name
	// shadowed by a journal, a destination Rel owned by another page. This
	// keeps a future Key or Resolve change from producing a rename whose
	// rewritten links open nothing.
	if got, ok := h.Resolve(moved.Key); !ok || got != moved {
		return RenameTarget{}, fmt.Errorf("cannot place page %q: no link would open it", name)
	}
	return RenameTarget{Name: name, Rel: rel, Path: newPath, Key: moved.Key}, nil
}

// renamed returns a name-only Index in which page sits at rel/newPath, with
// every Key rebuilt by buildNames, and the moved page within it. It touches
// no file.
func (idx *Index) renamed(page *PageMeta, rel, newPath string) (*Index, *PageMeta) {
	h := &Index{
		Layout:     idx.Layout,
		GraphPath:  idx.GraphPath,
		ByName:     make(map[string]*PageMeta),
		ByNameFold: make(map[string]*PageMeta),
		Pages:      slices.Clone(idx.Pages),
	}
	var moved *PageMeta
	for i := range h.Pages {
		q := &h.Pages[i]
		q.Key = ""
		if q.Path == page.Path {
			q.Rel, q.Path = rel, newPath
			q.Name = pageName(strings.TrimSuffix(path.Base(rel), ".md"))
			moved = q
		}
	}
	h.buildNames()
	return h, moved
}
