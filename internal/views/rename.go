package views

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// renameFile is one file a confirmed rename rewrites.
type renameFile struct {
	path, rel string
	seen      edit.Snapshot // content the plan was built from
	body      string        // content to write
	links     int
}

// renamePlan is everything a confirmed rename writes, built from one read of
// every indexed file. It holds copies, so a later index swap cannot change it.
type renamePlan struct {
	idx      *graph.Index // index the plan was built from (history remap uses it)
	page     graph.PageMeta
	to       graph.RenameTarget
	files    []renameFile  // walk order; includes the page's own file when it self-links
	pageRead edit.Snapshot // the page file as read
	pageSeen edit.Snapshot // what the rename step expects: pageRead, or the page's rewritten content
	links    int           // total rewritten links
}

// planRename checks name and builds the plan for renaming page (a page of
// a.idx). It writes nothing.
func (a *App) planRename(page *graph.PageMeta, name string) (*renamePlan, error) {
	to, err := a.idx.RenameTo(page, name)
	if err != nil {
		return nil, err
	}
	p := &renamePlan{idx: a.idx, page: *page, to: to}
	for _, path := range a.idx.Files() {
		meta, _ := a.idx.PageAt(path)
		rel := path
		if meta != nil {
			rel = meta.Rel
		}
		snap, err := a.readSnapshot(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %v", rel, err)
		}
		own := path == page.Path
		if !snap.Exists {
			if own {
				return nil, fmt.Errorf("%s no longer exists", rel)
			}
			continue
		}
		body, n := a.idx.RewriteLinks(snap.Content, page, to.Key)
		if own {
			p.pageRead, p.pageSeen = snap, snap
		}
		if n == 0 {
			continue
		}
		if own {
			p.pageSeen = edit.Snapshot{Content: body, Exists: true}
		}
		p.files = append(p.files, renameFile{path: path, rel: rel, seen: snap, body: body, links: n})
		p.links += n
	}
	return p, nil
}

// renameStop says where a confirmed rename stopped.
type renameStop struct {
	reason  string
	done    []string // rels rewritten, in write order
	pending []string // rels not rewritten, the failing one first
}

// renameResult is the outcome of a rename that wrote something.
type renameResult struct {
	stop       *renameStop // nil: the page was renamed
	reindexErr error       // the reindex after the writes failed
}

// applyRename carries out p. A non-nil error means it refused before writing
// anything. There is no retry and no rollback: it stops at the first failure
// and renames the page last.
func (a *App) applyRename(p *renamePlan) (renameResult, error) {
	if a.indexBuilding() {
		return renameResult{}, errors.New("index is updating — press y again")
	}
	if snap, err := a.readSnapshot(p.page.Path); err != nil || snap != p.pageRead {
		return renameResult{}, fmt.Errorf("%s changed on disk — nothing written; esc, then F2 to start again", p.page.Rel)
	}
	if err := edit.CheckRenameDest(p.page.Path, p.to.Path); err != nil {
		if errors.Is(err, edit.ErrExists) {
			return renameResult{}, fmt.Errorf("a file already exists at %s — nothing written", p.to.Rel)
		}
		return renameResult{}, fmt.Errorf("cannot check %s: %v", p.to.Rel, err)
	}

	rels := make([]string, len(p.files))
	for i, f := range p.files {
		rels[i] = f.rel
	}
	for i, f := range p.files {
		if err := edit.WriteFileIfUnchanged(f.path, f.seen, []byte(f.body)); err != nil {
			reason := fmt.Sprintf("cannot write %s: %v", f.rel, err)
			if errors.Is(err, edit.ErrChanged) {
				reason = f.rel + " changed on disk — nothing overwritten"
			}
			stop := &renameStop{reason: reason, done: rels[:i], pending: rels[i:]}
			return renameResult{stop: stop, reindexErr: a.reindex()}, nil
		}
	}

	if err := edit.RenameIfUnchanged(p.page.Path, p.to.Path, p.pageSeen); err != nil {
		var reason string
		switch {
		case errors.Is(err, edit.ErrChanged):
			reason = p.page.Rel + " changed on disk — page not renamed"
		case errors.Is(err, edit.ErrExists):
			reason = fmt.Sprintf("a file already exists at %s — page not renamed", p.to.Rel)
		default:
			reason = fmt.Sprintf("cannot rename %s: %v", p.page.Rel, err)
		}
		return renameResult{stop: &renameStop{reason: reason, done: rels}, reindexErr: a.reindex()}, nil
	}

	// The body PageView.loadResult stores for a page, so a self-linking
	// page's fold entry stays current.
	newBody := ""
	for _, f := range p.files {
		if f.path == p.page.Path {
			newBody = strings.TrimSpace(f.body) + "\n"
		}
	}
	a.remapHistory(p.idx, p.page.Path, p.to.Key)
	a.folds.rename(p.page.Path, p.to.Path, newBody)
	return renameResult{reindexErr: a.reindexTo(p.to.Key)}, nil
}

// remapHistory points every history entry that opened the page at path (in
// old) at key, so back/forward opens the renamed page.
func (a *App) remapHistory(old *graph.Index, path, key string) {
	for i := range a.hist {
		if m, ok := old.Resolve(a.hist[i].page); ok && m.Path == path {
			a.hist[i].page = key
		}
	}
}
